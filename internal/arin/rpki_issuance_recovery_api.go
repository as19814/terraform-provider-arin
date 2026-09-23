package arin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type RPKIIssuanceRecoveryReport struct {
	RequestSHA256     string    `json:"request_sha256"`
	RecoveryPeerID    string    `json:"recovery_peer_id"`
	Child             string    `json:"child_handle"`
	Parent            string    `json:"parent_handle"`
	Class             string    `json:"class_name"`
	SKI               string    `json:"ski"`
	Outcome           string    `json:"outcome"`
	Committed         bool      `json:"committed"`
	CertificateSHA256 string    `json:"certificate_sha256,omitempty"`
	Sent              time.Time `json:"sent"`
	Received          time.Time `json:"received"`
}

// RecoverRPKIIssuance observes when expectedCertificate is empty. Otherwise it
// commits only the explicitly selected certificate after fresh path validation.
func RecoverRPKIIssuance(ctx context.Context, config RPKIProvisioningReadConfig, validation RPKICertificateValidation, digest, expectedCertificate string) (*RPKIIssuanceRecoveryReport, error) {
	return recoverRPKIIssuance(ctx, config, validation, digest, expectedCertificate, nil, rrdpHTTPClient{})
}
func recoverRPKIIssuance(ctx context.Context, config RPKIProvisioningReadConfig, validation RPKICertificateValidation, digest, expectedCertificate string, clock func() time.Time, repository rrdpHTTPClient) (*RPKIIssuanceRecoveryReport, error) {
	if !exchangeDigest.MatchString(digest) || (expectedCertificate != "" && !exchangeDigest.MatchString(expectedCertificate)) {
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
	client := rpkiUpDownClient{Exchange: exchange, Child: child, Parent: parent}
	var o rpkiIssuanceRecoveryObservation
	if expectedCertificate == "" {
		o, err = client.observePendingIssuance(ctx, digest, validation, repository)
	} else {
		o, err = client.reconcilePendingIssuance(ctx, digest, expectedCertificate, validation, repository)
	}
	if err != nil {
		exchange.RecoveryOf = digest
		if peer, peerErr := exchange.peerID(); peerErr == nil {
			return nil, fmt.Errorf("issuance recovery failed (recovery journal peer %s): %w", peer, err)
		}
		return nil, err
	}
	hash := ""
	if o.Certificate != nil {
		hash, err = issuanceObservationHash(o)
		if err != nil {
			return nil, err
		}
	}
	return &RPKIIssuanceRecoveryReport{RequestSHA256: o.Plan.RequestSHA256, RecoveryPeerID: o.RecoveryPeerID, Child: o.Plan.Child, Parent: o.Plan.Parent, Class: o.Plan.Request.Class, SKI: o.Plan.SKI, Outcome: o.Outcome, Committed: expectedCertificate != "", CertificateSHA256: hash, Sent: o.Sent, Received: o.Received}, nil
}

// LoadRPKICertificateValidation loads bounded private JSON resource trust settings.
func LoadRPKICertificateValidation(path string) (RPKICertificateValidation, error) {
	var v RPKICertificateValidation
	fields := map[string]*string{"resource_anchor_pem": &v.AnchorPEM, "issuer_chain_pem": &v.IssuerChainPEM, "rrdp_cache_directory": &v.CacheDirectory, "manifest_history_directory": &v.HistoryDirectory}
	if err := loadPrivateRPKIConfigFields(path, fields, map[string]*[]string{"rrdp_notifications": &v.Notifications}); err != nil {
		return RPKICertificateValidation{}, err
	}
	for _, uri := range v.Notifications {
		if !validRRDPURL(uri) {
			return RPKICertificateValidation{}, errRPKIIdentityConfig
		}
	}
	return v, nil
}
