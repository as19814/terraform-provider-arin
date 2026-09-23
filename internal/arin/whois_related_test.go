package arin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func relationFixture(info whoisRelation, details bool) string {
	namespace := whoisNamespace
	if info.target == "delegation" {
		namespace = whoisRDNSNamespace
	}
	owner, target := whoisRecordSpec(info.owner), whoisRecordSpec(info.target)
	content := ""
	if details || info.relation == "parent" {
		content = whoisFixture(info.target, whoisFixtures()[info.target])
		if info.owner == "poc" {
			content = strings.Replace(content, " xmlns=", fmt.Sprintf(` pocHandle=%q pocFunction="T" xmlns=`, owner.Inputs[0].Example), 1)
		}
		if info.target == "poc" {
			content = strings.Replace(content, "</poc>", fmt.Sprintf(`<%ss><%sPocLinkRef handle=%q relPocFunction="T"/></%ss></poc>`, info.owner, info.owner, owner.Inputs[0].Example, info.owner), 1)
		}
	} else if info.target == "delegation" {
		content = `<delegationRef name="2.0.192.in-addr.arpa.">https://example.net</delegationRef>`
	} else {
		tag := info.target + "Ref"
		attrs := ""
		if info.owner == "poc" {
			tag = info.target + "PocLinkRef"
			attrs = fmt.Sprintf(` relPocHandle=%q relPocFunction="T"`, owner.Inputs[0].Example)
		}
		if info.target == "poc" {
			tag = "pocLinkRef"
			attrs = ` function="T"`
		}
		if info.target == "net" {
			attrs += ` startAddress="192.000.002.000" endAddress="192.0.2.255"`
		}
		content = fmt.Sprintf(`<%s handle=%q name="Example"%s>https://example.net</%s>`, tag, target.Inputs[0].Example, attrs, tag)
	}
	if info.relation == "parent" {
		return content
	}
	return fmt.Sprintf(`<%s xmlns=%q><limitExceeded>false</limitExceeded>%s</%s>`, info.root, namespace, content, info.root)
}
func TestWhoisRelationships(t *testing.T) {
	for _, spec := range WhoisRelationshipReads() {
		for _, details := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", spec.Name, details), func(t *testing.T) {
				info, _ := whoisRelationship(spec.Name)
				body := relationFixture(info, details)
				params := map[string]string{spec.Inputs[0].Name: spec.Inputs[0].Example, "show_details": fmt.Sprint(details)}
				ownerPath, _, _ := whoisOwnerPath(info, params)
				path := ownerPath + "/" + info.relation
				if details {
					path += "?showDetails=true"
				}
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Method != "GET" || r.URL.RequestURI() != path || r.Header.Get("Authorization") != "" {
						t.Error("unexpected relationship request")
					}
					fmt.Fprint(w, body)
				}))
				defer server.Close()
				c, _ := New(Config{APIKey: "must-not-send", WhoisBaseURL: server.URL})
				result, err := c.ReadRegistration(context.Background(), spec, params)
				if err != nil {
					t.Fatal(err)
				}
				records := result[spec.Output].([]any)
				if calls != 1 || len(records) != 1 || result["whois_xml"] != body {
					t.Fatal("lost response data or extra request")
				}
				record := records[0].(map[string]any)
				if (info.owner == "poc" || info.target == "poc") && len(record["poc_functions"].([]any)) != 1 {
					t.Fatal("lost POC association function")
				}
				if !details && info.target == "asn" && record["start_asn"] != nil {
					t.Fatal("invented ASN range for reference")
				}
				if info.target == "net" && record["start_address"] != "192.0.2.0" {
					t.Fatal("failed address normalization")
				}
			})
		}
	}
}
func TestWhoisRelationshipRolesAndOrdering(t *testing.T) {
	spec := WhoisRelationshipReads()[3]
	body := `<pocs xmlns="` + whoisNamespace + `"><pocLinkRef handle="Z-ARIN" function="T"/><pocLinkRef handle="A-ARIN" function="T"/><pocLinkRef handle="A-ARIN" function="AB"/></pocs>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
	defer server.Close()
	c, _ := New(Config{WhoisBaseURL: server.URL})
	result, err := c.ReadRegistration(context.Background(), spec, map[string]string{"handle": "EXAMPLE-1", "show_details": "false"})
	if err != nil {
		t.Fatal(err)
	}
	records := result["pocs"].([]any)
	if len(records) != 3 || records[0].(map[string]any)["handle"] != "A-ARIN" || records[0].(map[string]any)["poc_functions"].([]any)[0] != "AB" {
		t.Fatal("collapsed role links or unstable ordering")
	}
}
func TestWhoisRelationshipErrors(t *testing.T) {
	spec := WhoisRelationshipReads()[4]
	info, _ := whoisRelationship(spec.Name)
	owner := whoisFixture("org", whoisFixtures()["org"])
	noMatches := `<html><title>Whois-RWS</title><body>Sorry, no related resources were found for the handle provided.</body></html>`
	for _, tc := range []struct {
		name, body, owner string
		status, calls     int
		ok                bool
	}{
		{"empty", `<asns xmlns="` + whoisNamespace + `"><limitExceeded>false</limitExceeded></asns>`, owner, 200, 1, true},
		{"no_matches", noMatches, owner, 404, 2, true},
		{"plain_404", `Not Found`, owner, 404, 1, false},
		{"unknown_owner", noMatches, `<error/>`, 404, 2, false},
		{"wrong_owner", noMatches, whoisFixture("org", `<handle>OTHER</handle>`), 404, 2, false},
		{"forbidden", noMatches, owner, 403, 1, false},
		{"partial", `<asns xmlns="` + whoisNamespace + `"><limitExceeded>true</limitExceeded></asns>`, owner, 200, 1, false},
		{"wrong_root", `<nets xmlns="` + whoisNamespace + `"/>`, owner, 200, 1, false},
		{"wrong_record", `<asns xmlns="` + whoisNamespace + `"><orgRef handle="OTHER"/></asns>`, owner, 200, 1, false},
		{"foreign_record", `<asns xmlns="` + whoisNamespace + `"><asnRef xmlns="urn:other" handle="AS64496"/></asns>`, owner, 200, 1, false},
		{"missing_identity", `<asns xmlns="` + whoisNamespace + `"><asnRef/></asns>`, owner, 200, 1, false},
		{"nested_partial", strings.Replace(relationFixture(info, true), "</asn>", `<pocs><limitExceeded>true</limitExceeded></pocs></asn>`, 1), owner, 200, 1, false},
		{"referral", `<asns/>`, owner, 302, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path == "/rest/org/EXAMPLE-1" {
					fmt.Fprint(w, tc.owner)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c, _ := New(Config{WhoisBaseURL: server.URL})
			result, err := c.ReadRegistration(context.Background(), spec, map[string]string{"handle": "EXAMPLE-1", "show_details": "false"})
			if (err == nil) != tc.ok || calls != tc.calls {
				t.Fatalf("success=%v calls=%d: %v", err == nil, calls, err)
			}
			if tc.name == "no_matches" && (len(result["asns"].([]any)) != 0 || result["whois_xml"] != nil) {
				t.Fatal("invented XML or records for empty 404")
			}
		})
	}
}

func TestWhoisRelationshipValidation(t *testing.T) {
	for _, tc := range []struct{ name, relation, body, details string }{
		{"requested_details", "org_asns", `<asns><asnRef handle="AS64496"/></asns>`, "true"},
		{"wrong_poc", "poc_asns", `<asns><asnPocLinkRef handle="AS64496" relPocHandle="OTHER-ARIN"/></asns>`, "false"},
		{"missing_end", "org_nets", `<nets><netRef handle="NET-192-0-2-0-1" startAddress="192.0.2.0"/></nets>`, "false"},
		{"reversed_range", "org_nets", `<nets><netRef handle="NET-192-0-2-0-1" startAddress="192.0.2.255" endAddress="192.0.2.0"/></nets>`, "false"},
		{"mixed_family", "org_nets", `<nets><netRef handle="NET-192-0-2-0-1" startAddress="192.0.2.0" endAddress="2001:db8::"/></nets>`, "false"},
		{"foreign_handle", "org_asns", `<asns><asnRef xmlns:x="urn:foreign" x:handle="AS64496"/></asns>`, "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(tc.body, ">", ` xmlns="`+whoisNamespace+`">`, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			c, _ := New(Config{WhoisBaseURL: server.URL})
			for _, spec := range WhoisRelationshipReads() {
				if spec.Name != "whois_"+tc.relation {
					continue
				}
				if _, err := c.ReadRegistration(context.Background(), spec, map[string]string{"handle": spec.Inputs[0].Example, "show_details": tc.details}); err == nil {
					t.Fatal("accepted invalid relationship response")
				}
			}
		})
	}
}
