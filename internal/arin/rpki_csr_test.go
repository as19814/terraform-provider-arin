package arin

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"
)

func rpkiCSRTestExtensions(t *testing.T) []pkix.Extension {
	t.Helper()
	return []pkix.Extension{
		{Id: oidRPKIBasicConstraints, Critical: true, Value: []byte{0x30, 3, 1, 1, 0xff}},
		{Id: oidRPKIKeyUsage, Critical: true, Value: []byte{3, 2, 1, 6}},
		rpkiSIATestExtension(t),
	}
}

func TestRPKICACSR(t *testing.T) {
	identity, _ := cmsSigningFixture(t)
	key := identity.Signer
	encode := func(template *x509.CertificateRequest, signer crypto.Signer) []byte {
		t.Helper()
		der, err := x509.CreateCertificateRequest(rand.Reader, template, signer)
		if err != nil {
			t.Fatal(err)
		}
		return der
	}
	good := rpkiCSRTestExtensions(t)
	der := encode(&x509.CertificateRequest{ExtraExtensions: good}, key)
	if _, err := parseRPKICACSR(der); err != nil {
		t.Fatalf("valid CA CSR rejected: %v", err)
	}
	// KeyUsage is optional in a request, but CA BasicConstraints and SIA are not.
	if _, err := parseRPKICACSR(encode(&x509.CertificateRequest{ExtraExtensions: []pkix.Extension{good[0], good[2]}}, key)); err != nil {
		t.Fatal(err)
	}
	extensionCases := map[string][]pkix.Extension{
		"missing_all":             nil,
		"missing_ca":              {good[1], good[2]},
		"missing_sia":             {good[0], good[1]},
		"ee_request":              {{Id: oidRPKIBasicConstraints, Critical: true, Value: []byte{0x30, 0}}, good[2]},
		"path_length":             {{Id: oidRPKIBasicConstraints, Critical: true, Value: []byte{0x30, 6, 1, 1, 0xff, 2, 1, 0}}, good[2]},
		"noncritical_ca":          {{Id: oidRPKIBasicConstraints, Value: good[0].Value}, good[2]},
		"unknown_extension":       {good[0], good[2], {Id: asn1.ObjectIdentifier{1, 2, 3, 4}, Value: []byte{5, 0}}},
		"ip_resource_extension":   {good[0], good[2], {Id: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 7}, Critical: true, Value: []byte{0x30, 0}}},
		"digital_signature_usage": {good[0], good[2], {Id: oidRPKIKeyUsage, Critical: true, Value: []byte{3, 2, 7, 0x80}}},
		"noncritical_usage":       {good[0], good[2], {Id: oidRPKIKeyUsage, Value: good[1].Value}},
		"empty_extended_usage":    {good[0], good[2], {Id: oidRPKIExtendedKeyUsage, Value: []byte{0x30, 0}}},
		"duplicate_sia":           {good[0], good[2], good[2]},
	}
	for name, extensions := range extensionCases {
		t.Run(name, func(t *testing.T) {
			der := encode(&x509.CertificateRequest{ExtraExtensions: extensions}, key)
			if _, err := parseRPKICACSR(der); err == nil {
				t.Fatal("invalid CSR profile accepted")
			}
		})
	}
	for _, algorithm := range []x509.SignatureAlgorithm{x509.SHA384WithRSA, x509.SHA256WithRSAPSS} {
		der := encode(&x509.CertificateRequest{ExtraExtensions: good, SignatureAlgorithm: algorithm}, key)
		if _, err := parseRPKICACSR(der); err == nil {
			t.Fatal("unsupported signature algorithm accepted")
		}
	}
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, signer := range []crypto.Signer{weak, ec} {
		if _, err := parseRPKICACSR(encode(&x509.CertificateRequest{ExtraExtensions: good}, signer)); err == nil {
			t.Fatal("unsupported key accepted")
		}
	}
	unknownAttribute := pkix.AttributeTypeAndValueSET{Type: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 7}, Value: [][]pkix.AttributeTypeAndValue{{{Type: asn1.ObjectIdentifier{1, 2, 3}, Value: "private challenge"}}}}
	if _, err := parseRPKICACSR(encode(&x509.CertificateRequest{ExtraExtensions: good, Attributes: []pkix.AttributeTypeAndValueSET{unknownAttribute}}, key)); err == nil {
		t.Fatal("unsupported attribute accepted")
	}
}
