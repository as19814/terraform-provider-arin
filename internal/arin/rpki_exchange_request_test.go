package arin

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRPKIRequestEvidence(t *testing.T) {
	identity, xml := cmsSigningFixture(t)
	signed, err := signRPKICMS(xml, identity, cmsTrustNow(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(signed))
	directory := privateExchangeDir(t)
	lease, err := openRPKIExchange(directory, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.BeginSigned(digest, "publication-batch", cmsTrustNow(), signed); err != nil {
		t.Fatal(err)
	}
	evidencePath := lease.path + ".request.der"
	info, err := os.Stat(evidencePath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("request evidence is not private")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	lease, err = openRPKIExchange(directory, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	got, err := lease.PendingRequest()
	if err != nil || !bytes.Equal(got, signed) {
		t.Fatalf("reopen lost request: %v", err)
	}
	verified, err := verifyRPKICMS(got, rpkiCMSTrust{Anchor: identity.Anchor, Now: cmsTrustNow()})
	if err != nil || !bytes.Equal(verified.Content, xml) {
		t.Fatal("persisted CMS is not the signed request")
	}
	got[0] ^= 0xff
	got, err = lease.PendingRequest()
	if err != nil || !bytes.Equal(got, signed) {
		t.Fatal("caller mutated evidence")
	}
	other := []byte("different request")
	otherDigest := fmt.Sprintf("%x", sha256.Sum256(other))
	if err := lease.BeginSigned(otherDigest, "publication-batch", cmsTrustNow().Add(time.Second), other); err == nil {
		t.Fatal("pending request overwritten")
	}
	got, err = lease.PendingRequest()
	if err != nil || !bytes.Equal(got, signed) {
		t.Fatal("rejected begin changed evidence")
	}
	if err := lease.Complete(digest, cmsTrustNow()); err != nil {
		t.Fatal(err)
	}
	if _, err := lease.PendingRequest(); err == nil {
		t.Fatal("completed evidence presented as pending")
	}
	if err := lease.BeginSigned(otherDigest, "publication-batch", cmsTrustNow().Add(time.Second), other); err != nil {
		t.Fatal(err)
	}
	got, err = lease.PendingRequest()
	if err != nil || !bytes.Equal(got, other) {
		t.Fatal("next request did not replace sidecar")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 3 {
		t.Fatal("request evidence grew beyond journal, lock and one sidecar")
	}
}
func TestRPKIRequestEvidenceRejectsInvalidFiles(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt", "public", "symlink", "oversize", "directory"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			lease, err := openRPKIExchange(directory, testExchangePeer)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			raw := []byte("exact signed request")
			digest := fmt.Sprintf("%x", sha256.Sum256(raw))
			if err := lease.BeginSigned(digest, "updown-issue", cmsTrustNow(), raw); err != nil {
				t.Fatal(err)
			}
			path := lease.path + ".request.der"
			switch mode {
			case "missing":
				err = os.Remove(path)
			case "corrupt":
				err = os.WriteFile(path, []byte("changed"), 0600)
			case "public":
				err = os.Chmod(path, 0644)
			case "symlink":
				err = os.Rename(path, path+".target")
				if err == nil {
					err = os.Symlink(path+".target", path)
				}
			case "oversize":
				err = os.WriteFile(path, make([]byte, (4<<20)+1), 0600)
			case "directory":
				err = os.Remove(path)
				if err == nil {
					err = os.Mkdir(path, 0700)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := lease.PendingRequest(); err == nil {
				t.Fatal("invalid request evidence accepted")
			}
			state, err := lease.State()
			if err != nil || state.Pending == nil || state.Pending.RequestSHA256 != digest {
				t.Fatal("evidence failure cleared pending mutation")
			}
		})
	}
}
func TestRPKIRequestEvidencePreflight(t *testing.T) {
	directory := privateExchangeDir(t)
	lease, err := openRPKIExchange(directory, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := lease.BeginSigned(strings.Repeat("a", 64), "publication-batch", cmsTrustNow(), []byte("wrong digest")); err == nil {
		t.Fatal("mismatched evidence accepted")
	}
	if _, err := os.Stat(lease.path + ".request.der"); !os.IsNotExist(err) {
		t.Fatal("invalid evidence written")
	}
	// Failure to persist request evidence must leave no dispatch intent.
	if err := os.Mkdir(lease.path+".request.der", 0700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("request")
	digest := fmt.Sprintf("%x", sha256.Sum256(raw))
	if err := lease.BeginSigned(digest, "publication-batch", cmsTrustNow(), raw); err == nil {
		t.Fatal("failed evidence persistence accepted")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := openRPKIExchange(directory, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, err := reopened.State()
	if err != nil || state.Pending != nil || !state.LastSent.IsZero() {
		t.Fatal("persistence failure advanced journal")
	}
}
