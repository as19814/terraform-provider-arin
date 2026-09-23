package arin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type RPKIRevocationRecoveryReport struct {
	RequestSHA256  string    `json:"request_sha256"`
	RecoveryPeerID string    `json:"recovery_peer_id"`
	Child          string    `json:"child_handle"`
	Parent         string    `json:"parent_handle"`
	Class          string    `json:"class_name"`
	SKI            string    `json:"ski"`
	Outcome        string    `json:"outcome"`
	Committed      bool      `json:"committed"`
	Sent           time.Time `json:"sent"`
	Received       time.Time `json:"received"`
}

// ObserveRPKIRevocation reads authenticated inventory without clearing the pending
// mutation. Neither key absence nor class absence proves CRL publication.
func ObserveRPKIRevocation(ctx context.Context, config RPKIProvisioningReadConfig, digest string) (*RPKIRevocationRecoveryReport, error) {
	return observeRPKIRevocation(ctx, config, digest, nil)
}
func observeRPKIRevocation(ctx context.Context, config RPKIProvisioningReadConfig, digest string, clock func() time.Time) (*RPKIRevocationRecoveryReport, error) {
	if !exchangeDigest.MatchString(digest) {
		return nil, errRPKIExchangeState
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	child, parent := upDownToken(config.Child), upDownToken(config.Parent)
	if !upDownLabel(child) || !upDownLabel(parent) {
		return nil, errRPKIUpDown
	}
	exchange, err := config.BPKI.exchange()
	if err != nil {
		return nil, err
	}
	exchange.MediaType = "application/rpki-updown"
	scope, _ := json.Marshal([]string{child, parent})
	exchange.PeerScope, exchange.Clock = string(scope), clock
	observation, err := (rpkiUpDownClient{Exchange: exchange, Child: child, Parent: parent}).observePendingRevocation(ctx, digest)
	if err != nil {
		exchange.RecoveryOf = digest
		if peer, peerErr := exchange.peerID(); peerErr == nil {
			return nil, fmt.Errorf("revocation observation failed (recovery journal peer %s): %w", peer, err)
		}
		return nil, err
	}
	p := observation.Plan
	return &RPKIRevocationRecoveryReport{RequestSHA256: p.RequestSHA256, RecoveryPeerID: observation.RecoveryPeerID, Child: p.Child, Parent: p.Parent, Class: p.Class, SKI: p.SKI, Outcome: observation.Outcome, Sent: observation.Sent, Received: observation.Received}, nil
}

// LoadRPKIProvisioningConfig loads private BPKI configuration and both handles.
// It rejects publication-only fields and accepts no private key bytes.
func LoadRPKIProvisioningConfig(path string) (RPKIProvisioningReadConfig, error) {
	var config RPKIProvisioningReadConfig
	fields := rpkiConfigFields(&config.BPKI)
	fields["child_handle"], fields["parent_handle"] = &config.Child, &config.Parent
	if err := loadPrivateRPKIConfig(path, fields); err != nil {
		return RPKIProvisioningReadConfig{}, err
	}
	if !upDownLabel(upDownToken(config.Child)) || !upDownLabel(upDownToken(config.Parent)) {
		return RPKIProvisioningReadConfig{}, errRPKIIdentityConfig
	}
	return config, nil
}
