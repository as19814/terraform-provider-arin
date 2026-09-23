package arin

import "time"

type rpkiRevocationReconciliation struct {
	Original       rpkiPendingExchange `json:"original"`
	RecoveryPeerID string              `json:"recovery_peer_id"`
	Class          string              `json:"class_name"`
	Proof          rpkiRevocationProof `json:"proof"`
	Sent           time.Time           `json:"sent"`
	Received       time.Time           `json:"received"`
}

func validRevocationReconciliation(r rpkiRevocationReconciliation) bool {
	p := r.Proof
	ski, err := upDownSKI(p.SKI)
	return err == nil && ski == p.SKI && upDownLabel(r.Class) && upDownToken(r.Class) == r.Class &&
		exchangeDigest.MatchString(p.CertificateSHA256) && exchangeDigest.MatchString(p.IssuerSHA256) &&
		exchangeDigest.MatchString(p.CRLSHA256) && exchangeDigest.MatchString(p.ManifestSHA256) && publicationURI(p.CRLURI) &&
		r.Original.Operation == "updown-revoke" && exchangeDigest.MatchString(r.Original.RequestSHA256) &&
		!r.Original.SigningTime.IsZero() && validExchangeTime(r.Original.SigningTime) &&
		exchangeDigest.MatchString(r.RecoveryPeerID) && validExchangeTime(r.Sent) && r.Sent.After(r.Original.SigningTime) &&
		!r.Received.IsZero() && validExchangeTime(r.Received)
}

// reconcileRevocation requires signed inventory and a checked, durably recorded
// proof obtained under this lease. Absence without a proof never clears Pending.
func (l *rpkiExchangeLease) reconcileRevocation(o rpkiRevocationRecoveryObservation) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.poisoned || l.state.Pending == nil || l.state.Pending.RequestSHA256 != o.Plan.RequestSHA256 || !l.state.Pending.SigningTime.Equal(o.Plan.SigningTime) || o.Outcome != "key_absent" || o.Proof == nil || o.Proof.SKI != o.Plan.SKI {
		return errRPKIExchangeState
	}
	receipt := rpkiRevocationReconciliation{Original: *l.state.Pending, RecoveryPeerID: o.RecoveryPeerID, Class: o.Plan.Class, Proof: *o.Proof, Sent: o.Sent.UTC(), Received: o.Received.UTC()}
	if !validRevocationReconciliation(receipt) || receipt.RecoveryPeerID == l.state.PeerID || !receipt.Sent.After(l.state.LastSent) || receipt.Received.Before(l.state.LastReceived) {
		return errRPKIExchangeState
	}
	l.state.ReconciledRevocation = &receipt
	l.state.LastSent, l.state.LastReceived = receipt.Sent, receipt.Received
	l.state.Pending = nil
	if err := l.save(); err != nil {
		l.poisoned = true
		return errRPKIExchangeState
	}
	return nil
}
