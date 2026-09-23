package arin

import (
	"bytes"
	"context"
	"crypto/x509"
	"os"
	"path/filepath"
	"time"
)

// Issuers is the explicitly configured immediate-issuer-to-anchor chain.
// Notifications identifies each issuer's RRDP repository in the same order.
// CacheDirectory and HistoryDirectory must already exist with private permissions.
type rpkiIssueRRDPValidation struct {
	Anchor           *x509.Certificate
	Issuers          []*x509.Certificate
	Notifications    []string
	CacheDirectory   string
	HistoryDirectory string
	Client           rrdpHTTPClient
}

// IssueWithRRDP completes an issue exchange only after repository retrieval,
// anchored manifest/path validation, allocation matching and durable history.
// Retrieval failure leaves the exchange pending; it never resubmits issuance.
func (c rpkiUpDownClient) IssueWithRRDP(ctx context.Context, input rpkiIssueRequest, v rpkiIssueRRDPValidation) (*rpkiResourceClass, error) {
	if len(v.Issuers) == 0 || len(v.Issuers) > 31 || len(v.Notifications) != len(v.Issuers) || v.Anchor == nil || len(v.Anchor.Raw) == 0 {
		return nil, errRPKIUpDown
	}
	if !filepath.IsAbs(v.CacheDirectory) {
		return nil, errRPKIRRDPCache
	}
	info, err := os.Lstat(v.CacheDirectory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errRPKIRRDPCache
	}
	issuers := make([]*x509.Certificate, len(v.Issuers))
	notifications := append([]string(nil), v.Notifications...)
	for i, issuer := range v.Issuers {
		if issuer == nil || len(issuer.Raw) == 0 || len(issuer.Raw) > 512000 || !validRRDPURL(notifications[i]) {
			return nil, errRPKIUpDown
		}
		issuers[i], err = x509.ParseCertificate(bytes.Clone(issuer.Raw))
		if err != nil || validateRPKICAProfile(issuers[i]) != nil {
			return nil, errRPKIUpDown
		}
	}
	if !bytes.Equal(issuers[len(issuers)-1].Raw, v.Anchor.Raw) {
		return nil, errRPKIUpDown
	}
	for i := 0; i < len(issuers)-1; i++ {
		if !bytes.Equal(issuers[i].RawIssuer, issuers[i+1].RawSubject) || issuers[i].CheckSignatureFrom(issuers[i+1]) != nil {
			return nil, errRPKIUpDown
		}
	}
	clock := c.Exchange.Clock
	if clock == nil {
		clock = time.Now
	}
	return c.IssueWithResourcePath(ctx, input, rpkiIssuePathValidation{Anchor: v.Anchor, Directory: v.HistoryDirectory, Resolve: func(ctx context.Context, class rpkiResourceClass) (rpkiIssuePath, error) {
		if len(class.Certificates) != 1 || !bytes.Equal(class.IssuerDER, issuers[0].Raw) {
			return rpkiIssuePath{}, errRPKIUpDown
		}
		leaf, err := x509.ParseCertificate(bytes.Clone(class.Certificates[0].DER))
		if err != nil {
			return rpkiIssuePath{}, errRPKIUpDown
		}
		path := append([]*x509.Certificate{leaf}, issuers...)
		return v.Client.RetrievePath(ctx, v.CacheDirectory, path, notifications, clock())
	}})
}
