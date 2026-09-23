package arin

import (
	"crypto/sha256"
	"fmt"
)

// pendingMutationContent verifies saved evidence without changing the journal.
func (c rpkiHTTPExchange) pendingMutationContent(lease *rpkiExchangeLease, operation string) ([]byte, rpkiPendingExchange, error) {
	if lease == nil {
		return nil, rpkiPendingExchange{}, errRPKIExchangeState
	}
	peer, err := c.peerID()
	if err != nil {
		return nil, rpkiPendingExchange{}, err
	}
	state, err := lease.State()
	if err != nil || state.PeerID != peer || state.Pending == nil || state.Pending.Operation != operation {
		return nil, rpkiPendingExchange{}, errRPKIExchangeState
	}
	raw, err := lease.PendingRequest()
	if err != nil {
		return nil, rpkiPendingExchange{}, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != state.Pending.RequestSHA256 {
		return nil, rpkiPendingExchange{}, errRPKIExchangeState
	}
	verified, err := verifyRPKICMS(raw, rpkiCMSTrust{Anchor: c.Identity.Anchor, Intermediates: c.Identity.Intermediates, Now: state.Pending.SigningTime})
	if err != nil || !verified.SigningTime.Equal(state.Pending.SigningTime) {
		return nil, rpkiPendingExchange{}, errRPKIExchangeState
	}
	return verified.Content, *state.Pending, nil
}
