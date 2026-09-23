package arin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"
)

func setupTestCertificate(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/setup-ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		t.Fatal("missing fixture certificate")
	}
	return base64.StdEncoding.EncodeToString(block.Bytes)
}
func TestRPKISetupMessages(t *testing.T) {
	cert := setupTestCertificate(t)
	for _, tc := range []struct{ kind, attrs, ta, extra string }{
		{"child_request", `child_handle="child/1"`, "child", ""},
		{"parent_response", `child_handle="child/1" parent_handle="parent" service_uri="https://example.net/updown"`, "parent", `<offer/><referral referrer="parent" contact_uri="mailto:rpki@example.net">YWJj</referral><referral referrer="other">ZGVm</referral>`},
		{"publisher_request", `publisher_handle="publisher"`, "publisher", `<referral referrer="parent">YWJj</referral>`},
		{"repository_response", `publisher_handle="publisher" service_uri="http://example.net/publication" sia_base="rsync://example.net/repo/" rrdp_notification_uri="https://example.net/notification.xml"`, "repository", ""},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			xml := `<` + tc.kind + ` xmlns="` + RPKISetupNamespace + `" version="1" tag="  test  tag " ` + tc.attrs + `><` + tc.ta + `_bpki_ta>` + cert + `</` + tc.ta + `_bpki_ta>` + tc.extra + `</` + tc.kind + `>`
			got, err := ParseRPKISetup([]byte(xml))
			if err != nil {
				t.Fatal(err)
			}
			if got.Type != tc.kind || got.Tag == nil || *got.Tag != "test tag" || len(got.CertificateSHA256) != 64 || got.NotAfter == "" || got.NotBefore == "" {
				t.Fatal("lost setup metadata")
			}
			if tc.kind == "parent_response" && (!got.Offer || len(got.Referrals) != 2 || got.Referrals[0].AuthorizationBase64 != "YWJj") {
				t.Fatal("lost parent publication metadata")
			}
			empty, err := ParseRPKISetup([]byte(strings.Replace(xml, `tag="  test  tag "`, `tag=""`, 1)))
			if err != nil || empty.Tag == nil || *empty.Tag != "" {
				t.Fatal("empty tag lost")
			}
			absent, err := ParseRPKISetup([]byte(strings.Replace(xml, `tag="  test  tag "`, "", 1)))
			if err != nil || absent.Tag != nil {
				t.Fatal("absent tag invented")
			}
		})
	}
}
func TestRPKISetupRejectsInvalid(t *testing.T) {
	cert := setupTestCertificate(t)
	good := `<parent_response xmlns="` + RPKISetupNamespace + `" version="1" child_handle="child" parent_handle="parent" service_uri="https://example.net/updown"><parent_bpki_ta>` + cert + `</parent_bpki_ta></parent_response>`
	replace := func(a, b string) string { return strings.Replace(good, a, b, 1) }
	der, _ := base64.StdEncoding.DecodeString(cert)
	der[len(der)-1] ^= 1
	cases := map[string]string{
		"namespace": replace(RPKISetupNamespace, "urn:foreign"), "version": replace(`version="1"`, `version="2"`),
		"missing_attribute": replace(`parent_handle="parent"`, ""), "duplicate_attribute": replace(`version="1"`, `version="1" version="1"`),
		"foreign_attribute": replace(`parent_handle="parent"`, `xmlns:x="urn:foreign" x:parent_handle="parent"`),
		"handle":            replace(`child_handle="child"`, `child_handle="bad handle"`), "url": replace("https://example.net/updown", "file:///tmp/data"),
		"certificate": replace(cert, "YWJj"), "signature": replace(cert, base64.StdEncoding.EncodeToString(der)), "base64": replace(cert, "!!!!"),
		"duplicate_certificate": replace(`</parent_response>`, `<parent_bpki_ta>`+cert+`</parent_bpki_ta></parent_response>`),
		"foreign_child":         replace(`<parent_bpki_ta>`, `<parent_bpki_ta xmlns="urn:foreign">`),
		"duplicate_offer":       replace(`</parent_response>`, `<offer/><offer/></parent_response>`),
		"offer_text":            replace(`</parent_response>`, `<offer>bad</offer></parent_response>`),
		"nested_certificate":    replace(cert, `<nested/>`+cert), "unknown_child": replace(`</parent_response>`, `<other/></parent_response>`),
		"doctype": `<!DOCTYPE x SYSTEM "file:///etc/passwd">` + good, "multiple_roots": good + good, "trailing_text": good + "x",
		"oversized":                 strings.Repeat(" ", 4<<20) + good,
		"referral_without_referrer": replace(`</parent_response>`, `<referral>YWJj</referral></parent_response>`),
		"referral_invalid_base64":   replace(`</parent_response>`, `<referral referrer="p">!</referral></parent_response>`),
	}
	for name, xml := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseRPKISetup([]byte(xml)); err == nil {
				t.Fatal("accepted invalid setup")
			}
		})
	}
}

func TestRPKISetupCertificateProperties(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"expired", "not_ca", "wrong_usage", "resources", "different_issuer"} {
		t.Run(mode, func(t *testing.T) {
			cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test CA"}, NotBefore: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
			switch mode {
			case "not_ca":
				cert.IsCA = false
			case "wrong_usage":
				cert.KeyUsage = x509.KeyUsageDigitalSignature
			case "resources":
				cert.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 7}, Value: []byte{0x30, 0x00}}}
			}
			parent := *cert
			if mode == "different_issuer" {
				parent.Subject = pkix.Name{CommonName: "Other CA"}
			}
			der, err := x509.CreateCertificate(rand.Reader, cert, &parent, &key.PublicKey, key)
			if err != nil {
				t.Fatal(err)
			}
			xml := `<child_request xmlns="` + RPKISetupNamespace + `" version="1" child_handle="test"><child_bpki_ta>` + base64.StdEncoding.EncodeToString(der) + `</child_bpki_ta></child_request>`
			_, err = ParseRPKISetup([]byte(xml))
			if mode == "expired" && err != nil {
				t.Fatal("historical document must remain readable", err)
			}
			if mode != "expired" && err == nil {
				t.Fatal("accepted invalid BPKI CA")
			}
		})
	}
}

func TestRPKISetupLimits(t *testing.T) {
	cert := setupTestCertificate(t)
	prefix := `<publisher_request xmlns="` + RPKISetupNamespace + `" version="1" publisher_handle="` + strings.Repeat("a", 255) + `"><publisher_bpki_ta>` + cert + `</publisher_bpki_ta><referral referrer="p">`
	suffix := `</referral></publisher_request>`
	for _, size := range []int{512000, 512001} {
		token := base64.StdEncoding.EncodeToString(make([]byte, size))
		_, err := ParseRPKISetup([]byte(prefix + token + suffix))
		if size == 512000 && err != nil {
			t.Fatal(err)
		}
		if size > 512000 && err == nil {
			t.Fatal("accepted oversized token")
		}
	}
	_, err := ParseRPKISetup([]byte(strings.Replace(prefix, strings.Repeat("a", 255), strings.Repeat("a", 256), 1) + "YWJj" + suffix))
	if err == nil {
		t.Fatal("accepted oversized handle")
	}
}
