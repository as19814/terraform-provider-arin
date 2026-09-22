package arin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const orgXML = `<org xmlns="http://www.arin.net/regrws/core/v1"><handle>EXAMPLE-1</handle><orgName>Example &amp; Co</orgName><registrationDate>2026-01-01</registrationDate></org>`

func TestGetOrganization(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/rest/org/EXAMPLE-1" || r.URL.RawQuery != "" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "ApiKey test-secret" || r.Header.Get("Accept") != "application/xml" || r.Header.Get("User-Agent") != "test-agent" {
			t.Error("incorrect headers")
		}
		fmt.Fprint(w, orgXML)
	}))
	defer server.Close()
	client, err := New(Config{APIKey: "test-secret", BaseURL: server.URL, UserAgent: "test-agent"})
	if err != nil {
		t.Fatal(err)
	}
	org, err := client.GetOrganization(context.Background(), "EXAMPLE-1")
	if err != nil {
		t.Fatal(err)
	}
	if org.Name != "Example & Co" || org.RegistrationDate != "2026-01-01" {
		t.Fatalf("unexpected organization: %+v", org)
	}
}

func TestErrorResponses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		status     int
		body, code string
	}{
		{"not found", 404, `<error><code>E_OBJECT_NOT_FOUND</code><message>test-secret is unavailable</message></error>`, "E_OBJECT_NOT_FOUND"},
		{"authentication", 401, `<error><code>E_AUTHENTICATION</code><message>test-secret</message></error>`, "E_AUTHENTICATION"},
		{"rate limit", 429, `<error><code>E_TOO_MANY_REQUESTS</code></error>`, "E_TOO_MANY_REQUESTS"},
		{"proxy HTML", 502, `<html>test-secret</html>`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var count atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer s.Close()
			c, err := New(Config{APIKey: "test-secret", BaseURL: s.URL})
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.GetOrganization(context.Background(), "EXAMPLE-1")
			var ae *APIError
			if !errors.As(err, &ae) || ae.StatusCode != tc.status || ae.Code != tc.code {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(err.Error(), "test-secret") {
				t.Fatal("credential leaked in error")
			}
			if IsNotFound(err) != (tc.status == 404) {
				t.Fatal("incorrect not-found classification")
			}
			if count.Load() != 1 {
				t.Fatal("request was retried")
			}
		})
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer dest.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL, http.StatusTemporaryRedirect)
	}))
	defer src.Close()
	supplied := &http.Client{Timeout: time.Minute}
	c, err := New(Config{APIKey: "secret", BaseURL: src.URL, HTTPClient: supplied})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetOrganization(context.Background(), "EXAMPLE-1")
	var ae *APIError
	if !errors.As(err, &ae) || ae.StatusCode != 307 || calls.Load() != 0 {
		t.Fatalf("redirect was not rejected: %v", err)
	}
	if supplied.Timeout != time.Minute || supplied.CheckRedirect != nil {
		t.Fatal("mutated supplied HTTP client")
	}
}

func TestInvalidPayloads(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", `<org>`, `<html/>`, `<org xmlns="http://www.arin.net/regrws/core/v1"/>`, strings.Replace(orgXML, "EXAMPLE-1", "OTHER-1", 1), strings.Repeat("x", maxResponseBytes+1)} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		c, err := New(Config{APIKey: "secret", BaseURL: s.URL})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.GetOrganization(context.Background(), "EXAMPLE-1"); err == nil {
			t.Error("invalid payload accepted")
		}
		s.Close()
	}
}

func TestRequestCancellationAndTimeout(t *testing.T) {
	t.Parallel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	c, err := New(Config{APIKey: "secret", BaseURL: s.URL, Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = c.GetOrganization(ctx, "EXAMPLE-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if _, err = c.GetOrganization(context.Background(), "EXAMPLE-1"); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout not enforced: %v", err)
	}
}

func TestConfigurationAndHandleValidation(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{ProductionURL, OTEURL, "http://127.0.0.1:1234", "http://[::1]:1234", "https://example.net/"} {
		if err := ValidateBaseURL(raw); err != nil {
			t.Errorf("valid URL rejected: %s", raw)
		}
	}
	for _, raw := range []string{"", ":bad", "https://", "http://example.net", "https://user:pass@example.net", "https://example.net/rest", "https://example.net?apikey=secret", "https://example.net#fragment"} {
		if err := ValidateBaseURL(raw); err == nil {
			t.Errorf("invalid URL accepted: %s", raw)
		}
	}
	for _, key := range []string{"", "  ", "secret\r\nheader"} {
		if _, err := New(Config{APIKey: key}); err == nil {
			t.Error("invalid key accepted")
		}
	}
	c, err := New(Config{APIKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if c.baseURL != ProductionURL || c.http.Timeout != DefaultTimeout {
		t.Fatal("incorrect defaults")
	}
	for _, handle := range []string{"", "../net", "A/B", "A?apikey=secret", "A#fragment"} {
		if _, err := c.GetOrganization(context.Background(), handle); err == nil {
			t.Error("invalid handle accepted")
		}
	}
}
