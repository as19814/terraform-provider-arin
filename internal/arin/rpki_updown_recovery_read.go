package arin

import (
	"context"
	"encoding/json"
	"time"
)

type rpkiRevocationRecoveryObservation struct {
	Plan                    rpkiRevocationRecoveryPlan
	RecoveryPeerID, Outcome string
	Sent, Received          time.Time
}

// observePendingRevocation reads authenticated inventory under the original
// lease. Key absence does not prove CRL publication or authorize replay.
func (client rpkiUpDownClient) observePendingRevocation(ctx context.Context, digest string) (observation rpkiRevocationRecoveryObservation, err error) {
	c := client.Exchange
	scope, _ := json.Marshal([]string{upDownToken(client.Child), upDownToken(client.Parent)})
	c.PeerScope = string(scope)

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
			observation = rpkiRevocationRecoveryObservation{}
			err = e
		}
	}()
	state, err := lease.State()
	if err != nil || state.Pending == nil || state.Pending.RequestSHA256 != digest {
		return observation, errRPKIExchangeState
	}
	plan, err := client.pendingRevocationPlan(lease)
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
	objects, err := (rpkiUpDownClient{Exchange: probe, Child: client.Child, Parent: client.Parent}).List(ctx)
	if err != nil {
		return observation, err
	}
	outcome, err := classifyRevocationInventory(plan, objects)
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
	observation = rpkiRevocationRecoveryObservation{Plan: plan, RecoveryPeerID: recoveryPeer, Outcome: outcome, Sent: recovered.LastSent, Received: recovered.LastReceived}
	return observation, nil
}
