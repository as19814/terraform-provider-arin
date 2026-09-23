package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIPublicationRecoveryRead(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	wrong, _ := cmsSigningFixture(t)
	uri := "rsync://repo.example/module/object.cer"
	payload := []byte("published")
	hash := fmt.Sprintf("%x", sha256.Sum256(payload))
	batch, err := buildRPKIPublicationBatch([]rpkiPublicationChange{{URI: uri, DER: payload}})
	if err != nil {
		t.Fatal(err)
	}
	original, err := signRPKICMS(batch, local, cmsTrustNow(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(original))
	for _, mode := range []string{"after", "before", "conflict", "old_reply", "wrong_signer", "unavailable", "same_second", "wrong_digest", "original_locked", "recovery_rollback"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			var calls atomic.Int32
			var exchange rpkiHTTPExchange
			var peer string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				// The original mutation must remain locked throughout the recovery request.
				if lease, err := openRPKIExchange(directory, peer); err == nil {
					_ = lease.Close()
					t.Error("original peer unlocked during recovery")
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				request, err := verifyRPKICMS(raw, rpkiCMSTrust{Anchor: local.Anchor, Now: cmsTrustNow().Add(2 * time.Second)})
				if err != nil || string(request.Content) != rpkiPublicationListQuery || !request.SigningTime.After(cmsTrustNow()) {
					t.Error("recovery resent mutation or used stale signing time")
					return
				}
				if mode == "unavailable" {
					w.WriteHeader(503)
					return
				}
				objects := `<list uri="` + uri + `" hash="` + hash + `"/>`
				if mode == "before" {
					objects = ""
				}
				if mode == "conflict" {
					objects = `<list uri="` + uri + `" hash="` + strings.Repeat("c", 64) + `"/>`
				}
				signing := remote
				if mode == "wrong_signer" {
					signing = wrong
				}
				now := cmsTrustNow().Add(2 * time.Second)
				if mode == "old_reply" {
					now = cmsTrustNow()
				}
				if mode == "recovery_rollback" && calls.Load() > 1 {
					now = cmsTrustNow().Add(time.Second)
				}
				response, err := signRPKICMS([]byte(publicationTestReply(objects)), signing, now, time.Time{})
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/rpki-publication")
				_, _ = w.Write(response)
			}))
			defer server.Close()
			now := cmsTrustNow().Add(2 * time.Second)
			if mode == "same_second" {
				now = cmsTrustNow()
			}
			exchange = rpkiHTTPExchange{Endpoint: server.URL, MediaType: "application/rpki-publication", Directory: directory, PeerScope: "publisher", Identity: local, PeerAnchor: remote.Anchor, Clock: func() time.Time { return now }}
			peer, err = exchange.peerID()
			if err != nil {
				t.Fatal(err)
			}
			lease, err := openRPKIExchange(directory, peer)
			if err != nil {
				t.Fatal(err)
			}
			if err := lease.Begin(strings.Repeat("d", 64), "publication-list", cmsTrustNow().Add(-time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := lease.Complete(strings.Repeat("d", 64), cmsTrustNow().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := lease.BeginSigned(digest, "publication-batch", cmsTrustNow(), original); err != nil {
				t.Fatal(err)
			}
			path := lease.path
			if mode != "original_locked" {
				if err := lease.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				defer lease.Close()
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			expected := digest
			if mode == "wrong_digest" {
				expected = strings.Repeat("e", 64)
			}
			observation, err := exchange.observePendingPublication(context.Background(), expected)
			valid := mode == "after" || mode == "before" || mode == "conflict" || mode == "recovery_rollback"
			if (err == nil) != valid {
				t.Fatalf("valid=%v err=%v", valid, err)
			}
			if valid {
				want := map[string]string{"after": "matches_after", "before": "matches_before", "conflict": "conflict", "recovery_rollback": "matches_after"}[mode]
				if observation.Outcome != want || observation.Plan.RequestSHA256 != digest || observation.RecoveryPeerID == peer || !observation.Sent.After(cmsTrustNow()) || observation.Received.Before(cmsTrustNow().Add(time.Second)) {
					t.Fatal("wrong recovery observation")
				}
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("recovery altered original pending journal")
			}
			if mode == "recovery_rollback" {
				if _, err := exchange.observePendingPublication(context.Background(), digest); err == nil || calls.Load() != 2 {
					t.Fatal("recovery receive watermark rolled back")
				}
			}
			if mode == "old_reply" || mode == "wrong_signer" || mode == "unavailable" {
				if _, err := exchange.observePendingPublication(context.Background(), digest); err == nil || calls.Load() != 1 {
					t.Fatal("failed recovery read retried automatically")
				}
			}
			if mode == "same_second" || mode == "wrong_digest" || mode == "original_locked" {
				if calls.Load() != 0 {
					t.Fatal("invalid recovery dispatched")
				}
			}
		})
	}
}

func TestRPKIRecoveryPeerIdentity(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	exchange := rpkiHTTPExchange{Endpoint: "https://repo.example/publication", MediaType: "application/rpki-publication", PeerScope: "publisher", Identity: local, PeerAnchor: remote.Anchor}
	legacy, _ := json.Marshal(struct {
		Endpoint, Media, Scope string
		Local, Peer            [32]byte
	}{exchange.Endpoint, exchange.MediaType, exchange.PeerScope, sha256.Sum256(local.Anchor.Raw), sha256.Sum256(remote.Anchor.Raw)})
	peer, err := exchange.peerID()
	if err != nil || peer != fmt.Sprintf("%x", sha256.Sum256(legacy)) {
		t.Fatal("normal peer identity changed")
	}
	exchange.RecoveryOf = strings.Repeat("a", 64)
	recovery, err := exchange.peerID()
	if err != nil || recovery == peer {
		t.Fatal("recovery scope collided with original peer")
	}
	exchange.RecoveryOf = strings.Repeat("b", 64)
	other, err := exchange.peerID()
	if err != nil || other == recovery {
		t.Fatal("distinct mutation recovery scopes collided")
	}
	exchange.RecoveryOf = "invalid"
	if _, err := exchange.peerID(); err == nil {
		t.Fatal("invalid recovery digest accepted")
	}
}
