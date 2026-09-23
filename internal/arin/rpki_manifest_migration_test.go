package arin

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRPKIManifestHistoryMigration(t *testing.T) {
	f, pubs := manifestPathFixture(t, "valid")
	old := f.certs[2]
	template := *old
	template.SerialNumber = new(big.Int).Add(old.SerialNumber, big.NewInt(1))
	template.ExtraExtensions = old.Extensions
	raw, err := x509.CreateCertificate(rand.Reader, &template, &template, &f.keys[2].PublicKey, f.keys[2])
	if err != nil {
		t.Fatal(err)
	}
	next, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	now := cmsTrustNow().Add(10 * time.Minute)
	oldPEM, nextPEM := certificateTestPEM("CERTIFICATE", old.Raw), certificateTestPEM("CERTIFICATE", next.Raw)
	for _, mode := range []string{"new", "merge_newer", "merge_older", "conflict", "time_conflict", "locked_source", "locked_destination", "missing_source", "corrupt_source", "corrupt_destination", "same_anchor", "expired", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			dir := privateExchangeDir(t)
			if _, err := verifyAndRecordRPKIManifestPath(dir, f.certs, old, pubs, now); err != nil {
				t.Fatal(err)
			}
			oldPath := filepath.Join(dir, "rpki-manifests-"+rpkiManifestDigest(old.Raw)+".json")
			nextPath := filepath.Join(dir, "rpki-manifests-"+rpkiManifestDigest(next.Raw)+".json")
			original, _ := os.ReadFile(oldPath)
			migrated := nextPEM
			at := now
			if mode == "merge_newer" || mode == "merge_older" || mode == "conflict" || mode == "time_conflict" {
				h, err := readRPKIManifestHistory(oldPath, rpkiManifestDigest(old.Raw))
				if err != nil {
					t.Fatal(err)
				}
				h.Anchor = rpkiManifestDigest(next.Raw)
				for id, w := range h.Entries {
					switch mode {
					case "merge_newer":
						w.Number = "2"
						w.ThisUpdate = w.ThisUpdate.Add(time.Minute)
					case "merge_older":
						// Raise the source watermark while retaining the older destination.
						source := h
						source.Entries = map[string]rpkiManifestWatermark{}
						for k, v := range h.Entries {
							v.Number = "2"
							v.ThisUpdate = v.ThisUpdate.Add(time.Minute)
							source.Entries[k] = v
						}
						source.Anchor = rpkiManifestDigest(old.Raw)
						if err := saveRPKIManifestHistory(oldPath, source); err != nil {
							t.Fatal(err)
						}
						original, _ = os.ReadFile(oldPath)
					case "conflict":
						w.SHA256 = rpkiManifestDigest([]byte("conflict"))
					case "time_conflict":
						w.Number = "2"
					}
					h.Entries[id] = w
				}
				if err := saveRPKIManifestHistory(nextPath, h); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "locked_source":
				if err := os.Mkdir(oldPath+".lock", 0700); err != nil {
					t.Fatal(err)
				}
			case "locked_destination":
				if err := os.Mkdir(nextPath+".lock", 0700); err != nil {
					t.Fatal(err)
				}
			case "missing_source":
				if err := os.Remove(oldPath); err != nil {
					t.Fatal(err)
				}
			case "corrupt_source":
				if err := os.WriteFile(oldPath, []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			case "corrupt_destination":
				if err := os.WriteFile(nextPath, []byte("bad"), 0600); err != nil {
					t.Fatal(err)
				}
			case "same_anchor":
				migrated = oldPEM
			case "expired":
				at = next.NotAfter.Add(time.Second)
			case "symlink":
				if err := os.Symlink(oldPath, nextPath); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(nextPath)
			err = migrateRPKIManifestHistory(dir, oldPEM, migrated, at)
			success := mode == "new" || mode == "merge_newer" || mode == "merge_older"
			if success && err != nil {
				t.Fatal(err)
			}
			if !success && err == nil {
				t.Fatal("unsafe migration accepted")
			}
			after, _ := os.ReadFile(nextPath)
			if !success && !bytes.Equal(before, after) {
				t.Fatal("failed migration changed destination")
			}
			if mode != "missing_source" && mode != "corrupt_source" {
				source, _ := os.ReadFile(oldPath)
				if !bytes.Equal(source, original) {
					t.Fatal("source history changed")
				}
			}
			for _, p := range []string{oldPath, nextPath} {
				_, e := os.Stat(p + ".lock")
				retained := (mode == "locked_source" && p == oldPath) || (mode == "locked_destination" && p == nextPath)
				if retained && e != nil {
					t.Fatal("foreign lock removed")
				}
				if !retained && !os.IsNotExist(e) {
					t.Fatal("migration lock leaked")
				}
			}
			if success {
				if err := migrateRPKIManifestHistory(dir, oldPEM, nextPEM, now); err != nil {
					t.Fatalf("repeat migration: %v", err)
				}
				h, err := readRPKIManifestHistory(nextPath, rpkiManifestDigest(next.Raw))
				if err != nil {
					t.Fatal(err)
				}
				if len(h.Entries) != 2 {
					t.Fatal("lost issuer watermarks")
				}
				if mode != "new" {
					for _, w := range h.Entries {
						if w.Number != "2" {
							t.Fatal("watermark lowered")
						}
					}
				}
				renewedPath := append([]*x509.Certificate(nil), f.certs...)
				renewedPath[2] = next
				_, err = verifyAndRecordRPKIManifestPath(dir, renewedPath, next, pubs, now)
				if mode == "new" && err != nil {
					t.Fatal(err)
				}
				if mode != "new" && err == nil {
					t.Fatal("rollback accepted under replacement anchor")
				}
			}
		})
	}
}

func TestRPKIManifestHistoryMigrationChangedKey(t *testing.T) {
	old, oldPubs := manifestPathFixture(t, "valid")
	next, nextPubs := manifestPathFixture(t, "valid")
	dir := privateExchangeDir(t)
	now := cmsTrustNow().Add(10 * time.Minute)
	for _, fixture := range []struct {
		path []*x509.Certificate
		pubs []rpkiPathPublication
	}{{old.certs, oldPubs}, {next.certs, nextPubs}} {
		if _, err := verifyAndRecordRPKIManifestPath(dir, fixture.path, fixture.path[2], fixture.pubs, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrateRPKIManifestHistory(dir, certificateTestPEM("CERTIFICATE", old.certs[2].Raw), certificateTestPEM("CERTIFICATE", next.certs[2].Raw), now); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rpki-manifests-"+rpkiManifestDigest(next.certs[2].Raw)+".json")
	history, err := readRPKIManifestHistory(path, rpkiManifestDigest(next.certs[2].Raw))
	if err != nil || len(history.Entries) != 4 {
		t.Fatal("distinct issuer histories were not preserved")
	}
	if _, err := verifyAndRecordRPKIManifestPath(dir, next.certs, next.certs[2], nextPubs, now); err != nil {
		t.Fatal(err)
	}
}
