package arin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const networkV4 = `{"objectClassName":"ip network","handle":"NET-192-0-2-0-1","name":"EXAMPLE-V4","startAddress":"192.0.2.0","endAddress":"192.0.2.255","ipVersion":"v4","type":"DIRECT ALLOCATION","entities":[{"handle":"EXAMPLE-1","roles":["registrant"]}],"cidr0_cidrs":[{"v4prefix":"192.0.2.0","length":24}]}`
const networkV6 = `{"objectClassName":"ip network","handle":"NET6-2001-DB8-1","name":"EXAMPLE-V6","startAddress":"2001:DB8::","endAddress":"2001:DB8:FFFF:FFFF:FFFF:FFFF:FFFF:FFFF","ipVersion":"v6","type":"ASSIGNMENT","entities":[{"handle":"EXAMPLE-1","roles":["registrant"]}],"cidr0_cidrs":[{"v6prefix":"2001:DB8::","length":32}]}`

func TestListOrganizationNetworks(t *testing.T) {
	t.Parallel()
	contactOnly := strings.Replace(networkV4, `"registrant"`, `"technical"`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/registry/ips/reverse_search/entity" || r.URL.Query().Get("handle") != "EXAMPLE-1" || len(r.URL.Query()) != 1 {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("sent API key to public RDAP")
		}
		if r.Header.Get("Accept") != "application/rdap+json" {
			t.Error("incorrect Accept header")
		}
		fmt.Fprintf(w, `{"ipSearchResults":[%s,%s,%s]}`, networkV6, contactOnly, networkV4)
	}))
	defer server.Close()
	c, err := New(Config{APIKey: "secret", RDAPBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	nets, err := c.ListOrganizationNetworks(context.Background(), "EXAMPLE-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 || nets[0].Name != "EXAMPLE-V4" || nets[1].CIDRs[0] != "2001:db8::/32" || nets[1].StartAddress != "2001:db8::" {
		t.Fatalf("unexpected networks: %+v", nets)
	}
}

func TestNetworkSearchResponses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		wantError  bool
	}{
		{"empty", `{"ipSearchResults":[]}`, false},
		{"wrong payload", `{}`, true},
		{"null", `{"ipSearchResults":null}`, true},
		{"malformed", `{`, true},
		{"truncated", `{"ipSearchResults":[],"notices":[{"type":"result set truncated due to excessive load"}]}`, true},
		{"truncated description", `{"ipSearchResults":[],"notices":[{"description":["Results have been truncated"]}]}`, true},
		{"pagination", `{"ipSearchResults":[],"links":[{"rel":"next","href":"https://example.com"}]}`, true},
		{"duplicate handles", `{"ipSearchResults":[` + networkV4 + `,` + networkV4 + `]}`, true},
		{"invalid range", `{"ipSearchResults":[` + strings.Replace(networkV4, "192.0.2.255", "192.0.1.255", 1) + `]}`, true},
		{"invalid version", `{"ipSearchResults":[` + strings.Replace(networkV4, `"v4"`, `"v6"`, 1) + `]}`, true},
		{"CIDR beyond range", `{"ipSearchResults":[` + strings.Replace(networkV4, `"length":24`, `"length":23`, 1) + `]}`, true},
		{"invalid CIDR", `{"ipSearchResults":[` + strings.Replace(networkV4, `"length":24`, `"length":129`, 1) + `]}`, true},
		{"unrelated terms notice", `{"ipSearchResults":[],"notices":[{"title":"Terms of Service"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
			defer server.Close()
			c, err := New(Config{RDAPBaseURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.ListOrganizationNetworks(context.Background(), "EXAMPLE-1")
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected result: %+v, %v", result, err)
			}
		})
	}
}

func TestNoNetworksVersusUnknownOrganization(t *testing.T) {
	t.Parallel()
	for _, exists := range []bool{true, false} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/registry/entity/EXAMPLE-1" && exists {
				fmt.Fprint(w, `{"objectClassName":"entity","handle":"EXAMPLE-1"}`)
				return
			}
			w.WriteHeader(404)
			fmt.Fprint(w, `{"errorCode":404,"title":"Not Found"}`)
		}))
		c, err := New(Config{RDAPBaseURL: server.URL})
		if err != nil {
			t.Fatal(err)
		}
		nets, err := c.ListOrganizationNetworks(context.Background(), "EXAMPLE-1")
		if exists && (err != nil || len(nets) != 0) {
			t.Fatalf("expected empty inventory: %v", err)
		}
		if !exists && !IsNotFound(err) {
			t.Fatalf("expected missing org error: %v", err)
		}
		server.Close()
	}
}

func TestRDAPConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ base, rdap, want string }{
		{"", "", RDAPProductionURL}, {OTEURL, "", RDAPOTEURL}, {OTEURL + "/", "", RDAPOTEURL}, {OTEURL, "https://example.net", "https://example.net"}, {"http://localhost:1234", "", ""},
	} {
		c, err := New(Config{BaseURL: tc.base, RDAPBaseURL: tc.rdap})
		if err != nil {
			t.Fatal(err)
		}
		if c.rdapBaseURL != tc.want {
			t.Fatalf("wrong RDAP default: %s", c.rdapBaseURL)
		}
		if _, err = c.GetOrganization(context.Background(), "EXAMPLE-1"); err == nil || !strings.Contains(err.Error(), "API_KEY") {
			t.Fatalf("missing key should block Reg-RWS: %v", err)
		}
		if tc.want == "" {
			if _, err = c.ListOrganizationNetworks(context.Background(), "EXAMPLE-1"); err == nil {
				t.Fatal("custom origin used implicit production RDAP")
			}
		}
	}
	if _, err := New(Config{RDAPBaseURL: "http://example.net"}); err == nil {
		t.Fatal("accepted insecure RDAP origin")
	}
}

func TestRDAPErrorRedaction(t *testing.T) {
	t.Parallel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_ = json.NewEncoder(w).Encode(map[string]any{"title": "Forbidden", "description": []string{"secret-key not permitted"}})
	}))
	defer s.Close()
	c, err := New(Config{APIKey: "secret-key", RDAPBaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ListOrganizationNetworks(context.Background(), "EXAMPLE-1")
	if err == nil || strings.Contains(err.Error(), "secret-key") || !strings.Contains(err.Error(), "403") {
		t.Fatalf("unexpected error: %v", err)
	}
}
