package arin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func upDownListFixture(t *testing.T) string {
	t.Helper()
	identity, _ := cmsSigningFixture(t)
	cert := base64.StdEncoding.EncodeToString(identity.Anchor.Raw)
	return `<class class_name="class" cert_url="https://repo.example/ca.cer,rsync://repo.example/module/ca.cer" resource_set_as="19814,64500-64510" resource_set_ipv4="192.0.2.0/26,192.0.2.66-192.0.2.76" resource_set_ipv6="2001:db8::/48,2001:db8:2::-2001:db8:5::" resource_set_notafter="2027-01-01T00:00:00Z" suggested_sia_head="rsync://repo.example/module/child/"><certificate cert_url="rsync://repo.example/module/child.cer" req_resource_set_as="">` + cert + `</certificate><issuer>` + cert + `</issuer></class>`
}

func TestUpDownResourceSets(t *testing.T) {
	for _, tc := range []struct {
		s      string
		family int
		valid  bool
	}{
		{"", 0, true}, {"0,19814,4294967295", 0, true}, {"1-5,6-10", 0, true},
		{"01", 0, false}, {"4294967296", 0, false}, {"2-1", 0, false}, {"1-5,5-6", 0, false}, {"1,", 0, false}, {"+1", 0, false},
		{"0.0.0.0/0", 4, true}, {"192.0.2.0/24,192.0.3.0/24", 4, true}, {"192.0.2.1-192.0.2.2", 4, true},
		{"192.0.2.1/24", 4, false}, {"192.0.2.0/24,192.0.2.128/25", 4, false}, {"192.0.2.3-192.0.2.1", 4, false}, {"192.0.2.1", 4, false},
		{"::/0", 6, true}, {"2001:db8::/32", 6, true}, {"2001:db8::-2001:db8::ffff", 6, true},
		{"::ffff:192.0.2.1/128", 6, false}, {"fe80::1%en0-fe80::2%en0", 6, false}, {"192.0.2.0/24", 6, false}, {"2001:db8::/32", 4, false},
	} {
		t.Run(tc.s, func(t *testing.T) {
			if upDownResources(tc.s, tc.family) != tc.valid {
				t.Fatal("wrong resource-set validation")
			}
		})
	}
}

func TestUpDownListParsing(t *testing.T) {
	class := upDownListFixture(t)
	body := upDownTestReply("list_response", class)
	got, rejected, err := parseUpDownList([]byte(body), "child", "parent")
	if err != nil || rejected != nil || len(got) != 1 {
		t.Fatalf("list failed: %v", err)
	}
	c := got[0]
	if c.Name != "class" || c.ASN != "19814,64500-64510" || len(c.Certificates) != 1 || c.Certificates[0].RequestedASN == nil || *c.Certificates[0].RequestedASN != "" || c.Certificates[0].RequestedIPv4 != nil || !bytes.Equal(c.IssuerDER, c.Certificates[0].DER) || c.NotAfter.Year() != 2027 {
		t.Fatal("class fields lost")
	}
	got, rejected, err = parseUpDownList([]byte(upDownTestReply("list_response", "")), "child", "parent")
	if err != nil || rejected != nil || got == nil || len(got) != 0 {
		t.Fatal("empty list failed")
	}
	for name, bad := range map[string]string{
		"identity":         strings.Replace(body, `sender="parent"`, `sender="other"`, 1),
		"type":             strings.Replace(body, "list_response", "issue_response", 1),
		"duplicate_class":  upDownTestReply("list_response", class+class),
		"missing_resource": strings.Replace(body, `resource_set_as="19814,64500-64510"`, "", 1),
		"missing_issuer":   strings.ReplaceAll(strings.ReplaceAll(body, "<issuer>", "<unexpected>"), "</issuer>", "</unexpected>"),
		"foreign_class":    strings.Replace(body, "<class ", `<class xmlns="urn:foreign" `, 1),
		"overlap":          strings.Replace(body, "19814,64500-64510", "1-5,3-6", 1),
		"invalid_date":     strings.Replace(body, "2027-01-01T00:00:00Z", "2027-01-01T00:00:00+00:00", 1),
		"missing_rsync":    strings.ReplaceAll(body, "rsync://", "https://"),
		"bad_sia":          strings.Replace(body, "module/child/", "module/child", 1),
		"bad_cert":         strings.Replace(body, "</certificate>", "!invalid</certificate>", 1),
		"unknown_attr":     strings.Replace(body, "<certificate ", `<certificate unknown="x" `, 1),
		"nested_issuer":    strings.Replace(body, "<issuer>", "<issuer><extra/>", 1),
	} {
		t.Run(name, func(t *testing.T) {
			got, rejected, err := parseUpDownList([]byte(bad), "child", "parent")
			if err == nil || got != nil || rejected != nil {
				t.Fatal("invalid list accepted")
			}
		})
	}
}

func TestUpDownListExchange(t *testing.T) {
	class := upDownListFixture(t)
	for _, mode := range []string{"list", "empty", "rejected", "malformed"} {
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
				n, err := parseXML(verified.Content)
				if err != nil {
					t.Error(err)
					return
				}
				a, err := upDownAttrs(n, "message", "version", "sender", "recipient", "type")
				if err != nil || a["version"] != "1" || a["type"] != "list" || a["sender"] != "child" || a["recipient"] != "parent" || len(n.Children) != 0 || strings.TrimSpace(n.Text) != "" {
					t.Error("invalid list query")
					return
				}
				reply := upDownTestReply("list_response", class)
				switch mode {
				case "empty":
					reply = upDownTestReply("list_response", "")
				case "rejected":
					reply = upDownTestReply("error_response", `<status>2001</status>`)
				case "malformed":
					reply = upDownTestReply("list_response", `<class/>`)
				}
				signed, err := signRPKICMS([]byte(reply), remote, cmsTrustNow(), time.Time{})
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
				got, err := client.List(context.Background())
				switch mode {
				case "list":
					if err != nil || len(got) != 1 {
						t.Fatalf("list failed: %v", err)
					}
				case "empty":
					if err != nil || got == nil || len(got) != 0 {
						t.Fatal("empty failed")
					}
				case "rejected":
					var rejection *rpkiUpDownError
					if !errors.As(err, &rejection) || rejection.Code != 2001 || got != nil {
						t.Fatal("wrong error")
					}
				default:
					if err == nil || got != nil {
						t.Fatal("malformed accepted")
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
			if mode == "malformed" {
				if calls.Load() != 1 || state.Pending == nil || !state.LastReceived.IsZero() {
					t.Fatal("invalid reply completed or retried")
				}
			} else if calls.Load() != 2 || state.Pending != nil || !state.LastReceived.Equal(cmsTrustNow()) {
				t.Fatal("completion not saved")
			}
		})
	}
}
