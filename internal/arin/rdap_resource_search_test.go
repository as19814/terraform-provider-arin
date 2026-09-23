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

func TestRDAPResourceSearchVariants(t *testing.T) {
	for _, family := range []struct{ spec, output, endpoint, key, fixture string }{
		{"rdap_networks", "networks", "ips", "ipSearchResults", "rdap_network"},
		{"rdap_asns", "asns", "autnums", "autnumSearchResults", "asn"},
	} {
		t.Run(family.spec, func(t *testing.T) {
			fixture, err := os.ReadFile("testdata/" + family.fixture + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var record map[string]any
			if err := json.Unmarshal(fixture, &record); err != nil {
				t.Fatal(err)
			}
			handle := record["handle"].(string)
			for _, by := range []string{"handle", "name", "entity_handle", "entity_name", "entity_email"} {
				roles := []string{"any"}
				if strings.HasPrefix(by, "entity_") {
					roles = append(roles, "abuse", "noc", "technical")
				}
				for _, role := range roles {
					t.Run(by+"/"+role, func(t *testing.T) {
						query := "Example A&B + /?#%*"
						if by == "handle" {
							query = strings.ToLower(handle) + "*"
						}
						if by == "entity_handle" {
							query = "POC-ARIN"
						}
						if by == "entity_email" {
							query = "noc+rdap@example.net"
						}
						path := "/registry/" + family.endpoint
						parameter := by
						if strings.HasPrefix(by, "entity_") {
							path += "/reverse_search/entity"
							parameter = strings.TrimPrefix(by, "entity_")
							if parameter == "name" {
								parameter = "fn"
							}
						}
						calls := 0
						server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							calls++
							wantParams := 1
							if role != "any" {
								wantParams++
							}
							if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.URL.Path != path || r.URL.Query().Get(parameter) != query || len(r.URL.Query()) != wantParams {
								t.Error("unexpected search request")
							}
							if role != "any" && r.URL.Query().Get("role") != role {
								t.Error("missing role filter")
							}
							// Keep an associated record whose registrant differs from the queried POC.
							fmt.Fprintf(w, `{%q:[%s]}`, family.key, fixture)
						}))
						defer server.Close()
						c, _ := New(Config{APIKey: "must-not-send", RDAPBaseURL: server.URL})
						got, err := c.searchRDAPResources(context.Background(), family.spec, by, query, role)
						if err != nil {
							t.Fatal(err)
						}
						records := got[family.output].([]any)
						if calls != 1 || len(records) != 1 || records[0].(map[string]any)["handle"] != handle || !strings.Contains(records[0].(map[string]any)["rdap_json"].(string), "objectClassName") {
							t.Fatal("lost associated resource or extra request")
						}
					})
				}
			}
		})
	}
}

func TestRDAPResourceSearchResponses(t *testing.T) {
	for _, family := range []struct{ spec, output, key, fixture string }{{"rdap_networks", "networks", "ipSearchResults", "rdap_network"}, {"rdap_asns", "asns", "autnumSearchResults", "asn"}} {
		t.Run(family.spec, func(t *testing.T) {
			fixture, err := os.ReadFile("testdata/" + family.fixture + ".json")
			if err != nil {
				t.Fatal(err)
			}
			wrap := func(body string) string { return fmt.Sprintf(`{%q:[%s]}`, family.key, body) }
			for name, tc := range map[string]struct {
				status int
				body   string
				ok     bool
			}{
				"empty": {200, wrap(""), true}, "missing": {404, `{"errorCode":404}`, true},
				"proxy": {404, `Not Found`, false}, "contradictory_error": {404, strings.TrimSuffix(wrap(string(fixture)), "}") + `,"errorCode":404}`, false},
				"null": {200, fmt.Sprintf(`{%q:null}`, family.key), false}, "missing_array": {200, `{}`, false}, "json": {200, `{`, false},
				"wrong_collection": {200, `{"domainSearchResults":[]}`, false}, "error_200": {200, strings.TrimSuffix(wrap(""), "}") + `,"errorCode":500}`, false},
				"duplicate":          {200, wrap(string(fixture) + "," + string(fixture)), false},
				"truncated":          {200, strings.TrimSuffix(wrap(""), "}") + `,"notices":[{"title":"Truncated"}]}`, false},
				"pagination":         {200, strings.TrimSuffix(wrap(""), "}") + `,"links":[{"rel":"next"}]}`, false},
				"nested_partial":     {200, wrap(strings.Replace(string(fixture), `"roles":["registrant"]`, `"roles":["registrant"],"entities":[{"handle":"POC","notices":[{"title":"Incomplete"}]}]`, 1)), false},
				"missing_registrant": {200, wrap(strings.Replace(string(fixture), `"handle":"EXAMPLE-1",`, "", 1)), false},
				"forbidden":          {403, `{"errorCode":404}`, false}, "throttled": {429, `{}`, false}, "server": {500, `{}`, false}, "referral": {302, `{}`, false},
			} {
				t.Run(name, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
					defer server.Close()
					c, _ := New(Config{RDAPBaseURL: server.URL})
					result, err := c.searchRDAPResources(context.Background(), family.spec, "name", "Example*", "any")
					if (err == nil) != tc.ok {
						t.Fatalf("success=%v, expected %v", err == nil, tc.ok)
					}
					if tc.ok && len(result[family.output].([]any)) != 0 {
						t.Fatal("expected empty result")
					}
				})
			}
			t.Run("handle_mismatch", func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, wrap(string(fixture))) }))
				defer server.Close()
				c, _ := New(Config{RDAPBaseURL: server.URL})
				if _, err := c.searchRDAPResources(context.Background(), family.spec, "handle", "OTHER*", "any"); err == nil {
					t.Fatal("accepted mismatched handle")
				}
			})
		})
	}
}
func TestRDAPResourceSearchValidation(t *testing.T) {
	for _, tc := range []struct {
		by, query, role string
		ok              bool
	}{
		{"handle", "NET-*", "any", true}, {"entity_handle", "ORG-1", "technical", true}, {"entity_name", "Réseau & Co*", "abuse", true}, {"entity_email", "noc+test@example.net", "noc", true},
		{"handle", "NET-*", "abuse", false}, {"entity_handle", "POC-ARIN", "registrant", false}, {"entity_role", "technical", "any", false}, {"name", "bad**", "any", false}, {"handle", "bad?query", "any", false}, {"entity_email", "bad\n", "any", false}, {"name", "", "any", false},
	} {
		if (validateRDAPResourceSearch(tc.by, tc.query, tc.role) == nil) != tc.ok {
			t.Errorf("incorrect validation: %+v", tc)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid input reached server") }))
	defer server.Close()
	c, _ := New(Config{RDAPBaseURL: server.URL})
	if _, err := c.searchRDAPResources(context.Background(), "rdap_networks", "name", "Example", "noc"); err == nil {
		t.Fatal("accepted invalid filter")
	}
}
func TestRDAPResourceSearchIPv6AndASNRange(t *testing.T) {
	for _, tc := range []struct{ spec, key, body string }{
		{"rdap_networks", "ipSearchResults", `{"objectClassName":"ip network","handle":"NET6-2001-DB8-1","ipVersion":"v6","startAddress":"2001:db8::","endAddress":"2001:db8::ffff","cidr0_cidrs":[{"v6prefix":"2001:db8::","length":112}]}`},
		{"rdap_asns", "autnumSearchResults", `{"objectClassName":"autnum","handle":"AS64496-64500","startAutnum":64496,"endAutnum":64500}`},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{%q:[%s]}`, tc.key, tc.body) }))
			defer server.Close()
			c, _ := New(Config{RDAPBaseURL: server.URL})
			if _, err := c.searchRDAPResources(context.Background(), tc.spec, "name", "Example", "any"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
