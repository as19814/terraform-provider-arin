package arin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIRRDPDiskRefresh(t *testing.T) {
	for _, mode := range []string{"delta", "fallback", "failed", "rollback", "304", "session", "delta_failed", "hash_fallback", "same_serial"} {
		t.Run(mode, func(t *testing.T) {
			dir := privateExchangeDir(t)
			now := cmsTrustNow()
			var phase, calls, snapshots atomic.Int32
			first := rrdpSnapshotTestBody(rrdpDeltaPublish(rrdpTestObjectURI, nil, []byte("old")))
			next := []byte(fmt.Sprintf(`<snapshot xmlns="%s" version="1" session_id="%s" serial="4">%s</snapshot>`, rrdpNamespace, rrdpTestSession, rrdpDeltaPublish(rrdpTestObjectURI, nil, []byte("new"))))
			if mode == "session" {
				next = []byte(strings.ReplaceAll(string(next), rrdpTestSession, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"))
			}
			delta := rrdpDeltaTestBody("4", rrdpDeltaPublish(rrdpTestObjectURI, []byte("old"), []byte("new")))
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				uri := server.URL + "/notification.xml"
				if info, err := os.Stat(filepath.Join(dir, rrdpDiskPrefix(uri)+".json.lock")); err != nil || !info.IsDir() {
					t.Error("HTTP dispatched without repository lease")
				}
				saved, err := readRRDPDisk(filepath.Join(dir, rrdpDiskPrefix(uri)+".json"), uri)
				if err != nil || saved == nil || saved.Record.LastAttempt.IsZero() {
					t.Error("attempt missing before request")
				}
				saved.Close()
				if phase.Load() == 0 {
					if r.URL.Path == "/snapshot.xml" {
						snapshots.Add(1)
						w.Write(first)
						return
					}
					w.Header().Set("Last-Modified", now.Format(http.TimeFormat))
					w.Write(rrdpNotificationTestBody("3", fmt.Sprintf(`<snapshot uri="%s/snapshot.xml" hash="%x"/>`, server.URL, sha256.Sum256(first))))
					return
				}
				switch mode {
				case "failed":
					w.WriteHeader(500)
					return
				case "304":
					if r.Header.Get("If-Modified-Since") != now.Format(http.TimeFormat) {
						t.Error("conditional metadata lost")
					}
					w.WriteHeader(304)
					return
				case "same_serial":
					w.Write(rrdpNotificationTestBody("3", fmt.Sprintf(`<snapshot uri="%s/snapshot4.xml" hash="%x"/>`, server.URL, sha256.Sum256(next))))
					return
				case "rollback":
					w.Write(rrdpNotificationTestBody("2", fmt.Sprintf(`<snapshot uri="%s/snapshot.xml" hash="%x"/>`, server.URL, sha256.Sum256(first))))
					return
				}
				if r.URL.Path == "/snapshot4.xml" {
					snapshots.Add(1)
					if mode == "delta_failed" {
						w.WriteHeader(500)
						return
					}
					w.Write(next)
					return
				}
				if r.URL.Path == "/delta.xml" {
					if mode == "fallback" || mode == "delta_failed" {
						w.WriteHeader(500)
					} else {
						if mode == "hash_fallback" {
							w.Write(append(delta, 'x'))
						} else {
							w.Write(delta)
						}
					}
					return
				}
				notification := rrdpNotificationTestBody("4", fmt.Sprintf(`<snapshot uri="%s/snapshot4.xml" hash="%x"/><delta serial="4" uri="%s/delta.xml" hash="%x"/>`, server.URL, sha256.Sum256(next), server.URL, sha256.Sum256(delta)))
				if mode == "session" {
					notification = []byte(strings.ReplaceAll(string(notification), rrdpTestSession, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"))
				}
				w.Write(notification)
			}))
			defer server.Close()
			client := rrdpHTTPClient{Transport: server.Client().Transport}
			uri := server.URL + "/notification.xml"
			cache, err := client.RefreshDiskPersistent(context.Background(), dir, uri, now)
			if err != nil {
				t.Fatal(err)
			}
			oldPack := cache.Record.Pack
			defer cache.Close() // Keep an old reader open while a new generation commits.
			polled, err := client.RefreshDiskPersistent(context.Background(), dir, uri, now.Add(time.Second))
			if !errors.Is(err, errRPKIRRDPPoll) || calls.Load() != 2 {
				t.Fatal("poll interval lost")
			}
			polled.Close()
			phase.Store(1)
			newer, err := client.RefreshDiskPersistent(context.Background(), dir, uri, now.Add(time.Minute))
			defer newer.Close()
			failure := mode == "failed" || mode == "rollback" || mode == "delta_failed" || mode == "same_serial"
			if (err != nil) != failure {
				t.Fatalf("refresh: %v", err)
			}
			want := "new"
			if failure || mode == "304" {
				want = "old"
			}
			data, err := newer.Objects.ReadObject(rrdpTestObjectURI)
			if err != nil || string(data) != want {
				t.Fatalf("objects: %v", err)
			}
			old, err := cache.Objects.ReadObject(rrdpTestObjectURI)
			if err != nil || string(old) != "old" {
				t.Fatal("prior reader changed")
			}
			reopened, err := readRRDPDisk(filepath.Join(dir, rrdpDiskPrefix(uri)+".json"), uri)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if !reopened.Record.LastAttempt.Equal(now.Add(time.Minute)) {
				t.Fatal("attempt not durable")
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 2 {
				t.Fatal("staging or obsolete generation leaked")
			}
			if mode == "delta" && snapshots.Load() != 1 {
				t.Fatal("delta replaced by snapshot")
			}
			if mode == "fallback" && snapshots.Load() != 2 {
				t.Fatal("failed delta did not fall back")
			}
			if want == "new" {
				if _, err := os.Stat(filepath.Join(dir, oldPack)); !os.IsNotExist(err) {
					t.Fatal("obsolete generation retained")
				}
			}
		})
	}
}

func TestRPKIRRDPDiskMigrationAndCorruption(t *testing.T) {
	for _, mode := range []string{"migrate", "empty", "locked", "version", "scope", "traversal", "overlap", "missing_pack", "pack_permissions", "state_permissions", "pack_symlink", "extra_field"} {
		t.Run(mode, func(t *testing.T) {
			dir := privateExchangeDir(t)
			now := cmsTrustNow()
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
			defer server.Close()
			uri := server.URL + "/notification.xml"
			path := filepath.Join(dir, rrdpDiskPrefix(uri)+".json")
			old := rrdpCache{Repository: rrdpRepository{NotificationURI: uri, Session: rrdpTestSession, Serial: "3", Objects: map[string][]byte{rrdpTestObjectURI: []byte("old"), rrdpTestObjectURI + "2": []byte("two")}}, Snapshot: rrdpFileReference{URI: server.URL + "/snapshot.xml"}, LastModified: now, LastAttempt: now}
			if mode == "empty" {
				old.Repository = rrdpRepository{NotificationURI: uri}
				old.Snapshot = rrdpFileReference{}
				old.LastModified = time.Time{}
			}
			if err := saveRRDPCache(path, old); err != nil {
				t.Fatal(err)
			}
			client := rrdpHTTPClient{Transport: server.Client().Transport}
			if mode == "migrate" || mode == "empty" {
				got, err := client.RefreshDiskPersistent(context.Background(), dir, uri, now.Add(time.Second))
				defer got.Close()
				if !errors.Is(err, errRPKIRRDPPoll) || got == nil || calls.Load() != 0 {
					t.Fatalf("migration bypassed polling: %v", err)
				}
				if got.Record.LastAttempt != old.LastAttempt || got.Record.LastModified != old.LastModified || got.Record.Serial != old.Repository.Serial || got.Record.Session != old.Repository.Session || got.Record.Snapshot != old.Snapshot {
					t.Fatal("migration discarded cache state")
				}
				if mode == "migrate" {
					data, err := got.Objects.ReadObject(rrdpTestObjectURI)
					if err != nil || string(data) != "old" {
						t.Fatal("migration discarded objects")
					}
				}
				return
			}
			initial, err := client.RefreshDiskPersistent(context.Background(), dir, uri, now.Add(time.Second))
			if !errors.Is(err, errRPKIRRDPPoll) {
				t.Fatal(err)
			}
			record := initial.Record
			initial.Close()
			packPath := filepath.Join(dir, record.Pack)
			switch mode {
			case "locked":
				if err := os.Mkdir(path+".lock", 0700); err != nil {
					t.Fatal(err)
				}
			case "version":
				record.Version = 99
			case "scope":
				record.NotificationURI = "https://other.example/notification.xml"
			case "traversal":
				record.Pack = "../elsewhere"
			case "overlap":
				e := record.Entries[rrdpTestObjectURI+"2"]
				e.Offset = record.Entries[rrdpTestObjectURI].Offset
				record.Entries[rrdpTestObjectURI+"2"] = e
			case "missing_pack":
				if err := os.Remove(packPath); err != nil {
					t.Fatal(err)
				}
			case "pack_permissions":
				if err := os.Chmod(packPath, 0644); err != nil {
					t.Fatal(err)
				}
			case "state_permissions":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "pack_symlink":
				if err := os.Rename(packPath, packPath+".other"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(packPath+".other", packPath); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "version" || mode == "scope" || mode == "traversal" || mode == "overlap" || mode == "extra_field" {
				raw, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "extra_field" {
					raw = append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"unexpected":true}`)...)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.RefreshDiskPersistent(context.Background(), dir, uri, now.Add(time.Minute))
			defer got.Close()
			if !errors.Is(err, errRPKIRRDPCache) || got != nil || calls.Load() != 0 {
				t.Fatalf("invalid durable cache did not stop before HTTP: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(before) {
				t.Fatal("invalid state was replaced")
			}
		})
	}
}

func TestRPKIRRDPDiskDeltaChain(t *testing.T) {
	for _, mode := range []string{"valid", "empty", "late_hash", "old_hash", "gap"} {
		t.Run(mode, func(t *testing.T) {
			dir := privateExchangeDir(t)
			snapshot := rrdpSnapshotTestBody(rrdpDeltaPublish(rrdpTestObjectURI, nil, []byte("old")))
			base, err := spoolRRDPSnapshot(context.Background(), dir, strings.NewReader(string(snapshot)), &rrdpNotification{Session: rrdpTestSession, Serial: "3", Snapshot: rrdpFileReference{Hash: sha256.Sum256(snapshot)}})
			if err != nil {
				t.Fatal(err)
			}
			defer base.Close()
			delta4 := rrdpDeltaTestBody("4", rrdpDeltaPublish(rrdpTestObjectURI, []byte("old"), []byte("new")))
			prior := []byte("new")
			if mode == "old_hash" {
				prior = []byte("wrong")
			}
			changes := rrdpDeltaWithdraw(rrdpTestObjectURI, prior)
			if mode != "empty" {
				changes += rrdpDeltaPublish(rrdpTestObjectURI+"2", nil, []byte("added"))
			}
			delta5 := rrdpDeltaTestBody("5", changes)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/4" {
					w.Write(delta4)
				} else {
					w.Write(delta5)
					if mode == "late_hash" {
						w.Write([]byte(" "))
					}
				}
			}))
			defer server.Close()
			n := &rrdpNotification{Session: rrdpTestSession, Serial: "5", Deltas: []rrdpDeltaReference{{Serial: "4", File: rrdpFileReference{URI: server.URL + "/4", Hash: sha256.Sum256(delta4)}}, {Serial: "5", File: rrdpFileReference{URI: server.URL + "/5", Hash: sha256.Sum256(delta5)}}}}
			if mode == "gap" {
				n.Deltas[1].Serial = "6"
			}
			client := rrdpHTTPClient{Transport: server.Client().Transport}
			candidate, err := client.diskDeltaCandidate(context.Background(), dir, n, &rrdpDiskCache{Record: rrdpDiskRecord{Session: rrdpTestSession, Serial: "3"}, Objects: base})
			if err != nil {
				t.Fatal(err)
			}
			valid := mode == "valid" || mode == "empty"
			if (candidate != nil) != valid {
				t.Fatal("partial delta chain exposed")
			}
			if valid {
				if _, err := candidate.ReadObject(rrdpTestObjectURI); !os.IsNotExist(err) {
					t.Fatal("withdrawn object retained")
				}
				if mode == "valid" {
					data, err := candidate.ReadObject(rrdpTestObjectURI + "2")
					if err != nil || string(data) != "added" {
						t.Fatal("new object missing")
					}
				}
				if err := candidate.Close(); err != nil {
					t.Fatal(err)
				}
			}
			old, err := base.ReadObject(rrdpTestObjectURI)
			if err != nil || string(old) != "old" {
				t.Fatal("base altered by delta chain")
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 1 {
				t.Fatal("candidate files leaked")
			}
		})
	}
}
