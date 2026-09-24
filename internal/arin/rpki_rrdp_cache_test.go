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
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIRRDPPersistentRestart(t *testing.T) {
	directory := privateExchangeDir(t)
	now := cmsTrustNow()
	var calls atomic.Int32
	var mode atomic.Int32
	snapshot := rrdpSnapshotTestBody(`<publish uri="` + rrdpTestObjectURI + `">bmV3</publish>`)
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		uri := server.URL + "/notification.xml"
		path := filepath.Join(directory, "rpki-rrdp-"+rpkiManifestDigest([]byte(uri))+".json")
		saved, err := readRRDPCache(path, uri)
		if err != nil || saved == nil || saved.LastAttempt.IsZero() {
			t.Error("attempt not persisted before HTTP")
		}
		if mode.Load() == 2 {
			w.WriteHeader(500)
			return
		}
		if mode.Load() == 1 {
			if r.Header.Get("If-Modified-Since") != now.Format(http.TimeFormat) {
				t.Error("conditional header lost across restart")
			}
			w.WriteHeader(304)
			return
		}
		if r.URL.Path == "/snapshot.xml" {
			_, _ = w.Write(snapshot)
			return
		}
		w.Header().Set("Last-Modified", now.Format(http.TimeFormat))
		_, _ = w.Write(rrdpNotificationTestBody("3", fmt.Sprintf(`<snapshot uri="%s/snapshot.xml" hash="%x"/>`, server.URL, sha256.Sum256(snapshot))))
	}))
	defer server.Close()
	client := rrdpHTTPClient{Transport: server.Client().Transport}
	uri := server.URL + "/notification.xml"
	path := filepath.Join(directory, "rpki-rrdp-"+rpkiManifestDigest([]byte(uri))+".json")
	got, err := client.RefreshPersistent(context.Background(), directory, uri, now)
	if err != nil || string(got.Repository.Objects[rrdpTestObjectURI]) != "new" {
		t.Fatalf("initial refresh: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("cache is not private")
	}
	got.Repository.Objects[rrdpTestObjectURI][0] = 'X'
	if _, err := client.RefreshPersistent(context.Background(), directory, uri, now.Add(30*time.Second)); !errors.Is(err, errRPKIRRDPPoll) || calls.Load() != 2 {
		t.Fatal("restart bypassed polling interval")
	}
	mode.Store(1)
	got, err = client.RefreshPersistent(context.Background(), directory, uri, now.Add(time.Minute))
	if err != nil || string(got.Repository.Objects[rrdpTestObjectURI]) != "new" {
		t.Fatalf("conditional refresh: %v", err)
	}
	mode.Store(2)
	got, err = client.RefreshPersistent(context.Background(), directory, uri, now.Add(2*time.Minute))
	if err == nil || string(got.Repository.Objects[rrdpTestObjectURI]) != "new" {
		t.Fatal("failed fetch lost previous objects")
	}
	saved, err := readRRDPCache(path, uri)
	if err != nil || !saved.LastAttempt.Equal(now.Add(2*time.Minute)) {
		t.Fatal("failed attempt was not persisted")
	}
	if _, err := client.RefreshPersistent(context.Background(), directory, uri, now.Add(150*time.Second)); !errors.Is(err, errRPKIRRDPPoll) || calls.Load() != 4 {
		t.Fatal("failed fetch bypassed polling interval")
	}
}

func TestRPKIRRDPPersistentConcurrentFailure(t *testing.T) {
	directory := privateExchangeDir(t)
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; w.WriteHeader(500) }))
	defer server.Close()
	client := rrdpHTTPClient{Transport: server.Client().Transport}
	done := make(chan error, 1)
	go func() {
		_, err := client.RefreshPersistent(context.Background(), directory, server.URL, cmsTrustNow())
		done <- err
	}()
	<-entered
	_, err := client.RefreshPersistent(context.Background(), directory, server.URL, cmsTrustNow().Add(time.Minute))
	close(release)
	firstErr := <-done
	if !errors.Is(err, errRPKIRRDPCache) || firstErr == nil {
		t.Fatal("concurrent refresh or failed retrieval accepted")
	}
	path := filepath.Join(directory, "rpki-rrdp-"+rpkiManifestDigest([]byte(server.URL))+".json")
	saved, err := readRRDPCache(path, server.URL)
	if err != nil || saved == nil || saved.Repository.Session != "" || !saved.LastAttempt.Equal(cmsTrustNow()) {
		t.Fatal("initial failure did not retain attempt")
	}
	if _, err := client.RefreshPersistent(context.Background(), directory, server.URL, cmsTrustNow().Add(time.Second)); !errors.Is(err, errRPKIRRDPPoll) {
		t.Fatal("initial failure lost polling interval")
	}
}

func TestRPKIRRDPCacheRejectsInvalidState(t *testing.T) {
	for _, mode := range []string{"corrupt", "version", "unknown", "duplicate", "scope", "serial", "snapshot", "public", "symlink", "lock"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			uri := "https://repository.example/notification.xml"
			path := filepath.Join(directory, "rpki-rrdp-"+rpkiManifestDigest([]byte(uri))+".json")
			record := rrdpCacheRecord{Version: 1, Cache: rrdpCache{Repository: rrdpRepository{NotificationURI: uri, Session: rrdpTestSession, Serial: "1"}, Snapshot: rrdpFileReference{URI: "https://repository.example/snapshot.xml"}, LastAttempt: cmsTrustNow()}}
			switch mode {
			case "version":
				record.Version = 2
			case "scope":
				record.Cache.Repository.NotificationURI = "https://other.example/notification.xml"
			case "serial":
				record.Cache.Repository.Serial = "01"
			case "snapshot":
				record.Cache.Snapshot.URI = "http://repository.example/snapshot.xml"
			}
			raw, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "corrupt":
				raw = []byte("{")
			case "unknown":
				raw = append([]byte(`{"extra":true,`), raw[1:]...)
			case "duplicate":
				raw = append([]byte(`{"version":1,`), raw[1:]...)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "public":
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
			case "lock":
				if err := os.Mkdir(path+".lock", 0700); err != nil {
					t.Fatal(err)
				}
			}
			client := rrdpHTTPClient{Transport: rrdpTestTransport(func(*http.Request) (*http.Response, error) {
				t.Error("invalid cache reached HTTP")
				return nil, errors.New("unexpected request")
			})}
			if _, err := client.RefreshPersistent(context.Background(), directory, uri, cmsTrustNow().Add(time.Minute)); !errors.Is(err, errRPKIRRDPCache) {
				t.Fatalf("invalid cache accepted: %v", err)
			}
		})
	}
}
