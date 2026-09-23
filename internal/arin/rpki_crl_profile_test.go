package arin

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

func TestRPKICRLProfile(t *testing.T) {
	f := resourceCertificateFixture(t)
	issuer, key := f.certs[1], f.keys[1]
	makeCRL := func(template *x509.RevocationList) *x509.RevocationList {
		t.Helper()
		raw, err := x509.CreateRevocationList(rand.Reader, template, issuer, key)
		if err != nil {
			t.Fatal(err)
		}
		crl, err := x509.ParseRevocationList(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := crl.CheckSignatureFrom(issuer); err != nil {
			t.Fatal(err)
		}
		return crl
	}
	base := func() *x509.RevocationList {
		return &x509.RevocationList{Number: big.NewInt(0), ThisUpdate: cmsTrustNow(), NextUpdate: cmsTrustNow().Add(time.Hour)}
	}
	for _, withEntry := range []bool{false, true} {
		template := base()
		if withEntry {
			template.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(12), RevocationTime: cmsTrustNow().Add(-time.Minute)}}
		}
		if err := validateRPKICRLProfile(makeCRL(template)); err != nil {
			t.Fatalf("valid CRL rejected: %v", err)
		}
	}
	for name, mutate := range map[string]func(*x509.RevocationList){
		"unknown_noncritical": func(c *x509.RevocationList) {
			c.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 3, 99}, Value: []byte{5, 0}}}
		},
		"delta": func(c *x509.RevocationList) {
			c.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 27}, Value: []byte{2, 1, 0}}}
		},
		"indirect_scope": func(c *x509.RevocationList) {
			c.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 28}, Value: []byte{0x30, 0}}}
		},
		"reason_code": func(c *x509.RevocationList) {
			c.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(12), RevocationTime: cmsTrustNow(), ReasonCode: 1}}
		},
		"entry_extension": func(c *x509.RevocationList) {
			c.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(12), RevocationTime: cmsTrustNow(), ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 3, 99}, Value: []byte{5, 0}}}}}
		},
		"duplicate_serial": func(c *x509.RevocationList) {
			e := x509.RevocationListEntry{SerialNumber: big.NewInt(12), RevocationTime: cmsTrustNow()}
			c.RevokedCertificateEntries = []x509.RevocationListEntry{e, e}
		},
		"future_revocation": func(c *x509.RevocationList) {
			c.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(12), RevocationTime: cmsTrustNow().Add(time.Minute)}}
		},
		"sha384": func(c *x509.RevocationList) { c.SignatureAlgorithm = x509.SHA384WithRSA },
		"pss":    func(c *x509.RevocationList) { c.SignatureAlgorithm = x509.SHA256WithRSAPSS },
	} {
		t.Run(name, func(t *testing.T) {
			template := base()
			mutate(template)
			crl := makeCRL(template)
			if err := validateRPKICRLProfile(crl); err == nil {
				t.Fatal("out-of-profile CRL accepted")
			}
			// An unrelated CRL entry must not make an invalid CRL usable for this leaf.
			if got, err := verifyRPKICertificatePath(f.certs, f.certs[2], []*x509.RevocationList{crl, f.crls[1]}, cmsTrustNow()); err == nil || got != nil {
				t.Fatal("path accepted an invalid CRL profile")
			}
		})
	}
}
