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
	network, err := os.ReadFile("testdata/rdap_network.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Method != "GET" {
			t.Error("public read sent credentials or used wrong method")
		}
		switch r.URL.RequestURI() {
		case "/registry/entities?handle=EXAMPLE-1":
			fmt.Fprintf(w, `{"entitySearchResults":[%s]}`, entity)
		case "/registry/domains/rirSearch1/rdap-up/2.0.192.in-addr.arpa.":
			fmt.Fprint(w, `{"objectClassName":"domain","ldhName":"192.in-addr.arpa."}`)
		case "/registry/domain/2.0.192.in-addr.arpa.":
			fmt.Fprint(w, `{"objectClassName":"domain","ldhName":"2.0.192.in-addr.arpa."}`)
		case "/registry/ips?handle=NET-192-0-2-0-1":
			fmt.Fprintf(w, `{"ipSearchResults":[%s]}`, network)
		case "/registry/autnums?handle=AS64496":
			fmt.Fprintf(w, `{"autnumSearchResults":[%s]}`, asn)
		case "/registry/ip/192.0.2.1", "/registry/ips/rirSearch1/rdap-up/192.0.2.1":
			_, _ = w.Write(network)
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
		params := map[string]string{"role": "any", "relation": "up", "active_only": "false", "name": "2.0.192.in-addr.arpa.", "handle": "EXAMPLE-1", "org_handle": "EXAMPLE-1", "asn": "64496", "query": "192.0.2.1"}
		if s.Name == "rdap_entities" {
			params["search_by"] = "handle"
			params["query"] = "EXAMPLE-1"
		}
		if s.Name == "rdap_networks" {
			params["search_by"] = "handle"
			params["query"] = "NET-192-0-2-0-1"
		}
		if s.Name == "rdap_asns" {
			params["search_by"] = "handle"
			params["query"] = "AS64496"
		}
		result, err := c.ReadRegistration(context.Background(), s, params)
		if err != nil {
			t.Fatal(err)
		}
		switch s.Name {
		case "rdap_networks", "rdap_asns", "rdap_network_hierarchy":
			if len(result[s.Output].([]any)) != 1 {
				t.Fatal("missing resource search result")
			}
		case "rdap_domains":
			if len(result["domains"].([]any)) != 1 {
				t.Fatal("missing domain hierarchy result")
			}
		case "rdap_domain":
			if result["name"] != "2.0.192.in-addr.arpa." {
				t.Fatal("incorrect domain")
			}
		case "rdap_entities":
			if len(result["entities"].([]any)) != 1 {
				t.Fatal("missing entity search result")
			}
		case "rdap_entity":
			if result["handle"] != "EXAMPLE-1" || result["vcard_json"] != nil || len(result["emails"].([]any)) != 0 {
				t.Fatal("incorrect entity with omitted contact fields")
			}
		case "rdap_network":
			if result["handle"] != "NET-192-0-2-0-1" {
				t.Fatal("incorrect network")
			}
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
				if _, err := c.ReadRegistration(context.Background(), s, map[string]string{"handle": "EXAMPLE-1", "org_handle": "EXAMPLE-1", "asn": "64496"}); err == nil {
					t.Errorf("%s accepted incomplete or mismatched response", s.Name)
				}
			}
		}
		server.Close()
	}
}
