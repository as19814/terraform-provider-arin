package arin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func whoisFixture(root, content string) string {
	namespace := whoisNamespace
	if root == "delegation" {
		namespace = whoisRDNSNamespace
	}
	return `<` + root + ` xmlns="` + namespace + `">` + content + `</` + root + `>`
}
func whoisFixtures() map[string]string {
	return map[string]string{
		"org":        `<handle>EXAMPLE-1</handle><name>Example</name><canAllocate>Y</canAllocate><streetAddress><line number="1">Second</line><line number="0">First</line></streetAddress>`,
		"customer":   `<handle>C00000001</handle><name>Example Customer</name><iso3166-1><code2>US</code2></iso3166-1>`,
		"poc":        `<handle>EXAMPLE-ARIN</handle><firstName>Ada</firstName><lastName>Example</lastName><isRoleAccount>N</isRoleAccount><emails><email>noc@example.net</email></emails><phones><phone><number>+1-555-0100</number><type><code>O</code></type></phone></phones>`,
		"asn":        `<handle>AS64496</handle><name>EXAMPLE-AS</name><startAsNumber>64496</startAsNumber><endAsNumber>64497</endAsNumber><orgRef handle="EXAMPLE-1">https://example.net</orgRef>`,
		"net":        `<handle>NET-192-0-2-0-1</handle><name>EXAMPLE-NET</name><startAddress>192.000.002.000</startAddress><endAddress>192.0.2.255</endAddress><version>4</version><netBlocks><netBlock><startAddress>192.0.2.0</startAddress><endAddress>192.0.2.255</endAddress><cidrLength>24</cidrLength><type>DA</type></netBlock></netBlocks>`,
		"delegation": `<name>2.0.192.in-addr.arpa.</name><nameservers><nameserver>NS1.EXAMPLE.NET</nameserver></nameservers><delegationKeys><delegationKey><keyTag>12345</keyTag><algorithm>13</algorithm><digestType>2</digestType><digest>abcd</digest></delegationKey></delegationKeys>`,
	}
}
func TestWhoisLookups(t *testing.T) {
	for _, spec := range WhoisRecordReads() {
		t.Run(spec.Name, func(t *testing.T) {
			kind := strings.TrimPrefix(spec.Name, "whois_")
			pathKind := kind
			identity := spec.Inputs[0].Example
			if kind == "delegation" {
				pathKind = "rdns"
				identity = strings.TrimSuffix(identity, ".")
			}
			body := whoisFixture(spec.Root, whoisFixtures()[kind]+`<extension xmlns="urn:example">keep-me</extension>`)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.RequestURI() != "/rest/"+pathKind+"/"+identity+"?showDetails=true" || r.Header.Get("Authorization") != "" || r.Header.Get("Accept") != "application/xml" {
					t.Error("unexpected Whois request")
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "must-not-send", WhoisBaseURL: server.URL})
			params := map[string]string{spec.Inputs[0].Name: spec.Inputs[0].Example, "show_details": "true"}
			record, err := c.ReadRegistration(context.Background(), spec, params)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || record["whois_xml"] != body {
				t.Fatal("extra requests or incomplete raw XML")
			}
			if kind == "net" && (record["ip_version"] != "v4" || record["start_address"] != "192.0.2.0") {
				t.Fatal("network normalization failed")
			}
			if kind == "org" && record["street_address"].([]any)[0] != "First" {
				t.Fatal("incorrect line ordering")
			}
		})
	}
}
func TestWhoisResponseValidation(t *testing.T) {
	spec := WhoisRecordReads()[0]
	base := whoisFixtures()["org"]
	for name, body := range map[string]string{
		"identity":              whoisFixture("org", strings.Replace(base, "EXAMPLE-1", "OTHER-1", 1)),
		"type":                  whoisFixture("customer", base),
		"namespace":             `<org xmlns="urn:bad"><handle>EXAMPLE-1</handle></org>`,
		"missing":               whoisFixture("org", `<name>Example</name>`),
		"spoofed_reg_namespace": whoisFixture("org", `<handle xmlns="http://www.arin.net/regrws/core/v1">EXAMPLE-1</handle>`),
		"spoofed":               whoisFixture("org", `<handle xmlns="urn:bad">EXAMPLE-1</handle>`),
		"duplicate":             whoisFixture("org", base+`<handle>EXAMPLE-1</handle>`),
		"nested_truncation":     whoisFixture("org", base+`<resources><nets><limitExceeded>true</limitExceeded></nets></resources>`),
		"bad_limit":             whoisFixture("org", base+`<resources><limitExceeded>maybe</limitExceeded></resources>`),
		"directive":             `<!DOCTYPE org SYSTEM "https://example.net">` + whoisFixture("org", base),
		"multiple":              whoisFixture("org", base) + whoisFixture("org", base),
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			c, _ := New(Config{WhoisBaseURL: server.URL})
			if _, err := c.ReadRegistration(context.Background(), spec, map[string]string{"handle": "EXAMPLE-1", "show_details": "false"}); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
	for _, status := range []int{302, 403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/elsewhere")
				w.WriteHeader(status)
				fmt.Fprint(w, whoisFixture("org", base))
			}))
			defer server.Close()
			c, _ := New(Config{WhoisBaseURL: server.URL})
			if _, err := c.ReadRegistration(context.Background(), spec, map[string]string{"handle": "EXAMPLE-1", "show_details": "false"}); err == nil || calls != 1 {
				t.Fatal("ignored status or followed a redirect")
			}
		})
	}
}
func TestWhoisOrigins(t *testing.T) {
	for _, tc := range []struct{ base, override, want string }{{"", "", WhoisProductionURL}, {OTEURL, "", WhoisOTEURL}, {OTEURL, "https://example.net", "https://example.net"}, {"http://localhost:1234", "", ""}} {
		c, err := New(Config{BaseURL: tc.base, WhoisBaseURL: tc.override})
		if err != nil || c.whoisBaseURL != tc.want {
			t.Fatal("incorrect Whois origin")
		}
		if tc.want == "" {
			if _, err := c.ReadRegistration(context.Background(), WhoisRecordReads()[0], map[string]string{"handle": "EXAMPLE-1", "show_details": "false"}); err == nil {
				t.Fatal("custom origin fell back to production")
			}
		}
	}
	for _, origin := range []string{"http://example.net", "https://user:pass@example.net", "https://example.net/path", "https://example.net?key=value"} {
		if _, err := New(Config{WhoisBaseURL: origin}); err == nil {
			t.Fatal("accepted invalid Whois origin")
		}
	}
}

func TestWhoisInvalidTypedFields(t *testing.T) {
	for _, tc := range []struct{ kind, from, to string }{
		{"org", "<canAllocate>Y", "<canAllocate>maybe"},
		{"asn", "<endAsNumber>64497", "<endAsNumber>64495"},
		{"asn", "<endAsNumber>64497", "<endAsNumber>4294967296"},
		{"net", "<version>4", "<version>6"},
		{"net", "<cidrLength>24", "<cidrLength>25"},
		{"net", "<cidrLength>24", "<cidrLength>129"},
		{"delegation", "<keyTag>12345", "<keyTag>65536"},
		{"delegation", "<digest>abcd", "<digest>xyz"},
	} {
		t.Run(tc.kind+tc.to, func(t *testing.T) {
			var spec ReadSpec
			for _, s := range WhoisRecordReads() {
				if s.Name == "whois_"+tc.kind {
					spec = s
				}
			}
			body := whoisFixture(tc.kind, strings.Replace(whoisFixtures()[tc.kind], tc.from, tc.to, 1))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			c, _ := New(Config{WhoisBaseURL: server.URL})
			if _, err := c.ReadRegistration(context.Background(), spec, map[string]string{spec.Inputs[0].Name: spec.Inputs[0].Example, "show_details": "false"}); err == nil {
				t.Fatal("accepted invalid typed fields")
			}
		})
	}
}
