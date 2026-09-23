package arin

import (
	"context"
	"encoding/pem"
	"strings"
	"time"
)

// DiscoverRPKIIssuerChain returns an immediate-issuer-first PEM chain ending at
// the explicitly configured anchor. It discovers certificates only within the
// supplied, ordered RRDP repositories and validates current manifests, CRLs,
// resource authorization and durable history before returning. IssuerChainPEM
// must be empty: supplying an explicit chain and requesting discovery conflict.
func DiscoverRPKIIssuerChain(ctx context.Context, certificatePEM string, validation RPKICertificateValidation) (string, error) {
	return discoverRPKIIssuerChain(ctx, certificatePEM, validation, time.Now(), rrdpHTTPClient{})
}

func discoverRPKIIssuerChain(ctx context.Context, certificatePEM string, validation RPKICertificateValidation, now time.Time, repository rrdpHTTPClient) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if validation.IssuerChainPEM != "" {
		return "", errRPKIUpDown
	}
	leaves, err := rpkiPEMCertificates(certificatePEM, 1)
	if err != nil || len(leaves) != 1 {
		return "", errRPKIUpDown
	}
	anchors, err := rpkiPEMCertificates(validation.AnchorPEM, 1)
	if err != nil || len(anchors) != 1 {
		return "", errRPKIUpDown
	}
	path, err := repository.DiscoverPath(ctx, validation.CacheDirectory, leaves[0], anchors[0], validation.Notifications, now)
	if err != nil {
		return "", err
	}
	if _, err := verifyAndRecordRPKIManifestPath(validation.HistoryDirectory, path.Certificates, anchors[0], path.Publications, now); err != nil {
		return "", err
	}
	var result strings.Builder
	for _, issuer := range path.Certificates[1:] {
		result.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: issuer.Raw}))
	}
	return result.String(), nil
}
