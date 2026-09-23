package arin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRPKIIssuanceReconciliationState(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	ski, err := rpkiPublicKeyIdentifier(local.Anchor.RawSubjectPublicKeyInfo)
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"valid", "wrong_digest", "wrong_operation", "wrong_time", "ambiguous", "conflict", "same_peer", "old_sent", "old_received", "save_failure", "missing_certificate", "wrong_class", "wrong_key", "bad_certificate"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			lease, err := openRPKIExchange(directory, testExchangePeer)
			if err != nil {
				t.Fatal(err)
			}
			now := cmsTrustNow()
			if err := lease.Begin(strings.Repeat("d", 64), "updown-list", now.Add(-time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := lease.Complete(strings.Repeat("d", 64), now); err != nil {
				t.Fatal(err)
			}
			operation := "updown-issue"
			if mode == "wrong_operation" {
				operation = "updown-revoke"
			}
			if err := lease.Begin(testExchangeDigest, operation, now); err != nil {
				t.Fatal(err)
			}
			observation := rpkiIssuanceRecoveryObservation{Plan: rpkiIssuanceRecoveryPlan{RequestSHA256: testExchangeDigest, SigningTime: now, SKI: ski, Request: RPKICertificateRequest{Class: "class"}}, Certificate: &RPKIIssuedCertificate{Class: "class", SKI: ski, CertificatePEM: certificateTestPEM("CERTIFICATE", local.Anchor.Raw)}, RecoveryPeerID: strings.Repeat("e", 64), Outcome: "matches_request", Sent: now.Add(time.Second), Received: now.Add(time.Second)}
			originalPath := lease.path
			switch mode {
			case "missing_certificate":
				observation.Certificate = nil
			case "wrong_class":
				observation.Certificate.Class = "other"
			case "wrong_key":
				observation.Certificate.SKI = "AAAAAAAAAAAAAAAAAAAAAAAAAAA"
			case "bad_certificate":
				observation.Certificate.CertificatePEM = "invalid"
			case "wrong_digest":
				observation.Plan.RequestSHA256 = strings.Repeat("f", 64)
			case "wrong_time":
				observation.Plan.SigningTime = now.Add(-time.Second)
			case "ambiguous", "conflict":
				observation.Outcome = mode
			case "same_peer":
				observation.RecoveryPeerID = testExchangePeer
			case "old_sent":
				observation.Sent = now
			case "old_received":
				observation.Received = now.Add(-time.Second)
			case "save_failure":
				lease.path = filepath.Join(directory, "missing", "journal.json")
			}
			err = lease.reconcileIssuance(observation)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode=%s err=%v", mode, err)
			}
			lease.path = originalPath
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := openRPKIExchange(directory, testExchangePeer)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			state, err := reopened.State()
			if err != nil {
				t.Fatal(err)
			}
			if mode == "valid" {
				if state.Pending != nil || state.ReconciledIssuance == nil || !state.LastSent.Equal(observation.Sent) {
					t.Fatal("valid reconciliation not persisted")
				}
				if err := reopened.Begin(strings.Repeat("f", 64), "updown-issue", now); err == nil {
					t.Fatal("reconciliation allowed signing-time rollback")
				}
				if err := reopened.Begin(strings.Repeat("f", 64), "updown-issue", now.Add(2*time.Second)); err != nil {
					t.Fatal(err)
				}
			} else if state.Pending == nil || state.ReconciledIssuance != nil || !state.LastSent.Equal(now) {
				t.Fatal("rejected reconciliation changed durable journal")
			}
		})
	}
}
func TestRPKIIssuanceReconciliationCorruption(t *testing.T) {
	for _, mode := range []string{"certificate_digest", "class", "key", "operation", "original_digest", "sent", "received", "peer"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			now := cmsTrustNow()
			receipt := &rpkiIssuanceReconciliation{Original: rpkiPendingExchange{RequestSHA256: testExchangeDigest, Operation: "updown-issue", SigningTime: now}, RecoveryPeerID: strings.Repeat("c", 64), Class: "class", SKI: "AAAAAAAAAAAAAAAAAAAAAAAAAAA", CertificateSHA256: strings.Repeat("d", 64), Sent: now.Add(time.Second), Received: now.Add(time.Second)}
			state := rpkiExchangeState{Version: 1, PeerID: testExchangePeer, LastSent: receipt.Sent, LastReceived: receipt.Received, ReconciledIssuance: receipt}
			switch mode {
			case "certificate_digest":
				receipt.CertificateSHA256 = "invalid"
			case "class":
				receipt.Class = ""
			case "key":
				receipt.SKI = "invalid"
			case "operation":
				receipt.Original.Operation = "updown-revoke"
			case "original_digest":
				receipt.Original.RequestSHA256 = "invalid"
			case "sent":
				receipt.Sent = now.Add(2 * time.Second)
			case "received":
				receipt.Received = now.Add(2 * time.Second)
			case "peer":
				receipt.RecoveryPeerID = testExchangePeer
			}
			raw, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "rpki-exchange-"+testExchangePeer+".json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			if lease, err := openRPKIExchange(directory, testExchangePeer); err == nil {
				_ = lease.Close()
				t.Fatal("corrupt reconciliation accepted")
			}
		})
	}
}
