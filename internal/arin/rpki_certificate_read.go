package arin

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"time"
)

// ReadRPKICertificate selects a CSR key within one class from signed inventory,
// then validates its current resource path. Nil means the key or class is absent.
func ReadRPKICertificate(ctx context.Context, config RPKIProvisioningReadConfig, input RPKICertificateRequest, validation RPKICertificateValidation) (*RPKIIssuedCertificate, error) {
	return readRPKICertificate(ctx, config, input, validation, nil, rrdpHTTPClient{})
}
func readRPKICertificate(ctx context.Context, config RPKIProvisioningReadConfig, input RPKICertificateRequest, validation RPKICertificateValidation, clock func() time.Time, repository rrdpHTTPClient) (*RPKIIssuedCertificate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	exchange, err := config.BPKI.exchange()
	if err != nil {
		return nil, err
	}
	exchange.MediaType, exchange.Clock = "application/rpki-updown", clock
	return (rpkiUpDownClient{Exchange: exchange, Child: config.Child, Parent: config.Parent}).readCertificate(ctx, input, validation, repository)
}

func (client rpkiUpDownClient) readCertificate(ctx context.Context, input RPKICertificateRequest, validation RPKICertificateValidation, repository rrdpHTTPClient) (*RPKIIssuedCertificate, error) {
	clock := client.Exchange.Clock

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
	child, parent := upDownToken(client.Child), upDownToken(client.Parent)
	if !upDownLabel(child) || !upDownLabel(parent) {
		return nil, errRPKIUpDown
	}
	input.Class = upDownToken(input.Class)
	request := rpkiIssueRequest{Class: input.Class, CSRDER: blocks[0], RequestedASN: input.RequestedASN, RequestedIPv4: input.RequestedIPv4, RequestedIPv6: input.RequestedIPv6}
	if _, err := buildUpDownIssue(child, parent, request); err != nil {
		return nil, err
	}
	anchors, err := rpkiPEMCertificates(validation.AnchorPEM, 1)
	if err != nil || len(anchors) != 1 {
		return nil, errRPKIUpDown
	}
	issuers, err := rpkiPEMCertificates(validation.IssuerChainPEM, 31)
	if err != nil || len(issuers) == 0 || len(issuers) != len(validation.Notifications) || !bytes.Equal(issuers[len(issuers)-1].Raw, anchors[0].Raw) {
		return nil, errRPKIUpDown
	}
	classes, err := client.List(ctx)
	if err != nil {
		return nil, err
	}
	var selected *rpkiResourceClass
	for _, class := range classes {
		if class.Name != input.Class {
			continue
		}
		for _, item := range class.Certificates {
			cert, err := x509.ParseCertificate(item.DER)
			if err != nil {
				return nil, errRPKIUpDown
			}
			if !bytes.Equal(cert.RawSubjectPublicKeyInfo, csr.RawSubjectPublicKeyInfo) {
				continue
			}
			if selected != nil {
				return nil, errRPKIUpDown
			}
			c := class
			c.Certificates = []rpkiResourceCertificate{item}
			selected = &c
		}
	}
	if selected == nil {
		return nil, nil
	}
	if clock == nil {
		clock = time.Now
	}
	now := clock()
	wanted := map[string]string{"class_name": input.Class}
	for k, v := range map[string]*string{"req_resource_set_as": input.RequestedASN, "req_resource_set_ipv4": input.RequestedIPv4, "req_resource_set_ipv6": input.RequestedIPv6} {
		if v != nil {
			wanted[k] = *v
		}
	}
	if err := validateUpDownIssuedClass(csr, wanted, selected, now); err != nil {
		return nil, err
	}
	if !bytes.Equal(selected.IssuerDER, issuers[0].Raw) {
		return nil, errRPKIUpDown
	}
	leaf, err := x509.ParseCertificate(selected.Certificates[0].DER)
	if err != nil {
		return nil, errRPKIUpDown
	}
	path, err := repository.RetrievePath(ctx, validation.CacheDirectory, append([]*x509.Certificate{leaf}, issuers...), validation.Notifications, now)
	if err != nil {
		return nil, err
	}
	if err := validateIssuedResourcePath(selected, path, rpkiIssuePathValidation{Anchor: anchors[0], Directory: validation.HistoryDirectory}, now); err != nil {
		return nil, err
	}
	ski, err := rpkiPublicKeyIdentifier(csr.RawSubjectPublicKeyInfo)
	if err != nil {
		return nil, err
	}
	encode := func(raw []byte) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}))
	}
	return &RPKIIssuedCertificate{Class: selected.Name, SKI: ski, CertificatePEM: encode(leaf.Raw), CertificateURLs: selected.Certificates[0].URLs, IssuerPEM: encode(selected.IssuerDER), NotAfter: leaf.NotAfter.UTC().Format(time.RFC3339)}, nil
}
