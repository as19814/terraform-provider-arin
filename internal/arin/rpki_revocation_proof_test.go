package arin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

func rewriteRevocationManifestFiles(t *testing.T, p rpkiPathPublication, f resourcePathFixture, expired ...bool) rpkiPathPublication {
	t.Helper()
	m, err := decodeRPKIManifest(p.ManifestDER)
	if err != nil {
		t.Fatal(err)
	}
	content := manifestContentFixture()
	if len(expired) > 0 && expired[0] {
		content.ThisUpdate = cmsTrustNow().Add(-2 * time.Minute)
		content.NextUpdate = cmsTrustNow().Add(-time.Minute)
	}
	content.Files = nil
	for name, data := range p.Files {
		hash := sha256.Sum256(data)
		content.Files = append(content.Files, manifestTestFile{name, asn1.BitString{Bytes: hash[:], BitLength: 256}})
	}
	base, _ := cmsTestFixture(t)
	base.Encap = cmsTestEncap{Type: cmsManifestOID, Content: cmsTestDER(t, content)}
	base.Certs = asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: m.Signer.Raw}
	base.Signers[0].SID = asn1.RawValue{Class: 2, Tag: 0, Bytes: m.Signer.SubjectKeyId}
	hash := sha256.Sum256(base.Encap.Content)
	base.Signers[0].Attrs = []cmsTestAttribute{{cmsContentTypeOID, []asn1.RawValue{cmsTestValue(t, cmsManifestOID)}}, {cmsDigestOID, []asn1.RawValue{cmsTestValue(t, hash[:])}}}
	p.ManifestDER = manifestCMSEncode(t, base, f.keys[0], false)
	return p
}
func TestRPKIRevocationProof(t *testing.T) {
	for _, mode := range []string{"valid", "not_revoked", "still_published", "renamed_certificate", "reissued_key", "future_revocation", "wrong_key", "wrong_anchor", "wrong_issuer", "wrong_crldp", "overclaim", "changed_crl", "stale_manifest", "missing_upstream", "malformed_certificate"} {
		t.Run(mode, func(t *testing.T) {
			fixture := "revoked_child"
			if mode == "not_revoked" {
				fixture = "valid"
			}
			if mode == "wrong_crldp" || mode == "overclaim" {
				fixture = mode
			}
			f, pubs := manifestPathFixture(t, fixture)
			p := pubs[0]
			if mode != "still_published" {
				delete(p.Files, "child.cer")
			}
			if mode == "renamed_certificate" {
				p.Files["renamed.cer"] = f.certs[0].Raw
			}
			if mode == "overclaim" || mode == "wrong_crldp" {
				p.Files["issuer.crl"] = resourcePathCRL(t, f.certs[1], f.keys[1], f.certs[0].SerialNumber).Raw
			}
			if mode == "reissued_key" {
				cert := *f.certs[0]
				cert.SerialNumber = big.NewInt(999)
				cert.ExtraExtensions = cert.Extensions
				der, err := x509.CreateCertificate(rand.Reader, &cert, f.certs[1], &f.keys[0].PublicKey, f.keys[1])
				if err != nil {
					t.Fatal(err)
				}
				p.Files["renewed.cer"] = der
			}
			if mode == "future_revocation" {
				der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{Number: big.NewInt(2), ThisUpdate: cmsTrustNow().Add(-time.Minute), NextUpdate: cmsTrustNow().Add(time.Hour), RevokedCertificateEntries: []x509.RevocationListEntry{{SerialNumber: f.certs[0].SerialNumber, RevocationTime: cmsTrustNow().Add(time.Minute)}}}, f.certs[1], f.keys[1])
				if err != nil {
					t.Fatal(err)
				}
				p.Files["issuer.crl"] = der
			}
			p = rewriteRevocationManifestFiles(t, p, f)
			ski, err := rpkiPublicKeyIdentifier(f.certs[0].RawSubjectPublicKeyInfo)
			if err != nil {
				t.Fatal(err)
			}
			anchor := f.certs[2]
			issuers := f.certs[1:]
			upstream := pubs[1:]
			der := f.certs[0].Raw
			switch mode {
			case "wrong_key":
				ski = "AAAAAAAAAAAAAAAAAAAAAAAAAAA"
			case "wrong_anchor":
				anchor = f.certs[1]
			case "wrong_issuer":
				issuers = f.certs[2:]
				upstream = nil
			case "changed_crl":
				p.Files["issuer.crl"] = []byte("tampered")
			case "stale_manifest":
				p = rewriteRevocationManifestFiles(t, p, f, true)
			case "missing_upstream":
				upstream = nil
			case "malformed_certificate":
				der = []byte("invalid")
			}
			proof, err := checkRPKIRevocationProof(der, ski, issuers, anchor, upstream, p, cmsTrustNow())
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode %s: %v", mode, err)
			}
			if err == nil && (proof.CertificateSHA256 != rpkiManifestDigest(der) || proof.IssuerSHA256 != rpkiManifestDigest(f.certs[1].Raw) || proof.CRLSHA256 != rpkiManifestDigest(p.Files["issuer.crl"]) || proof.ManifestSHA256 != rpkiManifestDigest(p.ManifestDER)) {
				t.Fatal("proof did not bind exact evidence")
			}
		})
	}
}
