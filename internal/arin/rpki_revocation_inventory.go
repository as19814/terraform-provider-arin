package arin

import (
	"bytes"
	"context"
	"crypto/x509"
	"time"
)

// RPKIRevocationValidation supplies the retained certificate and explicit
// resource trust settings used to prove an uncertain revocation.
type RPKIRevocationValidation struct {
	// AllowExpired accepts proven expiry and withdrawal as a distinct outcome.
	AllowExpired bool
	// AllowClassAbsent requires retained authenticated historical class evidence.
	AllowClassAbsent    bool
	PriorCertificatePEM string
	Path                RPKICertificateValidation
}

// validateRevocationInventory consumes authenticated parent inventory. A missing
// class requires separately authenticated historical evidence of the issuer.
func validateRevocationInventory(ctx context.Context, plan rpkiRevocationRecoveryPlan, classes []rpkiResourceClass, validation RPKIRevocationValidation, repository rrdpHTTPClient, now time.Time, historical *rpkiResourceClass) (*rpkiRevocationProof, error) {
	outcome, err := classifyRevocationInventory(plan, classes)
	if err != nil || (outcome != "key_absent" && !(outcome == "class_absent" && validation.AllowClassAbsent && historical != nil && historical.Name == plan.Class)) {
		return nil, errRPKIUpDown
	}
	prior, err := rpkiPEMCertificates(validation.PriorCertificatePEM, 1)
	if err != nil || len(prior) != 1 {
		return nil, errRPKIUpDown
	}
	ski, err := rpkiPublicKeyIdentifier(prior[0].RawSubjectPublicKeyInfo)
	if err != nil || ski != plan.SKI {
		return nil, errRPKIUpDown
	}
	v := validation.Path
	anchors, err := rpkiPEMCertificates(v.AnchorPEM, 1)
	if err != nil || len(anchors) != 1 {
		return nil, errRPKIUpDown
	}
	issuers, err := rpkiPEMCertificates(v.IssuerChainPEM, 31)
	if err != nil || len(issuers) == 0 || len(issuers) != len(v.Notifications) || !bytes.Equal(issuers[len(issuers)-1].Raw, anchors[0].Raw) {
		return nil, errRPKIUpDown
	}
	if outcome == "class_absent" {
		classes = []rpkiResourceClass{*historical}
	}
	for _, class := range classes {
		if class.Name == plan.Class && !bytes.Equal(class.IssuerDER, issuers[0].Raw) {
			return nil, errRPKIUpDown
		}
	}
	path, err := repository.RetrieveRevocationPath(ctx, v.CacheDirectory, append([]*x509.Certificate{prior[0]}, issuers...), v.Notifications, now)
	if err != nil {
		return nil, err
	}
	return verifyAndRecordRPKIRetirementProof(v.HistoryDirectory, path.Certificates[0].Raw, plan.SKI, path.Certificates[1:], anchors[0], path.Publications[1:], path.Publications[0], now, validation.AllowExpired)
}
