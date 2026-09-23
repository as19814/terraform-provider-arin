package arin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestWhoisNetworkQueries(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		params     map[string]string
		collection bool
	}{
		{"whois_ip", "/rest/ip/192.0.2.1", map[string]string{"address": "192.0.2.1"}, false},
		{"whois_cidr", "/rest/cidr/192.0.2.0/24", map[string]string{"prefix": "192.0.2.0/24"}, false},
		{"whois_cidr_networks", "/rest/cidr/192.0.2.0/24/less", map[string]string{"prefix": "192.0.2.0/24", "relation": "less"}, true},
		{"whois_cidr_networks", "/rest/cidr/192.0.2.0/24/more?showARIN=false&showDetails=true", map[string]string{"prefix": "192.0.2.0/24", "relation": "more", "show_arin": "false", "show_details": "true"}, true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			body := whoisFixture("net", whoisFixtures()["net"])
			if tc.collection {
				if tc.params["show_details"] != "true" {
					body = `<netRef handle="NET-192-0-2-0-1" startAddress="192.0.2.0" endAddress="192.0.2.255"/>`
				}
				body = whoisFixture("nets", `<limitExceeded>false</limitExceeded>`+body)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.RequestURI() != tc.path || r.Header.Get("Authorization") != "" {
					t.Error("unexpected Whois query")
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			c, _ := New(Config{WhoisBaseURL: server.URL, APIKey: "must-not-send"})
			for _, spec := range WhoisNetworkReads() {
				if spec.Name == tc.name {
					result, err := c.ReadRegistration(context.Background(), spec, tc.params)
					if err != nil {
						t.Fatal(err)
					}
					if result["whois_xml"] != body {
						t.Fatal("lost XML")
					}
					if tc.collection && len(result["networks"].([]any)) != 1 {
						t.Fatal("lost network")
					}
				}
			}
			if tc.params["show_details"] == "false" || tc.params["show_arin"] == "true" {
				t.Fatal("mutated caller parameters when applying defaults")
			}
		})
	}
}

func TestWhoisNetworkQueryErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body     string
		status         int
		collection, ok bool
	}{
		{"missing_ip", `<html><title>Whois-RWS</title>Sorry, no record was found for the handle provided.</html>`, 404, false, false},
		{"missing_hierarchy", `<html><title>Whois-RWS</title>Sorry, there were no results.</html>`, 404, true, true},
		{"plain_404", `Not Found`, 404, true, false},
		{"empty_hierarchy", whoisFixture("nets", `<limitExceeded>false</limitExceeded>`), 200, true, true},
		{"truncated", whoisFixture("nets", `<limitExceeded>true</limitExceeded>`), 200, true, false},
		{"missing_range", whoisFixture("nets", `<netRef handle="NET-192-0-2-0-1"/>`), 200, true, false},
		{"unrelated", whoisFixture("nets", `<netRef handle="NET-198-51-100-0-1" startAddress="198.51.100.0" endAddress="198.51.100.255"/>`), 200, true, false},
		{"wrong_family", whoisFixture("nets", `<netRef handle="NET6-2001-DB8-1" startAddress="2001:db8::" endAddress="2001:db8::ffff"/>`), 200, true, false},
		{"wrong_single", whoisFixture("net", strings.ReplaceAll(whoisFixtures()["net"], "192.0.2.", "198.51.100.")), 200, false, false},
		{"wrong_root", whoisFixture("nets", ``), 200, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			c, _ := New(Config{WhoisBaseURL: server.URL})
			spec := WhoisNetworkReads()[0]
			params := map[string]string{"address": "192.0.2.1"}
			if tc.collection {
				spec = WhoisNetworkReads()[2]
				params = map[string]string{"prefix": "192.0.2.0/24", "relation": "less"}
			}
			_, err := c.ReadRegistration(context.Background(), spec, params)
			if (err == nil) != tc.ok {
				t.Fatalf("success=%v: %v", err == nil, err)
			}
		})
	}
}

func TestWhoisNetworkBlockGaps(t *testing.T) {
	record := map[string]any{"start_address": "192.0.2.0", "end_address": "192.0.2.255", "net_blocks": []any{
		map[string]any{"start_address": "192.0.2.192", "end_address": "192.0.2.255"},
		map[string]any{"start_address": "192.0.2.0", "end_address": "192.0.2.127"},
	}}
	for _, relation := range []string{"less", "more"} {
		if err := validateWhoisNetworkMatch(record, netip.MustParseAddr("192.0.2.150"), netip.MustParseAddr("192.0.2.150"), relation); err == nil {
			t.Fatal("accepted address in a gap")
		}
		if err := validateWhoisNetworkMatch(record, netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.1"), relation); err != nil {
			t.Fatal(err)
		}
	}
	record["net_blocks"].([]any)[0].(map[string]any)["start_address"] = "192.0.2.128"
	if err := validateWhoisNetworkMatch(record, netip.MustParseAddr("192.0.2.0"), netip.MustParseAddr("192.0.2.255"), "less"); err != nil {
		t.Fatal("failed contiguous multi-block coverage", err)
	}
}

func TestWhoisShowPOCs(t *testing.T) {
	for _, details := range []bool{false, true} {
		path := "/rest/org/EXAMPLE-1?showPocs=true"
		if details {
			path = "/rest/org/EXAMPLE-1?showDetails=true&showPocs=true"
		}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RequestURI() != path {
				t.Error("wrong POC query")
			}
			fmt.Fprint(w, whoisFixture("org", whoisFixtures()["org"]+`<pocs><pocLinkRef handle="EXAMPLE-ARIN" function="T"/></pocs>`))
		}))
		c, _ := New(Config{WhoisBaseURL: server.URL})
		result, err := c.ReadRegistration(context.Background(), WhoisRecordReads()[0], map[string]string{"handle": "EXAMPLE-1", "show_pocs": "true", "show_details": fmt.Sprint(details)})
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result["whois_xml"].(string), "pocLinkRef") {
			t.Fatal("lost POCs")
		}
	}
}

func TestWhoisIPv6QueryNormalization(t *testing.T) {
	body := whoisFixture("net", `<handle>NET6-2001-DB8-1</handle><startAddress>2001:DB8::</startAddress><endAddress>2001:DB8::FFFF:FFFF:FFFF:FFFF</endAddress><version>6</version>`)
	for _, tc := range []struct {
		spec        int
		input, path string
	}{
		{0, "2001:0db8:0:0::1", "/rest/ip/2001:db8::1"},
		{1, "2001:0db8:0:0::/64", "/rest/cidr/2001:db8::/64"},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.RequestURI() != tc.path {
				t.Error("noncanonical IPv6 request")
			}
			fmt.Fprint(w, body)
		}))
		c, _ := New(Config{WhoisBaseURL: server.URL})
		spec := WhoisNetworkReads()[tc.spec]
		result, err := c.ReadRegistration(context.Background(), spec, map[string]string{spec.Inputs[0].Name: tc.input})
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		if result["start_address"] != "2001:db8::" || result["ip_version"] != "v6" {
			t.Fatal("noncanonical IPv6 result")
		}
	}
}
