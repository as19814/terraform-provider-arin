package arin

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func manifestCMSFixture(t *testing.T) (cmsTestSignedData, *rsa.PrivateKey) {
	t.Helper()
	sd, _ := cmsTestFixture(t)
	f := resourceCertificateFixture(t)
	ee := signManifestEE(t, manifestEETemplate(t, f), f)
	key := f.keys[0]
	sd.Certs = asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: ee.Raw}
	sd.Signers[0].SID = asn1.RawValue{Class: 2, Tag: 0, Bytes: ee.SubjectKeyId}
	sd.Encap.Type = cmsManifestOID
	sd.Encap.Content = cmsTestDER(t, manifestContentFixture())
	digest := sha256.Sum256(sd.Encap.Content)
	sd.Signers[0].Attrs = []cmsTestAttribute{
		{cmsContentTypeOID, []asn1.RawValue{cmsTestValue(t, cmsManifestOID)}},
		{cmsDigestOID, []asn1.RawValue{cmsTestValue(t, digest[:])}},
	}
	return sd, key
}

func manifestCMSEncode(t testing.TB, sd cmsTestSignedData, key *rsa.PrivateKey, keepCRL bool) []byte {
	t.Helper()
	der := cmsTestEncode(t, sd, key)
	if keepCRL {
		return der
	}
	// Rebuild the SignedData sequence without the CRL field; signedAttrs and
	// eContent are unchanged, so the original signature remains valid.
	root, _ := cmsRaw(der)
	outer, _ := cmsChildren(root, 0, 16, 2, false)
	explicit, _ := cmsChildren(outer[1], 2, 0, 1, false)
	fields, _ := cmsChildren(explicit[0], 0, 16, 6, false)
	var body []byte
	for i, f := range fields {
		if i != 4 {
			body = append(body, f.FullBytes...)
		}
	}
	signed := cmsTestDER(t, asn1.RawValue{Tag: 16, IsCompound: true, Bytes: body})
	return cmsTestDER(t, cmsTestOuter{cmsSignedDataOID, asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: signed}})
}

func TestRPKIManifestCMS(t *testing.T) {
	sd, key := manifestCMSFixture(t)
	der := manifestCMSEncode(t, sd, key, false)
	got, err := decodeRPKIManifest(der)
	if err != nil {
		t.Fatal(err)
	}
	if got.Signer.SerialNumber.Int64() != 3 || got.Content.Number.Sign() != 0 || len(got.Content.Files) != 1 {
		t.Fatal("decoded content mismatch")
	}
	if _, err := decodeRPKICMS(der); err == nil {
		t.Fatal("manifest accepted as provisioning message")
	}
	provisioning, provisioningKey := cmsTestFixture(t)
	if _, err := decodeRPKIManifest(cmsTestEncode(t, provisioning, provisioningKey)); err == nil {
		t.Fatal("provisioning message accepted as manifest")
	}
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("OpenSSL not installed")
	}
	path := filepath.Join(t.TempDir(), "manifest.der")
	if err := os.WriteFile(path, der, 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(openssl, "cms", "-verify", "-binary", "-inform", "DER", "-in", path, "-noverify").Output()
	if err != nil || !bytes.Equal(output, sd.Encap.Content) {
		t.Fatalf("OpenSSL signature verification failed: %v", err)
	}
}

func TestRPKIManifestCMSAttributes(t *testing.T) {
	base, key := manifestCMSFixture(t)
	for _, mode := range []string{"no_time", "signing_time", "binary_time", "both", "different_times", "legacy_algorithm", "embedded_crl", "extra_certificate", "missing_digest", "duplicate_attribute", "wrong_type", "wrong_signed_type", "wrong_sid", "changed_content", "bad_content", "extra_attribute"} {
		t.Run(mode, func(t *testing.T) {
			var sd cmsTestSignedData
			if _, err := asn1.Unmarshal(cmsTestDER(t, base), &sd); err != nil {
				t.Fatal(err)
			}
			valid := true
			keepCRL := false
			addTime := func(oid asn1.ObjectIdentifier, v any) {
				sd.Signers[0].Attrs = append(sd.Signers[0].Attrs, cmsTestAttribute{oid, []asn1.RawValue{cmsTestValue(t, v)}})
			}
			switch mode {
			case "no_time":
			case "signing_time":
				addTime(cmsSigningTimeOID, cmsTrustNow())
			case "binary_time":
				addTime(cmsBinaryTimeOID, cmsTrustNow().Unix())
			case "both":
				addTime(cmsSigningTimeOID, cmsTrustNow())
				addTime(cmsBinaryTimeOID, cmsTrustNow().Unix())
			case "different_times":
				addTime(cmsSigningTimeOID, cmsTrustNow())
				addTime(cmsBinaryTimeOID, cmsTrustNow().Add(time.Hour).Unix())
			case "legacy_algorithm":
				sd.Signers[0].Algorithm.Algorithm = cmsSHA256RSAOID
			case "embedded_crl":
				keepCRL = true
				valid = false
			case "extra_certificate":
				sd.Certs.FullBytes = nil
				sd.Certs.Bytes = append(bytes.Clone(sd.Certs.Bytes), sd.Certs.Bytes...)
				valid = false
			case "missing_digest":
				sd.Signers[0].Attrs = sd.Signers[0].Attrs[:1]
				valid = false
			case "duplicate_attribute":
				sd.Signers[0].Attrs = append(sd.Signers[0].Attrs, sd.Signers[0].Attrs[0])
				valid = false
			case "wrong_type":
				sd.Encap.Type = cmsXMLOID
				valid = false
			case "wrong_signed_type":
				sd.Signers[0].Attrs[0].Values = []asn1.RawValue{cmsTestValue(t, cmsXMLOID)}
				valid = false
			case "wrong_sid":
				sd.Signers[0].SID = asn1.RawValue{Class: 2, Tag: 0, Bytes: []byte{9}}
				valid = false
			case "changed_content":
				sd.Encap.Content[0] ^= 1
				valid = false
			case "bad_content":
				sd.Encap.Content = []byte{0x30, 0}
				h := sha256.Sum256(sd.Encap.Content)
				sd.Signers[0].Attrs[1].Values = []asn1.RawValue{cmsTestValue(t, h[:])}
				valid = false
			case "extra_attribute":
				addTime(cmsRSAOID, 1)
				valid = false
			}
			der := manifestCMSEncode(t, sd, key, keepCRL)
			_, err := decodeRPKIManifest(der)
			if (err == nil) != valid {
				t.Fatalf("valid=%v error=%v", valid, err)
			}
		})
	}
}

func TestRPKIManifestCMSTampering(t *testing.T) {
	sd, key := manifestCMSFixture(t)
	der := manifestCMSEncode(t, sd, key, false)
	for _, mode := range []string{"signature", "trailing", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			changed := bytes.Clone(der)
			switch mode {
			case "signature":
				changed[len(changed)-1] ^= 1
			case "trailing":
				changed = append(changed, 0)
			case "truncated":
				changed = changed[:len(changed)-1]
			}
			if _, err := decodeRPKIManifest(changed); err == nil {
				t.Fatal("accepted tampered manifest")
			}
		})
	}
}
