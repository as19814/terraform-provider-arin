package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDownloadSelectors(t *testing.T) {
	cases := []struct {
		request DownloadRequest
		path    string
	}{
		{DownloadRequest{Kind: "bulk_whois", Format: "zip"}, "/public/rest/downloads/bulkwhois"},
		{DownloadRequest{Kind: "bulk_whois", Format: "xml"}, "/public/rest/downloads/bulkwhois/asns+nets+orgs+pocs.xml"},
		{DownloadRequest{Kind: "bulk_whois", Format: "txt", Objects: []string{"pocs", "asns"}}, "/public/rest/downloads/bulkwhois/asns+pocs.txt"},
		{DownloadRequest{Kind: "invalid_pocs", Format: "zip"}, "/public/rest/downloads/nvpr"},
	}
	for _, tc := range cases {
		path, err := tc.request.path()
		if err != nil || path != tc.path {
			t.Fatalf("selector: %s %v", path, err)
		}
	}
	for _, req := range []DownloadRequest{{}, {Kind: "other", Format: "zip"}, {Kind: "bulk_whois", Format: "json"}, {Kind: "bulk_whois", Format: "zip", Objects: []string{"../nets"}}, {Kind: "bulk_whois", Format: "zip", Objects: []string{"nets", "nets"}}, {Kind: "invalid_pocs", Format: "xml"}, {Kind: "invalid_pocs", Format: "zip", Objects: []string{"nets"}}} {
		if req.Validate() == nil {
			t.Fatal("invalid selector accepted")
		}
	}
}
func TestDownloadStreamingAndLimits(t *testing.T) {
	const key = "secret &+?#"
	for _, mode := range []string{"complete", "chunked", "oversized", "chunked_oversized", "forbidden", "redirect", "html", "encoded", "unsafe_filename", "empty", "truncated", "writer_error"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			content := bytes.Repeat([]byte("streamed-data"), 100)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "GET" || r.URL.Path != "/public/rest/downloads/bulkwhois/asns+nets.xml" || r.URL.Query().Get("apikey") != key || len(r.URL.Query()) != 1 || r.Header.Get("Authorization") != "" || r.Header.Get("Accept-Encoding") != "identity" {
					t.Error("incorrect download endpoint or authentication")
				}
				if mode == "forbidden" {
					w.WriteHeader(403)
					fmt.Fprint(w, key+url.QueryEscape(key))
					return
				}
				if mode == "redirect" {
					w.Header().Set("Location", "/must-not-follow?apikey="+url.QueryEscape(key))
					w.WriteHeader(302)
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				w.Header().Set("Content-Disposition", `attachment; filename="arin_db.xml"`)
				if mode == "html" {
					w.Header().Set("Content-Type", "text/html")
				}
				if mode == "encoded" {
					w.Header().Set("Content-Encoding", "gzip")
				}
				if mode == "unsafe_filename" {
					w.Header().Set("Content-Disposition", `attachment; filename="../arin_db.xml"`)
				}
				if mode == "empty" {
					return
				}
				if mode == "truncated" {
					w.Header().Set("Content-Length", fmt.Sprint(len(content)+10))
				}
				if strings.HasPrefix(mode, "chunked") {
					w.(http.Flusher).Flush()
				}
				w.Write(content)
			}))
			defer server.Close()
			c, err := New(Config{APIKey: key, BaseURL: server.URL, DownloadBaseURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			var writer io.Writer = &out
			if mode == "writer_error" {
				writer = downloadErrorWriter{key}
			}
			limit := int64(len(content))
			if mode == "truncated" {
				limit += 100
			}
			if strings.Contains(mode, "oversized") {
				limit--
			}
			metadata, err := c.DownloadTo(context.Background(), DownloadRequest{Kind: "bulk_whois", Format: "xml", Objects: []string{"nets", "asns"}}, writer, limit)
			if mode == "complete" || mode == "chunked" {
				if err != nil || metadata.SizeBytes != int64(len(content)) || metadata.SHA256 != fmt.Sprintf("%x", sha256.Sum256(content)) || metadata.Filename != "arin_db.xml" || !bytes.Equal(out.Bytes(), content) {
					t.Fatalf("download not complete: %v", err)
				}
			} else {
				if err == nil || metadata != nil {
					t.Fatal("invalid download accepted")
				}
				if strings.Contains(err.Error(), key) || strings.Contains(err.Error(), url.QueryEscape(key)) {
					t.Fatal("credential leaked")
				}
			}
			if int64(out.Len()) > limit || calls.Load() != 1 {
				t.Fatal("limit exceeded or redirect followed")
			}
		})
	}
}

type downloadErrorWriter struct{ message string }

func (w downloadErrorWriter) Write([]byte) (int, error) { return 0, errors.New(w.message) }

type downloadTransport func(*http.Request) (*http.Response, error)

func (f downloadTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDownloadOriginsAndErrorRedaction(t *testing.T) {
	for _, tc := range []struct{ base, want string }{{ProductionURL, DownloadProductionURL}, {OTEURL, DownloadOTEURL}, {"https://example.test", ""}} {
		c, err := New(Config{BaseURL: tc.base})
		if err != nil || c.downloadBaseURL != tc.want {
			t.Fatal("wrong download default")
		}
	}
	if _, err := New(Config{DownloadBaseURL: "https://example.test/path?apikey=secret"}); err == nil {
		t.Fatal("unsafe download origin accepted")
	}
	calls := 0
	c, _ := New(Config{APIKey: "secret", HTTPClient: &http.Client{Transport: downloadTransport(func(r *http.Request) (*http.Response, error) { calls++; return nil, fmt.Errorf("leaking %s", r.URL) })}})
	if _, err := c.DownloadTo(context.Background(), DownloadRequest{Kind: "bulk_whois", Format: "zip"}, io.Discard, 100); err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "apikey") {
		t.Fatal("transport credential leak")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.DownloadTo(ctx, DownloadRequest{Kind: "bulk_whois", Format: "zip"}, io.Discard, 100)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("context cancellation lost")
	}
	before := calls
	for _, limit := range []int64{0, -1} {
		if _, err := c.DownloadTo(context.Background(), DownloadRequest{Kind: "bulk_whois", Format: "zip"}, io.Discard, limit); err == nil {
			t.Fatal("invalid byte limit accepted")
		}
	}
	if calls != before {
		t.Fatal("invalid configuration made requests")
	}
}

func TestDownloadLargerThanRegistrationLimit(t *testing.T) {
	block := bytes.Repeat([]byte("x"), 64<<10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		for i := 0; i < 80; i++ {
			w.Write(block)
		}
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL, DownloadBaseURL: server.URL})
	result, err := c.DownloadTo(context.Background(), DownloadRequest{Kind: "bulk_whois", Format: "txt", Objects: []string{"asns"}}, io.Discard, 5<<20)
	if err != nil || result.SizeBytes != 5<<20 {
		t.Fatalf("streaming inherited registration limit: %v", err)
	}
}
