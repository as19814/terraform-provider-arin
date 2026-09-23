package arin

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testExchangePeer = strings.Repeat("a", 64)
var testExchangeDigest = strings.Repeat("b", 64)

func TestRPKIExchangePersistence(t *testing.T) {
	dir := privateExchangeDir(t)
	now := cmsTrustNow()
	lease, err := openRPKIExchange(dir, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openRPKIExchange(dir, testExchangePeer); err == nil {
		t.Fatal("concurrent peer lease accepted")
	}
	other, err := openRPKIExchange(dir, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Begin(testExchangeDigest, "publication-list", now); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(lease.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved rpkiExchangeState
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Pending == nil || saved.Pending.RequestSHA256 != testExchangeDigest || !saved.LastSent.Equal(now) {
		t.Fatal("request not durably recorded")
	}
	state, err := lease.State()
	if err != nil {
		t.Fatal(err)
	}
	state.Pending.RequestSHA256 = "changed"
	state, _ = lease.State()
	if state.Pending.RequestSHA256 != testExchangeDigest {
		t.Fatal("state alias escaped")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	lease, err = openRPKIExchange(dir, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := lease.Begin(testExchangeDigest, "publication-list", now); err == nil {
		t.Fatal("pending request replayed")
	}
	if err := lease.Complete(strings.Repeat("d", 64), now); err == nil {
		t.Fatal("uncorrelated response completed request")
	}
	if err := lease.Complete(testExchangeDigest, now); err != nil {
		t.Fatal(err)
	}
	if err := lease.Begin(testExchangeDigest, "publication-list", now.Add(-time.Second)); err == nil {
		t.Fatal("outgoing timestamp rolled back")
	}
	if err := lease.Begin(testExchangeDigest, "publication-list", now); err != nil {
		t.Fatal(err)
	}
	if err := lease.Complete(testExchangeDigest, now.Add(-time.Second)); err == nil {
		t.Fatal("incoming timestamp rolled back")
	}
	if err := lease.Complete(testExchangeDigest, now); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := lease.State(); err == nil {
		t.Fatal("closed lease usable")
	}
	reopened, err := openRPKIExchange(dir, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	final, _ := reopened.State()
	if final.Pending != nil || !final.LastReceived.Equal(now) || !final.LastSent.Equal(now) {
		t.Fatal("completed history lost")
	}
	info, err := os.Stat(reopened.path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("journal permissions")
	}
}
func TestRPKIExchangeCorruption(t *testing.T) {
	for _, mode := range []string{"empty", "missing_fields", "duplicate_fields", "unknown_fields", "trailing", "version", "peer", "pending_time", "pending_hash", "file_mode", "symlink", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			dir := privateExchangeDir(t)
			lease, err := openRPKIExchange(dir, testExchangePeer)
			if err != nil {
				t.Fatal(err)
			}
			path := lease.path
			if err := lease.Begin(testExchangeDigest, "updown-list", cmsTrustNow()); err != nil {
				t.Fatal(err)
			}
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "empty":
				raw = nil
			case "missing_fields":
				raw = []byte(`{"version":1,"peer_id":"` + testExchangePeer + `"}`)
			case "duplicate_fields":
				raw = []byte(strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1))
			case "unknown_fields":
				raw = []byte(strings.Replace(string(raw), `"version":1`, `"version":1,"other":1`, 1))
			case "trailing":
				raw = append(raw, []byte(`{}`)...)
			case "version":
				raw = []byte(strings.Replace(string(raw), `"version":1`, `"version":2`, 1))
			case "peer":
				raw = []byte(strings.ReplaceAll(string(raw), testExchangePeer, strings.Repeat("e", 64)))
			case "pending_time":
				var s rpkiExchangeState
				_ = json.Unmarshal(raw, &s)
				s.Pending.SigningTime = s.Pending.SigningTime.Add(time.Second)
				raw, _ = json.Marshal(s)
			case "pending_hash":
				raw = []byte(strings.ReplaceAll(string(raw), testExchangeDigest, "bad"))
			case "file_mode":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+".target"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".target", path); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				raw = []byte(strings.Repeat(" ", 65537))
			}
			if mode != "file_mode" && mode != "symlink" {
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if reopened, err := openRPKIExchange(dir, testExchangePeer); err == nil {
				_ = reopened.Close()
				t.Fatal("corrupt journal accepted")
			}
			if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
				t.Fatal("failed open retained live lock")
			}
		})
	}
}
func TestRPKIExchangeFailedWrite(t *testing.T) {
	for _, completion := range []bool{false, true} {
		dir := privateExchangeDir(t)
		lease, err := openRPKIExchange(dir, testExchangePeer)
		if err != nil {
			t.Fatal(err)
		}
		if completion {
			if err := lease.Begin(testExchangeDigest, "updown-list", cmsTrustNow()); err != nil {
				t.Fatal(err)
			}
		}
		// Force atomic rename to fail using a real filesystem collision.
		if err := os.Remove(lease.path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(lease.path, 0700); err != nil {
			t.Fatal(err)
		}
		if completion {
			err = lease.Complete(testExchangeDigest, cmsTrustNow())
		} else {
			err = lease.Begin(testExchangeDigest, "updown-list", cmsTrustNow())
		}
		if err == nil {
			t.Fatal("failed persistence reported success")
		}
		if _, err := lease.State(); err == nil {
			t.Fatal("failed lease remained usable")
		}
		if err := lease.Begin(testExchangeDigest, "updown-list", cmsTrustNow()); err == nil {
			t.Fatal("failed lease permitted retry")
		}
		if err := lease.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
func TestRPKIExchangeCrashHelper(t *testing.T) {
	dir := os.Getenv("ARIN_TEST_EXCHANGE_CRASH_DIR")
	if dir == "" {
		return
	}
	lease, err := openRPKIExchange(dir, testExchangePeer)
	if err != nil {
		os.Exit(2)
	}
	if err := lease.Begin(testExchangeDigest, "updown-issue", cmsTrustNow()); err != nil {
		os.Exit(3)
	}
	os.Exit(0) // Simulated abrupt exit without releasing the lock.
}
func TestRPKIExchangeCrash(t *testing.T) {
	dir := privateExchangeDir(t)
	command := exec.Command(os.Args[0], "-test.run=^TestRPKIExchangeCrashHelper$")
	command.Env = append(os.Environ(), "ARIN_TEST_EXCHANGE_CRASH_DIR="+dir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("crash helper: %v: %s", err, output)
	}
	if lease, err := openRPKIExchange(dir, testExchangePeer); err == nil {
		_ = lease.Close()
		t.Fatal("stale crash lock silently removed")
	}
	// Simulate explicit operator reconciliation of the dead-process lock only.
	lock := filepath.Join(dir, "rpki-exchange-"+testExchangePeer+".json.lock")
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	lease, err := openRPKIExchange(dir, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	state, err := lease.State()
	if err != nil || state.Pending == nil || state.Pending.Operation != "updown-issue" {
		t.Fatal("crash lost pending request")
	}
	if err := lease.Begin(testExchangeDigest, "updown-issue", cmsTrustNow()); err == nil {
		t.Fatal("crashed request replay permitted")
	}
}

func privateExchangeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRPKIExchangeInputs(t *testing.T) {
	dir := privateExchangeDir(t)
	for _, peer := range []string{"", "../peer", strings.Repeat("a", 63), strings.Repeat("A", 64)} {
		if lease, err := openRPKIExchange(dir, peer); err == nil {
			_ = lease.Close()
			t.Fatal("invalid peer accepted")
		}
	}
	if lease, err := openRPKIExchange("relative", testExchangePeer); err == nil {
		_ = lease.Close()
		t.Fatal("relative directory accepted")
	}
	if lease, err := openRPKIExchange(filepath.Join(dir, "missing"), testExchangePeer); err == nil {
		_ = lease.Close()
		t.Fatal("missing directory accepted")
	}
	wide := privateExchangeDir(t)
	if err := os.Chmod(wide, 0755); err != nil {
		t.Fatal(err)
	}
	if lease, err := openRPKIExchange(wide, testExchangePeer); err == nil {
		_ = lease.Close()
		t.Fatal("nonprivate directory accepted")
	}
	lease, err := openRPKIExchange(dir, testExchangePeer)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	for _, tc := range []struct {
		digest, operation string
		when              time.Time
	}{
		{"bad", "updown-list", cmsTrustNow()},
		{testExchangeDigest, "invalid/operation", cmsTrustNow()},
		{testExchangeDigest, "updown-list", time.Time{}},
		{testExchangeDigest, "updown-list", cmsTrustNow().Add(time.Nanosecond)},
	} {
		if err := lease.Begin(tc.digest, tc.operation, tc.when); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	// A second process must see the same active lock, not merely a Go mutex.
	command := exec.Command(os.Args[0], "-test.run=^TestRPKIExchangeCrashHelper$")
	command.Env = append(os.Environ(), "ARIN_TEST_EXCHANGE_CRASH_DIR="+dir)
	err = command.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 2 {
		t.Fatalf("cross-process lock not enforced: %v", err)
	}
	state, err := lease.State()
	if err != nil || state.Pending != nil || !state.LastSent.IsZero() {
		t.Fatal("rejected calls changed state")
	}
}
