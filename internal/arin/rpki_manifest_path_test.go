package arin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
)

func manifestPathFixture(t *testing.T, mode string) (resourcePathFixture, []rpkiPathPublication) {
	t.Helper()
	f := resourceCertificateFixture(t)
	const directory = "rsync://repo.example/module/child/"
	for i := 1; i >= 0; i-- {
		c := *f.certs[i]
		c.ExtraExtensions = nil
		for _, e := range c.Extensions {
			if e.Id.String() != "2.5.29.31" {
				c.ExtraExtensions = append(c.ExtraExtensions, e)
			}
		}
		c.CRLDistributionPoints = []string{directory + "issuer.crl"}
		if mode == "wrong_crldp" && i == 0 {
			c.CRLDistributionPoints = []string{directory + "other.crl"}
		}
		if mode == "overclaim" && i == 0 {
			for j, e := range c.ExtraExtensions {
				if e.Id.Equal(oidRPKIASResources) {
					c.ExtraExtensions[j] = resourceTestAS(t, cmsTestDER(t, []int{65000}))
				}
			}
		}
		der, err := x509.CreateCertificate(rand.Reader, &c, f.certs[i+1], &f.keys[i].PublicKey, f.keys[i+1])
		if err != nil {
			t.Fatal(err)
		}
		f.certs[i], err = x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
	}
	base, _ := cmsTestFixture(t)
	out := make([]rpkiPathPublication, 2)
	for i := 0; i < 2; i++ {
		issuer, key := f.certs[i+1], f.keys[i+1]
		c := manifestEETemplate(t, f)
		c.SerialNumber = big.NewInt(int64(100 + i))
		c.AuthorityKeyId = issuer.SubjectKeyId
		c.ExtraExtensions = nil
		for _, e := range manifestEETemplate(t, f).ExtraExtensions {
			switch e.Id.String() {
			case "2.5.29.35", "2.5.29.31", "1.3.6.1.5.5.7.1.11":
				continue
			}
			if e.Id.Equal(oidRPKIASResources) {
				e = resourceTestAS(t, []byte{5, 0})
			}
			c.ExtraExtensions = append(c.ExtraExtensions, e)
		}
		c.CRLDistributionPoints = []string{directory + "issuer.crl"}
		c.ExtraExtensions = append(c.ExtraExtensions, pkix.Extension{Id: oidRPKISIA, Value: cmsTestDER(t, []rpkiAccessDescription{{Method: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 11}, Location: asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte(directory + "manifest.mft")}}})})
		raw, err := x509.CreateCertificate(rand.Reader, &c, issuer, &f.keys[0].PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		ee, err := x509.ParseCertificate(raw)
		if err != nil {
			t.Fatal(err)
		}
		var revoked *big.Int
		if mode == "revoked_child" && i == 0 {
			revoked = f.certs[i].SerialNumber
		}
		crl := resourcePathCRL(t, issuer, key, revoked)
		files := map[string][]byte{"child.cer": f.certs[i].Raw, "issuer.crl": crl.Raw}
		content := manifestContentFixture()
		content.Files = nil
		for name, data := range files {
			if mode == "unlisted_child" && i == 0 && name == "child.cer" {
				continue
			}
			hash := sha256.Sum256(data)
			content.Files = append(content.Files, manifestTestFile{name, asn1.BitString{Bytes: hash[:], BitLength: 256}})
		}
		sd := base
		sd.Signers = append([]cmsTestSigner(nil), base.Signers...)
		sd.Certs = asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: ee.Raw}
		sd.Encap = cmsTestEncap{Type: cmsManifestOID, Content: cmsTestDER(t, content)}
		hash := sha256.Sum256(sd.Encap.Content)
		sd.Signers[0].SID = asn1.RawValue{Class: 2, Tag: 0, Bytes: ee.SubjectKeyId}
		sd.Signers[0].Attrs = []cmsTestAttribute{{cmsContentTypeOID, []asn1.RawValue{cmsTestValue(t, cmsManifestOID)}}, {cmsDigestOID, []asn1.RawValue{cmsTestValue(t, hash[:])}}}
		out[i] = rpkiPathPublication{ManifestDER: manifestCMSEncode(t, sd, f.keys[0], false), ManifestURI: directory + "manifest.mft", ChildURI: directory + "child.cer", Files: files}
	}
	return f, out
}

func TestRPKIManifestPath(t *testing.T) {
	for _, mode := range []string{"valid", "wrong_anchor", "missing_manifest", "swapped_manifests", "wrong_child_uri", "changed_child", "wrong_crldp", "revoked_child", "unlisted_child", "overclaim", "forged_parsed_fields"} {
		t.Run(mode, func(t *testing.T) {
			f, pubs := manifestPathFixture(t, mode)
			anchor := f.certs[2]
			switch mode {
			case "wrong_anchor":
				anchor = f.certs[1]
			case "missing_manifest":
				pubs = pubs[:1]
			case "swapped_manifests":
				pubs[0], pubs[1] = pubs[1], pubs[0]
			case "wrong_child_uri":
				pubs[0].ChildURI = "rsync://elsewhere.example/module/child.cer"
			case "changed_child":
				pubs[0].Files["child.cer"] = f.certs[1].Raw
			case "forged_parsed_fields":
				f.certs[0].CRLDistributionPoints = nil
				f.certs[0].IsCA = false
			}
			got, err := verifyRPKIManifestPath(f.certs, anchor, pubs, cmsTrustNow())
			valid := mode == "valid" || mode == "forged_parsed_fields"
			if (err == nil) != valid {
				t.Fatalf("valid=%v err=%v", valid, err)
			}
			if valid && (got.ASN == nil || got.ASN.Inherit || len(got.ASN.Ranges) != 1 || got.ASN.Ranges[0] != (rpkiASRange{64500, 64510})) {
				t.Fatal("resolved resources mismatch")
			}
		})
	}
}
