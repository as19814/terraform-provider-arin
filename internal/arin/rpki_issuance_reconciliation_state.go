package arin

import (
	"crypto/sha256"
	"fmt"
	"time"
)

type rpkiIssuanceReconciliation struct {
	Original          rpkiPendingExchange `json:"original"`
	RecoveryPeerID    string              `json:"recovery_peer_id"`
	CertificateSHA256 string              `json:"certificate_sha256"`
	Class             string              `json:"class_name"`
	SKI               string              `json:"ski"`
	Sent              time.Time           `json:"sent"`
	Received          time.Time           `json:"received"`
}

func validIssuanceReconciliation(r rpkiIssuanceReconciliation) bool {
	ski, err := upDownSKI(r.SKI)
	return err == nil && ski == r.SKI && upDownLabel(r.Class) && upDownToken(r.Class) == r.Class && exchangeDigest.MatchString(r.CertificateSHA256) && r.Original.Operation == "updown-issue" && exchangeDigest.MatchString(r.Original.RequestSHA256) && !r.Original.SigningTime.IsZero() && validExchangeTime(r.Original.SigningTime) && exchangeDigest.MatchString(r.RecoveryPeerID) && validExchangeTime(r.Sent) && r.Sent.After(r.Original.SigningTime) && !r.Received.IsZero() && validExchangeTime(r.Received)
}
func issuanceObservationHash(o rpkiIssuanceRecoveryObservation) (string, error) {
	if o.Outcome != "matches_request" || o.Certificate == nil || o.Certificate.Class != o.Plan.Request.Class || o.Certificate.SKI != o.Plan.SKI {
		return "", errRPKIExchangeState
	}
	certs, err := rpkiPEMCertificates(o.Certificate.CertificatePEM, 1)
	if err != nil || len(certs) != 1 {
		return "", errRPKIExchangeState
	}
	ski, err := rpkiPublicKeyIdentifier(certs[0].RawSubjectPublicKeyInfo)
	if err != nil || ski != o.Plan.SKI {
		return "", errRPKIExchangeState
	}
	return fmt.Sprintf("%x", sha256.Sum256(certs[0].Raw)), nil
}

// reconcileIssuance is called only after fresh signed inventory and resource-path
// validation, under the original lease, match the explicitly selected certificate.
func (l *rpkiExchangeLease) reconcileIssuance(o rpkiIssuanceRecoveryObservation) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.poisoned || l.state.Pending == nil || l.state.Pending.RequestSHA256 != o.Plan.RequestSHA256 || !l.state.Pending.SigningTime.Equal(o.Plan.SigningTime) {
		return errRPKIExchangeState
	}
	hash, err := issuanceObservationHash(o)
	if err != nil {
		return err
	}
	receipt := rpkiIssuanceReconciliation{Original: *l.state.Pending, RecoveryPeerID: o.RecoveryPeerID, CertificateSHA256: hash, Class: o.Certificate.Class, SKI: o.Certificate.SKI, Sent: o.Sent.UTC(), Received: o.Received.UTC()}
	if !validIssuanceReconciliation(receipt) || receipt.RecoveryPeerID == l.state.PeerID || !receipt.Sent.After(l.state.LastSent) || receipt.Received.Before(l.state.LastReceived) {
		return errRPKIExchangeState
	}
	l.state.ReconciledIssuance = &receipt
	l.state.LastSent, l.state.LastReceived = receipt.Sent, receipt.Received
	l.state.Pending = nil
	if err := l.save(); err != nil {
		l.poisoned = true
		return errRPKIExchangeState
	}
	return nil
}
