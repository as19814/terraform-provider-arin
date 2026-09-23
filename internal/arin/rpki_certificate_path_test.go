package arin

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

type resourcePathFixture struct {
	certs []*x509.Certificate
	keys  []*rsa.PrivateKey
	crls  []*x509.RevocationList
}

func resourceCertificateFixture(t *testing.T) resourcePathFixture {
	t.Helper()
	var f resourcePathFixture
	// Build anchor first, then reverse to the verifier's leaf-first order.
	for i := 0; i < 3; i++ {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		ext := resourceTestAS(t, []byte{5, 0})
		if i == 0 {
			ext = resourceTestAS(t, resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, struct{ Min, Max int64 }{64500, 64510})}))
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 1)), Subject: pkix.Name{CommonName: []string{"root", "middle", "leaf"}[i]}, NotBefore: cmsTrustNow().Add(-time.Hour), NotAfter: cmsTrustNow().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, ExtraExtensions: []pkix.Extension{ext}}
		parent, signer := template, key
		if i > 0 {
			parent, signer = f.certs[i-1], f.keys[i-1]
		}
		raw, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(raw)
		if err != nil {
			t.Fatal(err)
		}
		f.certs = append(f.certs, cert)
		f.keys = append(f.keys, key)
	}
	f.certs[0], f.certs[2] = f.certs[2], f.certs[0]
	f.keys[0], f.keys[2] = f.keys[2], f.keys[0]
	for i := 1; i < len(f.certs); i++ {
		f.crls = append(f.crls, resourcePathCRL(t, f.certs[i], f.keys[i], nil))
	}
	return f
}
func resourcePathCRL(t *testing.T, issuer *x509.Certificate, key *rsa.PrivateKey, revoked *big.Int) *x509.RevocationList {
	t.Helper()
	template := &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: cmsTrustNow().Add(-time.Minute), NextUpdate: cmsTrustNow().Add(time.Hour)}
	if revoked != nil {
		template.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: revoked, RevocationTime: cmsTrustNow().Add(-time.Minute)}}
	}
	raw, err := x509.CreateRevocationList(rand.Reader, template, issuer, key)
	if err != nil {
		t.Fatal(err)
	}
	crl, err := x509.ParseRevocationList(raw)
	if err != nil {
		t.Fatal(err)
	}
	return crl
}
func TestRPKICertificatePath(t *testing.T) {
	f := resourceCertificateFixture(t)
	anchor := f.certs[2]
	got, err := verifyRPKICertificatePath(f.certs, anchor, f.crls, cmsTrustNow())
	if err != nil || got.ASN == nil || got.ASN.Inherit || got.ASN.Ranges[0] != (rpkiASRange{64500, 64510}) {
		t.Fatalf("valid resource path rejected: %v", err)
	}
	if len(f.certs[0].UnhandledCriticalExtensions) == 0 {
		t.Fatal("verification mutated input certificate")
	}
	if _, err := verifyRPKICertificatePath([]*x509.Certificate{anchor}, anchor, nil, cmsTrustNow()); err != nil {
		t.Fatalf("explicit anchor rejected: %v", err)
	}
	for name, crls := range map[string][]*x509.RevocationList{
		"missing": nil, "missing_middle": f.crls[:1], "nil_crl": {nil},
		"revoked_leaf":   {resourcePathCRL(t, f.certs[1], f.keys[1], f.certs[0].SerialNumber), f.crls[1]},
		"revoked_middle": {f.crls[0], resourcePathCRL(t, anchor, f.keys[2], f.certs[1].SerialNumber)},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := verifyRPKICertificatePath(f.certs, anchor, crls, cmsTrustNow()); err == nil || got != nil {
				t.Fatal("unverified revocation accepted")
			}
		})
	}
	for _, now := range []time.Time{cmsTrustNow().Add(-2 * time.Hour), cmsTrustNow().Add(2 * time.Hour)} {
		if _, err := verifyRPKICertificatePath(f.certs, anchor, f.crls, now); err == nil {
			t.Fatal("expired or future path accepted")
		}
	}
	for name, path := range map[string][]*x509.Certificate{
		"empty": nil, "nil_cert": {nil, anchor}, "reordered": {f.certs[0], anchor, f.certs[1]}, "missing_issuer": {f.certs[0], anchor}, "duplicate": {f.certs[0], f.certs[1], f.certs[1], anchor},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := verifyRPKICertificatePath(path, anchor, f.crls, cmsTrustNow()); err == nil || got != nil {
				t.Fatal("invalid exact path accepted")
			}
		})
	}
	if _, err := verifyRPKICertificatePath(f.certs, f.certs[1], f.crls, cmsTrustNow()); err == nil {
		t.Fatal("wrong configured anchor accepted")
	}
}

func TestRPKICertificatePathRejectsOverclaimAndUnknownCritical(t *testing.T) {
	f := resourceCertificateFixture(t)
	for _, mode := range []string{"overclaim", "unknown_critical", "signature", "forged_fields"} {
		t.Run(mode, func(t *testing.T) {
			leaf := *f.certs[0]
			leaf.ExtraExtensions = append([]pkix.Extension(nil), leaf.Extensions...)
			switch mode {
			case "overclaim":
				for i, e := range leaf.ExtraExtensions {
					if e.Id.Equal(oidRPKIASResources) {
						leaf.ExtraExtensions[i] = resourceTestAS(t, resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, int64(64511))}))
					}
				}
			case "unknown_critical":
				leaf.ExtraExtensions = append(leaf.ExtraExtensions, pkix.Extension{Id: asn1.ObjectIdentifier{1, 2, 3, 99}, Critical: true, Value: []byte{5, 0}})
			}
			raw, err := x509.CreateCertificate(rand.Reader, &leaf, f.certs[1], &f.keys[0].PublicKey, f.keys[1])
			if err != nil {
				t.Fatal(err)
			}
			if mode == "signature" || mode == "forged_fields" {
				raw[len(raw)-1] ^= 1
			}
			parsed, err := x509.ParseCertificate(raw)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "forged_fields" {
				parsed.Signature = f.certs[0].Signature
				parsed.RawTBSCertificate = f.certs[0].RawTBSCertificate
			}
			path := []*x509.Certificate{parsed, f.certs[1], f.certs[2]}
			if got, err := verifyRPKICertificatePath(path, f.certs[2], f.crls, cmsTrustNow()); err == nil || got != nil {
				t.Fatal("invalid authenticated resource path accepted")
			}
		})
	}
}
