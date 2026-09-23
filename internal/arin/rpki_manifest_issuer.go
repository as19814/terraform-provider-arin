package arin

import (
	"bytes"
	"crypto/x509"
	"slices"
	"strings"
	"time"
)

// issuerCheckedManifest is checked against the supplied issuer, not a trust
// anchor. The caller must authenticate that issuer's exact resource path and
// enforce persistent manifest rollback protection before using this result.
type issuerCheckedManifest struct {
	Content *rpkiManifestContent
	CRL     *x509.RevocationList
	CRLURI  string
}

// Files are keyed by manifest filename, not URI. No locations are fetched.
func checkRPKIManifestForIssuer(der, issuerDER []byte, manifestURI string, files map[string][]byte, now time.Time) (*issuerCheckedManifest, error) {
	if now.IsZero() || len(issuerDER) == 0 || len(issuerDER) > 512000 || len(der) == 0 || len(der) > 4<<20 || !publicationURI(manifestURI) {
		return nil, errRPKIManifest
	}
	slash := strings.LastIndexByte(manifestURI, '/')
	directory, name := manifestURI[:slash+1], manifestURI[slash+1:]
	if !validRPKIManifestFilename(name) || !strings.HasSuffix(name, ".mft") {
		return nil, errRPKIManifest
	}
	issuer, err := x509.ParseCertificate(bytes.Clone(issuerDER))
	if err != nil || validateRPKICAProfile(issuer) != nil || now.Before(issuer.NotBefore) || now.After(issuer.NotAfter) {
		return nil, errRPKIManifest
	}
	sia, err := rpkiCASIA(issuer.Extensions)
	if err != nil {
		return nil, err
	}
	var descriptions []rpkiAccessDescription
	if !rpkiCSRDER(sia, &descriptions) {
		return nil, errRPKIManifest
	}
	repositoryMatch, manifestMatch := false, false
	for _, d := range descriptions {
		uri := string(d.Location.Bytes)
		if d.Method.Equal(oidRPKIRepository) && uri == directory {
			repositoryMatch = true
		}
		if d.Method.Equal(oidRPKIManifest) && uri == manifestURI {
			manifestMatch = true
		}
	}
	if !repositoryMatch || !manifestMatch {
		return nil, errRPKIManifest
	}
	m, err := decodeRPKIManifest(bytes.Clone(der))
	if err != nil {
		return nil, err
	}
	ee := m.Signer
	if now.Before(ee.NotBefore) || now.After(ee.NotAfter) || !bytes.Equal(ee.RawIssuer, issuer.RawSubject) || !bytes.Equal(ee.AuthorityKeyId, issuer.SubjectKeyId) || ee.CheckSignatureFrom(issuer) != nil {
		return nil, errRPKIManifest
	}
	locations, err := rpkiEESIA(ee.Extensions)
	if err != nil || !slices.Contains(locations, manifestURI) {
		return nil, errRPKIManifest
	}
	// The EE profile already requires inheritance. Every inherited family must
	// exist in this issuer; upstream resolution belongs to issuer path validation.
	child, err := parseRPKICertificateResources(ee.Extensions)
	if err != nil {
		return nil, err
	}
	parent, err := parseRPKICertificateResources(issuer.Extensions)
	if err != nil {
		return nil, err
	}
	if (child.ASN != nil && parent.ASN == nil) || (child.IPv4 != nil && parent.IPv4 == nil) || (child.IPv6 != nil && parent.IPv6 == nil) {
		return nil, errRPKIManifest
	}
	if _, exists := m.Content.Files[name]; exists {
		return nil, errRPKIManifest
	}
	// Bound the listed input before hashing. Unlisted repository objects are not
	// candidates for revocation checking, even if they have a larger CRL number.
	total := 0
	for filename := range m.Content.Files {
		size := len(files[filename])
		if size > 4<<20 {
			return nil, errRPKIManifest
		}
		total += size
		if total > 64<<20 {
			return nil, errRPKIManifest
		}
	}
	if err := m.Content.checkFiles(now, files); err != nil {
		return nil, err
	}
	var crlName string
	for filename := range m.Content.Files {
		if strings.HasSuffix(filename, ".crl") {
			crlName = filename
		}
	}
	crlURI := directory + crlName
	if !slices.Contains(ee.CRLDistributionPoints, crlURI) {
		return nil, errRPKIManifest
	}
	crl, err := x509.ParseRevocationList(bytes.Clone(files[crlName]))
	if err != nil || !resourceCertificateUnrevoked(ee, issuer, []*x509.RevocationList{crl}, now) {
		return nil, errRPKIManifest
	}
	return &issuerCheckedManifest{Content: m.Content, CRL: crl, CRLURI: crlURI}, nil
}
