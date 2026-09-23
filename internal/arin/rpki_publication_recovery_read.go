package arin

import (
	"context"
	"time"
)

type rpkiPublicationRecoveryObservation struct {
	Plan                    rpkiPublicationRecoveryPlan
	RecoveryPeerID, Outcome string
	Sent, Received          time.Time
}

// observePendingPublication holds the original peer's lease while a distinct,
// digest-scoped recovery journal records an authenticated inventory exchange.
// It never resends the mutation or clears its pending record. The response is
// checked against both original and recovery receive-time watermarks.
func (c rpkiHTTPExchange) observePendingPublication(ctx context.Context, digest string) (rpkiPublicationRecoveryObservation, error) {
	return c.readPendingPublication(ctx, digest, "")
}

// reconcilePendingPublication requires an explicitly selected observed state.
// It commits the decision while still holding the original peer's lease.
func (c rpkiHTTPExchange) reconcilePendingPublication(ctx context.Context, digest, expected string) (rpkiPublicationRecoveryObservation, error) {
	if expected != "matches_before" && expected != "matches_after" {
		return rpkiPublicationRecoveryObservation{}, errRPKIExchangeState
	}
	return c.readPendingPublication(ctx, digest, expected)
}
func (c rpkiHTTPExchange) readPendingPublication(ctx context.Context, digest, expected string) (observation rpkiPublicationRecoveryObservation, err error) {
	if c.RecoveryOf != "" || !exchangeDigest.MatchString(digest) {
		return observation, errRPKIExchangeState
	}
	if err := ctx.Err(); err != nil {
		return observation, err
	}
	peer, err := c.peerID()
	if err != nil {
		return observation, err
	}
	lease, err := openRPKIExchangeMode(c.Directory, peer, false)
	if err != nil {
		return observation, err
	}
	defer func() {
		if e := lease.Close(); e != nil {
			observation = rpkiPublicationRecoveryObservation{}
			err = e
		}
	}()
	state, err := lease.State()
	if err != nil || state.Pending == nil || state.Pending.RequestSHA256 != digest {
		return observation, errRPKIExchangeState
	}
	plan, err := c.pendingPublicationPlan(lease)
	if err != nil {
		return observation, err
	}
	probe := c
	probe.RecoveryOf = digest
	probe.MinimumSent = state.LastSent.Add(time.Second)
	if c.MinimumSent.After(probe.MinimumSent) {
		probe.MinimumSent = c.MinimumSent
	}
	probe.MinimumReceived = state.LastReceived
	if c.MinimumReceived.After(probe.MinimumReceived) {
		probe.MinimumReceived = c.MinimumReceived
	}
	recoveryPeer, err := probe.peerID()
	if err != nil || recoveryPeer == peer {
		return observation, errRPKIExchangeState
	}
	objects, err := (rpkiPublicationClient{Exchange: probe}).List(ctx)
	if err != nil {
		return observation, err
	}
	outcome, err := classifyPublicationInventory(plan, objects)
	if err != nil {
		return observation, err
	}
	// Read the completed probe's durable timestamps while the original peer is
	// still locked. A failed or concurrent recovery remains fail-closed.
	recovery, err := openRPKIExchangeMode(c.Directory, recoveryPeer, false)
	if err != nil {
		return observation, err
	}
	recovered, stateErr := recovery.State()
	closeErr := recovery.Close()
	if stateErr != nil || closeErr != nil || recovered.Pending != nil || recovered.LastSent.Before(probe.MinimumSent) || recovered.LastReceived.IsZero() || recovered.LastReceived.Before(probe.MinimumReceived) {
		return observation, errRPKIExchangeState
	}
	observation = rpkiPublicationRecoveryObservation{Plan: plan, RecoveryPeerID: recoveryPeer, Outcome: outcome, Sent: recovered.LastSent, Received: recovered.LastReceived}
	if expected != "" {
		if outcome != expected {
			return observation, errRPKIExchangeState
		}
		if err := ctx.Err(); err != nil {
			return observation, err
		}
		if err := lease.reconcilePublication(observation); err != nil {
			return observation, err
		}
	}
	return observation, nil
}
