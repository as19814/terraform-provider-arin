package arin

import (
	"bytes"
	"crypto/x509"
	"slices"
	"strings"
	"time"
)

// One issuer publication point for each non-anchor certificate, in the same
// leaf-first order as the CA path. ChildURI identifies that certificate's file.
type rpkiPathPublication struct {
	ManifestDER []byte
	ManifestURI string
	ChildURI    string
	Files       map[string][]byte
}

// verifyRPKIManifestPath walks down from the configured anchor. Each issuer
// must already have a verified path before its manifest can select the next
// certificate and CRL. Persistent rollback protection is still a caller duty;
// this function neither fetches data nor records accepted manifest versions.
func verifyRPKIManifestPath(path []*x509.Certificate, anchor *x509.Certificate, publications []rpkiPathPublication, now time.Time) (*rpkiCertificateResources, error) {
	if len(path) == 0 || len(path) > 32 || len(publications) != len(path)-1 {
		return nil, errRPKIManifest
	}
	total := 0
	for _, p := range publications {
		if len(p.ManifestDER) > 4<<20 || len(p.Files) > 10000 {
			return nil, errRPKIManifest
		}
		total += len(p.ManifestDER)
		for _, data := range p.Files {
			if len(data) > 4<<20 {
				return nil, errRPKIManifest
			}
			total += len(data)
			if total > 128<<20 {
				return nil, errRPKIManifest
			}
		}
		if total > 128<<20 {
			return nil, errRPKIManifest
		}
	}
	resources, err := verifyRPKICertificatePath(path[len(path)-1:], anchor, nil, now)
	if err != nil {
		return nil, err
	}
	var crls []*x509.RevocationList
	for i := len(path) - 2; i >= 0; i-- {
		if path[i] == nil || len(path[i].Raw) == 0 || len(path[i].Raw) > 512000 {
			return nil, errRPKIManifest
		}
		child, err := x509.ParseCertificate(path[i].Raw)
		if err != nil {
			return nil, err
		}
		p := publications[i]
		checked, err := checkRPKIManifestForIssuer(p.ManifestDER, path[i+1].Raw, p.ManifestURI, p.Files, now)
		if err != nil {
			return nil, err
		}
		directory := p.ManifestURI[:strings.LastIndexByte(p.ManifestURI, '/')+1]
		name, ok := strings.CutPrefix(p.ChildURI, directory)
		if !ok || !validRPKIManifestFilename(name) || !strings.HasSuffix(name, ".cer") {
			return nil, errRPKIManifest
		}
		if _, listed := checked.Content.Files[name]; !listed || !bytes.Equal(p.Files[name], child.Raw) || !slices.Contains(child.CRLDistributionPoints, checked.CRLURI) {
			return nil, errRPKIManifest
		}
		crls = append(crls, checked.CRL)
		resources, err = verifyRPKICertificatePath(path[i:], anchor, crls, now)
		if err != nil {
			return nil, err
		}
	}
	return resources, nil
}
