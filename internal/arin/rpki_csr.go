package arin

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
)

var oidRPKIExtensionRequest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 14}
var oidRPKIBasicConstraints = asn1.ObjectIdentifier{2, 5, 29, 19}
var oidRPKIKeyUsage = asn1.ObjectIdentifier{2, 5, 29, 15}
var oidRPKIExtendedKeyUsage = asn1.ObjectIdentifier{2, 5, 29, 37}

// This is the CA request profile. Subject reuse policy remains the parent's
// responsibility; this routine does not authorize reuse of a subject name.
func parseRPKICACSR(der []byte) (*x509.CertificateRequest, error) {
	if len(der) < 4 || len(der) > 512000 {
		return nil, errRPKIUpDown
	}
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil || csr.Version != 0 || csr.SignatureAlgorithm != x509.SHA256WithRSA || csr.CheckSignature() != nil {
		return nil, errRPKIUpDown
	}
	key, ok := csr.PublicKey.(*rsa.PublicKey)
	if !ok || key.N.BitLen() != 2048 || key.E != 65537 {
		return nil, errRPKIUpDown
	}
	var outer struct {
		Info      asn1.RawValue
		Algorithm pkix.AlgorithmIdentifier
		Signature asn1.BitString
	}
	if !rpkiCSRDER(der, &outer) || !bytes.Equal(outer.Algorithm.Parameters.FullBytes, []byte{5, 0}) {
		return nil, errRPKIUpDown
	}
	var info struct {
		Version    int
		Subject    asn1.RawValue
		PublicKey  asn1.RawValue
		Attributes []asn1.RawValue `asn1:"tag:0"`
	}
	if !rpkiCSRDER(csr.RawTBSCertificateRequest, &info) || len(info.Attributes) != 1 {
		return nil, errRPKIUpDown
	}
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		Key       asn1.BitString
	}
	if !rpkiCSRDER(csr.RawSubjectPublicKeyInfo, &spki) || !spki.Algorithm.Algorithm.Equal(asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}) || !bytes.Equal(spki.Algorithm.Parameters.FullBytes, []byte{5, 0}) {
		return nil, errRPKIUpDown
	}
	var attribute struct {
		Type   asn1.ObjectIdentifier
		Values []asn1.RawValue `asn1:"set"`
	}
	if !rpkiCSRDER(info.Attributes[0].FullBytes, &attribute) || !attribute.Type.Equal(oidRPKIExtensionRequest) || len(attribute.Values) != 1 {
		return nil, errRPKIUpDown
	}
	var extensions []pkix.Extension
	if !rpkiCSRDER(attribute.Values[0].FullBytes, &extensions) {
		return nil, errRPKIUpDown
	}
	seen := make(map[string]bool)
	ca := false
	for _, e := range extensions {
		if seen[e.Id.String()] {
			return nil, errRPKIUpDown
		}
		seen[e.Id.String()] = true
		switch {
		case e.Id.Equal(oidRPKIBasicConstraints):
			var constraints struct{ CA bool }
			if !e.Critical || !rpkiCSRDER(e.Value, &constraints) || !constraints.CA {
				return nil, errRPKIUpDown
			}
			ca = true
		case e.Id.Equal(oidRPKIKeyUsage):
			var usage asn1.BitString
			if !e.Critical || !rpkiCSRDER(e.Value, &usage) || usage.BitLength > 7 || usage.BitLength < 6 {
				return nil, errRPKIUpDown
			}
			for bit := 0; bit < 5; bit++ {
				if usage.At(bit) != 0 {
					return nil, errRPKIUpDown
				}
			}
			if usage.At(5) == 0 && usage.At(6) == 0 {
				return nil, errRPKIUpDown
			}
		case e.Id.Equal(oidRPKIExtendedKeyUsage):
			var purposes []asn1.ObjectIdentifier
			if !rpkiCSRDER(e.Value, &purposes) || len(purposes) == 0 {
				return nil, errRPKIUpDown
			}
		case e.Id.Equal(oidRPKISIA):
		default:
			return nil, errRPKIUpDown
		}
	}
	if !ca {
		return nil, errRPKIUpDown
	}
	if _, err := rpkiCASIA(extensions); err != nil {
		return nil, err
	}
	return csr, nil
}

// Re-encoding detects trailing fields that encoding/asn1 otherwise ignores.
func rpkiCSRDER[T any](der []byte, value *T) bool {
	rest, err := asn1.Unmarshal(der, value)
	if err != nil || len(rest) != 0 {
		return false
	}
	canonical, err := asn1.Marshal(*value)
	return err == nil && bytes.Equal(der, canonical)
}
