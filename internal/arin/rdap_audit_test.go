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

func TestRDAPCompleteRegistrationJSON(t *testing.T) {
	asn, err := os.ReadFile("testdata/asn.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body string
		decode     func([]byte) (map[string]any, error)
	}{
		{"network", networkV4, decodeRDAPNetwork}, {"asn", string(asn), decodeRDAPASN},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.TrimSuffix(strings.TrimSpace(tc.body), "}") + `,"extension":{"integer":9007199254740993},"links":[{"rel":"self","href":"https://example.net"}]}`
			record, err := tc.decode([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			var original, returned map[string]json.RawMessage
			if json.Unmarshal([]byte(body), &original) != nil || json.Unmarshal([]byte(record["rdap_json"].(string)), &returned) != nil {
				t.Fatal("invalid raw JSON")
			}
			if string(original["extension"]) != string(returned["extension"]) || len(original) != len(returned) {
				t.Fatal("raw JSON lost fields or number precision")
			}
			malformed := strings.TrimSuffix(body, "}") + `,"entities":[{"networks":[{"links":[{"rel":"next"}]}]}]}`
			if _, err := tc.decode([]byte(malformed)); err == nil {
				t.Fatal("accepted partial embedded registration")
			}
		})
	}
	record, err := decodeRDAPASN([]byte(strings.TrimSuffix(strings.TrimSpace(string(asn)), "}") + `,"type":"DIRECT ALLOCATION"}`))
	if err != nil || record["asn_type"] != "DIRECT ALLOCATION" {
		t.Fatal("lost ASN registration type")
	}
}

func TestRDAPInventoryRejectsAmbiguousErrors(t *testing.T) {
	for _, family := range []string{"networks", "asns"} {
		for _, tc := range []struct {
			name, body, entity string
			ok                 bool
		}{
			{"confirmed", `{"errorCode":404}`, `{"objectClassName":"entity","handle":"EXAMPLE-1"}`, true},
			{"plain", `Not Found`, `{"objectClassName":"entity","handle":"EXAMPLE-1"}`, false},
			{"wrong_code", `{"errorCode":500}`, `{"objectClassName":"entity","handle":"EXAMPLE-1"}`, false},
			{"contradictory", `{"errorCode":404,"ipSearchResults":[{}]}`, `{"objectClassName":"entity","handle":"EXAMPLE-1"}`, false},
			{"partial", `{"errorCode":404,"notices":[{"title":"truncated"}]}`, `{"objectClassName":"entity","handle":"EXAMPLE-1"}`, false},
			{"entity_partial", `{"errorCode":404}`, `{"objectClassName":"entity","handle":"EXAMPLE-1","entities":[{"autnums":[{"notices":[{"title":"truncated"}]}]}]}`, false},
			{"entity_wrong", `{"errorCode":404}`, `{"objectClassName":"entity","handle":"WRONG"}`, false},
		} {
			t.Run(family+tc.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/registry/entity/EXAMPLE-1" {
						fmt.Fprint(w, tc.entity)
						return
					}
					w.WriteHeader(404)
					fmt.Fprint(w, tc.body)
				}))
				defer server.Close()
				c, _ := New(Config{RDAPBaseURL: server.URL})
				var err error
				if family == "networks" {
					_, err = c.ListOrganizationNetworks(context.Background(), "EXAMPLE-1")
				} else {
					_, err = c.readPublic(context.Background(), ReadSpec{Name: "asns"}, map[string]string{"org_handle": "EXAMPLE-1"})
				}
				if (err == nil) != tc.ok {
					t.Fatalf("success=%v expected %v", err == nil, tc.ok)
				}
			})
		}
	}
}
func TestRDAPInventoryCompleteness(t *testing.T) {
	asn, err := os.ReadFile("testdata/asn.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, family, body string }{
		{"network_nested", "networks", `{"ipSearchResults":[` + strings.TrimSuffix(networkV4, "}") + `,"entities":[{"handle":"EXAMPLE-1","roles":["registrant"],"entities":[{"notices":[{"title":"truncated"}]}]}]}]}`},
		{"network_cidr_gap", "networks", `{"ipSearchResults":[` + strings.Replace(networkV4, `"length":24`, `"length":25`, 1) + `]}`},
		{"network_error", "networks", `{"errorCode":500,"ipSearchResults":[]}`},
		{"asn_nested", "asns", `{"autnumSearchResults":[` + strings.TrimSuffix(strings.TrimSpace(string(asn)), "}") + `,"entities":[{"handle":"EXAMPLE-1","roles":["registrant"],"entities":[{"links":[{"rel":"next"}]}]}]}]}`},
		{"asn_error", "asns", `{"errorCode":500,"autnumSearchResults":[]}`},
		{"contacts_duplicate", "org_pocs", `{"objectClassName":"entity","handle":"EXAMPLE-1","entities":[{"handle":"POC"},{"handle":"poc"}]}`},
		{"contacts_nested", "org_pocs", `{"objectClassName":"entity","handle":"EXAMPLE-1","entities":[{"handle":"POC","entities":[{"notices":[{"title":"truncated"}]}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
			defer server.Close()
			c, _ := New(Config{RDAPBaseURL: server.URL})
			var err error
			if tc.family == "networks" {
				_, err = c.ListOrganizationNetworks(context.Background(), "EXAMPLE-1")
			} else {
				_, err = c.readPublic(context.Background(), ReadSpec{Name: tc.family}, map[string]string{"org_handle": "EXAMPLE-1"})
			}
			if err == nil {
				t.Fatal("accepted incomplete inventory")
			}
		})
	}
}

func TestRDAPHelp(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"minimal", `{"rdapConformance":["rdap_level_0"]}`, 200, true},
		{"full", `{"rdapConformance":["rdap_level_0","reverse_search"],"reverse_search_properties":[{"searchableResourceType":"ips","relatedResourceType":"entity","property":"handle"},{"searchableResourceType":"autnums","relatedResourceType":"entity","property":"fn"}],"extension":9007199254740993}`, 200, true},
		{"missing", `{}`, 200, false}, {"null", `null`, 200, false}, {"bad", `{`, 200, false},
		{"empty_identifier", `{"rdapConformance":["rdap_level_0",""]}`, 200, false},
		{"incomplete_property", `{"rdapConformance":["rdap_level_0"],"reverse_search_properties":[{"property":"handle"}]}`, 200, false},
		{"duplicate", `{"rdapConformance":["rdap_level_0"],"reverse_search_properties":[{"searchableResourceType":"ips","relatedResourceType":"entity","property":"handle"},{"searchableResourceType":"ips","relatedResourceType":"entity","property":"handle"}]}`, 200, false},
		{"truncated", `{"rdapConformance":["rdap_level_0"],"notices":[{"title":"truncated"}]}`, 200, false},
		{"error", `{"rdapConformance":["rdap_level_0"],"errorCode":500}`, 200, false},
		{"missing_service", `{"errorCode":404}`, 404, false}, {"unsupported", `{"errorCode":501}`, 501, false}, {"referral", `{}`, 302, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.RequestURI() != "/registry/help" || r.Header.Get("Authorization") != "" {
					t.Error("unexpected help request")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "must-not-send", RDAPBaseURL: server.URL})
			record, err := c.readRDAPHelp(context.Background())
			if calls != 1 || (err == nil) != tc.ok {
				t.Fatalf("unexpected response: %v", err)
			}
			if tc.name == "full" {
				if record["reverse_search_properties"].([]any)[0].(map[string]any)["resource_type"] != "autnums" || !strings.Contains(record["rdap_json"].(string), "9007199254740993") {
					t.Fatal("lost ordering or JSON precision")
				}
			}
		})
	}
}
