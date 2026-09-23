package arin

import (
	"bytes"
	"context"
	"crypto/x509"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type rpkiIssuePath struct {
	Certificates []*x509.Certificate
	Publications []rpkiPathPublication
}

type rpkiIssuePathValidation struct {
	Anchor    *x509.Certificate
	Directory string
	// Resolve retrieves a publication snapshot after issuance. It must not
	// submit another issuance request. Trust configuration is supplied separately.
	Resolve func(context.Context, rpkiResourceClass) (rpkiIssuePath, error)
}

// IssueWithResourcePath holds the exchange pending until the returned
// certificate is present in a manifest-backed path and history is persisted.
// This is private plumbing, not yet complete allocation/request matching.
func (c rpkiUpDownClient) IssueWithResourcePath(ctx context.Context, input rpkiIssueRequest, v rpkiIssuePathValidation) (*rpkiResourceClass, error) {
	if v.Anchor == nil || len(v.Anchor.Raw) == 0 || len(v.Anchor.Raw) > 512000 || v.Resolve == nil || !filepath.IsAbs(v.Directory) {
		return nil, errRPKIUpDown
	}
	anchor, err := x509.ParseCertificate(bytes.Clone(v.Anchor.Raw))
	if err != nil || validateRPKICAProfile(anchor) != nil {
		return nil, errRPKIUpDown
	}
	info, err := os.Lstat(v.Directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errRPKIManifestState
	}
	v.Anchor = anchor
	clock := c.Exchange.Clock
	if clock == nil {
		clock = time.Now
	}
	if _, err := verifyRPKICertificatePath([]*x509.Certificate{anchor}, anchor, nil, clock()); err != nil {
		return nil, err
	}
	return c.issue(ctx, input, func(class *rpkiResourceClass, now time.Time) error {
		if class == nil || len(class.Certificates) != 1 {
			return errRPKIUpDown
		}
		// Resolver inputs own their buffers so retrieval cannot alter the signed
		// response being accepted by the exchange validator.
		copyClass := *class
		copyClass.IssuerDER = bytes.Clone(class.IssuerDER)
		copyClass.Certificates = append([]rpkiResourceCertificate(nil), class.Certificates...)
		copyClass.Certificates[0].DER = bytes.Clone(class.Certificates[0].DER)
		copyClass.Certificates[0].RequestedASN = cloneIssueString(class.Certificates[0].RequestedASN)
		copyClass.Certificates[0].RequestedIPv4 = cloneIssueString(class.Certificates[0].RequestedIPv4)
		copyClass.Certificates[0].RequestedIPv6 = cloneIssueString(class.Certificates[0].RequestedIPv6)
		path, err := v.Resolve(ctx, copyClass)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return validateIssuedResourcePath(class, path, v, clock())
	})
}

func cloneIssueString(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func validateIssuedResourcePath(class *rpkiResourceClass, path rpkiIssuePath, v rpkiIssuePathValidation, now time.Time) error {
	if class == nil || len(class.Certificates) != 1 || len(path.Certificates) < 2 || len(path.Publications) != len(path.Certificates)-1 || path.Certificates[0] == nil || path.Certificates[1] == nil {
		return errRPKIUpDown
	}
	issued := class.Certificates[0]
	if !bytes.Equal(issued.DER, path.Certificates[0].Raw) || !bytes.Equal(class.IssuerDER, path.Certificates[1].Raw) || !slices.Contains(strings.Split(issued.URLs, ","), path.Publications[0].ChildURI) {
		return errRPKIUpDown
	}
	_, err := verifyAndRecordRPKIManifestPath(v.Directory, path.Certificates, v.Anchor, path.Publications, now)
	return err
}
