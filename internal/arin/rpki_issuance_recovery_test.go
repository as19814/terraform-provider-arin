package arin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRPKIPendingIssuancePlan(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{ExtraExtensions: rpkiCSRTestExtensions(t)}, local.Signer)
	if err != nil {
		t.Fatal(err)
	}
	asn, ipv4 := "64500-64510", ""
	body, err := buildUpDownIssue("child", "parent", rpkiIssueRequest{Class: "class", CSRDER: csr, RequestedASN: &asn, RequestedIPv4: &ipv4})
	if err != nil {
		t.Fatal(err)
	}
	wanted, err := parseIssuanceRecoveryPlan(body, "child", "parent")
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := json.Marshal([]string{"child", "parent"})
	for _, mode := range []string{"valid", "wrong_peer", "wrong_operation", "wrong_time", "corrupt_signature", "wrong_payload", "wrong_child", "wrong_parent", "wrong_sender", "wrong_recipient", "missing_evidence"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			exchange := rpkiHTTPExchange{Endpoint: "https://repo.example/publication", MediaType: "application/rpki-updown", Directory: directory, PeerScope: string(scope), Identity: local, PeerAnchor: remote.Anchor}
			peer, err := exchange.peerID()
			if err != nil {
				t.Fatal(err)
			}
			lease, err := openRPKIExchange(directory, peer)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			payload := body
			if mode == "wrong_payload" {
				payload = []byte(strings.Replace(string(body), `type="issue"`, `type="list"`, 1))
			}
			if mode == "wrong_sender" {
				payload = []byte(strings.Replace(string(body), `sender="child"`, `sender="other"`, 1))
			}
			if mode == "wrong_recipient" {
				payload = []byte(strings.Replace(string(body), `recipient="parent"`, `recipient="other"`, 1))
			}
			signed, err := signRPKICMS(payload, local, cmsTrustNow(), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "corrupt_signature" {
				signed[len(signed)-1] ^= 1
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(signed))
			operation := "updown-issue"
			if mode == "wrong_operation" {
				operation = "updown-revoke"
			}
			recorded := cmsTrustNow()
			if mode == "wrong_time" {
				recorded = recorded.Add(time.Second)
			}
			if mode == "missing_evidence" {
				err = lease.Begin(digest, operation, recorded)
			} else {
				err = lease.BeginSigned(digest, operation, recorded, signed)
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "wrong_peer" {
				exchange.Endpoint = "https://other.example/provisioning"
			}
			client := rpkiUpDownClient{Exchange: exchange, Child: "child", Parent: "parent"}
			if mode == "wrong_child" {
				client.Child = "other"
			}
			if mode == "wrong_parent" {
				client.Parent = "other"
			}
			plan, err := client.pendingIssuancePlan(lease)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode=%s err=%v", mode, err)
			}
			if err == nil {
				if plan.RequestSHA256 != digest || !plan.SigningTime.Equal(recorded) || plan.Request.Class != "class" || plan.SKI != wanted.SKI || plan.Request.CSRPEM != wanted.Request.CSRPEM || plan.Request.RequestedASN == nil || *plan.Request.RequestedASN != asn || plan.Request.RequestedIPv4 == nil || *plan.Request.RequestedIPv4 != "" || plan.Request.RequestedIPv6 != nil || plan.Child != "child" || plan.Parent != "parent" {
					t.Fatal("recorded issuance intent lost")
				}
			}
			state, err := lease.State()
			if err != nil || state.Pending == nil || state.Pending.RequestSHA256 != digest {
				t.Fatal("inspection cleared pending mutation")
			}
		})
	}
}

func TestRPKIIssuanceRecoveryPayload(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{ExtraExtensions: rpkiCSRTestExtensions(t)}, local.Signer)
	if err != nil {
		t.Fatal(err)
	}
	asn := "64500"
	raw, err := buildUpDownIssue("child", "parent", rpkiIssueRequest{Class: "class", CSRDER: csr, RequestedASN: &asn})
	if err != nil {
		t.Fatal(err)
	}
	good := string(raw)
	for _, bad := range []string{
		"", strings.Replace(good, `type="issue"`, `type="list"`, 1),
		strings.Replace(good, `sender="child"`, `sender="other"`, 1), strings.Replace(good, `recipient="parent"`, `recipient="other"`, 1),
		strings.Replace(good, `class_name="class"`, `class_name=""`, 1),
		strings.Replace(good, `req_resource_set_as="64500"`, `req_resource_set_as="64510-64500"`, 1),
		strings.Replace(good, `class_name="class"`, `class_name="class" extra="x"`, 1),
		strings.Replace(good, "</request>", "<nested/></request>", 1),
		strings.Replace(good, "</message>", "<request/></message>", 1),
		strings.Replace(good, "</request>", "!bad</request>", 1),
		strings.Replace(good, rpkiUpDownNamespace, "urn:wrong", 1),
	} {
		if _, err := parseIssuanceRecoveryPlan([]byte(bad), "child", "parent"); err == nil {
			t.Fatal("malformed issuance evidence accepted")
		}
	}
	csr[len(csr)-1] ^= 1
	bad := strings.Replace(good, strings.Split(strings.Split(good, `req_resource_set_as="64500">`)[1], "</request>")[0], base64.StdEncoding.EncodeToString(csr), 1)
	if _, err := parseIssuanceRecoveryPlan([]byte(bad), "child", "parent"); err == nil {
		t.Fatal("invalid CSR signature accepted")
	}
}
