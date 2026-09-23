package arin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRPKIRevocationReconciliationState(t *testing.T) {
	for _, mode := range []string{"valid", "wrong_digest", "wrong_operation", "wrong_time", "ambiguous", "conflict", "same_peer", "old_sent", "old_received", "save_failure", "missing_proof", "wrong_class", "wrong_key", "bad_crl"} {
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
			operation := "updown-revoke"
			if mode == "wrong_operation" {
				operation = "updown-issue"
			}
			if err := lease.Begin(testExchangeDigest, operation, now); err != nil {
				t.Fatal(err)
			}
			observation := rpkiRevocationRecoveryObservation{Plan: rpkiRevocationRecoveryPlan{RequestSHA256: testExchangeDigest, SigningTime: now, SKI: "AAAAAAAAAAAAAAAAAAAAAAAAAAA", Class: "class"}, Proof: &rpkiRevocationProof{CertificateSHA256: strings.Repeat("a", 64), IssuerSHA256: strings.Repeat("b", 64), CRLSHA256: strings.Repeat("c", 64), ManifestSHA256: strings.Repeat("d", 64), CRLURI: "rsync://repo.example/module/issuer.crl", SKI: "AAAAAAAAAAAAAAAAAAAAAAAAAAA"}, RecoveryPeerID: strings.Repeat("e", 64), Outcome: "key_absent", Sent: now.Add(time.Second), Received: now.Add(time.Second)}
			originalPath := lease.path
			switch mode {
			case "missing_proof":
				observation.Proof = nil
			case "wrong_class":
				observation.Plan.Class = ""
			case "wrong_key":
				observation.Proof.SKI = "BBBBBBBBBBBBBBBBBBBBBBBBBBA"
			case "bad_crl":
				observation.Proof.CRLSHA256 = "invalid"
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
			err = lease.reconcileRevocation(observation)
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
				if state.Pending != nil || state.ReconciledRevocation == nil || !state.LastSent.Equal(observation.Sent) {
					t.Fatal("valid reconciliation not persisted")
				}
				if err := reopened.Begin(strings.Repeat("f", 64), "updown-revoke", now); err == nil {
					t.Fatal("reconciliation allowed signing-time rollback")
				}
				if err := reopened.Begin(strings.Repeat("f", 64), "updown-revoke", now.Add(2*time.Second)); err != nil {
					t.Fatal(err)
				}
			} else if state.Pending == nil || state.ReconciledRevocation != nil || !state.LastSent.Equal(now) {
				t.Fatal("rejected reconciliation changed durable journal")
			}
		})
	}
}
func TestRPKIRevocationReconciliationCorruption(t *testing.T) {
	for _, mode := range []string{"certificate_digest", "issuer_digest", "crl_digest", "manifest_digest", "crl_uri", "class", "key", "operation", "original_digest", "sent", "received", "peer", "expiry_without_check", "check_without_expiry", "before_expiry", "bad_expiry", "expiry_offset", "class_evidence_digest"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			now := cmsTrustNow()
			receipt := &rpkiRevocationReconciliation{Original: rpkiPendingExchange{RequestSHA256: testExchangeDigest, Operation: "updown-revoke", SigningTime: now}, RecoveryPeerID: strings.Repeat("c", 64), Class: "class", Proof: rpkiRevocationProof{CertificateSHA256: strings.Repeat("a", 64), IssuerSHA256: strings.Repeat("b", 64), CRLSHA256: strings.Repeat("c", 64), ManifestSHA256: strings.Repeat("d", 64), CRLURI: "rsync://repo.example/module/issuer.crl", SKI: "AAAAAAAAAAAAAAAAAAAAAAAAAAA"}, Sent: now.Add(time.Second), Received: now.Add(time.Second)}
			state := rpkiExchangeState{Version: 1, PeerID: testExchangePeer, LastSent: receipt.Sent, LastReceived: receipt.Received, ReconciledRevocation: receipt}
			switch mode {
			case "class_evidence_digest":
				receipt.Proof.ClassEvidenceSHA256 = "invalid"
			case "expiry_without_check":
				receipt.Proof.ExpiredAt = now.Format(time.RFC3339Nano)
			case "check_without_expiry":
				receipt.Proof.CheckedAt = now.Format(time.RFC3339Nano)
			case "before_expiry":
				receipt.Proof.ExpiredAt = now.Format(time.RFC3339Nano)
				receipt.Proof.CheckedAt = now.Add(-time.Second).Format(time.RFC3339Nano)
			case "bad_expiry":
				receipt.Proof.ExpiredAt = "bad"
				receipt.Proof.CheckedAt = now.Format(time.RFC3339Nano)
			case "expiry_offset":
				receipt.Proof.ExpiredAt = now.Format("2006-01-02T15:04:05+00:00")
				receipt.Proof.CheckedAt = now.Add(time.Second).Format(time.RFC3339Nano)
			case "issuer_digest":
				receipt.Proof.IssuerSHA256 = "invalid"
			case "crl_digest":
				receipt.Proof.CRLSHA256 = "invalid"
			case "manifest_digest":
				receipt.Proof.ManifestSHA256 = "invalid"
			case "crl_uri":
				receipt.Proof.CRLURI = "invalid"
			case "certificate_digest":
				receipt.Proof.CertificateSHA256 = "invalid"
			case "class":
				receipt.Class = ""
			case "key":
				receipt.Proof.SKI = "invalid"
			case "operation":
				receipt.Original.Operation = "updown-issue"
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
