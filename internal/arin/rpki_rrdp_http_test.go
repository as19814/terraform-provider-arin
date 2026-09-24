package arin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIRRDPHTTPS(t *testing.T) {
	snapshot := rrdpSnapshotTestBody(`<publish uri="rsync://repo.example/module/a.cer">YWJj</publish>`)
	sum := sha256.Sum256(snapshot)
	modified := cmsTrustNow()
	var notifications, snapshots atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-API-Key") != "" {
			t.Error("unexpected method or credentials")
		}
		switch r.URL.Path {
		case "/notification.xml":
			notifications.Add(1)
			if r.Header.Get("If-Modified-Since") != "" {
				if r.Header.Get("If-Modified-Since") != modified.Format(http.TimeFormat) {
					t.Error("conditional timestamp mismatch")
				}
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Last-Modified", modified.Format(http.TimeFormat))
			body := rrdpNotificationTestBody("3", fmt.Sprintf(`<snapshot uri="%s/snapshot.xml" hash="%s"/>`, server.URL, hex.EncodeToString(sum[:])))
			_, _ = w.Write(body)
		case "/snapshot.xml":
			snapshots.Add(1)
			_, _ = w.Write(snapshot)
		case "/redirect":
			http.Redirect(w, r, "/notification.xml", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := rrdpHTTPClient{Transport: server.Client().Transport}
	result, err := client.FetchNotification(context.Background(), server.URL+"/redirect", time.Time{})
	if err != nil || result.NotModified || !result.LastModified.Equal(modified) {
		t.Fatalf("notification failed: %v", err)
	}
	objects, err := client.FetchSnapshot(context.Background(), result.Notification)
	if err != nil || string(objects["rsync://repo.example/module/a.cer"]) != "abc" {
		t.Fatalf("snapshot failed: %v", err)
	}
	cached, err := client.FetchNotification(context.Background(), server.URL+"/notification.xml", result.LastModified)
	if err != nil || !cached.NotModified || cached.Notification != nil || !cached.LastModified.Equal(modified) {
		t.Fatalf("conditional request failed: %v", err)
	}
	if notifications.Load() != 2 || snapshots.Load() != 1 {
		t.Fatal("unexpected HTTP request count")
	}
	wrong := *result.Notification
	wrong.Snapshot.Hash[0] ^= 1
	if _, err := client.FetchSnapshot(context.Background(), &wrong); err == nil {
		t.Fatal("hash mismatch accepted")
	}
	if _, err := (rrdpHTTPClient{}).FetchNotification(context.Background(), server.URL+"/notification.xml", time.Time{}); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
}

func TestRPKIRRDPHTTPFailures(t *testing.T) {
	var insecureCalls atomic.Int32
	insecure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { insecureCalls.Add(1) }))
	defer insecure.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/304":
			w.WriteHeader(http.StatusNotModified)
		case "/404":
			w.WriteHeader(http.StatusNotFound)
		case "/length":
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("small"))
		case "/stream":
			w.(http.Flusher).Flush()
			_, _ = w.Write([]byte("0123456789"))
		case "/modified":
			w.Header().Set("Last-Modified", "invalid")
			_, _ = w.Write([]byte("a"))
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/downgrade":
			http.Redirect(w, r, insecure.URL, http.StatusFound)
		case "/slow":
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	client := rrdpHTTPClient{Transport: server.Client().Transport, Timeout: 100 * time.Millisecond}
	for _, path := range []string{"/304", "/404", "/length", "/stream", "/modified", "/loop", "/downgrade", "/slow"} {
		t.Run(path, func(t *testing.T) {
			if _, _, _, err := client.get(context.Background(), server.URL+path, 8, time.Time{}); err == nil {
				t.Fatal("retrieval failure accepted")
			}
		})
	}
	if insecureCalls.Load() != 0 {
		t.Fatal("followed HTTPS downgrade")
	}
	for _, uri := range []string{insecure.URL, "https://user:pass@repo.example/notification.xml", "https://repo.example/n#fragment", "relative"} {
		if _, err := client.FetchNotification(context.Background(), uri, time.Time{}); err == nil {
			t.Fatal("unsafe URI accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.FetchNotification(ctx, server.URL, time.Time{}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
}

func TestRPKIRRDPHTTPErrorPrivacy(t *testing.T) {
	client := rrdpHTTPClient{Transport: rrdpTestTransport(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("secret response from " + r.URL.String())
	})}
	_, err := client.FetchNotification(context.Background(), "https://repo.example/n?token=private", time.Time{})
	if err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
		t.Fatal("transport details leaked")
	}
	client.Transport = rrdpTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("malformed"))}, nil
	})
	if _, err := client.FetchNotification(context.Background(), "https://repo.example/n", time.Time{}); err == nil {
		t.Fatal("invalid XML accepted")
	}
}

// rrdpTestTransport injects HTTP behavior without a live repository.
type rrdpTestTransport func(*http.Request) (*http.Response, error)

func (f rrdpTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
