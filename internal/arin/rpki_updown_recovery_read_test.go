package arin

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIRevocationRecoveryRead(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	wrong, _ := cmsSigningFixture(t)
	class := upDownListFixture(t)
	classes, _, err := parseUpDownList([]byte(upDownTestReply("list_response", class)), "child", "parent")
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(classes[0].Certificates[0].DER)
	if err != nil {
		t.Fatal(err)
	}
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		Key       asn1.BitString
	}
	if _, err := asn1.Unmarshal(cert.RawSubjectPublicKeyInfo, &spki); err != nil {
		t.Fatal(err)
	}
	keyHash := sha1.Sum(spki.Key.Bytes)
	ski := base64.RawURLEncoding.EncodeToString(keyHash[:])
	batch := []byte(`<message xmlns="` + rpkiUpDownNamespace + `" version="1" sender="child" recipient="parent" type="revoke"><key class_name="class" ski="` + ski + `"/></message>`)
	scope, _ := json.Marshal([]string{"child", "parent"})
	original, err := signRPKICMS(batch, local, cmsTrustNow(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(original))
	for _, mode := range []string{"present", "absent", "class_absent", "old_reply", "wrong_signer", "unavailable", "scheduled", "same_second", "wrong_digest", "original_locked", "recovery_rollback"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			var calls atomic.Int32
			var exchange rpkiHTTPExchange
			var peer string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				// The original mutation must remain locked throughout the recovery request.
				if lease, err := openRPKIExchange(directory, peer); err == nil {
					_ = lease.Close()
					t.Error("original peer unlocked during recovery")
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				request, err := verifyRPKICMS(raw, rpkiCMSTrust{Anchor: local.Anchor, Now: cmsTrustNow().Add(2 * time.Second)})
				if err != nil || !strings.Contains(string(request.Content), `type="list"`) || strings.Contains(string(request.Content), "<key") || !request.SigningTime.After(cmsTrustNow()) {
					t.Error("recovery resent mutation or used stale signing time")
					return
				}
				if mode == "unavailable" {
					w.WriteHeader(503)
					return
				}
				objects := class
				if mode == "absent" {
					objects = strings.Replace(class, "<certificate", "<removed", 1)
					start := strings.Index(objects, "<removed")
					end := strings.Index(objects, "</certificate>") + len("</certificate>")
					objects = objects[:start] + objects[end:]
				}
				if mode == "class_absent" {
					objects = ""
				}
				signing := remote
				if mode == "wrong_signer" {
					signing = wrong
				}
				now := cmsTrustNow().Add(2 * time.Second)
				if mode == "old_reply" {
					now = cmsTrustNow()
				}
				if mode == "recovery_rollback" && calls.Load() > 1 {
					now = cmsTrustNow().Add(time.Second)
				}
				reply := upDownTestReply("list_response", objects)
				if mode == "scheduled" {
					reply = upDownTestReply("error_response", "<status>1104</status>")
				}
				response, err := signRPKICMS([]byte(reply), signing, now, time.Time{})
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/rpki-updown")
				_, _ = w.Write(response)
			}))
			defer server.Close()
			now := cmsTrustNow().Add(2 * time.Second)
			if mode == "same_second" {
				now = cmsTrustNow()
			}
			exchange = rpkiHTTPExchange{Endpoint: server.URL, MediaType: "application/rpki-updown", Directory: directory, PeerScope: string(scope), Identity: local, PeerAnchor: remote.Anchor, Clock: func() time.Time { return now }}
			peer, err = exchange.peerID()
			if err != nil {
				t.Fatal(err)
			}
			lease, err := openRPKIExchange(directory, peer)
			if err != nil {
				t.Fatal(err)
			}
			if err := lease.Begin(strings.Repeat("d", 64), "updown-list", cmsTrustNow().Add(-time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := lease.Complete(strings.Repeat("d", 64), cmsTrustNow().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := lease.BeginSigned(digest, "updown-revoke", cmsTrustNow(), original); err != nil {
				t.Fatal(err)
			}
			path := lease.path
			if mode != "original_locked" {
				if err := lease.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				defer lease.Close()
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			expected := digest
			if mode == "wrong_digest" {
				expected = strings.Repeat("e", 64)
			}
			client := rpkiUpDownClient{Exchange: exchange, Child: "child", Parent: "parent"}
			observation, err := client.observePendingRevocation(context.Background(), expected)
			valid := mode == "present" || mode == "absent" || mode == "class_absent" || mode == "recovery_rollback"
			if (err == nil) != valid {
				t.Fatalf("valid=%v err=%v", valid, err)
			}
			if valid {
				want := map[string]string{"present": "key_present", "absent": "key_absent", "class_absent": "class_absent", "recovery_rollback": "key_present"}[mode]
				if observation.Outcome != want || observation.Plan.RequestSHA256 != digest || observation.RecoveryPeerID == peer || !observation.Sent.After(cmsTrustNow()) || observation.Received.Before(cmsTrustNow().Add(time.Second)) {
					t.Fatal("wrong recovery observation")
				}
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatal("recovery altered original pending journal")
			}

			if mode == "recovery_rollback" {
				if _, err := client.observePendingRevocation(context.Background(), digest); err == nil || calls.Load() != 2 {
					t.Fatal("recovery receive watermark rolled back")
				}
			}
			if mode == "old_reply" || mode == "wrong_signer" || mode == "unavailable" || mode == "scheduled" {
				if _, err := client.observePendingRevocation(context.Background(), digest); err == nil || calls.Load() != 1 {
					t.Fatal("failed recovery read retried automatically")
				}
			}
			if mode == "same_second" || mode == "wrong_digest" || mode == "original_locked" {
				if calls.Load() != 0 {
					t.Fatal("invalid recovery dispatched")
				}
			}
		})
	}
}

func TestRPKIRevocationInventoryClassification(t *testing.T) {
	class := upDownListFixture(t)
	classes, _, err := parseUpDownList([]byte(upDownTestReply("list_response", class)), "child", "parent")
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(classes[0].Certificates[0].DER)
	if err != nil {
		t.Fatal(err)
	}
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		Key       asn1.BitString
	}
	if _, err := asn1.Unmarshal(cert.RawSubjectPublicKeyInfo, &spki); err != nil {
		t.Fatal(err)
	}
	hash := sha1.Sum(spki.Key.Bytes)
	plan := rpkiRevocationRecoveryPlan{Class: "class", SKI: base64.RawURLEncoding.EncodeToString(hash[:])}
	other := classes[0]
	other.Name = "other"
	empty := classes[0]
	empty.Certificates = nil
	bad := classes[0]
	bad.Certificates = []rpkiResourceCertificate{{DER: []byte("invalid")}}
	for _, tc := range []struct {
		name    string
		classes []rpkiResourceClass
		want    string
	}{
		{"present", classes, "key_present"},
		{"multiple_same_key", []rpkiResourceClass{func() rpkiResourceClass {
			c := classes[0]
			c.Certificates = append(append([]rpkiResourceCertificate{}, c.Certificates...), c.Certificates...)
			return c
		}()}, "key_present"},
		{"other_class_only", []rpkiResourceClass{other}, "class_absent"},
		{"other_class_key", []rpkiResourceClass{empty, other}, "key_absent"},
		{"empty", nil, "class_absent"},
		{"duplicate_class", []rpkiResourceClass{empty, classes[0]}, ""},
		{"malformed", []rpkiResourceClass{bad}, ""},
		{"malformed_after_match", []rpkiResourceClass{classes[0], {Name: "other", Certificates: bad.Certificates}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := classifyRevocationInventory(plan, tc.classes)
			if got != tc.want || (err != nil) != (tc.want == "") {
				t.Fatalf("got %q err=%v", got, err)
			}
		})
	}
	plan.SKI = base64.RawURLEncoding.EncodeToString(cert.SubjectKeyId)
	if _, err := classifyRevocationInventory(plan, classes); err == nil {
		t.Fatal("trusted fixture's non-method-1 SKI")
	}
}
