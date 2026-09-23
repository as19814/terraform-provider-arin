package arin

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRPKIPendingRevocationPlan(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	ski := "AAAAAAAAAAAAAAAAAAAAAAAAAAA"
	body := []byte(`<message xmlns="` + rpkiUpDownNamespace + `" version="1" sender="child" recipient="parent" type="revoke"><key class_name="class" ski="` + ski + `"/></message>`)
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
				payload = []byte(strings.Replace(string(body), `type="revoke"`, `type="list"`, 1))
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
			operation := "updown-revoke"
			if mode == "wrong_operation" {
				operation = "updown-issue"
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
			plan, err := client.pendingRevocationPlan(lease)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode=%s err=%v", mode, err)
			}
			if err == nil {
				if plan.RequestSHA256 != digest || !plan.SigningTime.Equal(recorded) || plan.Class != "class" || plan.SKI != ski || plan.Child != "child" || plan.Parent != "parent" {
					t.Fatal("recorded revocation intent lost")
				}
			}
			state, err := lease.State()
			if err != nil || state.Pending == nil || state.Pending.RequestSHA256 != digest {
				t.Fatal("inspection cleared pending mutation")
			}
		})
	}
}

func TestRPKIRevocationRecoveryPayload(t *testing.T) {
	good := `<message xmlns="` + rpkiUpDownNamespace + `" version="1" sender="child" recipient="parent" type="revoke"><key class_name="class" ski="AAAAAAAAAAAAAAAAAAAAAAAAAAA"/></message>`
	for _, bad := range []string{
		"", strings.Replace(good, `type="revoke"`, `type="issue"`, 1),
		strings.Replace(good, `sender="child"`, `sender="other"`, 1),
		strings.Replace(good, `recipient="parent"`, `recipient="other"`, 1),
		strings.Replace(good, `class_name="class"`, `class_name=""`, 1),
		strings.Replace(good, `ski="AAAAAAAAAAAAAAAAAAAAAAAAAAA"`, `ski="bad"`, 1),
		strings.Replace(good, "/>", "><nested/></key>", 1),
		strings.Replace(good, "/>", ">text</key>", 1),
		strings.Replace(good, "</message>", "<key/></message>", 1),
		strings.Replace(good, `version="1"`, `version="2"`, 1),
		strings.Replace(good, `class_name="class"`, `unexpected="x" class_name="class"`, 1),
		strings.Replace(good, rpkiUpDownNamespace, "urn:wrong", 1),
	} {
		if _, err := parseRevocationRecoveryPlan([]byte(bad), "child", "parent"); err == nil {
			t.Fatal("invalid revocation accepted")
		}
	}
	padded := strings.Replace(good, `ski="AAAAAAAAAAAAAAAAAAAAAAAAAAA"`, `ski="AAAAAAAAAAAAAAAAAAAAAAAAAAA="`, 1)
	for _, body := range []string{good, padded} {
		plan, err := parseRevocationRecoveryPlan([]byte(body), "child", "parent")
		if err != nil || plan.SKI != "AAAAAAAAAAAAAAAAAAAAAAAAAAA" {
			t.Fatalf("valid key rejected: %v", err)
		}
	}
}
