package arin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRDAPDomainsByNameserver(t *testing.T) {
	fixture, err := os.ReadFile("testdata/rdap_domain.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"valid", `{"domainSearchResults":[` + strings.ReplaceAll(string(fixture), "2.0.192.in-addr.arpa.", "3.0.192.in-addr.arpa.") + `,` + string(fixture) + `]}`, 200, true},
		{"empty", `{"domainSearchResults":[]}`, 200, true},
		{"missing", `{"errorCode":404}`, 404, true},
		{"empty_missing", `{"domainSearchResults":[]}`, 404, true},
		{"plain_missing", `Not Found`, 404, false},
		{"contradiction", `{"errorCode":404,"domainSearchResults":[{}]}`, 404, false},
		{"wrong_nameserver", `{"domainSearchResults":[` + strings.ReplaceAll(string(fixture), "ns1.example.net", "ns3.example.net") + `]}`, 200, false},
		{"duplicate", `{"domainSearchResults":[` + string(fixture) + `,` + string(fixture) + `]}`, 200, false},
		{"null", `{"domainSearchResults":null}`, 200, false},
		{"wrong_shape", `{}`, 200, false},
		{"error", `{"errorCode":500,"domainSearchResults":[]}`, 200, false},
		{"partial", `{"domainSearchResults":[],"notices":[{"title":"truncated"}]}`, 200, false},
		{"next", `{"domainSearchResults":[],"links":[{"rel":"next"}]}`, 200, false},
		{"nested_partial", `{"domainSearchResults":[` + strings.Replace(string(fixture), `"ldhName":"NS2.EXAMPLE.NET."`, `"ldhName":"NS2.EXAMPLE.NET.","notices":[{"title":"truncated"}]`, 1) + `]}`, 200, false},
		{"unsupported", `{}`, 501, false}, {"referral", `{}`, 302, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.RequestURI() != "/registry/domains?nsLdhName=ns1.example.net" || r.Header.Get("Authorization") != "" {
					t.Error("unexpected search request")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "must-not-send", RDAPBaseURL: server.URL})
			record, err := c.searchRDAPDomainsByNameserver(context.Background(), "NS1.Example.NET.")
			if (err == nil) != tc.ok {
				t.Fatalf("success=%v expected=%v: %v", err == nil, tc.ok, err)
			}
			if tc.name == "valid" {
				records := record["domains"].([]any)
				if len(records) != 2 || records[0].(map[string]any)["name"] != "2.0.192.in-addr.arpa." {
					t.Fatal("incorrect sorting")
				}
			}
		})
	}
	for _, name := range []string{"", "*.example.net", "ns1.ex*", "ns1.example.net..", "ns1.example.net?x=y", "https://ns1.example.net", "münchen.example.net"} {
		if _, err := rdapNameserverName(name); err == nil {
			t.Errorf("accepted invalid name %q", name)
		}
	}
}
