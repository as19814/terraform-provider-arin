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

func TestRDAPNetworkQueries(t *testing.T) {
	for _, query := range []string{"192.0.2.1", "192.0.2.0/24", "2001:db8::1", "2001:db8::/32", "0.0.0.0/0", "::/0"} {
		if _, _, err := rdapNetworkQuery(query); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{"192.0.2.1/24", "2001:0db8::1", "fe80::1%en0", "::ffff:192.0.2.1", "../entity/OTHER", "192.0.2.0/33", "192.0.2.1?key=secret"} {
		if _, _, err := rdapNetworkQuery(query); err == nil {
			t.Fatalf("accepted invalid query %q", query)
		}
	}
}
func TestRDAPNetworkFailures(t *testing.T) {
	fixture, err := os.ReadFile("testdata/rdap_network.json")
	if err != nil {
		t.Fatal(err)
	}
	good := string(fixture)
	cases := map[string]string{
		"mismatch":       strings.ReplaceAll(good, "192.0.2.", "198.51.100."),
		"class":          strings.Replace(good, "ip network", "entity", 1),
		"family":         strings.Replace(good, `"ipVersion":"v4"`, `"ipVersion":"v6"`, 1),
		"range":          strings.Replace(good, `"endAddress":"192.0.2.255"`, `"endAddress":"192.0.1.255"`, 1),
		"cidr_gap":       strings.Replace(good, `"length":24`, `"length":25`, 1),
		"cidr_overflow":  strings.Replace(good, `"length":24`, `"length":23`, 1),
		"cidr_duplicate": strings.Replace(good, `{"v4prefix":"192.0.2.0","length":24}`, `{"v4prefix":"192.0.2.0","length":24},{"v4prefix":"192.0.2.0","length":24}`, 1),
		"truncated":      strings.Replace(good, `"status":["active"]`, `"notices":[{"type":"result set truncated"}]`, 1),
		"pagination":     strings.Replace(good, `"status":["active"]`, `"links":[{"rel":"next","href":"https://example.net"}]`, 1),
		"registrant":     strings.Replace(good, `"handle":"EXAMPLE-1"`, `"handle":""`, 1),
		"invalid_json":   "null",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			c, _ := New(Config{RDAPBaseURL: server.URL})
			if _, err := c.readRDAPNetwork(context.Background(), "192.0.2.1"); err == nil {
				t.Fatal("accepted invalid, mismatched or incomplete network")
			}
		})
	}
}
func TestRDAPNetworkIPv6AndFullPrefix(t *testing.T) {
	body := `{"objectClassName":"ip network","handle":"NET6-EXAMPLE","ipVersion":"v6","startAddress":"2001:db8::","endAddress":"2001:db8::ffff","cidr0_cidrs":[{"v6prefix":"2001:db8::8000","length":113},{"v6prefix":"2001:db8::","length":113}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Method != "GET" {
			t.Error("public lookup sent credentials or mutated state")
		}
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "must-not-send", RDAPBaseURL: server.URL})
	for _, query := range []string{"2001:db8::1", "2001:db8::/112"} {
		record, err := c.readRDAPNetwork(context.Background(), query)
		if err != nil || len(record["cidrs"].([]any)) != 2 || record["country"] != nil || record["parent_handle"] != nil {
			t.Fatalf("IPv6 lookup failed: %v", err)
		}
	}
	if _, err := c.readRDAPNetwork(context.Background(), "2001:db8::/111"); err == nil {
		t.Fatal("lookup only validated the first address of the requested prefix")
	}
}
func TestRDAPNetworkReferralNotFollowed(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Location", "/other")
		w.WriteHeader(301)
	}))
	defer server.Close()
	c, _ := New(Config{RDAPBaseURL: server.URL})
	if _, err := c.readRDAPNetwork(context.Background(), "192.0.2.1"); err == nil || calls != 1 {
		t.Fatal("followed referral or accepted redirect")
	}
}
