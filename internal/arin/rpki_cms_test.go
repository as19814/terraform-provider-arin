package arin

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type cmsTestAttribute struct {
	OID    asn1.ObjectIdentifier
	Values []asn1.RawValue `asn1:"set"`
}
type cmsTestSigner struct {
	Version   int
	SID       asn1.RawValue
	Digest    pkix.AlgorithmIdentifier
	Attrs     []cmsTestAttribute `asn1:"tag:0,set"`
	Algorithm pkix.AlgorithmIdentifier
	Signature []byte
}
type cmsTestEncap struct {
	Type    asn1.ObjectIdentifier
	Content []byte `asn1:"explicit,tag:0"`
}
type cmsTestSignedData struct {
	Version int
	Digests []pkix.AlgorithmIdentifier `asn1:"set"`
	Encap   cmsTestEncap
	Certs   asn1.RawValue
	CRLs    asn1.RawValue
	Signers []cmsTestSigner `asn1:"set"`
}
type cmsTestOuter struct {
	Type    asn1.ObjectIdentifier
	Content asn1.RawValue
}

func cmsTestDER(t testing.TB, v any) []byte {
	t.Helper()
	b, e := asn1.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func cmsTestValue(t testing.TB, v any) asn1.RawValue {
	return asn1.RawValue{FullBytes: cmsTestDER(t, v)}
}
func cmsTestFixture(t testing.TB) (cmsTestSignedData, *rsa.PrivateKey) {
	sd, key, _ := cmsTrustFixture(t)
	return sd, key
}
func cmsTrustFixture(t testing.TB) (cmsTestSignedData, *rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, SubjectKeyId: []byte{1, 2, 3}}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	ee := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Test EE"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature, SubjectKeyId: []byte{4, 5, 6}}
	eeDER, err := x509.CreateCertificate(rand.Reader, ee, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	crl, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: now.Add(-time.Minute), NextUpdate: now.Add(time.Hour)}, ca, key)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte(`<message xmlns="http://www.apnic.net/specs/rescerts/up-down/" version="1" sender="parent" recipient="child" type="list_response"/>`)
	digest := sha256.Sum256(content)
	attrs := []cmsTestAttribute{{cmsContentTypeOID, []asn1.RawValue{cmsTestValue(t, cmsXMLOID)}}, {cmsDigestOID, []asn1.RawValue{cmsTestValue(t, digest[:])}}, {cmsSigningTimeOID, []asn1.RawValue{cmsTestValue(t, now)}}}
	signer := cmsTestSigner{Version: 3, SID: asn1.RawValue{Class: 2, Tag: 0, Bytes: ee.SubjectKeyId}, Digest: pkix.AlgorithmIdentifier{Algorithm: cmsSHA256OID}, Attrs: attrs, Algorithm: pkix.AlgorithmIdentifier{Algorithm: cmsRSAOID, Parameters: asn1.NullRawValue}}
	sd := cmsTestSignedData{Version: 3, Digests: []pkix.AlgorithmIdentifier{{Algorithm: cmsSHA256OID}}, Encap: cmsTestEncap{cmsXMLOID, content}, Certs: asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: eeDER}, CRLs: asn1.RawValue{Class: 2, Tag: 1, IsCompound: true, Bytes: crl}, Signers: []cmsTestSigner{signer}}
	return sd, key, ca
}
func cmsTestEncode(t testing.TB, sd cmsTestSignedData, key *rsa.PrivateKey) []byte {
	t.Helper()
	for i := range sd.Signers {
		encoded, err := asn1.MarshalWithParams(sd.Signers[i].Attrs, "set")
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(encoded)
		sd.Signers[i].Signature, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
	}
	return cmsTestDER(t, cmsTestOuter{cmsSignedDataOID, asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: cmsTestDER(t, sd)}})
}
func TestRPKICMSProfile(t *testing.T) {
	sd, key := cmsTestFixture(t)
	der := cmsTestEncode(t, sd, key)
	got, err := decodeRPKICMS(der)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Content, sd.Encap.Content) || len(got.Certificates) != 1 || len(got.CRLs) != 1 || got.Signer.SerialNumber.Int64() != 2 || got.SigningTime.Unix() != time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC).Unix() {
		t.Fatal("profile contents lost")
	}
	// Independent OpenSSL verification checks the DER and CMS signature, without
	// pretending that this fixture's self-created CA is an authenticated peer.
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("OpenSSL not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "message.der")
	if err = os.WriteFile(path, der, 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(openssl, "cms", "-verify", "-binary", "-inform", "DER", "-in", path, "-noverify").Output()
	if err != nil {
		t.Fatalf("OpenSSL rejected generated CMS: %v", err)
	}
	if !bytes.Equal(output, sd.Encap.Content) {
		t.Fatal("OpenSSL payload mismatch")
	}
}
func TestRPKICMSProfileRejectsInvalid(t *testing.T) {
	base, key := cmsTestFixture(t)
	for _, mode := range []string{"version", "signer_version", "content_type", "missing_crl", "missing_certificate", "wrong_sid", "wrong_digest", "duplicate_attribute", "extra_attribute", "missing_time", "time_disagreement", "multiple_signers", "weak_algorithm", "wrong_signature_algorithm", "signature", "content", "trailing", "unsigned_attribute", "unsorted_attributes"} {
		t.Run(mode, func(t *testing.T) {
			var sd cmsTestSignedData
			if _, err := asn1.Unmarshal(cmsTestDER(t, base), &sd); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "version":
				sd.Version = 1
			case "signer_version":
				sd.Signers[0].Version = 1
			case "content_type":
				sd.Encap.Type = cmsSignedDataOID
			case "missing_crl":
				sd.CRLs = asn1.RawValue{Class: 2, Tag: 1, IsCompound: true}
			case "missing_certificate":
				sd.Certs = asn1.RawValue{Class: 2, Tag: 0, IsCompound: true}
			case "wrong_sid":
				sd.Signers[0].SID.Bytes = []byte{9}
				sd.Signers[0].SID.FullBytes = nil
			case "wrong_digest":
				for i := range sd.Signers[0].Attrs {
					if sd.Signers[0].Attrs[i].OID.Equal(cmsDigestOID) {
						sd.Signers[0].Attrs[i].Values = []asn1.RawValue{cmsTestValue(t, make([]byte, 32))}
					}
				}
			case "duplicate_attribute":
				sd.Signers[0].Attrs = append(sd.Signers[0].Attrs, sd.Signers[0].Attrs[0])
			case "extra_attribute":
				sd.Signers[0].Attrs = append(sd.Signers[0].Attrs, cmsTestAttribute{cmsRSAOID, []asn1.RawValue{cmsTestValue(t, 1)}})
			case "missing_time":
				filtered := []cmsTestAttribute{}
				for _, a := range sd.Signers[0].Attrs {
					if !a.OID.Equal(cmsSigningTimeOID) {
						filtered = append(filtered, a)
					}
				}
				sd.Signers[0].Attrs = filtered
			case "time_disagreement":
				sd.Signers[0].Attrs = append(sd.Signers[0].Attrs, cmsTestAttribute{cmsBinaryTimeOID, []asn1.RawValue{cmsTestValue(t, int64(1))}})
			case "multiple_signers":
				sd.Signers = append(sd.Signers, sd.Signers[0])
			case "weak_algorithm":
				sd.Digests[0].Algorithm = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
			case "wrong_signature_algorithm":
				sd.Signers[0].Algorithm.Algorithm = cmsSHA256OID
			}
			der := cmsTestEncode(t, sd, key)
			switch mode {
			case "signature":
				der[len(der)-1] ^= 1
			case "content":
				der = bytes.Replace(der, []byte(`sender="parent"`), []byte(`sender="evil!!"`), 1)
			case "trailing":
				der = append(der, 0)
			case "unsigned_attribute":
				// Append a seventh signer field using raw DER so it cannot be silently ignored.
				var si asn1.RawValue
				_, _ = asn1.Unmarshal(cmsTestDER(t, sd.Signers[0]), &si)
				si.FullBytes = nil
				si.Bytes = append(si.Bytes, 0xa1, 0)
				signers := cmsTestDER(t, asn1.RawValue{Class: 0, Tag: 17, IsCompound: true, Bytes: cmsTestDER(t, si)})
				signed := cmsTestDER(t, sd)
				var raw asn1.RawValue
				_, _ = asn1.Unmarshal(signed, &raw)
				old := cmsTestDER(t, struct {
					S []cmsTestSigner `asn1:"set"`
				}{sd.Signers})
				var oldRaw asn1.RawValue
				_, _ = asn1.Unmarshal(old, &oldRaw)
				raw.FullBytes = nil
				raw.Bytes = append(raw.Bytes[:len(raw.Bytes)-len(oldRaw.Bytes)], signers...)
				der = cmsTestDER(t, cmsTestOuter{cmsSignedDataOID, asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: cmsTestDER(t, raw)}})
			case "unsorted_attributes":
				ordered, err := asn1.MarshalWithParams(sd.Signers[0].Attrs, "set")
				if err != nil {
					t.Fatal(err)
				}
				var set asn1.RawValue
				if _, err := asn1.Unmarshal(ordered, &set); err != nil {
					t.Fatal(err)
				}
				var reversed []byte
				for data := set.Bytes; len(data) > 0; {
					var a asn1.RawValue
					rest, err := asn1.Unmarshal(data, &a)
					if err != nil {
						t.Fatal(err)
					}
					reversed = append(bytes.Clone(a.FullBytes), reversed...)
					data = rest
				}
				unordered := cmsTestDER(t, asn1.RawValue{Class: 0, Tag: 17, IsCompound: true, Bytes: reversed})
				digest := sha256.Sum256(unordered)
				sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
				if err != nil {
					t.Fatal(err)
				}
				// Re-sign the non-DER ordering. Its mathematical signature is valid,
				// so the profile ordering check must reject it independently.
				der = bytes.Replace(der, sd.Signers[0].Signature, sig, 1)
				ordered[0] = 0xa0
				unordered[0] = 0xa0
				if !bytes.Contains(der, ordered) {
					t.Fatal("signed attributes not found")
				}
				der = bytes.Replace(der, ordered, unordered, 1)
			}
			if _, err := decodeRPKICMS(der); err == nil {
				t.Fatal("invalid CMS accepted")
			}
		})
	}
}

func TestRPKICMSBinarySigningTime(t *testing.T) {
	for _, both := range []bool{false, true} {
		sd, key := cmsTestFixture(t)
		if !both {
			sd.Signers[0].Attrs = sd.Signers[0].Attrs[:2]
		}
		seconds := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC).Unix()
		sd.Signers[0].Attrs = append(sd.Signers[0].Attrs, cmsTestAttribute{cmsBinaryTimeOID, []asn1.RawValue{cmsTestValue(t, seconds)}})
		got, err := decodeRPKICMS(cmsTestEncode(t, sd, key))
		if err != nil {
			t.Fatal(err)
		}
		if got.SigningTime.Unix() != seconds {
			t.Fatal("binary signing time changed")
		}
	}
}
func FuzzRPKICMS(f *testing.F) {
	sd, key := cmsTestFixture(f)
	f.Add(cmsTestEncode(f, sd, key))
	f.Add([]byte{0x30, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4<<20 {
			return
		}
		_, _ = decodeRPKICMS(data)
	})
}
