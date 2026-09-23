package arin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type RPKIRevocationRecoveryReport struct {
	ClassEvidenceSHA256 string    `json:"class_evidence_sha256,omitempty"`
	RequestSHA256       string    `json:"request_sha256"`
	RecoveryPeerID      string    `json:"recovery_peer_id"`
	Child               string    `json:"child_handle"`
	Parent              string    `json:"parent_handle"`
	Class               string    `json:"class_name"`
	SKI                 string    `json:"ski"`
	Evidence            string    `json:"evidence,omitempty"`
	ExpiredAt           string    `json:"expired_at,omitempty"`
	CheckedAt           string    `json:"checked_at,omitempty"`
	Outcome             string    `json:"outcome"`
	Committed           bool      `json:"committed"`
	Sent                time.Time `json:"sent"`
	Received            time.Time `json:"received"`
	CertificateSHA256   string    `json:"certificate_sha256,omitempty"`
	IssuerSHA256        string    `json:"issuer_sha256,omitempty"`
	CRLSHA256           string    `json:"crl_sha256,omitempty"`
	ManifestSHA256      string    `json:"manifest_sha256,omitempty"`
	CRLURI              string    `json:"crl_uri,omitempty"`
}

// ObserveRPKIRevocation reads authenticated inventory without clearing the pending
// mutation. Neither key absence nor class absence proves CRL publication.
func ObserveRPKIRevocation(ctx context.Context, config RPKIProvisioningReadConfig, digest string) (*RPKIRevocationRecoveryReport, error) {
	return observeRPKIRevocation(ctx, config, digest, nil)
}
func observeRPKIRevocation(ctx context.Context, config RPKIProvisioningReadConfig, digest string, clock func() time.Time) (*RPKIRevocationRecoveryReport, error) {
	return recoverRPKIRevocation(ctx, config, nil, digest, "", clock, rrdpHTTPClient{})
}

// RecoverRPKIRevocation requires signed key absence and current anchored CRL
// evidence. Empty expectedCertificate observes the proof without clearing the
// journal; a DER certificate hash explicitly selects the revocation to commit.
func RecoverRPKIRevocation(ctx context.Context, config RPKIProvisioningReadConfig, validation RPKIRevocationValidation, digest, expectedCertificate string) (*RPKIRevocationRecoveryReport, error) {
	return recoverRPKIRevocation(ctx, config, &validation, digest, expectedCertificate, nil, rrdpHTTPClient{})
}
func recoverRPKIRevocation(ctx context.Context, config RPKIProvisioningReadConfig, validation *RPKIRevocationValidation, digest, expectedCertificate string, clock func() time.Time, repository rrdpHTTPClient) (*RPKIRevocationRecoveryReport, error) {
	if !exchangeDigest.MatchString(digest) || (expectedCertificate != "" && (!exchangeDigest.MatchString(expectedCertificate) || validation == nil)) {
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
	observation, err := (rpkiUpDownClient{Exchange: exchange, Child: child, Parent: parent}).readPendingRevocation(ctx, digest, expectedCertificate, validation, repository)
	if err != nil {
		exchange.RecoveryOf = digest
		if peer, peerErr := exchange.peerID(); peerErr == nil {
			return nil, fmt.Errorf("revocation recovery failed (recovery journal peer %s): %w", peer, err)
		}
		return nil, err
	}
	p := observation.Plan
	report := &RPKIRevocationRecoveryReport{RequestSHA256: p.RequestSHA256, RecoveryPeerID: observation.RecoveryPeerID, Child: p.Child, Parent: p.Parent, Class: p.Class, SKI: p.SKI, Outcome: observation.Outcome, Sent: observation.Sent, Received: observation.Received, Committed: expectedCertificate != ""}
	if p := observation.Proof; p != nil {
		report.ClassEvidenceSHA256 = p.ClassEvidenceSHA256
		report.Evidence = "revoked"
		if p.ExpiredAt != "" {
			report.Evidence = "expired_withdrawn"
			report.ExpiredAt = p.ExpiredAt
			report.CheckedAt = p.CheckedAt
		}
		report.CertificateSHA256, report.IssuerSHA256 = p.CertificateSHA256, p.IssuerSHA256
		report.CRLSHA256, report.ManifestSHA256, report.CRLURI = p.CRLSHA256, p.ManifestSHA256, p.CRLURI
	}
	return report, nil
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

// LoadRPKIRevocationValidation loads private JSON trust settings and the prior
// certificate. It uses the same strict bounded loader as issuance recovery.
func LoadRPKIRevocationValidation(path string) (RPKIRevocationValidation, error) {
	var v RPKIRevocationValidation
	fields := map[string]*string{"prior_certificate_pem": &v.PriorCertificatePEM, "resource_anchor_pem": &v.Path.AnchorPEM, "issuer_chain_pem": &v.Path.IssuerChainPEM, "rrdp_cache_directory": &v.Path.CacheDirectory, "manifest_history_directory": &v.Path.HistoryDirectory}
	if err := loadPrivateRPKIConfigFields(path, fields, map[string]*[]string{"rrdp_notifications": &v.Path.Notifications}); err != nil {
		return RPKIRevocationValidation{}, err
	}
	for _, uri := range v.Path.Notifications {
		if !validRRDPURL(uri) {
			return RPKIRevocationValidation{}, errRPKIIdentityConfig
		}
	}
	return v, nil
}
