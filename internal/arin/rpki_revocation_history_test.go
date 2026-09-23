package arin

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRPKIRevocationHistory(t *testing.T) {
	f, pubs := manifestPathFixture(t, "revoked_child")
	retiring := pubs[0]
	delete(retiring.Files, "child.cer")
	retiring = rewriteRevocationManifestFiles(t, retiring, f)
	ski, err := rpkiPublicKeyIdentifier(f.certs[0].RawSubjectPublicKeyInfo)
	if err != nil {
		t.Fatal(err)
	}
	now := cmsTrustNow().Add(10 * time.Minute)
	newer := rewriteManifestVersion(t, retiring, f, 1, cmsTrustNow().Add(time.Minute))
	dir := privateExchangeDir(t)
	filename := filepath.Join(dir, "rpki-manifests-"+rpkiManifestDigest(f.certs[2].Raw)+".json")
	verify := func(p rpkiPathPublication, upstream []rpkiPathPublication) error {
		_, err := verifyAndRecordRPKIRevocationProof(dir, f.certs[0].Raw, ski, f.certs[1:], f.certs[2], upstream, p, now)
		return err
	}
	if err := verify(retiring, pubs[1:]); err != nil {
		t.Fatal(err)
	}
	history, err := readRPKIManifestHistory(filename, rpkiManifestDigest(f.certs[2].Raw))
	if err != nil || len(history.Entries) != 2 {
		t.Fatal("did not record upstream and retiring manifests")
	}
	if err := verify(newer, pubs[1:]); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := verify(newer, pubs[1:]); err != nil {
		t.Fatal("same verified evidence failed reopen", err)
	}
	if err := verify(retiring, pubs[1:]); err == nil {
		t.Fatal("accepted revocation manifest rollback")
	}
	// A newer upstream version cannot be saved if the retiring manifest rolls back.
	upstream := []rpkiPathPublication{rewriteManifestVersion(t, pubs[1], f, 1, cmsTrustNow().Add(time.Minute))}
	if err := verify(retiring, upstream); err == nil {
		t.Fatal("mixed rollback accepted")
	}
	after, err := os.ReadFile(filename)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed proof partially advanced history")
	}
	if err := os.Mkdir(filename+".lock", 0700); err != nil {
		t.Fatal(err)
	}
	if err := verify(newer, pubs[1:]); err == nil {
		t.Fatal("bypassed history lock")
	}
	if err := os.Remove(filename + ".lock"); err != nil {
		t.Fatal(err)
	}
	// An invalid history path cannot be silently treated as empty history.
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filename, 0700); err != nil {
		t.Fatal(err)
	}
	if err := verify(newer, pubs[1:]); err == nil {
		t.Fatal("invalid history path accepted")
	}
}

func TestRPKIRevocationHistorySharesIssuanceWatermarks(t *testing.T) {
	f, pubs := manifestPathFixture(t, "revoked_child")
	retired := pubs[0]
	delete(retired.Files, "child.cer")
	retired = rewriteRevocationManifestFiles(t, retired, f)
	ski, err := rpkiPublicKeyIdentifier(f.certs[0].RawSubjectPublicKeyInfo)
	if err != nil {
		t.Fatal(err)
	}
	dir := privateExchangeDir(t)
	// Authenticate and record the upstream issuer before revocation recovery.
	now := cmsTrustNow().Add(10 * time.Minute)
	upstream := []rpkiPathPublication{rewriteManifestVersion(t, pubs[1], f, 1, cmsTrustNow().Add(time.Minute))}
	if _, err := verifyAndRecordRPKIManifestPath(dir, f.certs[1:], f.certs[2], upstream, now); err != nil {
		t.Fatal(err)
	}
	if proof, err := verifyAndRecordRPKIRevocationProof(dir, f.certs[0].Raw, ski, f.certs[1:], f.certs[2], pubs[1:], retired, now); err == nil || proof != nil {
		t.Fatal("revocation ignored existing issuer path watermark")
	}
	if _, err := verifyAndRecordRPKIRevocationProof(dir, f.certs[0].Raw, ski, f.certs[1:], f.certs[2], upstream, retired, now); err != nil {
		t.Fatal(err)
	}
}

func TestRPKIRevocationHistoryPreventsOldIssuance(t *testing.T) {
	f, pubs := manifestPathFixture(t, "valid")
	dir := privateExchangeDir(t)
	now := cmsTrustNow().Add(10 * time.Minute)
	if _, err := verifyAndRecordRPKIManifestPath(dir, f.certs, f.certs[2], pubs, now); err != nil {
		t.Fatal(err)
	}
	retired := pubs[0]
	retired.Files = maps.Clone(retired.Files)
	delete(retired.Files, "child.cer")
	retired.Files["issuer.crl"] = resourcePathCRL(t, f.certs[1], f.keys[1], f.certs[0].SerialNumber).Raw
	retired = rewriteRevocationManifestFiles(t, retired, f)
	retired = rewriteManifestVersion(t, retired, f, 1, cmsTrustNow().Add(time.Minute))
	ski, err := rpkiPublicKeyIdentifier(f.certs[0].RawSubjectPublicKeyInfo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyAndRecordRPKIRevocationProof(dir, f.certs[0].Raw, ski, f.certs[1:], f.certs[2], pubs[1:], retired, now); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyAndRecordRPKIManifestPath(dir, f.certs, f.certs[2], pubs, now); err == nil {
		t.Fatal("old issuance evidence resurrected a revoked key")
	}
}
