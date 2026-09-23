package arin

import (
	"context"
	"encoding/json"
	"time"
)

type rpkiIssuanceRecoveryObservation struct {
	Plan                    rpkiIssuanceRecoveryPlan
	RecoveryPeerID, Outcome string
	Sent, Received          time.Time
	Certificate             *RPKIIssuedCertificate
}

// observePendingIssuance reads authenticated inventory under the original
// lease. A match requires the saved CSR/request and validated resource path.
// Observation never authorizes replay or clears the original pending mutation.
func (client rpkiUpDownClient) observePendingIssuance(ctx context.Context, digest string, validation RPKICertificateValidation, repository rrdpHTTPClient) (rpkiIssuanceRecoveryObservation, error) {
	return client.readPendingIssuance(ctx, digest, "", validation, repository)
}
func (client rpkiUpDownClient) reconcilePendingIssuance(ctx context.Context, digest, expectedCertificate string, validation RPKICertificateValidation, repository rrdpHTTPClient) (rpkiIssuanceRecoveryObservation, error) {
	if !exchangeDigest.MatchString(expectedCertificate) {
		return rpkiIssuanceRecoveryObservation{}, errRPKIExchangeState
	}
	return client.readPendingIssuance(ctx, digest, expectedCertificate, validation, repository)
}
func (client rpkiUpDownClient) readPendingIssuance(ctx context.Context, digest, expectedCertificate string, validation RPKICertificateValidation, repository rrdpHTTPClient) (observation rpkiIssuanceRecoveryObservation, err error) {
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
			observation = rpkiIssuanceRecoveryObservation{}
			err = e
		}
	}()
	state, err := lease.State()
	if err != nil || state.Pending == nil || state.Pending.RequestSHA256 != digest {
		return observation, errRPKIExchangeState
	}
	plan, err := client.pendingIssuancePlan(lease)
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
	certificate, err := (rpkiUpDownClient{Exchange: probe, Child: client.Child, Parent: client.Parent}).readCertificate(ctx, plan.Request, validation, repository)
	if err != nil {
		return observation, err
	}
	outcome := "key_absent"
	if certificate != nil {
		outcome = "matches_request"
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
	observation = rpkiIssuanceRecoveryObservation{Plan: plan, RecoveryPeerID: recoveryPeer, Outcome: outcome, Sent: recovered.LastSent, Received: recovered.LastReceived, Certificate: certificate}
	if expectedCertificate != "" {
		hash, err := issuanceObservationHash(observation)
		if err != nil || hash != expectedCertificate {
			return observation, errRPKIExchangeState
		}
		if err := ctx.Err(); err != nil {
			return observation, err
		}
		if err := lease.reconcileIssuance(observation); err != nil {
			return observation, err
		}
	}
	return observation, nil
}
