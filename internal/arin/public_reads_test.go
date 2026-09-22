package arin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestPublicReads(t *testing.T) {
	asn, err := os.ReadFile("testdata/asn.json")
	if err != nil {
		t.Fatal(err)
	}
	entity, err := os.ReadFile("testdata/entity.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Method != "GET" {
			t.Error("public read sent credentials or used wrong method")
		}
		switch r.URL.RequestURI() {
		case "/registry/autnum/64496":
			_, _ = w.Write(asn)
		case "/registry/autnums/reverse_search/entity?handle=EXAMPLE-1":
			fmt.Fprintf(w, `{"autnumSearchResults":[%s]}`, asn)
		case "/registry/entity/EXAMPLE-1":
			_, _ = w.Write(entity)
		default:
			t.Errorf("unexpected public read %s", r.URL.RequestURI())
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c, err := New(Config{APIKey: "secret", RDAPBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range PublicReads() {
		result, err := c.ReadRegistration(context.Background(), s, map[string]string{"org_handle": "EXAMPLE-1", "asn": "64496"})
		if err != nil {
			t.Fatal(err)
		}
		switch s.Name {
		case "asn":
			if result["start_asn"] != int64(64496) {
				t.Fatal("incorrect ASN")
			}
		case "asns":
			if len(result["asns"].([]any)) != 1 {
				t.Fatal("missing ASN")
			}
		case "org_pocs":
			if result["pocs"].([]any)[0].(map[string]any)["handle"] != "EXAMPLE-ARIN" {
				t.Fatal("missing POC")
			}
		}
	}
}

func TestPublicIncompleteAndMismatchedRecords(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"asn", `{"objectClassName":"autnum","handle":"AS1","startAutnum":1,"endAutnum":1}`},
		{"asns", `{"autnumSearchResults":[],"notices":[{"type":"result set truncated due to excessive load"}]}`},
		{"asns", `{"autnumSearchResults":[],"links":[{"rel":"next"}]}`},
		{"asns", `{}`},
		{"org_pocs", `{"objectClassName":"entity","handle":"WRONG"}`},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
		c, _ := New(Config{RDAPBaseURL: server.URL})
		for _, s := range PublicReads() {
			if s.Name == tc.name {
				if _, err := c.ReadRegistration(context.Background(), s, map[string]string{"org_handle": "EXAMPLE-1", "asn": "64496"}); err == nil {
					t.Errorf("%s accepted incomplete or mismatched response", s.Name)
				}
			}
		}
		server.Close()
	}
}
