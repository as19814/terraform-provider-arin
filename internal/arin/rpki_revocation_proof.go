package arin

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"slices"
	"strings"
	"time"
)

type rpkiRevocationProof struct {
	CertificateSHA256 string `json:"certificate_sha256"`
	IssuerSHA256      string `json:"issuer_sha256"`
	CRLSHA256         string `json:"crl_sha256"`
	ManifestSHA256    string `json:"manifest_sha256"`
	CRLURI            string `json:"crl_uri"`
	SKI               string `json:"ski"`
}

// checkRPKIRevocationProof checks supplied repository evidence only. Callers must
// additionally bind the issuer to the signed resource class, prove key absence
// in fresh parent inventory and persist manifest rollback protection before use.
// It never changes an exchange journal or treats certificate absence as a CRL.
func checkRPKIRevocationProof(der []byte, ski string, issuers []*x509.Certificate, anchor *x509.Certificate, upstream []rpkiPathPublication, publication rpkiPathPublication, now time.Time) (*rpkiRevocationProof, error) {
	if len(der) == 0 || len(der) > 512000 || len(issuers) == 0 || len(issuers) > 31 || issuers[0] == nil || now.IsZero() {
		return nil, errRPKIUpDown
	}
	normalized, err := upDownSKI(ski)
	if err != nil || normalized != ski {
		return nil, errRPKIUpDown
	}
	if _, err := verifyRPKIManifestPath(issuers, anchor, upstream, now); err != nil {
		return nil, err
	}
	certificate, err := x509.ParseCertificate(bytes.Clone(der))
	if err != nil || validateRPKICAProfile(certificate) != nil || now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
		return nil, errRPKIUpDown
	}
	key, err := rpkiPublicKeyIdentifier(certificate.RawSubjectPublicKeyInfo)
	if err != nil || key != ski {
		return nil, errRPKIUpDown
	}
	issuer, err := x509.ParseCertificate(bytes.Clone(issuers[0].Raw))
	if err != nil {
		return nil, errRPKIUpDown
	}
	if !bytes.Equal(certificate.RawIssuer, issuer.RawSubject) || !bytes.Equal(certificate.AuthorityKeyId, issuer.SubjectKeyId) || certificate.CheckSignatureFrom(issuer) != nil {
		return nil, errRPKIUpDown
	}
	resources := [][]pkix.Extension{certificate.Extensions}
	for _, c := range issuers {
		if c == nil {
			return nil, errRPKIUpDown
		}
		parsed, err := x509.ParseCertificate(c.Raw)
		if err != nil {
			return nil, errRPKIUpDown
		}
		resources = append(resources, parsed.Extensions)
	}
	if _, err := resolveRPKIResourcePath(resources); err != nil {
		return nil, err
	}
	checked, err := checkRPKIManifestForIssuer(publication.ManifestDER, issuer.Raw, publication.ManifestURI, publication.Files, now)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(certificate.CRLDistributionPoints, checked.CRLURI) {
		return nil, errRPKIUpDown
	}
	revoked := false
	for _, entry := range checked.CRL.RevokedCertificateEntries {
		if entry.SerialNumber.Cmp(certificate.SerialNumber) == 0 {
			if entry.RevocationTime.After(now) {
				return nil, errRPKIUpDown
			}
			revoked = true
		}
	}
	if !revoked {
		return nil, errRPKIUpDown
	}
	// Key retirement withdraws all certificates for that key in this publication
	// point, including reissued certificates with different names or serials.
	for name := range checked.Content.Files {
		if !strings.HasSuffix(name, ".cer") {
			continue
		}
		cert, err := x509.ParseCertificate(publication.Files[name])
		if err != nil {
			return nil, errRPKIUpDown
		}
		key, err := rpkiPublicKeyIdentifier(cert.RawSubjectPublicKeyInfo)
		if err != nil || key == ski {
			return nil, errRPKIUpDown
		}
	}
	return &rpkiRevocationProof{CertificateSHA256: rpkiManifestDigest(certificate.Raw), IssuerSHA256: rpkiManifestDigest(issuer.Raw), CRLSHA256: rpkiManifestDigest(checked.CRL.Raw), ManifestSHA256: rpkiManifestDigest(publication.ManifestDER), CRLURI: checked.CRLURI, SKI: ski}, nil
}

// verifyAndRecordRPKIRevocationProof checks all supplied evidence and atomically
// records both upstream manifests and the retiring issuer's own manifest. It
// shares history with issuance, preventing rollback across the two workflows.
func verifyAndRecordRPKIRevocationProof(directory string, der []byte, ski string, issuers []*x509.Certificate, anchor *x509.Certificate, upstream []rpkiPathPublication, publication rpkiPathPublication, now time.Time) (*rpkiRevocationProof, error) {
	var proof *rpkiRevocationProof
	err := withRPKIManifestHistory(directory, anchor, func(history *rpkiManifestHistory) error {
		var err error
		proof, err = checkRPKIRevocationProof(der, ski, issuers, anchor, upstream, publication, now)
		if err != nil {
			return err
		}
		for i, p := range upstream {
			if err := rememberRPKIManifest(history, issuers[i+1].Raw, p); err != nil {
				return err
			}
		}
		return rememberRPKIManifest(history, issuers[0].Raw, publication)
	})
	if err != nil {
		return nil, err
	}
	return proof, nil
}
