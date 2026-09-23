package arin

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"
)

func manifestEETemplate(t *testing.T, f resourcePathFixture) x509.Certificate {
	t.Helper()
	c := *f.certs[0]
	c.IsCA = false
	c.BasicConstraintsValid = false
	c.KeyUsage = x509.KeyUsageDigitalSignature
	c.ExtraExtensions = nil
	for _, ext := range c.Extensions {
		switch ext.Id.String() {
		case "2.5.29.19", "2.5.29.15", "1.3.6.1.5.5.7.1.11":
			continue
		}
		c.ExtraExtensions = append(c.ExtraExtensions, ext)
	}
	c.ExtraExtensions = append(c.ExtraExtensions, pkix.Extension{Id: oidRPKISIA, Value: cmsTestDER(t, []rpkiAccessDescription{{Method: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 11}, Location: asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte("rsync://repo.example/module/issuer.mft")}}})})
	return c
}

func signManifestEE(t *testing.T, c x509.Certificate, f resourcePathFixture) *x509.Certificate {
	t.Helper()
	raw, err := x509.CreateCertificate(rand.Reader, &c, f.certs[1], &f.keys[0].PublicKey, f.keys[1])
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := parsed.CheckSignatureFrom(f.certs[1]); err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestRPKIManifestEEProfile(t *testing.T) {
	f := resourceCertificateFixture(t)
	baseCMS, _ := manifestCMSFixture(t)
	good := signManifestEE(t, manifestEETemplate(t, f), f)
	if err := validateRPKIManifestEEProfile(good); err != nil {
		t.Fatal(err)
	}
	if err := validateRPKICAProfile(good); err == nil {
		t.Fatal("EE accepted as CA")
	}
	c := manifestEETemplate(t, f)
	c.ExtraExtensions = append(c.ExtraExtensions, pkix.Extension{Id: oidRPKIIPResources, Critical: true, Value: cmsTestDER(t, []struct {
		AFI    []byte
		Choice asn1.RawValue
	}{{[]byte{0, 1}, asn1.NullRawValue}, {[]byte{0, 2}, asn1.NullRawValue}})})
	if err := validateRPKIManifestEEProfile(signManifestEE(t, c, f)); err != nil {
		t.Fatalf("inherited IP families rejected: %v", err)
	}
	if err := validateRPKIManifestEEProfile(f.certs[0]); err == nil {
		t.Fatal("CA accepted as EE")
	}
	for _, mode := range []string{"basic_constraints_false", "ca_usage", "extra_usage", "eku", "explicit_as", "explicit_ip", "no_resources", "missing_sia", "ca_sia", "critical_sia", "wrong_ski", "missing_aki", "missing_aia", "missing_crldp"} {
		t.Run(mode, func(t *testing.T) {
			c := manifestEETemplate(t, f)
			replace := func(oid asn1.ObjectIdentifier, ext *pkix.Extension) {
				t.Helper()
				for i, e := range c.ExtraExtensions {
					if e.Id.Equal(oid) {
						if ext == nil {
							c.ExtraExtensions = append(c.ExtraExtensions[:i], c.ExtraExtensions[i+1:]...)
						} else {
							c.ExtraExtensions[i] = *ext
						}
						return
					}
				}
				t.Fatal("missing fixture extension")
			}
			switch mode {
			case "basic_constraints_false":
				c.BasicConstraintsValid = true
			case "ca_usage":
				c.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
			case "extra_usage":
				c.KeyUsage |= x509.KeyUsageKeyEncipherment
			case "eku":
				c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
			case "explicit_as":
				ext := resourceTestAS(t, cmsTestDER(t, []int{64500}))
				replace(oidRPKIASResources, &ext)
			case "explicit_ip":
				c.ExtraExtensions = append(c.ExtraExtensions, pkix.Extension{Id: oidRPKIIPResources, Critical: true, Value: cmsTestDER(t, []struct {
					AFI    []byte
					Choice asn1.RawValue
				}{{[]byte{0, 1}, cmsTestValue(t, []asn1.BitString{{Bytes: []byte{192, 0, 2}, BitLength: 24}})}})})
			case "no_resources":
				replace(oidRPKIASResources, nil)
			case "missing_sia":
				replace(oidRPKISIA, nil)
			case "ca_sia":
				ext := rpkiSIATestExtension(t)
				replace(oidRPKISIA, &ext)
			case "critical_sia":
				for i := range c.ExtraExtensions {
					if c.ExtraExtensions[i].Id.Equal(oidRPKISIA) {
						c.ExtraExtensions[i].Critical = true
					}
				}
			case "wrong_ski":
				ext := pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 14}, Value: cmsTestDER(t, make([]byte, 20))}
				replace(ext.Id, &ext)
			case "missing_aki":
				ext := pkix.Extension{Id: asn1.ObjectIdentifier{2, 5, 29, 35}, Value: []byte{0x30, 0}}
				replace(ext.Id, &ext)
			case "missing_aia":
				replace(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 1}, nil)
				c.IssuingCertificateURL = nil
			case "missing_crldp":
				replace(asn1.ObjectIdentifier{2, 5, 29, 31}, nil)
				c.CRLDistributionPoints = nil
			}
			parsed := signManifestEE(t, c, f)
			if err := validateRPKIManifestEEProfile(parsed); err == nil {
				t.Fatal("invalid EE profile accepted")
			}
			sd := baseCMS
			sd.Certs = asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: parsed.Raw}
			sd.Signers = append([]cmsTestSigner(nil), baseCMS.Signers...)
			sd.Signers[0].SID = asn1.RawValue{Class: 2, Tag: 0, Bytes: parsed.SubjectKeyId}
			if _, err := decodeRPKIManifest(manifestCMSEncode(t, sd, f.keys[0], false)); err == nil {
				t.Fatal("manifest decoder accepted out-of-profile EE")
			}
		})
	}
}

func TestRPKIEESIA(t *testing.T) {
	method := asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 11}
	for _, tt := range []struct {
		name  string
		uris  []string
		valid bool
	}{
		{"rsync", []string{"rsync://repo.example/module/a.mft"}, true},
		{"alternates", []string{"https://repo.example/a.mft", "rsync://repo.example/module/a.mft"}, true},
		{"missing", nil, false},
		{"http_only", []string{"https://repo.example/a.mft"}, false},
		{"directory", []string{"rsync://repo.example/module/"}, false},
		{"fragment", []string{"rsync://repo.example/module/a.mft#x"}, false},
		{"userinfo", []string{"rsync://user@repo.example/module/a.mft"}, false},
		{"relative", []string{"a.mft"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var descriptions []rpkiAccessDescription
			for _, uri := range tt.uris {
				descriptions = append(descriptions, rpkiAccessDescription{method, asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte(uri)}})
			}
			ext := pkix.Extension{Id: oidRPKISIA, Value: cmsTestDER(t, descriptions)}
			got, err := rpkiEESIA([]pkix.Extension{ext})
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v err=%v", tt.valid, err)
			}
			if err == nil {
				for i := range got {
					if got[i] != tt.uris[i] {
						t.Fatal("location changed")
					}
				}
			}
		})
	}
}
