package arin

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"
	"time"
)

func TestRPKIManifestIssuer(t *testing.T) {
	f := resourceCertificateFixture(t)
	base, key := manifestCMSFixture(t)
	key = f.keys[0]
	const uri = "rsync://repo.example/module/child/manifest.mft"
	const crlURI = "rsync://repo.example/module/child/issuer.crl"
	for _, mode := range []string{"valid", "unlisted_crl", "missing_file", "changed_file", "wrong_issuer", "wrong_uri", "wrong_ee_location", "wrong_crldp", "expired_ee", "future_ee", "revoked_ee", "stale_crl", "wrong_crl_issuer", "missing_crl", "multiple_crls", "inherited_missing_family", "mismatched_validity_intervals", "manifest_self_reference", "oversized_file"} {
		t.Run(mode, func(t *testing.T) {
			template := manifestEETemplate(t, f)
			var exts []pkix.Extension
			for _, ext := range template.ExtraExtensions {
				if !ext.Id.Equal(oidRPKISIA) && ext.Id.String() != "2.5.29.31" {
					exts = append(exts, ext)
				}
			}
			location := uri
			if mode == "wrong_ee_location" {
				location = "rsync://repo.example/module/child/other.mft"
			}
			template.ExtraExtensions = append(exts, pkix.Extension{Id: oidRPKISIA, Value: cmsTestDER(t, []rpkiAccessDescription{{Method: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 11}, Location: asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte(location)}}})})
			template.CRLDistributionPoints = []string{crlURI}
			switch mode {
			case "wrong_crldp":
				template.CRLDistributionPoints = []string{"rsync://repo.example/module/child/other.crl"}
			case "expired_ee":
				template.NotAfter = cmsTrustNow().Add(-time.Second)
			case "future_ee":
				template.NotBefore = cmsTrustNow().Add(time.Second)
			case "inherited_missing_family":
				template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{Id: oidRPKIIPResources, Critical: true, Value: cmsTestDER(t, []struct {
					AFI    []byte
					Choice asn1.RawValue
				}{{[]byte{0, 1}, asn1.NullRawValue}})})
			}
			ee := signManifestEE(t, template, f)
			crl := resourcePathCRL(t, f.certs[1], f.keys[1], nil)
			if mode == "revoked_ee" {
				crl = resourcePathCRL(t, f.certs[1], f.keys[1], ee.SerialNumber)
			}
			if mode == "wrong_crl_issuer" {
				crl = resourcePathCRL(t, f.certs[2], f.keys[2], nil)
			}
			files := map[string][]byte{"issuer.crl": crl.Raw, "child.cer": f.certs[0].Raw}
			if mode == "missing_crl" {
				delete(files, "issuer.crl")
			}
			if mode == "multiple_crls" {
				files["other.crl"] = crl.Raw
			}
			if mode == "manifest_self_reference" {
				files["manifest.mft"] = []byte("not a possible self digest")
			}
			if mode == "oversized_file" {
				files["child.cer"] = make([]byte, (4<<20)+1)
			}
			content := manifestContentFixture()
			content.Files = nil
			for name, data := range files {
				h := sha256.Sum256(data)
				content.Files = append(content.Files, manifestTestFile{name, asn1.BitString{Bytes: h[:], BitLength: 256}})
			}
			now := cmsTrustNow()
			if mode == "stale_crl" {
				// The manifest and EE remain current while the CRL expires first.
				now = now.Add(time.Hour)
				template.NotAfter = now.Add(time.Hour)
				ee = signManifestEE(t, template, f)
				content.NextUpdate = now.Add(time.Hour)
			}
			if mode == "mismatched_validity_intervals" {
				content.NextUpdate = cmsTrustNow().Add(30 * time.Minute)
			}
			sd := base
			sd.Signers = append([]cmsTestSigner(nil), base.Signers...)
			sd.Certs = asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: ee.Raw}
			sd.Signers[0].SID = asn1.RawValue{Class: 2, Tag: 0, Bytes: ee.SubjectKeyId}
			sd.Encap.Content = cmsTestDER(t, content)
			digest := sha256.Sum256(sd.Encap.Content)
			sd.Signers[0].Attrs = []cmsTestAttribute{{cmsContentTypeOID, []asn1.RawValue{cmsTestValue(t, cmsManifestOID)}}, {cmsDigestOID, []asn1.RawValue{cmsTestValue(t, digest[:])}}}
			der := manifestCMSEncode(t, sd, key, false)
			issuerDER := f.certs[1].Raw
			manifestURI := uri
			switch mode {
			case "wrong_issuer":
				issuerDER = f.certs[2].Raw
			case "wrong_uri":
				manifestURI = "rsync://repo.example/module/child/other.mft"
			case "missing_file":
				delete(files, "child.cer")
			case "changed_file":
				files["child.cer"] = []byte("changed")
			case "unlisted_crl":
				files["unlisted.crl"] = resourcePathCRL(t, f.certs[1], f.keys[1], ee.SerialNumber).Raw
			}
			got, err := checkRPKIManifestForIssuer(der, issuerDER, manifestURI, files, now)
			valid := mode == "valid" || mode == "unlisted_crl" || mode == "mismatched_validity_intervals"
			if (err == nil) != valid {
				t.Fatalf("valid=%v err=%v", valid, err)
			}
			if valid {
				if got.CRLURI != crlURI || !bytes.Equal(got.CRL.Raw, crl.Raw) {
					t.Fatal("wrong selected CRL")
				}
				original := bytes.Clone(got.CRL.Raw)
				files["issuer.crl"][0] ^= 1
				if !bytes.Equal(got.CRL.Raw, original) {
					t.Fatal("selected CRL aliases input")
				}
			}
		})
	}
}

func TestRPKIManifestIssuerRejectsInvalidInputs(t *testing.T) {
	for _, input := range []struct {
		der, issuer []byte
		uri         string
		now         time.Time
	}{
		{nil, nil, "", time.Time{}},
		{[]byte{1}, []byte{1}, "rsync://repo.example/module/a.mft", cmsTrustNow()},
		{make([]byte, (4<<20)+1), []byte{1}, "rsync://repo.example/module/a.mft", cmsTrustNow()},
		{[]byte{1}, make([]byte, 512001), "rsync://repo.example/module/a.mft", cmsTrustNow()},
	} {
		if _, err := checkRPKIManifestForIssuer(input.der, input.issuer, input.uri, nil, input.now); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
}
