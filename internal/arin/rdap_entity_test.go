package arin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRDAPEntity(t *testing.T) {
	body, err := os.ReadFile("testdata/rdap_entity.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.RequestURI() != "/registry/entity/example-1" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected authenticated or non-lookup request")
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "must-not-send", RDAPBaseURL: server.URL})
	got, err := c.readRDAPEntity(context.Background(), "example-1")
	if err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "org" || len(got["names"].([]any)) != 2 || got["emails"].([]any)[0] != "noc@example.net" || len(got["phones"].([]any)) != 2 || got["address_labels"].([]any)[0] != "Example Street\nExample City" {
		t.Fatal("contact fields lost")
	}
	if got["entities"].([]any)[0].(map[string]any)["roles"].([]any)[0] != "abuse" {
		t.Fatal("missing roles")
	}
	if !strings.Contains(got["vcard_json"].(string), "x-example") || !strings.Contains(got["rdap_json"].(string), "9007199254740993") {
		t.Fatal("raw fields lost")
	}
}

func TestRDAPEntityRejectsInvalidResponses(t *testing.T) {
	for name, body := range map[string]string{
		"class":         `{"handle":"EXAMPLE-1","objectClassName":"autnum"}`,
		"handle":        `{"handle":"OTHER","objectClassName":"entity"}`,
		"json":          `{`,
		"truncated":     `{"handle":"EXAMPLE-1","objectClassName":"entity","notices":[{"type":"result set truncated"}]}`,
		"nested":        `{"handle":"EXAMPLE-1","objectClassName":"entity","entities":[{"handle":"POC","entities":[{"remarks":[{"title":"Incomplete"}]}]}]}`,
		"paginated":     `{"handle":"EXAMPLE-1","objectClassName":"entity","entities":[{"handle":"POC","links":[{"rel":"next"}]}]}`,
		"missing_ref":   `{"handle":"EXAMPLE-1","objectClassName":"entity","entities":[{}]}`,
		"duplicate_ref": `{"handle":"EXAMPLE-1","objectClassName":"entity","entities":[{"handle":"POC"},{"handle":"poc"}]}`,
		"card":          `{"handle":"EXAMPLE-1","objectClassName":"entity","vcardArray":["vcard",[["fn",{},"text",{}]]]}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			c, _ := New(Config{RDAPBaseURL: server.URL})
			if _, err := c.readRDAPEntity(context.Background(), "EXAMPLE-1"); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}
func TestRDAPContact(t *testing.T) {
	for _, raw := range []string{"", "null"} {
		got, err := rdapContact(json.RawMessage(raw))
		if err != nil || got["kind"] != nil || got["vcard_json"] != nil || len(got["names"].([]any)) != 0 {
			t.Fatal("optional card mishandled")
		}
	}
	for _, raw := range []string{`{}`, `[]`, `["wrong",[]]`, `["vcard",null]`, `["vcard",[["fn",null,"text","Example"]]]`, `["vcard",[["fn",{},"text",null]]]`, `["vcard",[["tel",{},"text",2]]]`, `["vcard",[["adr",{"label":[]},"text",[]]]]`} {
		if _, err := rdapContact(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted malformed card %s", raw)
		}
	}
}

func TestRDAPEntityHTTPFailures(t *testing.T) {
	for _, status := range []int{301, 404, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			followed := false
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed = true; t.Error("followed referral") }))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
			}))
			defer server.Close()
			c, _ := New(Config{RDAPBaseURL: server.URL})
			_, err := c.readRDAPEntity(context.Background(), "EXAMPLE-1")
			if err == nil || followed {
				t.Fatal("HTTP failure accepted")
			}
			if status == 404 && !IsNotFound(err) {
				t.Fatal("lost not-found status")
			}
		})
	}
}
