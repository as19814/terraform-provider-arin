package arin

import "time"

type rpkiPublicationReconciliation struct {
	Original       rpkiPendingExchange `json:"original"`
	RecoveryPeerID string              `json:"recovery_peer_id"`
	Outcome        string              `json:"outcome"`
	Sent           time.Time           `json:"sent"`
	Received       time.Time           `json:"received"`
}

func validPublicationReconciliation(r rpkiPublicationReconciliation) bool {
	return r.Original.Operation == "publication-batch" && exchangeDigest.MatchString(r.Original.RequestSHA256) && !r.Original.SigningTime.IsZero() && validExchangeTime(r.Original.SigningTime) && exchangeDigest.MatchString(r.RecoveryPeerID) && (r.Outcome == "matches_before" || r.Outcome == "matches_after") && validExchangeTime(r.Sent) && r.Sent.After(r.Original.SigningTime) && !r.Received.IsZero() && validExchangeTime(r.Received)
}

// reconcilePublication is called only after the recovery reader authenticates
// inventory and matches the explicitly selected outcome under the same lease.
// It records an observation, not a historical claim about transaction execution.
func (l *rpkiExchangeLease) reconcilePublication(o rpkiPublicationRecoveryObservation) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.poisoned || l.state.Pending == nil || l.state.Pending.RequestSHA256 != o.Plan.RequestSHA256 || !l.state.Pending.SigningTime.Equal(o.Plan.SigningTime) {
		return errRPKIExchangeState
	}
	receipt := rpkiPublicationReconciliation{Original: *l.state.Pending, RecoveryPeerID: o.RecoveryPeerID, Outcome: o.Outcome, Sent: o.Sent.UTC(), Received: o.Received.UTC()}
	if !validPublicationReconciliation(receipt) || receipt.RecoveryPeerID == l.state.PeerID || !receipt.Sent.After(l.state.LastSent) || receipt.Received.Before(l.state.LastReceived) {
		return errRPKIExchangeState
	}
	l.state.ReconciledPublication = &receipt
	l.state.LastSent = receipt.Sent
	l.state.LastReceived = receipt.Received
	l.state.Pending = nil
	if err := l.save(); err != nil {
		l.poisoned = true
		return errRPKIExchangeState
	}
	return nil
}
