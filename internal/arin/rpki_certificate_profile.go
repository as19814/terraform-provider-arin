package arin

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"net/url"
	"strings"
)

// Original resource-extension profiles. Signature/path, revocation and
// resource-allocation validation remain separate requirements.
func validateRPKICAProfile(cert *x509.Certificate) error {
	return validateRPKICertificateProfile(cert, false)
}

func validateRPKIManifestEEProfile(cert *x509.Certificate) error {
	return validateRPKICertificateProfile(cert, true)
}

// Callers must supply certificates freshly parsed from DER.
func validateRPKICertificateProfile(cert *x509.Certificate, manifestEE bool) error {
	if err := validateRPKICertificateNamesAndTimes(cert); err != nil {
		return err
	}
	key, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || key.N.BitLen() != 2048 || key.E != 65537 || cert.SignatureAlgorithm != x509.SHA256WithRSA || cert.SerialNumber == nil || cert.SerialNumber.Sign() <= 0 || cert.SerialNumber.BitLen() > 159 {
		return errRPKIUpDown
	}
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		Key       asn1.BitString
	}
	if !rpkiCSRDER(cert.RawSubjectPublicKeyInfo, &spki) || !spki.Algorithm.Algorithm.Equal(asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}) || !bytes.Equal(spki.Algorithm.Parameters.FullBytes, []byte{5, 0}) {
		return errRPKIUpDown
	}
	outer, err := rpkiResourceSequence(cert.Raw)
	if err != nil || len(outer) != 3 {
		return errRPKIUpDown
	}
	tbs, err := rpkiResourceSequence(cert.RawTBSCertificate)
	if err != nil {
		return errRPKIUpDown
	}
	var algorithm pkix.AlgorithmIdentifier
	if !rpkiCSRDER(outer[1].FullBytes, &algorithm) || !bytes.Equal(algorithm.Parameters.FullBytes, []byte{5, 0}) || !bytes.Equal(outer[1].FullBytes, tbs[2].FullBytes) {
		return errRPKIUpDown
	}
	selfSigned := bytes.Equal(cert.RawIssuer, cert.RawSubject) && cert.CheckSignatureFrom(cert) == nil
	seen := make(map[string]bool)
	for _, e := range cert.Extensions {
		oid := e.Id.String()
		if seen[oid] {
			return errRPKIUpDown
		}
		seen[oid] = true
		switch oid {
		case "2.5.29.19":
			if manifestEE {
				return errRPKIUpDown
			}
			var bc struct{ CA bool }
			if !e.Critical || !rpkiCSRDER(e.Value, &bc) || !bc.CA {
				return errRPKIUpDown
			}
		case "2.5.29.15":
			usage := []byte{3, 2, 1, 6}
			if manifestEE {
				usage = []byte{3, 2, 7, 0x80}
			}
			if !e.Critical || !bytes.Equal(e.Value, usage) {
				return errRPKIUpDown
			}
		case "2.5.29.14":
			var ski []byte
			digest := sha1.Sum(spki.Key.Bytes) // RFC 5280 method 1 key identifier, not a signature.
			if e.Critical || !rpkiCSRDER(e.Value, &ski) || !bytes.Equal(ski, digest[:]) {
				return errRPKIUpDown
			}
		case "2.5.29.35":
			var aki struct {
				KeyID []byte `asn1:"tag:0"`
			}
			if e.Critical || !rpkiCSRDER(e.Value, &aki) || len(aki.KeyID) != 20 || (selfSigned && !bytes.Equal(aki.KeyID, cert.SubjectKeyId)) {
				return errRPKIUpDown
			}
		case "2.5.29.32":
			if !e.Critical || !validRPKIPolicies(e.Value) {
				return errRPKIUpDown
			}
		case "1.3.6.1.5.5.7.1.1":
			if e.Critical || selfSigned || !validRPKIAIA(e.Value) {
				return errRPKIUpDown
			}
		case "2.5.29.31":
			if e.Critical || selfSigned || !validRPKICRLDP(e.Value) {
				return errRPKIUpDown
			}
		case "1.3.6.1.5.5.7.1.11", "1.3.6.1.5.5.7.1.7", "1.3.6.1.5.5.7.1.8":
			// Dedicated decoders below enforce these extension profiles.
		default:
			return errRPKIUpDown
		}
	}
	required := []string{"2.5.29.15", "2.5.29.14", "2.5.29.32", "1.3.6.1.5.5.7.1.11"}
	if !manifestEE {
		required = append(required, "2.5.29.19")
	}
	for _, oid := range required {
		if !seen[oid] {
			return errRPKIUpDown
		}
	}
	if (manifestEE || !selfSigned) && (!seen["2.5.29.35"] || !seen["2.5.29.31"] || !seen["1.3.6.1.5.5.7.1.1"]) {
		return errRPKIUpDown
	}
	if manifestEE {
		if cert.IsCA {
			return errRPKIUpDown
		}
		if _, err := rpkiEESIA(cert.Extensions); err != nil {
			return err
		}
	} else if _, err := rpkiCASIA(cert.Extensions); err != nil {
		return err
	}
	resources, err := parseRPKICertificateResources(cert.Extensions)
	if err != nil {
		return err
	}
	if manifestEE && ((resources.ASN != nil && !resources.ASN.Inherit) || (resources.IPv4 != nil && !resources.IPv4.Inherit) || (resources.IPv6 != nil && !resources.IPv6.Inherit)) {
		return errRPKIUpDown
	}

	return nil
}

func validRPKIProfileURI(s string) bool {
	if len(s) == 0 || len(s) > 4096 {
		return false
	}
	for _, r := range s {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	u, err := url.Parse(s)
	return err == nil && u.IsAbs()
}

func validRPKIPolicies(der []byte) bool {
	type qualifier struct {
		ID    asn1.ObjectIdentifier
		Value asn1.RawValue
	}
	var policies []struct {
		ID         asn1.ObjectIdentifier
		Qualifiers []qualifier `asn1:"optional"`
	}
	if !rpkiCSRDER(der, &policies) || len(policies) != 1 || !policies[0].ID.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 14, 2}) || len(policies[0].Qualifiers) > 1 {
		return false
	}
	if len(policies[0].Qualifiers) == 1 {
		q := policies[0].Qualifiers[0]
		if !q.ID.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 2, 1}) || q.Value.Class != 0 || q.Value.Tag != 22 || q.Value.IsCompound || !validRPKIProfileURI(string(q.Value.Bytes)) {
			return false
		}
	}
	return true
}

func validRPKIAIA(der []byte) bool {
	var descriptions []rpkiAccessDescription
	if !rpkiCSRDER(der, &descriptions) || len(descriptions) == 0 {
		return false
	}
	rsync := false
	for _, d := range descriptions {
		if !d.Method.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 2}) || d.Location.Class != 2 || d.Location.Tag != 6 || d.Location.IsCompound {
			return false
		}
		uri := string(d.Location.Bytes)
		if !validRPKIProfileURI(uri) {
			return false
		}
		if strings.HasPrefix(uri, "rsync:") {
			if !publicationURI(uri) || strings.HasSuffix(uri, "/") {
				return false
			}
			rsync = true
		}
	}
	return rsync
}

func validRPKICRLDP(der []byte) bool {
	points, err := rpkiResourceSequence(der)
	if err != nil || len(points) != 1 {
		return false
	}
	fields, err := rpkiResourceSequence(points[0].FullBytes)
	if err != nil || len(fields) != 1 || fields[0].Class != 2 || fields[0].Tag != 0 || !fields[0].IsCompound {
		return false
	}
	var fullName asn1.RawValue
	rest, err := asn1.Unmarshal(fields[0].Bytes, &fullName)
	if err != nil || len(rest) != 0 || fullName.Class != 2 || fullName.Tag != 0 || !fullName.IsCompound {
		return false
	}
	rsync := false
	for remaining := fullName.Bytes; len(remaining) > 0; {
		var name asn1.RawValue
		remaining, err = asn1.Unmarshal(remaining, &name)
		if err != nil || name.Class != 2 || name.Tag != 6 || name.IsCompound {
			return false
		}
		uri := string(name.Bytes)
		if !validRPKIProfileURI(uri) {
			return false
		}
		if strings.HasPrefix(uri, "rsync:") {
			if !publicationURI(uri) || strings.HasSuffix(uri, "/") {
				return false
			}
			rsync = true
		}
	}
	return rsync
}
