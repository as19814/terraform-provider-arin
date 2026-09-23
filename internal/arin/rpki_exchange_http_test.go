package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIHTTPExchange(t *testing.T) {
	for _, media := range []string{"application/rpki-updown", "application/rpki-publication"} {
		t.Run(media, func(t *testing.T) {
			local, _ := cmsSigningFixture(t)
			remote, _ := cmsSigningFixture(t)
			dir := privateExchangeDir(t)
			requestXML := []byte(`<request id="one"/>`)
			responseXML := []byte(`<response id="one"/>`)
			var config rpkiHTTPExchange
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "POST" || r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != media || r.Header.Get("Accept") != media || r.URL.RawQuery != "" {
					t.Error("unexpected protocol request")
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				verified, err := verifyRPKICMS(body, rpkiCMSTrust{Anchor: local.Anchor, Now: cmsTrustNow()})
				if err != nil || !bytes.Equal(verified.Content, requestXML) {
					t.Error("invalid signed request")
					return
				}
				peer, err := config.peerID()
				if err != nil {
					t.Error(err)
					return
				}
				raw, err := os.ReadFile(filepath.Join(dir, "rpki-exchange-"+peer+".json"))
				if err != nil {
					t.Error(err)
					return
				}
				var state rpkiExchangeState
				if json.Unmarshal(raw, &state) != nil || state.Pending == nil || state.Pending.RequestSHA256 != fmt.Sprintf("%x", sha256.Sum256(body)) {
					t.Error("dispatch preceded durable journal")
					return
				}
				signed, err := signRPKICMS(responseXML, remote, cmsTrustNow(), time.Time{})
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", media)
				_, _ = w.Write(signed)
			}))
			defer server.Close()
			config = rpkiHTTPExchange{Endpoint: server.URL + "/api", MediaType: media, Directory: dir, PeerScope: "child/parent", Identity: local, PeerAnchor: remote.Anchor, Clock: cmsTrustNow}
			validate := func(request, response []byte) error {
				if !bytes.Equal(request, requestXML) || !bytes.Equal(response, responseXML) {
					return errors.New("mismatch")
				}
				return nil
			}
			for i := 0; i < 2; i++ {
				out, err := config.exchange(context.Background(), "list", requestXML, validate)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(out, responseXML) {
					t.Fatal("response changed")
				}
			}
			if calls.Load() != 2 {
				t.Fatal("unexpected request count")
			}
			peer, _ := config.peerID()
			lease, err := openRPKIExchange(dir, peer)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			state, err := lease.State()
			if err != nil || state.Pending != nil || !state.LastReceived.Equal(cmsTrustNow()) {
				t.Fatal("response watermark not saved")
			}
		})
	}
}
func TestRPKIHTTPUncertainResponse(t *testing.T) {
	for _, mode := range []string{"lost", "redirect", "status", "content_type", "encoding", "oversized", "truncated", "signature", "wrong_peer", "correlation"} {
		t.Run(mode, func(t *testing.T) {
			local, _ := cmsSigningFixture(t)
			remote, _ := cmsSigningFixture(t)
			dir := privateExchangeDir(t)
			var calls atomic.Int32
			reply, err := signRPKICMS([]byte(`<response/>`), remote, cmsTrustNow(), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "signature" {
				reply[len(reply)-1] ^= 1
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/rpki-updown")
				switch mode {
				case "lost":
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
					return
				case "redirect":
					w.Header().Set("Location", "/secret-target")
					w.WriteHeader(307)
					return
				case "status":
					w.WriteHeader(500)
					_, _ = w.Write([]byte("secret-error-details"))
					return
				case "content_type":
					w.Header().Set("Content-Type", "application/xml")
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				case "oversized":
					w.Header().Set("Content-Length", fmt.Sprint((4<<20)+1))
					return
				case "truncated":
					w.Header().Set("Content-Length", fmt.Sprint(len(reply)+1))
				}
				_, _ = w.Write(reply)
			}))
			defer server.Close()
			config := rpkiHTTPExchange{Endpoint: server.URL, MediaType: "application/rpki-updown", Directory: dir, PeerScope: "child/parent", Identity: local, PeerAnchor: remote.Anchor, Clock: cmsTrustNow}
			if mode == "wrong_peer" {
				config.PeerAnchor = local.Anchor
			}
			validateCalls := 0
			validate := func([]byte, []byte) error {
				validateCalls++
				if mode == "correlation" {
					return errors.New("secret-validator-details")
				}
				return nil
			}
			out, err := config.exchange(context.Background(), "issue", []byte(`<request/>`), validate)
			if err == nil || out != nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("uncertain response returned payload or leaked details")
			}
			if mode != "correlation" && validateCalls != 0 {
				t.Fatal("unverified content reached protocol validator")
			}
			if _, err := config.exchange(context.Background(), "issue", []byte(`<request/>`), validate); err == nil {
				t.Fatal("pending request retried")
			}
			if calls.Load() != 1 {
				t.Fatalf("expected one POST, got %d", calls.Load())
			}
			peer, _ := config.peerID()
			lease, err := openRPKIExchange(dir, peer)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			state, err := lease.State()
			if err != nil || state.Pending == nil || !state.LastReceived.IsZero() {
				t.Fatal("uncertain response cleared pending or advanced watermark")
			}
		})
	}
}
func TestRPKIHTTPPreflight(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	config := rpkiHTTPExchange{Endpoint: "https://example.net/protocol", MediaType: "application/rpki-updown", Directory: privateExchangeDir(t), PeerScope: "child/parent", Identity: local, PeerAnchor: remote.Anchor, Clock: cmsTrustNow}
	if _, err := config.exchange(context.Background(), "list", []byte(`<request/>`), nil); err == nil {
		t.Fatal("missing validator accepted")
	}
	for _, endpoint := range []string{"http://example.net/protocol", "https://user:pass@example.net", "https://example.net/#fragment", "relative"} {
		bad := config
		bad.Endpoint = endpoint
		if _, err := bad.peerID(); err == nil {
			t.Fatal("invalid endpoint accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := config.exchange(ctx, "list", []byte(`<request/>`), func([]byte, []byte) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	peer, _ := config.peerID()
	lease, err := openRPKIExchange(config.Directory, peer)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	state, _ := lease.State()
	if state.Pending != nil || !state.LastSent.IsZero() {
		t.Fatal("canceled preflight created pending request")
	}
}

func TestRPKIHTTPResponseWatermark(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	var calls atomic.Int32
	var config rpkiHTTPExchange
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		peer, err := config.peerID()
		if err != nil {
			t.Error(err)
			return
		}
		if lease, err := openRPKIExchange(config.Directory, peer); err == nil {
			_ = lease.Close()
			t.Error("exchange released lock during dispatch")
		}
		when := cmsTrustNow()
		if count > 1 {
			when = when.Add(-time.Second)
		}
		signed, err := signRPKICMS([]byte(`<response/>`), remote, when, time.Time{})
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/rpki-updown")
		_, _ = w.Write(signed)
	}))
	defer server.Close()
	config = rpkiHTTPExchange{Endpoint: server.URL, MediaType: "application/rpki-updown", Directory: privateExchangeDir(t), PeerScope: "child/parent", Identity: local, PeerAnchor: remote.Anchor, Clock: cmsTrustNow}
	validate := func([]byte, []byte) error { return nil }
	if _, err := config.exchange(context.Background(), "list", []byte(`<request/>`), validate); err != nil {
		t.Fatal(err)
	}
	if _, err := config.exchange(context.Background(), "list", []byte(`<request/>`), validate); err == nil {
		t.Fatal("response timestamp rollback accepted")
	}
	if _, err := config.exchange(context.Background(), "list", []byte(`<request/>`), validate); err == nil {
		t.Fatal("rollback retried")
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected retry")
	}
}
