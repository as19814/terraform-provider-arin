package arin

import (
	"bytes"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func rewriteManifestVersion(t *testing.T, p rpkiPathPublication, f resourcePathFixture, number int64, update time.Time) rpkiPathPublication {
	t.Helper()
	m, err := decodeRPKIManifest(p.ManifestDER)
	if err != nil {
		t.Fatal(err)
	}
	content := manifestContentFixture()
	content.Number = big.NewInt(number)
	content.ThisUpdate = update
	content.Files = nil
	for name, hash := range m.Content.Files {
		content.Files = append(content.Files, manifestTestFile{name, asn1.BitString{Bytes: hash[:], BitLength: 256}})
	}
	base, _ := cmsTestFixture(t)
	base.Encap = cmsTestEncap{Type: cmsManifestOID, Content: cmsTestDER(t, content)}
	base.Certs = asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: m.Signer.Raw}
	base.Signers[0].SID = asn1.RawValue{Class: 2, Tag: 0, Bytes: m.Signer.SubjectKeyId}
	hash := sha256.Sum256(base.Encap.Content)
	base.Signers[0].Attrs = []cmsTestAttribute{{cmsContentTypeOID, []asn1.RawValue{cmsTestValue(t, cmsManifestOID)}}, {cmsDigestOID, []asn1.RawValue{cmsTestValue(t, hash[:])}}}
	p.ManifestDER = manifestCMSEncode(t, base, f.keys[0], false)
	return p
}

func TestRPKIManifestHistory(t *testing.T) {
	f, pubs := manifestPathFixture(t, "valid")
	dir := privateExchangeDir(t)
	anchor := f.certs[2]
	now := cmsTrustNow().Add(10 * time.Minute)
	verify := func(p []rpkiPathPublication) error {
		_, err := verifyAndRecordRPKIManifestPath(dir, f.certs, anchor, p, now)
		return err
	}
	if err := verify(pubs); err != nil {
		t.Fatal(err)
	}
	if err := verify(pubs); err != nil {
		t.Fatalf("identical cached manifests rejected: %v", err)
	}
	path := filepath.Join(dir, "rpki-manifests-"+rpkiManifestDigest(anchor.Raw)+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("history must be private")
	}
	newer := append([]rpkiPathPublication(nil), pubs...)
	newer[0] = rewriteManifestVersion(t, pubs[0], f, 1, cmsTrustNow().Add(time.Minute))
	if err := verify(newer); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || bytes.Equal(before, after) {
		t.Fatal("history not advanced")
	}
	if err := verify(pubs); err == nil {
		t.Fatal("rollback accepted after reopening")
	}
	for _, tt := range []struct {
		number int64
		update time.Time
	}{{1, cmsTrustNow().Add(2 * time.Minute)}, {2, cmsTrustNow()}, {2, cmsTrustNow().Add(time.Minute)}} {
		bad := append([]rpkiPathPublication(nil), newer...)
		bad[0] = rewriteManifestVersion(t, newer[0], f, tt.number, tt.update)
		if err := verify(bad); err == nil {
			t.Fatal("conflicting number or non-increasing time accepted")
		}
	}
	unchanged, _ := os.ReadFile(path)
	if !bytes.Equal(after, unchanged) {
		t.Fatal("rejected manifest changed history")
	}
	// An otherwise newer manifest cannot update history if another issuer's
	// publication fails. The operation saves all path watermarks together.
	bad := append([]rpkiPathPublication(nil), newer...)
	bad[0] = rewriteManifestVersion(t, newer[0], f, 2, cmsTrustNow().Add(2*time.Minute))
	bad[1].ManifestDER = []byte{0}
	if err := verify(bad); err == nil {
		t.Fatal("invalid path accepted")
	}
	unchanged, _ = os.ReadFile(path)
	if !bytes.Equal(after, unchanged) {
		t.Fatal("failed path partially updated history")
	}
	if err := os.Mkdir(path+".lock", 0700); err != nil {
		t.Fatal(err)
	}
	if err := verify(newer); err == nil {
		t.Fatal("existing lock ignored")
	}
	if err := os.Remove(path + ".lock"); err != nil {
		t.Fatal(err)
	}
	if err := verify(newer); err != nil {
		t.Fatal(err)
	}
}

func TestRPKIManifestHistoryRejectsCorruption(t *testing.T) {
	f, pubs := manifestPathFixture(t, "valid")
	for _, mode := range []string{"malformed", "unknown_field", "wrong_anchor", "negative_number", "oversized_number", "bad_hash", "fractional_time", "public_file", "symlink", "public_directory"} {
		t.Run(mode, func(t *testing.T) {
			dir := privateExchangeDir(t)
			anchor := f.certs[2]
			verify := func() error {
				_, err := verifyAndRecordRPKIManifestPath(dir, f.certs, anchor, pubs, cmsTrustNow())
				return err
			}
			if err := verify(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "rpki-manifests-"+rpkiManifestDigest(anchor.Raw)+".json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var history rpkiManifestHistory
			if err := json.Unmarshal(raw, &history); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "malformed":
				raw = []byte("{")
			case "unknown_field":
				raw = append([]byte(`{"unknown":true,`), raw[1:]...)
			case "wrong_anchor":
				history.Anchor = rpkiManifestDigest([]byte("other"))
				raw, _ = json.Marshal(history)
			case "negative_number", "oversized_number", "bad_hash", "fractional_time":
				for id, w := range history.Entries {
					switch mode {
					case "negative_number":
						w.Number = "-1"
					case "oversized_number":
						w.Number = new(big.Int).Lsh(big.NewInt(1), 159).String()
					case "bad_hash":
						w.SHA256 = "bad"
					case "fractional_time":
						w.ThisUpdate = w.ThisUpdate.Add(time.Nanosecond)
					}
					history.Entries[id] = w
					break
				}
				raw, _ = json.Marshal(history)
			case "public_file":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "public_directory":
				if err := os.Chmod(dir, 0755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := path + ".target"
				if err := os.Rename(path, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "symlink" {
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := verify(); err == nil {
				t.Fatal("unsafe or corrupt history accepted")
			}
		})
	}
}
