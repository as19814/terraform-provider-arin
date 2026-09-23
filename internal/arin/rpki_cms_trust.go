package arin

import (
	"bytes"
	"crypto/x509"
	"errors"
	"time"
)

type rpkiCMSTrust struct {
	// Anchor must come from the authenticated out-of-band setup exchange. System
	// roots and certificates embedded in the message never establish trust.
	Anchor        *x509.Certificate
	Intermediates []*x509.Certificate
	Now           time.Time
	// LastSigningTime is the persisted high-water mark for this peer. Equal times
	// are allowed by RFC 6492; message correlation remains a protocol-layer check.
	LastSigningTime time.Time
}
type verifiedRPKICMS struct {
	Content     []byte
	SigningTime time.Time
}

var errRPKICMSTrust = errors.New("RPKI CMS trust, revocation or signing-time validation failed")

// verifyRPKICMS authenticates the envelope to one configured BPKI anchor. The
// caller must still validate XML identity/response semantics and durably commit
// the returned signing time before subsequent exchanges with the same peer.
func verifyRPKICMS(der []byte, trust rpkiCMSTrust) (*verifiedRPKICMS, error) {
	if trust.Anchor == nil || trust.Now.IsZero() || len(trust.Intermediates) > 32 {
		return nil, errRPKICMSTrust
	}
	anchor, err := x509.ParseCertificate(trust.Anchor.Raw)
	if err != nil || !anchor.IsCA || !anchor.BasicConstraintsValid {
		return nil, errRPKICMSTrust
	}
	envelope, err := decodeRPKICMS(der)
	if err != nil {
		return nil, err
	}
	if envelope.SigningTime.Before(trust.LastSigningTime) || envelope.SigningTime.After(trust.Now.Add(5*time.Minute)) {
		return nil, errRPKICMSTrust
	}
	if envelope.Signer.KeyUsage != 0 && envelope.Signer.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return nil, errRPKICMSTrust
	}
	roots := x509.NewCertPool()
	roots.AddCert(anchor)
	intermediates := x509.NewCertPool()
	for _, cert := range envelope.Certificates {
		if cert.IsCA {
			intermediates.AddCert(cert)
		}
	}
	for _, cert := range trust.Intermediates {
		if cert == nil {
			return nil, errRPKICMSTrust
		}
		parsed, err := x509.ParseCertificate(cert.Raw)
		if err != nil || !parsed.IsCA {
			return nil, errRPKICMSTrust
		}
		intermediates.AddCert(parsed)
	}
	chains, err := envelope.Signer.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, CurrentTime: trust.Now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	if err != nil {
		return nil, errRPKICMSTrust
	}
	for _, chain := range chains {
		if len(chain) < 2 || !bytes.Equal(chain[len(chain)-1].Raw, anchor.Raw) {
			continue
		}
		valid := true
		for i, cert := range chain {
			for _, ext := range cert.Extensions {
				if ext.Id.String() == "1.3.6.1.5.5.7.1.7" || ext.Id.String() == "1.3.6.1.5.5.7.1.8" {
					valid = false
				}
			}
			if i+1 < len(chain) && !cmsCertificateUnrevoked(cert, chain[i+1], envelope.CRLs, trust.Now) {
				valid = false
			}
		}
		if valid {
			return &verifiedRPKICMS{Content: bytes.Clone(envelope.Content), SigningTime: envelope.SigningTime}, nil
		}
	}
	return nil, errRPKICMSTrust
}

func cmsCertificateUnrevoked(cert, issuer *x509.Certificate, crls []*x509.RevocationList, now time.Time) bool {
	var latest *x509.RevocationList
	for _, crl := range crls {
		if !bytes.Equal(crl.RawIssuer, issuer.RawSubject) || crl.CheckSignatureFrom(issuer) != nil {
			continue
		}
		if len(crl.AuthorityKeyId) == 0 || (len(issuer.SubjectKeyId) > 0 && !bytes.Equal(crl.AuthorityKeyId, issuer.SubjectKeyId)) {
			return false
		}
		if crl.Number == nil || crl.Number.Sign() < 0 {
			return false
		}
		// Scoped, indirect and delta CRLs need additional semantics. Never treat one
		// as a complete issuer-wide list, even if an extension is noncritical.
		for _, ext := range crl.Extensions {
			if ext.Critical || ext.Id.String() == "2.5.29.27" || ext.Id.String() == "2.5.29.28" {
				return false
			}
		}
		for _, entry := range crl.RevokedCertificateEntries {
			if entry.SerialNumber == nil || entry.ReasonCode == 8 {
				return false
			}
			for _, ext := range entry.Extensions {
				if ext.Critical || ext.Id.String() == "2.5.29.29" {
					return false
				}
			}
		}
		if latest == nil {
			latest = crl
			continue
		}
		order := crl.Number.Cmp(latest.Number)
		if order == 0 && !bytes.Equal(crl.Raw, latest.Raw) {
			return false
		}
		if order > 0 {
			if crl.ThisUpdate.Before(latest.ThisUpdate) {
				return false
			}
			latest = crl
		} else if order < 0 && crl.ThisUpdate.After(latest.ThisUpdate) {
			return false
		}
	}
	if latest == nil || latest.ThisUpdate.After(now) || !latest.NextUpdate.After(now) || !latest.NextUpdate.After(latest.ThisUpdate) {
		return false
	}
	for _, entry := range latest.RevokedCertificateEntries {
		if entry.SerialNumber.Cmp(cert.SerialNumber) == 0 {
			return false
		}
	}
	return true
}
