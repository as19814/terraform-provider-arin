package arin

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func TestRPKIResponseEvidenceSurvivesPendingRequest(t *testing.T) {
	dir := privateExchangeDir(t)
	l, err := openRPKIExchange(dir, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	request := []byte("list request")
	response := []byte("authenticated response")
	digest := rpkiManifestDigest(request)
	if err := l.BeginSigned(digest, "updown-list", cmsTrustNow(), request); err != nil {
		t.Fatal(err)
	}
	if err := l.CompleteSigned(digest, cmsTrustNow(), response); err != nil {
		t.Fatal(err)
	}
	oldPath := l.path + ".response-" + rpkiManifestDigest(response) + ".der"
	info, err := os.Stat(oldPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("response is not private")
	}
	next := []byte("revoke request")
	nextDigest := rpkiManifestDigest(next)
	if err := l.BeginSigned(nextDigest, "updown-revoke", cmsTrustNow().Add(time.Second), next); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	l, err = openRPKIExchange(dir, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	receipt, got, err := l.CompletedResponse()
	if err != nil || !bytes.Equal(got, response) || receipt.Request.Operation != "updown-list" || receipt.Request.RequestSHA256 != digest {
		t.Fatal("lost accepted response during pending mutation")
	}
	got[0] ^= 1
	state, err := l.State()
	if err != nil {
		t.Fatal(err)
	}
	state.LastResponse.SHA256 = "changed"
	_, got, err = l.CompletedResponse()
	if err != nil || !bytes.Equal(got, response) {
		t.Fatal("caller changed retained evidence")
	}
	replacement := []byte("revoke response")
	if err := l.CompleteSigned(nextDigest, cmsTrustNow().Add(time.Second), replacement); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("obsolete response retained after durable replacement")
	}
	receipt, got, err = l.CompletedResponse()
	if err != nil || !bytes.Equal(got, replacement) || receipt.Request.Operation != "updown-revoke" {
		t.Fatal("replacement lost")
	}
}

func TestRPKIResponseEvidenceRejectsInvalidFiles(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt", "public", "symlink", "oversize", "directory"} {
		t.Run(mode, func(t *testing.T) {
			l, err := openRPKIExchange(privateExchangeDir(t), testExchangePeer)
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			raw := []byte("response")
			digest := rpkiManifestDigest([]byte("request"))
			if err := l.Begin(digest, "updown-list", cmsTrustNow()); err != nil {
				t.Fatal(err)
			}
			if err := l.CompleteSigned(digest, cmsTrustNow(), raw); err != nil {
				t.Fatal(err)
			}
			if err := l.Begin(digest, "updown-revoke", cmsTrustNow().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			path := l.path + ".response-" + rpkiManifestDigest(raw) + ".der"
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
			if _, _, err := l.CompletedResponse(); err == nil {
				t.Fatal("unsafe evidence accepted")
			}
			state, err := l.State()
			if err != nil || state.Pending == nil || state.Pending.Operation != "updown-revoke" {
				t.Fatal("evidence failure cleared pending mutation")
			}
		})
	}
}

func TestRPKIResponsePersistenceFailureRetainsPending(t *testing.T) {
	dir := privateExchangeDir(t)
	l, err := openRPKIExchange(dir, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("response")
	digest := rpkiManifestDigest([]byte("request"))
	if err := l.Begin(digest, "updown-list", cmsTrustNow()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(l.path+".response-"+rpkiManifestDigest(raw)+".der", 0700); err != nil {
		t.Fatal(err)
	}
	if err := l.CompleteSigned(digest, cmsTrustNow(), raw); err == nil {
		t.Fatal("failed persistence accepted")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	l, err = openRPKIExchange(dir, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	state, err := l.State()
	if err != nil || state.Pending == nil || state.LastResponse != nil || !state.LastReceived.IsZero() {
		t.Fatal("failed persistence completed exchange")
	}
}

func TestRPKIResponseJournalFailurePreservesEvidence(t *testing.T) {
	l, err := openRPKIExchange(privateExchangeDir(t), testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	digest := rpkiManifestDigest([]byte("request"))
	old := []byte("old response")
	if err := l.Begin(digest, "updown-list", cmsTrustNow()); err != nil {
		t.Fatal(err)
	}
	if err := l.CompleteSigned(digest, cmsTrustNow(), old); err != nil {
		t.Fatal(err)
	}
	if err := l.Begin(digest, "updown-list", cmsTrustNow().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	// A directory at the journal destination forces atomic replacement to fail.
	if err := os.Rename(l.path, l.path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(l.path, 0700); err != nil {
		t.Fatal(err)
	}
	next := []byte("new response")
	if err := l.CompleteSigned(digest, cmsTrustNow().Add(time.Second), next); err == nil {
		t.Fatal("failed journal commit accepted")
	}
	for _, raw := range [][]byte{old, next} {
		got, err := os.ReadFile(l.path + ".response-" + rpkiManifestDigest(raw) + ".der")
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatal("commit failure removed recovery evidence")
		}
	}
	if _, err := l.State(); err == nil {
		t.Fatal("failed commit lease remained usable")
	}
}
