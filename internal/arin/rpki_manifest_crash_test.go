package arin

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRPKIManifestCrashHelper(t *testing.T) {
	dir := os.Getenv("ARIN_TEST_MANIFEST_CRASH_DIR")
	if dir == "" {
		return
	}
	raw, err := os.ReadFile(filepath.Join(dir, "anchor.pem"))
	if err != nil {
		os.Exit(2)
	}
	anchors, err := rpkiPEMCertificates(string(raw), 1)
	if err != nil || len(anchors) != 1 {
		os.Exit(3)
	}
	_ = withRPKIManifestHistory(dir, anchors[0], func(h *rpkiManifestHistory) error {
		// Simulate an exit after lock acquisition and in-memory changes, before
		// the atomic history commit. Deferred cleanup must not run.
		h.Entries = map[string]rpkiManifestWatermark{}
		os.Exit(23)
		return nil
	})
	os.Exit(4)
}

func TestRPKIManifestCrashRecovery(t *testing.T) {
	f, pubs := manifestPathFixture(t, "valid")
	dir := privateExchangeDir(t)
	anchor := f.certs[2]
	now := cmsTrustNow().Add(10 * time.Minute)
	newer := append([]rpkiPathPublication(nil), pubs...)
	newer[0] = rewriteManifestVersion(t, pubs[0], f, 1, cmsTrustNow().Add(time.Minute))
	if _, err := verifyAndRecordRPKIManifestPath(dir, f.certs, anchor, newer, now); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rpki-manifests-"+rpkiManifestDigest(anchor.Raw)+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "anchor.pem"), []byte(certificateTestPEM("CERTIFICATE", anchor.Raw)), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestRPKIManifestCrashHelper$")
	command.Env = append(os.Environ(), "ARIN_TEST_MANIFEST_CRASH_DIR="+dir)
	output, err := command.CombinedOutput()
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != 23 {
		t.Fatalf("crash helper did not exit at the expected point: %v: %s", err, output)
	}
	if _, err := verifyAndRecordRPKIManifestPath(dir, f.certs, anchor, newer, now); err == nil {
		t.Fatal("crash lock bypassed")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("crash or blocked read changed history")
	}
	// Wait above proves this specific owner exited. Model removal of only its
	// empty lock directory, with no changes to history or transaction evidence.
	if err := os.Remove(path + ".lock"); err != nil {
		t.Fatal(err)
	}
	if err := syncRPKIExchangeDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyAndRecordRPKIManifestPath(dir, f.certs, anchor, pubs, now); err == nil {
		t.Fatal("rollback accepted after manual crash-lock recovery")
	}
	after, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected rollback changed recovered history")
	}
	if _, err := verifyAndRecordRPKIManifestPath(dir, f.certs, anchor, newer, now); err != nil {
		t.Fatalf("valid path rejected after recovery: %v", err)
	}
}
