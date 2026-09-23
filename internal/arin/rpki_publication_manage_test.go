package arin

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestRPKIPublicationBundlePlan(t *testing.T) {
	a, b, c := "rsync://repo.example/module/a.cer", "rsync://repo.example/module/b.mft", "rsync://repo.example/module/c.crl"
	old := base64.StdEncoding.EncodeToString([]byte("old"))
	next := base64.StdEncoding.EncodeToString([]byte("new"))
	_, prior, err := DecodeRPKIPublicationObjects(map[string]string{a: old, b: old})
	if err != nil {
		t.Fatal(err)
	}
	changes, err := planRPKIPublicationBundle(map[string]string{a: next, c: next}, prior)
	if err != nil || len(changes) != 3 {
		t.Fatalf("plan: %v", err)
	}
	if changes[0].URI != a || changes[0].OldSHA256 != prior[a] || string(changes[0].DER) != "new" || changes[1].URI != b || !changes[1].Withdraw || changes[1].OldSHA256 != prior[b] || changes[2].URI != c || changes[2].OldSHA256 != "" {
		t.Fatal("incorrect atomic preconditions")
	}
	changes, err = planRPKIPublicationBundle(map[string]string{a: old, b: old}, prior)
	if err != nil || len(changes) != 0 {
		t.Fatal("unchanged objects mutated")
	}
	for _, desired := range []map[string]string{{a: ""}, {a: "not-base64"}, {"https://example.test/object": old}, {a: "b2xk\n"}, {"rsync://repo.example/module/": old}, {a: base64.StdEncoding.EncodeToString(make([]byte, (3<<20)+1))}} {
		if _, _, err := DecodeRPKIPublicationObjects(desired); err == nil {
			t.Fatal("invalid desired objects accepted")
		}
	}
	if _, err := planRPKIPublicationBundle(nil, map[string]string{a: strings.Repeat("z", 64)}); err == nil {
		t.Fatal("invalid previous hash accepted")
	}
}
