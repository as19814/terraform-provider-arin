package arin

import (
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"os"
	"strings"
	"testing"
)

func TestBuildRPKISetupRequest(t *testing.T) {
	cert, err := os.ReadFile("testdata/setup-ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(cert)
	for _, kind := range []string{"child_request", "publisher_request"} {
		for _, tag := range []*string{nil, ptrSetupTag(""), ptrSetupTag("  one & <two> \"  ")} {
			req := RPKISetupRequest{Type: kind, Handle: "example/child_1", CertificatePEM: string(cert), Tag: tag}
			if kind == "publisher_request" {
				req.Referrals = []RPKISetupRequestReferral{{Referrer: "parent/1", AuthorizationBase64: "Y W\nJj"}, {Referrer: "parent/2", AuthorizationBase64: "ZGVm"}}
			}
			document, err := BuildRPKISetupRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			again, err := BuildRPKISetupRequest(req)
			if err != nil || string(again) != string(document) {
				t.Fatal("unstable generation")
			}
			var raw struct {
				XMLName     xml.Name
				Version     string                     `xml:"version,attr"`
				Tag         *string                    `xml:"tag,attr"`
				Child       string                     `xml:"child_handle,attr"`
				Publisher   string                     `xml:"publisher_handle,attr"`
				ChildTA     string                     `xml:"child_bpki_ta"`
				PublisherTA string                     `xml:"publisher_bpki_ta"`
				Referrals   []RPKISetupRequestReferral `xml:"referral"`
			}
			if err := xml.Unmarshal(document, &raw); err != nil {
				t.Fatal(err)
			}
			if raw.XMLName.Space != RPKISetupNamespace || raw.XMLName.Local != kind || raw.Version != "1" || (tag == nil) != (raw.Tag == nil) {
				t.Fatal("wrong envelope")
			}
			expected := base64.StdEncoding.EncodeToString(block.Bytes)
			if kind == "child_request" && (raw.Child != req.Handle || raw.ChildTA != expected || len(raw.Referrals) != 0) {
				t.Fatal("wrong child request")
			}
			if kind == "publisher_request" && (raw.Publisher != req.Handle || raw.PublisherTA != expected || len(raw.Referrals) != 2 || raw.Referrals[0].AuthorizationBase64 != "YWJj") {
				t.Fatal("wrong publisher request")
			}
			parsed, err := ParseRPKISetup(document)
			if err != nil {
				t.Fatal(err)
			}
			if tag != nil && *tag != "" && *parsed.Tag != `one & <two> "` {
				t.Fatal("tag escaping or normalization changed content")
			}
		}
	}
}
func ptrSetupTag(s string) *string { return &s }
func TestBuildRPKISetupRequestRejectsInvalid(t *testing.T) {
	cert, err := os.ReadFile("testdata/setup-ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*RPKISetupRequest){
		"response_type":         func(r *RPKISetupRequest) { r.Type = "parent_response" },
		"invalid_handle":        func(r *RPKISetupRequest) { r.Handle = "bad&handle" },
		"tag_control":           func(r *RPKISetupRequest) { r.Tag = ptrSetupTag("bad\x00") },
		"tag_utf8":              func(r *RPKISetupRequest) { r.Tag = ptrSetupTag(string([]byte{0xff})) },
		"tag_size":              func(r *RPKISetupRequest) { r.Tag = ptrSetupTag(strings.Repeat("a", 1025)) },
		"extra_pem":             func(r *RPKISetupRequest) { r.CertificatePEM += string(cert) },
		"malformed_first_block": func(r *RPKISetupRequest) { r.CertificatePEM = "-----BEGIN CERTIFICATE-----\nbroken\n" + string(cert) },
		"leading_junk":          func(r *RPKISetupRequest) { r.CertificatePEM = "junk\n" + string(cert) },
		"trailing_junk":         func(r *RPKISetupRequest) { r.CertificatePEM += "junk" },
		"wrong_pem_type": func(r *RPKISetupRequest) {
			r.CertificatePEM = strings.ReplaceAll(string(cert), "CERTIFICATE", "PRIVATE KEY")
		},
		"bad_certificate": func(r *RPKISetupRequest) {
			r.CertificatePEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid")}))
		},
		"pem_headers": func(r *RPKISetupRequest) {
			b, _ := pem.Decode(cert)
			b.Headers = map[string]string{"X": "Y"}
			r.CertificatePEM = string(pem.EncodeToMemory(b))
		},
		"child_referral": func(r *RPKISetupRequest) {
			r.Referrals = []RPKISetupRequestReferral{{Referrer: "p", AuthorizationBase64: "YWJj"}}
		},
		"bad_referral": func(r *RPKISetupRequest) {
			r.Type = "publisher_request"
			r.Referrals = []RPKISetupRequestReferral{{Referrer: "p", AuthorizationBase64: "!"}}
		},
		"bad_referrer": func(r *RPKISetupRequest) {
			r.Type = "publisher_request"
			r.Referrals = []RPKISetupRequestReferral{{Referrer: "p&", AuthorizationBase64: "YWJj"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := RPKISetupRequest{Type: "child_request", Handle: "c", CertificatePEM: string(cert)}
			change(&req)
			if _, err := BuildRPKISetupRequest(req); err == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
}
