package arin

import (
	"bytes"
	"crypto/x509"
	"time"
)

// The caller must supply the CRL selected by an authenticated current manifest
// and the certificate's CRLDP. This primitive rejects ambiguous issuer CRLs and
// deliberately does not use CRL numbers for selection (RFC 9829).
func resourceCertificateUnrevoked(cert, issuer *x509.Certificate, crls []*x509.RevocationList, now time.Time) bool {
	var selected *x509.RevocationList
	for _, crl := range crls {
		if !bytes.Equal(crl.RawIssuer, issuer.RawSubject) || crl.CheckSignatureFrom(issuer) != nil {
			continue
		}
		if !bytes.Equal(crl.AuthorityKeyId, issuer.SubjectKeyId) || validateRPKICRLProfile(crl) != nil {
			return false
		}
		if selected != nil && !bytes.Equal(selected.Raw, crl.Raw) {
			return false
		}
		selected = crl
	}
	if selected == nil || selected.ThisUpdate.After(now) || !selected.NextUpdate.After(now) || !selected.NextUpdate.After(selected.ThisUpdate) {
		return false
	}
	for _, entry := range selected.RevokedCertificateEntries {
		if entry.SerialNumber.Cmp(cert.SerialNumber) == 0 {
			return false
		}
	}
	return true
}
