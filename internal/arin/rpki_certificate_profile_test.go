package arin

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"
)

func TestRPKICAProfile(t *testing.T) {
	f := resourceCertificateFixture(t)
	for _, c := range f.certs {
		if err := validateRPKICAProfile(c); err != nil {
			t.Fatal(err)
		}
	}
	replace := func(c *x509.Certificate, oid string, fn func(*pkix.Extension)) {
		for i := range c.ExtraExtensions {
			if c.ExtraExtensions[i].Id.String() == oid {
				fn(&c.ExtraExtensions[i])
				return
			}
		}
		t.Fatal("missing test extension")
	}
	remove := func(c *x509.Certificate, oid string) {
		for i, e := range c.ExtraExtensions {
			if e.Id.String() == oid {
				c.ExtraExtensions = append(c.ExtraExtensions[:i], c.ExtraExtensions[i+1:]...)
				return
			}
		}
		t.Fatal("missing test extension")
	}
	for name, mutate := range map[string]func(*x509.Certificate){
		"unknown_noncritical": func(c *x509.Certificate) {
			c.ExtraExtensions = append(c.ExtraExtensions, pkix.Extension{Id: asn1.ObjectIdentifier{1, 2, 3, 99}, Value: []byte{5, 0}})
		},
		"extended_key_usage": func(c *x509.Certificate) {
			c.ExtraExtensions = append(c.ExtraExtensions, pkix.Extension{Id: oidRPKIExtendedKeyUsage, Value: resourceTestDER(t, []asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 1}})})
		},
		"sha384": func(c *x509.Certificate) { c.SignatureAlgorithm = x509.SHA384WithRSA },
		"wrong_ski": func(c *x509.Certificate) {
			replace(c, "2.5.29.14", func(e *pkix.Extension) { e.Value = resourceTestDER(t, make([]byte, 20)) })
		},
		"critical_ski":       func(c *x509.Certificate) { replace(c, "2.5.29.14", func(e *pkix.Extension) { e.Critical = true }) },
		"noncritical_policy": func(c *x509.Certificate) { replace(c, "2.5.29.32", func(e *pkix.Extension) { e.Critical = false }) },
		"wrong_policy": func(c *x509.Certificate) {
			replace(c, "2.5.29.32", func(e *pkix.Extension) {
				e.Value = resourceTestDER(t, []struct{ ID asn1.ObjectIdentifier }{{asn1.ObjectIdentifier{1, 2, 3, 4}}})
			})
		},
		"missing_policy": func(c *x509.Certificate) { remove(c, "2.5.29.32"); c.PolicyIdentifiers = nil; c.Policies = nil },
		"missing_sia":    func(c *x509.Certificate) { remove(c, "1.3.6.1.5.5.7.1.11") },
		"missing_aia":    func(c *x509.Certificate) { remove(c, "1.3.6.1.5.5.7.1.1"); c.IssuingCertificateURL = nil },
		"missing_crldp":  func(c *x509.Certificate) { remove(c, "2.5.29.31"); c.CRLDistributionPoints = nil },
		"path_length": func(c *x509.Certificate) {
			replace(c, "2.5.29.19", func(e *pkix.Extension) { e.Value = []byte{0x30, 6, 1, 1, 0xff, 2, 1, 0} })
		},
		"extra_key_usage": func(c *x509.Certificate) {
			replace(c, "2.5.29.15", func(e *pkix.Extension) { e.Value = []byte{3, 2, 1, 0x86} })
		},
	} {
		t.Run(name, func(t *testing.T) {
			template := *f.certs[0]
			template.ExtraExtensions = append([]pkix.Extension(nil), template.Extensions...)
			mutate(&template)
			raw, err := x509.CreateCertificate(rand.Reader, &template, f.certs[1], &f.keys[0].PublicKey, f.keys[1])
			if err != nil {
				t.Fatal(err)
			}
			cert, err := x509.ParseCertificate(raw)
			if err != nil {
				if name != "critical_ski" {
					t.Fatal(err)
				}
				if _, pathErr := verifyRPKICertificatePath([]*x509.Certificate{{Raw: raw}, f.certs[1], f.certs[2]}, f.certs[2], f.crls, cmsTrustNow()); pathErr == nil {
					t.Fatal("path bypassed parser rejection")
				}
				return
			}
			if err := cert.CheckSignatureFrom(f.certs[1]); err != nil {
				t.Fatal(err)
			}
			if err := validateRPKICAProfile(cert); err == nil {
				t.Fatal("signed out-of-profile certificate accepted")
			}
			if _, err := verifyRPKICertificatePath([]*x509.Certificate{cert, f.certs[1], f.certs[2]}, f.certs[2], f.crls, cmsTrustNow()); err == nil {
				t.Fatal("path accepted invalid certificate profile")
			}
		})
	}
}

func TestRPKIPolicyQualifier(t *testing.T) {
	type qualifier struct {
		ID    asn1.ObjectIdentifier
		Value asn1.RawValue
	}
	type policy struct {
		ID         asn1.ObjectIdentifier
		Qualifiers []qualifier `asn1:"optional"`
	}
	id := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 14, 2}
	cps := qualifier{asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 2, 1}, asn1.RawValue{Tag: 22, Bytes: []byte("https://example.net/cps")}}
	for _, q := range [][]qualifier{nil, {cps}} {
		if !validRPKIPolicies(resourceTestDER(t, []policy{{id, q}})) {
			t.Fatal("valid optional CPS qualifier rejected")
		}
	}
	wrong := cps
	wrong.ID = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 2, 2}
	utf8 := cps
	utf8.Value.Tag = 12
	relative := cps
	relative.Value = asn1.RawValue{Tag: 22, Bytes: []byte("/cps")}
	for _, q := range [][]qualifier{{cps, cps}, {wrong}, {utf8}, {relative}} {
		if validRPKIPolicies(resourceTestDER(t, []policy{{id, q}})) {
			t.Fatal("invalid policy qualifier accepted")
		}
	}
}

func TestRPKIIssuerReferences(t *testing.T) {
	aia := func(method asn1.ObjectIdentifier, uri string) []byte {
		return resourceTestDER(t, []rpkiAccessDescription{{method, asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte(uri)}}})
	}
	method := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 2}
	if !validRPKIAIA(aia(method, "rsync://repo.example/module/issuer.cer")) {
		t.Fatal("valid AIA rejected")
	}
	for _, der := range [][]byte{aia(method, "https://repo.example/issuer.cer"), aia(oidRPKIManifest, "rsync://repo.example/module/x.cer"), aia(method, "rsync://repo.example/module/")} {
		if validRPKIAIA(der) {
			t.Fatal("invalid AIA accepted")
		}
	}
	f := resourceCertificateFixture(t)
	for _, e := range f.certs[0].Extensions {
		if e.Id.String() == "2.5.29.31" && !validRPKICRLDP(e.Value) {
			t.Fatal("valid CRLDP rejected")
		}
	}
	if validRPKICRLDP([]byte{0x30, 0}) {
		t.Fatal("empty CRLDP accepted")
	}
}
