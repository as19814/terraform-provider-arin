package arin

import (
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

func upDownTestReply(kind, payload string) string {
	return `<message xmlns="` + rpkiUpDownNamespace + `" version="1" sender="parent" recipient="child" type="` + kind + `">` + payload + `</message>`
}
func TestUpDownRevokeValidation(t *testing.T) {
	ski := base64.RawURLEncoding.EncodeToString(make([]byte, 20))
	good := upDownTestReply("revoke_response", `<key class_name="class" ski="`+ski+`"/>`)
	for _, body := range []string{good, strings.Replace(good, ski, ski+"=", 1)} {
		rejected, err := validateUpDownRevoke([]byte(body), "child", "parent", "class", ski)
		if err != nil || rejected != nil {
			t.Fatalf("valid response rejected: %v", err)
		}
	}
	for name, body := range map[string]string{
		"sender":                   strings.Replace(good, `sender="parent"`, `sender="unknown"`, 1),
		"recipient":                strings.Replace(good, `recipient="child"`, `recipient="other"`, 1),
		"class":                    strings.Replace(good, `class_name="class"`, `class_name="wrong"`, 1),
		"ski":                      strings.Replace(good, ski, base64.RawURLEncoding.EncodeToString([]byte("12345678901234567890")), 1),
		"type":                     strings.Replace(good, "revoke_response", "issue_response", 1),
		"version":                  strings.Replace(good, `version="1"`, `version="2"`, 1),
		"namespace":                strings.Replace(good, rpkiUpDownNamespace, "urn:other", 1),
		"unknown_attribute":        strings.Replace(good, `ski="`, `extra="x" ski="`, 1),
		"duplicate_attribute":      strings.Replace(good, `class_name="class"`, `class_name="class" class_name="class"`, 1),
		"missing_key":              upDownTestReply("revoke_response", ""),
		"multiple_keys":            strings.Replace(good, "</message>", `<key class_name="class" ski="`+ski+`"/></message>`, 1),
		"wrong_error_order":        upDownTestReply("error_response", `<description xml:lang="en-US">x</description><status>1302</status>`),
		"unknown_status":           upDownTestReply("error_response", `<status>9999</status>`),
		"scheduled":                upDownTestReply("error_response", `<status>1104</status>`),
		"processing":               upDownTestReply("error_response", `<status>1101</status>`),
		"missing_english":          upDownTestReply("error_response", `<status>1302</status><description xml:lang="fr">x</description>`),
		"unknown_description_attr": upDownTestReply("error_response", `<status>1302</status><description xml:lang="en-US" extra="x">x</description>`),
	} {
		t.Run(name, func(t *testing.T) {
			rejected, err := validateUpDownRevoke([]byte(body), "child", "parent", "class", ski)
			if err == nil || rejected != nil {
				t.Fatal("invalid or uncertain response accepted")
			}
		})
	}
	for _, payload := range []string{`<status>1302</status>`, `<status>1302</status><description xml:lang="en-US">private information</description><description xml:lang="fr">texte</description>`} {
		rejected, err := validateUpDownRevoke([]byte(upDownTestReply("error_response", payload)), "child", "parent", "class", ski)
		if err != nil || rejected == nil || rejected.Code != 1302 || strings.Contains(rejected.Error(), "private") {
			t.Fatal("incorrect protocol rejection")
		}
	}
}

func TestUpDownRevokeExchange(t *testing.T) {
	ski := base64.RawURLEncoding.EncodeToString(make([]byte, 20))
	for _, mode := range []string{"success", "rejection", "wrong_key", "scheduled"} {
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
				request, err := parseXML(verified.Content)
				if err != nil {
					t.Error(err)
					return
				}
				a, err := upDownAttrs(request, "message", "version", "sender", "recipient", "type")
				if err != nil || a["sender"] != "child" || a["recipient"] != "parent" || a["version"] != "1" || a["type"] != "revoke" || len(request.Children) != 1 {
					t.Error("invalid revoke request")
					return
				}
				k, err := upDownAttrs(request.Children[0], "key", "class_name", "ski")
				if err != nil || k["class_name"] != "class" || k["ski"] != ski {
					t.Error("wrong revoke target")
					return
				}
				if r.Header.Get("Content-Type") != "application/rpki-updown" || r.Header.Get("Authorization") != "" {
					t.Error("wrong headers")
					return
				}
				reply := upDownTestReply("revoke_response", `<key class_name="class" ski="`+ski+`"/>`)
				switch mode {
				case "rejection":
					reply = upDownTestReply("error_response", `<status>1302</status>`)
				case "scheduled":
					reply = upDownTestReply("error_response", `<status>1104</status>`)
				case "wrong_key":
					reply = strings.Replace(reply, ski, base64.RawURLEncoding.EncodeToString([]byte("12345678901234567890")), 1)
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
			apiConfig := certificateTestConfig(t, config)
			for i := 0; i < 2; i++ {
				var err error
				if i == 0 {
					err = client.Revoke(context.Background(), "class", ski)
				} else {
					err = revokeRPKICertificate(context.Background(), apiConfig, "class", ski, cmsTrustNow)
				}
				switch mode {
				case "success":
					if err != nil {
						t.Fatal(err)
					}
				case "rejection":
					var rejected *rpkiUpDownError
					if !errors.As(err, &rejected) || rejected.Code != 1302 {
						t.Fatalf("wrong rejection: %v", err)
					}
				default:
					if err == nil {
						t.Fatal("unconfirmed revoke accepted")
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
			uncertain := mode == "wrong_key" || mode == "scheduled"
			if uncertain {
				if calls.Load() != 1 || state.Pending == nil || !state.LastReceived.IsZero() {
					t.Fatal("uncertain request was completed or retried")
				}
			} else if calls.Load() != 2 || state.Pending != nil || !state.LastReceived.Equal(cmsTrustNow()) {
				t.Fatal("confirmed exchange not saved")
			}
		})
	}
}

func TestUpDownRevokePreflight(t *testing.T) {
	ski := base64.RawURLEncoding.EncodeToString(make([]byte, 20))
	for _, bad := range []string{"", "\x00", "\xff", strings.Repeat("x", 1025)} {
		c := rpkiUpDownClient{Exchange: rpkiHTTPExchange{MediaType: "application/rpki-updown"}, Child: "child", Parent: "parent"}
		if err := c.Revoke(context.Background(), bad, ski); !errors.Is(err, errRPKIUpDown) {
			t.Fatalf("bad class passed preflight: %v", err)
		}
		c.Child = bad
		if err := c.Revoke(context.Background(), "class", ski); !errors.Is(err, errRPKIUpDown) {
			t.Fatalf("bad child passed preflight: %v", err)
		}
	}
	for _, bad := range []string{"", strings.Repeat("A", 26), strings.Repeat("A", 28), "AAAAAAAAAAAAAAAAAAAAAAAAAAB"} {
		if _, err := upDownSKI(bad); err == nil {
			t.Fatal("invalid key identifier accepted")
		}
	}
}
