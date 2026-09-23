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

func TestRDAPDomain(t *testing.T) {
	fixture, err := os.ReadFile("testdata/rdap_domain.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.RequestURI() != "/registry/domain/2.0.192.in-addr.arpa." || r.Header.Get("Authorization") != "" {
			t.Error("unexpected domain lookup")
		}
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "must-not-send", RDAPBaseURL: server.URL})
	got, err := c.readRDAPDomain(context.Background(), "2.0.192.IN-ADDR.ARPA")
	if err != nil {
		t.Fatal(err)
	}
	if got["name"] != "2.0.192.in-addr.arpa." || got["zone_signed"] != false || got["delegation_signed"] != true || got["max_sig_life"] != int64(604800) || got["network_handle"] != "NET-192-0-2-0-1" || got["org_handles"].([]any)[0] != "EXAMPLE-1" {
		t.Fatal("incorrect domain metadata")
	}
	ns := got["nameservers"].([]any)[0].(map[string]any)
	if ns["name"] != "ns1.example.net" || ns["ipv4_addresses"].([]any)[0] != "192.0.2.1" || ns["ipv6_addresses"].([]any)[0] != "2001:db8::1" {
		t.Fatal("incorrect nameservers/glue")
	}
	if got["ds_records"].([]any)[0].(map[string]any)["key_tag"] != int64(12345) || got["key_records"].([]any)[0].(map[string]any)["public_key"] != "AQIDBA==" {
		t.Fatal("missing DNSSEC data")
	}
	if !strings.Contains(got["rdap_json"].(string), "9007199254740993") {
		t.Fatal("lost extension value")
	}
}

func TestRDAPDomainOptionalFields(t *testing.T) {
	got, err := decodeRDAPDomain([]byte(`{"objectClassName":"domain","ldhName":"8.B.D.0.1.0.0.2.IP6.ARPA"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"handle", "unicode_name", "zone_signed", "delegation_signed", "max_sig_life", "network_handle"} {
		if got[field] != nil {
			t.Errorf("%s should be null", field)
		}
	}
	for _, field := range []string{"nameservers", "org_handles", "ds_records", "key_records", "events"} {
		if len(got[field].([]any)) != 0 {
			t.Errorf("%s should be empty", field)
		}
	}
	if got["name"] != "8.b.d.0.1.0.0.2.ip6.arpa." {
		t.Fatal("IPv6 name not normalized")
	}
	got, err = decodeRDAPDomain([]byte(`{"objectClassName":"domain","ldhName":"2.0.192.in-addr.arpa.","secureDNS":{"dsData":[{"keyTag":0}],"keyData":[{"flags":0}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	ds := got["ds_records"].([]any)[0].(map[string]any)
	if ds["key_tag"] != int64(0) || ds["digest"] != nil || ds["algorithm"] != nil {
		t.Fatal("optional DS fields were fabricated")
	}
}

func TestRDAPDomainInvalidResponses(t *testing.T) {
	fixture, err := os.ReadFile("testdata/rdap_domain.json")
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"class":        func(v map[string]any) { v["objectClassName"] = "entity" },
		"name":         func(v map[string]any) { v["ldhName"] = "example.net" },
		"missing_name": func(v map[string]any) { delete(v, "ldhName") },
		"ns_class":     func(v map[string]any) { v["nameservers"].([]any)[0].(map[string]any)["objectClassName"] = "domain" },
		"duplicate_ns": func(v map[string]any) { v["nameservers"].([]any)[0].(map[string]any)["ldhName"] = "NS1.EXAMPLE.NET." },
		"glue_family": func(v map[string]any) {
			v["nameservers"].([]any)[0].(map[string]any)["ipAddresses"] = map[string]any{"v4": []string{"2001:db8::1"}}
		},
		"network_class": func(v map[string]any) { v["network"].(map[string]any)["objectClassName"] = "autnum" },
		"missing_org":   func(v map[string]any) { delete(v["entities"].([]any)[0].(map[string]any), "handle") },
		"ds_range": func(v map[string]any) {
			v["secureDNS"].(map[string]any)["dsData"].([]any)[0].(map[string]any)["keyTag"] = 65536
		},
		"ds_digest": func(v map[string]any) {
			v["secureDNS"].(map[string]any)["dsData"].([]any)[0].(map[string]any)["digest"] = "not hex"
		},
		"key_range": func(v map[string]any) {
			v["secureDNS"].(map[string]any)["keyData"].([]any)[0].(map[string]any)["protocol"] = -1
		},
		"lifetime":     func(v map[string]any) { v["secureDNS"].(map[string]any)["maxSigLife"] = -1 },
		"flag_type":    func(v map[string]any) { v["secureDNS"].(map[string]any)["zoneSigned"] = "false" },
		"root_partial": func(v map[string]any) { v["notices"] = []any{map[string]any{"type": "result truncated"}} },
		"ns_partial": func(v map[string]any) {
			v["nameservers"].([]any)[0].(map[string]any)["notices"] = []any{map[string]any{"title": "Incomplete"}}
		},
		"network_partial": func(v map[string]any) { v["network"].(map[string]any)["links"] = []any{map[string]any{"rel": "next"}} },
		"dnssec_partial": func(v map[string]any) {
			v["secureDNS"].(map[string]any)["dsData"].([]any)[0].(map[string]any)["remarks"] = []any{map[string]any{"title": "truncated"}}
		},
		"entity_partial": func(v map[string]any) {
			v["entities"].([]any)[0].(map[string]any)["entities"].([]any)[0].(map[string]any)["links"] = []any{map[string]any{"rel": "next"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(fixture, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			body, _ := json.Marshal(value)
			if _, err := decodeRDAPDomain(body); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
	for _, body := range []string{`{`, `null`, `[]`, `{"objectClassName":"domain","ldhName":"2.0.192.in-addr.arpa.","nameservers":[null]}`} {
		if _, err := decodeRDAPDomain([]byte(body)); err == nil {
			t.Fatal("accepted invalid JSON structure")
		}
	}
}

func TestRDAPDomainLookupErrors(t *testing.T) {
	for _, status := range []int{200, 302, 404, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			referral := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed referral") }))
			defer referral.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", referral.URL)
				w.WriteHeader(status)
				fmt.Fprint(w, `{"objectClassName":"domain","ldhName":"3.0.192.in-addr.arpa."}`)
			}))
			defer server.Close()
			c, _ := New(Config{RDAPBaseURL: server.URL})
			if _, err := c.readRDAPDomain(context.Background(), "2.0.192.in-addr.arpa."); err == nil {
				t.Fatal("accepted mismatch or HTTP error")
			}
		})
	}
}
func TestRDAPDomainName(t *testing.T) {
	for _, name := range []string{"2.0.192.in-addr.arpa.", "2.0.192.IN-ADDR.ARPA", "0-127.2.0.192.in-addr.arpa.", "8.b.d.0.1.0.0.2.ip6.arpa.", "ip6.arpa"} {
		if _, err := rdapDomainName(name); err != nil {
			t.Errorf("rejected %s", name)
		}
	}
	for _, name := range []string{"", "example.net", "2.0.192.in-addr.arpa..", "2.0.192.in-addr.arpa./x", "2.0.192.in-addr.arpa?key=1", "*.ip6.arpa", "_invalid.ip6.arpa", "2..in-addr.arpa", strings.Repeat("a", 64) + ".ip6.arpa"} {
		if _, err := rdapDomainName(name); err == nil {
			t.Errorf("accepted %s", name)
		}
	}
}
