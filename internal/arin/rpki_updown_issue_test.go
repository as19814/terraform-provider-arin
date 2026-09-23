package arin

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func upDownIssueFixture(t *testing.T, certSIAMode ...string) (rpkiIssueRequest, string) {
	t.Helper()
	_, issuerKey, issuer := cmsTrustFixture(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "child resource CA"}, ExtraExtensions: rpkiCSRTestExtensions(t)}, key)
	if err != nil {
		t.Fatal(err)
	}
	certificateExtensions := []pkix.Extension{rpkiSIATestExtension(t)}
	if len(certSIAMode) > 0 {
		switch certSIAMode[0] {
		case "missing":
			certificateExtensions = nil
		case "changed":
			certificateExtensions[0].Value = bytes.ReplaceAll(certificateExtensions[0].Value, []byte("repo.example"), []byte("evil.example"))
		case "critical":
			certificateExtensions[0].Critical = true
		}
	}
	cert, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{SerialNumber: big.NewInt(42), Subject: pkix.Name{CommonName: "child resource CA"}, NotBefore: cmsTrustNow().Add(-time.Hour), NotAfter: cmsTrustNow().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, ExtraExtensions: certificateExtensions}, issuer, &key.PublicKey, issuerKey)
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	input := rpkiIssueRequest{Class: "class", CSRDER: csr, RequestedASN: &empty}
	class := `<class class_name="class" cert_url="rsync://repo.example/module/ca.cer" resource_set_as="" resource_set_ipv4="192.0.2.0/24" resource_set_ipv6="" resource_set_notafter="2026-09-23T13:00:00Z"><certificate cert_url="rsync://repo.example/module/child.cer" req_resource_set_as="">` + base64.StdEncoding.EncodeToString(cert) + `</certificate><issuer>` + base64.StdEncoding.EncodeToString(issuer.Raw) + `</issuer></class>`
	return input, upDownTestReply("issue_response", class)
}

func TestUpDownIssueValidation(t *testing.T) {
	input, reply := upDownIssueFixture(t)
	query, err := buildUpDownIssue("child", "parent", input)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := parseXML(query)
	a, _ := upDownAttrs(n.Children[0], "request", "class_name", "req_resource_set_as", "req_resource_set_ipv4", "req_resource_set_ipv6")
	if v, ok := a["req_resource_set_as"]; !ok || v != "" {
		t.Fatal("empty restriction omitted")
	}
	if _, ok := a["req_resource_set_ipv4"]; ok {
		t.Fatal("absent restriction added")
	}
	got, rejected, err := validateUpDownIssue(query, []byte(reply), "child", "parent", cmsTrustNow())
	if err != nil || rejected != nil || got == nil || len(got.Certificates) != 1 {
		t.Fatalf("issue validation failed: %v", err)
	}
	corruptCertificate := bytes.Clone(got.Certificates[0].DER)
	corruptCertificate[len(corruptCertificate)-1] ^= 1
	badSignature := strings.Replace(reply, base64.StdEncoding.EncodeToString(got.Certificates[0].DER), base64.StdEncoding.EncodeToString(corruptCertificate), 1)
	if got, _, err := validateUpDownIssue(query, []byte(badSignature), "child", "parent", cmsTrustNow()); err == nil || got != nil {
		t.Fatal("invalid certificate signature accepted")
	}
	other, _ := upDownIssueFixture(t)
	otherQuery, err := buildUpDownIssue("child", "parent", other)
	if err != nil {
		t.Fatal(err)
	}
	if got, _, err := validateUpDownIssue(otherQuery, []byte(reply), "child", "parent", cmsTrustNow()); err == nil || got != nil {
		t.Fatal("wrong public key accepted")
	}
	for name, bad := range map[string]string{
		"class":          strings.Replace(reply, `class_name="class"`, `class_name="other"`, 1),
		"sender":         strings.Replace(reply, `sender="parent"`, `sender="other"`, 1),
		"type":           strings.Replace(reply, "issue_response", "list_response", 1),
		"missing_echo":   strings.Replace(reply, `req_resource_set_as=""`, "", 1),
		"changed_echo":   strings.Replace(reply, `req_resource_set_as=""`, `req_resource_set_as="19814"`, 1),
		"added_echo":     strings.Replace(reply, `req_resource_set_as=""`, `req_resource_set_as="" req_resource_set_ipv4=""`, 1),
		"no_certificate": upDownTestReply("issue_response", `<class/>`),
		"scheduled":      upDownTestReply("error_response", `<status>1104</status>`),
	} {
		t.Run(name, func(t *testing.T) {
			if got, _, err := validateUpDownIssue(query, []byte(bad), "child", "parent", cmsTrustNow()); err == nil || got != nil {
				t.Fatal("invalid issue response accepted")
			}
		})
	}
	for _, now := range []time.Time{cmsTrustNow().Add(-2 * time.Hour), cmsTrustNow().Add(2 * time.Hour)} {
		if got, _, err := validateUpDownIssue(query, []byte(reply), "child", "parent", now); err == nil || got != nil {
			t.Fatal("invalid certificate dates accepted")
		}
	}
	corrupt := bytes.Clone(input.CSRDER)
	corrupt[len(corrupt)-1] ^= 1
	for name, bad := range map[string]rpkiIssueRequest{
		"corrupt_csr":   {Class: "class", CSRDER: corrupt},
		"invalid_class": {Class: "\x00", CSRDER: input.CSRDER},
		"missing_csr":   {Class: "class"},
		"oversized_csr": {Class: "class", CSRDER: make([]byte, 512001)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := buildUpDownIssue("child", "parent", bad); err == nil {
				t.Fatal("invalid issue request accepted")
			}
		})
	}
}

func TestUpDownIssueExchange(t *testing.T) {
	input, reply := upDownIssueFixture(t)
	_, wrongReply := upDownIssueFixture(t)
	for _, mode := range []string{"issued", "rejected", "wrong_key", "scheduled"} {
		t.Run(mode, func(t *testing.T) {
			local, _ := cmsSigningFixture(t)
			remote, _ := cmsSigningFixture(t)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				verified, err := verifyRPKICMS(raw, rpkiCMSTrust{Anchor: local.Anchor, Now: cmsTrustNow()})
				if err != nil {
					t.Error(err)
					return
				}
				query, err := parseXML(verified.Content)
				if err != nil {
					t.Error(err)
					return
				}
				a, err := upDownAttrs(query, "message", "version", "sender", "recipient", "type")
				if err != nil || a["type"] != "issue" || a["sender"] != "child" || a["recipient"] != "parent" || len(query.Children) != 1 {
					t.Error("invalid issue message")
					return
				}
				p := query.Children[0]
				attrs, err := upDownAttrs(p, "request", "class_name", "req_resource_set_as", "req_resource_set_ipv4", "req_resource_set_ipv6")
				der, decodeErr := base64.StdEncoding.DecodeString(p.Text)
				if err != nil || decodeErr != nil || !bytes.Equal(der, input.CSRDER) || attrs["class_name"] != "class" {
					t.Error("wrong issue payload")
					return
				}
				response := reply
				switch mode {
				case "rejected":
					response = upDownTestReply("error_response", `<status>1204</status>`)
				case "scheduled":
					response = upDownTestReply("error_response", `<status>1104</status>`)
				case "wrong_key":
					response = wrongReply
				}
				signed, err := signRPKICMS([]byte(response), remote, cmsTrustNow(), time.Time{})
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/rpki-updown")
				_, _ = w.Write(signed)
			}))
			defer server.Close()
			config := rpkiHTTPExchange{Endpoint: server.URL, MediaType: "application/rpki-updown", Directory: privateExchangeDir(t), Identity: local, PeerAnchor: remote.Anchor, Clock: cmsTrustNow}
			client := rpkiUpDownClient{Exchange: config, Child: "child", Parent: "parent"}
			for i := 0; i < 2; i++ {
				got, err := client.Issue(context.Background(), input)
				switch mode {
				case "issued":
					if err != nil || got == nil {
						t.Fatalf("issue failed: %v", err)
					}
				case "rejected":
					var rejected *rpkiUpDownError
					if !errors.As(err, &rejected) || rejected.Code != 1204 || got != nil {
						t.Fatal("wrong rejection")
					}
				default:
					if err == nil || got != nil {
						t.Fatal("uncertain issue accepted")
					}
				}
			}
			scope, _ := json.Marshal([]string{"child", "parent"})
			config.PeerScope = string(scope)
			peer, _ := config.peerID()
			lease, err := openRPKIExchange(config.Directory, peer)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			state, err := lease.State()
			if err != nil {
				t.Fatal(err)
			}
			if mode == "wrong_key" || mode == "scheduled" {
				if calls.Load() != 1 || state.Pending == nil || !state.LastReceived.IsZero() {
					t.Fatal("uncertain issuance completed or retried")
				}
			} else if calls.Load() != 2 || state.Pending != nil || !state.LastReceived.Equal(cmsTrustNow()) {
				t.Fatal("exchange not durably completed")
			}
		})
	}
}
