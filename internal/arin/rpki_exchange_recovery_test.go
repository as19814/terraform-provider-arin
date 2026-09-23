package arin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRPKIReadRecovery(t *testing.T) {
	for _, operation := range []string{"publication-list", "updown-list"} {
		for _, previous := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "_first", true: "_later"}[previous], func(t *testing.T) {
				directory := privateExchangeDir(t)
				lease, err := openRPKIExchange(directory, testExchangePeer)
				if err != nil {
					t.Fatal(err)
				}
				received := time.Time{}
				if previous {
					received = cmsTrustNow().Add(-time.Minute)
					if err := lease.Begin(strings.Repeat("c", 64), operation, received); err != nil {
						t.Fatal(err)
					}
					if err := lease.Complete(strings.Repeat("c", 64), received); err != nil {
						t.Fatal(err)
					}
				}
				if err := lease.Begin(testExchangeDigest, operation, cmsTrustNow()); err != nil {
					t.Fatal(err)
				}
				if err := lease.Close(); err != nil {
					t.Fatal(err)
				}
				raw, err := InspectRPKIJournal(directory, testExchangePeer)
				if err != nil {
					t.Fatal(err)
				}
				var before rpkiExchangeState
				if err := json.Unmarshal(raw, &before); err != nil {
					t.Fatal(err)
				}
				if before.Pending == nil || before.RecoveredRead != nil {
					t.Fatal("inspection changed pending state")
				}
				raw, err = RecoverRPKIRead(directory, testExchangePeer, testExchangeDigest)
				if err != nil {
					t.Fatal(err)
				}
				var after rpkiExchangeState
				if err := json.Unmarshal(raw, &after); err != nil {
					t.Fatal(err)
				}
				if after.Pending != nil || after.RecoveredRead == nil || *after.RecoveredRead != *before.Pending || !after.LastSent.Equal(cmsTrustNow()) || !after.LastReceived.Equal(received) {
					t.Fatal("recovery lost evidence or changed watermarks")
				}
				if _, err := RecoverRPKIRead(directory, testExchangePeer, testExchangeDigest); err == nil {
					t.Fatal("repeated recovery accepted")
				}
				lease, err = openRPKIExchange(directory, testExchangePeer)
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
				copy, err := lease.State()
				if err != nil {
					t.Fatal(err)
				}
				copy.RecoveredRead.Operation = "changed"
				copy, err = lease.State()
				if err != nil || copy.RecoveredRead.Operation != operation {
					t.Fatal("recovery metadata aliases state")
				}
				// A same-second retry could produce identical signed bytes and digest.
				if err := lease.Begin(testExchangeDigest, operation, cmsTrustNow()); err == nil {
					t.Fatal("recovered signing time reused")
				}
				if err := lease.Begin(strings.Repeat("d", 64), operation, cmsTrustNow().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				if err := lease.Complete(strings.Repeat("d", 64), cmsTrustNow()); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestRPKIRecoveryRejectsMutationsAndMismatch(t *testing.T) {
	for _, operation := range []string{"publication-batch", "updown-issue", "updown-revoke", "future-operation", "publication-list"} {
		t.Run(operation, func(t *testing.T) {
			directory := privateExchangeDir(t)
			lease, err := openRPKIExchange(directory, testExchangePeer)
			if err != nil {
				t.Fatal(err)
			}
			if err := lease.Begin(testExchangeDigest, operation, cmsTrustNow()); err != nil {
				t.Fatal(err)
			}
			path := lease.path
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			digest := testExchangeDigest
			if operation == "publication-list" {
				digest = strings.Repeat("e", 64)
			}
			if _, err := RecoverRPKIRead(directory, testExchangePeer, digest); err == nil {
				t.Fatal("unsafe recovery accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected recovery modified journal")
			}
		})
	}
}

func TestRPKIRecoveryMissingAndLocked(t *testing.T) {
	directory := privateExchangeDir(t)
	if _, err := InspectRPKIJournal(directory, testExchangePeer); err == nil {
		t.Fatal("missing journal accepted")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("inspection created journal")
	}
	lease, err := openRPKIExchange(directory, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, err := InspectRPKIJournal(directory, testExchangePeer); err == nil {
		t.Fatal("live lock bypassed")
	}
	if _, err := RecoverRPKIRead(directory, testExchangePeer, testExchangeDigest); err == nil {
		t.Fatal("recovery bypassed lock")
	}
	if _, err := os.Stat(filepath.Join(directory, "rpki-exchange-"+testExchangePeer+".json.lock")); err != nil {
		t.Fatal("lock removed")
	}
}
