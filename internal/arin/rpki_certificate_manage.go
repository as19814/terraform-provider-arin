package arin

import (
	"context"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"time"
)

// RPKICertificateRequest uses an existing resource key through its signed CSR.
// A nil requested resource set requests the entire allocation; empty excludes it.
type RPKICertificateRequest struct {
	Class, CSRPEM                              string
	RequestedASN, RequestedIPv4, RequestedIPv6 *string
}

// RPKICertificateValidation supplies an explicit resource trust path independently
// of the BPKI transport identity. IssuerChainPEM is immediate issuer through anchor.
// Only DiscoverRPKIIssuerChain accepts an empty IssuerChainPEM.
type RPKICertificateValidation struct {
	AnchorPEM, IssuerChainPEM        string
	Notifications                    []string
	CacheDirectory, HistoryDirectory string
}

type RPKIIssuedCertificate struct {
	Class, SKI, CertificatePEM, CertificateURLs, IssuerPEM, NotAfter string
}

// IssueRPKICertificate verifies the response, resource allocation and RRDP-backed
// resource path before completing its durable journal. Failure may leave issuance
// pending; callers must not automatically retry an uncertain request.
func IssueRPKICertificate(ctx context.Context, config RPKIProvisioningReadConfig, input RPKICertificateRequest, validation RPKICertificateValidation) (*RPKIIssuedCertificate, error) {
	return issueRPKICertificate(ctx, config, input, validation, nil, rrdpHTTPClient{})
}
func issueRPKICertificate(ctx context.Context, config RPKIProvisioningReadConfig, input RPKICertificateRequest, validation RPKICertificateValidation, clock func() time.Time, repository rrdpHTTPClient) (*RPKIIssuedCertificate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	blocks, err := rpkiPEMBlocks(input.CSRPEM, "CERTIFICATE REQUEST", 1)
	if err != nil || len(blocks) != 1 {
		return nil, errRPKIUpDown
	}
	csr, err := parseRPKICACSR(blocks[0])
	if err != nil {
		return nil, err
	}
	input.Class = upDownToken(input.Class)
	request := rpkiIssueRequest{Class: input.Class, CSRDER: blocks[0], RequestedASN: cloneIssueString(input.RequestedASN), RequestedIPv4: cloneIssueString(input.RequestedIPv4), RequestedIPv6: cloneIssueString(input.RequestedIPv6)}
	child, parent := upDownToken(config.Child), upDownToken(config.Parent)
	if !upDownLabel(child) || !upDownLabel(parent) {
		return nil, errRPKIUpDown
	}
	if _, err := buildUpDownIssue(child, parent, request); err != nil {
		return nil, err
	}
	anchors, err := rpkiPEMCertificates(validation.AnchorPEM, 1)
	if err != nil || len(anchors) != 1 {
		return nil, errRPKIUpDown
	}
	issuers, err := rpkiPEMCertificates(validation.IssuerChainPEM, 31)
	if err != nil || len(issuers) == 0 {
		return nil, errRPKIUpDown
	}
	ski, err := rpkiPublicKeyIdentifier(csr.RawSubjectPublicKeyInfo)
	if err != nil {
		return nil, err
	}
	exchange, err := config.BPKI.exchange()
	if err != nil {
		return nil, err
	}
	exchange.MediaType, exchange.Clock = "application/rpki-updown", clock
	got, err := (rpkiUpDownClient{Exchange: exchange, Child: child, Parent: parent}).IssueWithRRDP(ctx, request, rpkiIssueRRDPValidation{Anchor: anchors[0], Issuers: issuers, Notifications: append([]string(nil), validation.Notifications...), CacheDirectory: validation.CacheDirectory, HistoryDirectory: validation.HistoryDirectory, Client: repository})
	if err != nil {
		return nil, err
	}
	if got == nil || len(got.Certificates) != 1 {
		return nil, errRPKIUpDown
	}
	cert, err := x509.ParseCertificate(got.Certificates[0].DER)
	if err != nil {
		return nil, errRPKIUpDown
	}
	encode := func(raw []byte) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}))
	}
	return &RPKIIssuedCertificate{Class: got.Name, SKI: ski, CertificatePEM: encode(got.Certificates[0].DER), CertificateURLs: got.Certificates[0].URLs, IssuerPEM: encode(got.IssuerDER), NotAfter: cert.NotAfter.UTC().Format(time.RFC3339)}, nil
}

// RevokeRPKICertificate retires all certificates for this key in the selected
// class. Scheduled or uncertain responses remain pending in the exchange journal.
func RevokeRPKICertificate(ctx context.Context, config RPKIProvisioningReadConfig, class, ski string) error {
	return revokeRPKICertificate(ctx, config, class, ski, nil)
}
func revokeRPKICertificate(ctx context.Context, config RPKIProvisioningReadConfig, class, ski string, clock func() time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	child, parent, class := upDownToken(config.Child), upDownToken(config.Parent), upDownToken(class)
	ski, err := upDownSKI(ski)
	if err != nil || !upDownLabel(child) || !upDownLabel(parent) || !upDownLabel(class) {
		return errRPKIUpDown
	}
	exchange, err := config.BPKI.exchange()
	if err != nil {
		return err
	}
	exchange.MediaType, exchange.Clock = "application/rpki-updown", clock
	return (rpkiUpDownClient{Exchange: exchange, Child: child, Parent: parent}).Revoke(ctx, class, ski)
}

// RFC 5280 method 1 key identifier. SHA-1 is required here, not used as a signature.
func rpkiPublicKeyIdentifier(der []byte) (string, error) {
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		Key       asn1.BitString
	}
	if !rpkiCSRDER(der, &spki) || spki.Key.BitLength == 0 || spki.Key.BitLength != len(spki.Key.Bytes)*8 {
		return "", fmt.Errorf("invalid RPKI public key")
	}
	hash := sha1.Sum(spki.Key.Bytes)
	return base64.RawURLEncoding.EncodeToString(hash[:]), nil
}

// RPKICertificateRequestKey validates a CA CSR and identifies its resource key.
// It does not load BPKI credentials or perform network operations.
func RPKICertificateRequestKey(value string) (string, error) {
	blocks, err := rpkiPEMBlocks(value, "CERTIFICATE REQUEST", 1)
	if err != nil || len(blocks) != 1 {
		return "", errRPKIUpDown
	}
	csr, err := parseRPKICACSR(blocks[0])
	if err != nil {
		return "", err
	}
	return rpkiPublicKeyIdentifier(csr.RawSubjectPublicKeyInfo)
}
