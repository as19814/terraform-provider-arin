package arin

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"time"
)

// Verify the exact supplied leaf-first CA path against an explicitly configured
// resource anchor. CRLs must already be selected using authenticated manifests
// and CRLDPs; this function does not perform that selection. It checks current CRL
// status and profile-specific verified resource sets, not the entire RPKI profile.
func verifyRPKICertificatePath(path []*x509.Certificate, anchor *x509.Certificate, crls []*x509.RevocationList, now time.Time) (*rpkiCertificateResources, error) {
	if len(path) == 0 || len(path) > 32 || anchor == nil || now.IsZero() || len(crls) > 32 {
		return nil, errRPKIUpDown
	}
	parsed := make([]*x509.Certificate, len(path))
	extensions := make([][]pkix.Extension, len(path))
	total := 0
	for i, c := range path {
		if c == nil || len(c.Raw) == 0 || len(c.Raw) > 512000 {
			return nil, errRPKIUpDown
		}
		total += len(c.Raw)
		if total > 4<<20 {
			return nil, errRPKIUpDown
		}
		cert, err := x509.ParseCertificate(c.Raw)
		if err != nil || !cert.IsCA || !cert.BasicConstraintsValid || cert.KeyUsage&(x509.KeyUsageCertSign|x509.KeyUsageCRLSign) != (x509.KeyUsageCertSign|x509.KeyUsageCRLSign) {
			return nil, errRPKIUpDown
		}
		if err := validateRPKICAProfile(cert); err != nil {
			return nil, err
		}
		// Decode before allowing PKIX verification to treat these critical extensions
		// as understood. Do not alter caller-owned certificate objects.
		if _, err := parseRPKICertificateResources(cert.Extensions); err != nil {
			return nil, err
		}
		var unhandled []asn1.ObjectIdentifier
		for _, oid := range cert.UnhandledCriticalExtensions {
			if !oid.Equal(oidRPKIASResources) && !oid.Equal(oidRPKIIPResources) && !oid.Equal(oidRPKIASResourcesV2) && !oid.Equal(oidRPKIIPResourcesV2) {
				unhandled = append(unhandled, oid)
			}
		}
		cert.UnhandledCriticalExtensions = unhandled
		parsed[i], extensions[i] = cert, cert.Extensions
	}
	if !bytes.Equal(parsed[len(parsed)-1].Raw, anchor.Raw) {
		return nil, errRPKIUpDown
	}
	parsedCRLs := make([]*x509.RevocationList, len(crls))
	for i, c := range crls {
		if c == nil || len(c.Raw) == 0 || len(c.Raw) > 4<<20 {
			return nil, errRPKIUpDown
		}
		total += len(c.Raw)
		if total > 4<<20 {
			return nil, errRPKIUpDown
		}
		var err error
		parsedCRLs[i], err = x509.ParseRevocationList(c.Raw)
		if err != nil || validateRPKICRLProfile(parsedCRLs[i]) != nil {
			return nil, errRPKIUpDown
		}
	}
	roots, intermediates := x509.NewCertPool(), x509.NewCertPool()
	roots.AddCert(parsed[len(parsed)-1])
	for i := 1; i+1 < len(parsed); i++ {
		intermediates.AddCert(parsed[i])
	}
	chains, err := parsed[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	if err != nil {
		return nil, errRPKIUpDown
	}
	exact := false
	for _, chain := range chains {
		if len(chain) != len(parsed) {
			continue
		}
		match := true
		for i := range chain {
			if !bytes.Equal(chain[i].Raw, parsed[i].Raw) {
				match = false
				break
			}
		}
		if match {
			exact = true
			break
		}
	}
	if !exact {
		return nil, errRPKIUpDown
	}
	for i := 0; i+1 < len(parsed); i++ {
		cert, issuer := parsed[i], parsed[i+1]
		if !bytes.Equal(cert.RawIssuer, issuer.RawSubject) || len(cert.AuthorityKeyId) == 0 || !bytes.Equal(cert.AuthorityKeyId, issuer.SubjectKeyId) || !resourceCertificateUnrevoked(cert, issuer, parsedCRLs, now) {
			return nil, errRPKIUpDown
		}
	}
	return resolveRPKIResourcePath(extensions)
}
