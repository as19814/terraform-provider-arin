package arin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func domainSearchRecord(name string) string {
	return fmt.Sprintf(`{"objectClassName":"domain","ldhName":%q}`, name)
}
func TestRDAPDomainSearch(t *testing.T) {
	for _, tc := range []struct {
		relation, name, body string
		active               bool
		count                int
	}{
		{"top", "2.0.192.IN-ADDR.ARPA", domainSearchRecord("192.in-addr.arpa."), false, 1},
		{"up", "2.0.192.in-addr.arpa.", domainSearchRecord("0.192.in-addr.arpa."), true, 1},
		{"top", "192.in-addr.arpa.", domainSearchRecord("192.in-addr.arpa."), false, 1},
		{"down", "0.192.in-addr.arpa.", `{"domainSearchResults":[` + domainSearchRecord("3.0.192.in-addr.arpa.") + `,` + domainSearchRecord("2.0.192.in-addr.arpa.") + `]}`, false, 2},
		{"bottom", "2.0.192.in-addr.arpa.", `{"domainSearchResults":[` + domainSearchRecord("0.192.in-addr.arpa.") + `,` + domainSearchRecord("1.2.0.192.in-addr.arpa.") + `]}`, false, 2},
		{"up", "8.b.d.0.1.0.0.2.ip6.arpa.", domainSearchRecord("1.0.0.2.ip6.arpa."), false, 1},
		{"bottom", "8.b.d.0.1.0.0.2.ip6.arpa.", `{"domainSearchResults":[` + domainSearchRecord("8.b.d.0.1.0.0.2.ip6.arpa.") + `]}`, false, 1},
	} {
		t.Run(tc.relation+tc.name, func(t *testing.T) {
			calls := 0
			normalized, _ := rdapDomainName(tc.name)
			path := "/registry/domains/rirSearch1/rdap-" + tc.relation + "/" + normalized
			if tc.active {
				path += "?status=active"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.URL.RequestURI() != path {
					t.Error("unexpected hierarchy request")
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "must-not-send", RDAPBaseURL: server.URL})
			result, err := c.searchRDAPDomains(context.Background(), tc.name, tc.relation, tc.active)
			if err != nil {
				t.Fatal(err)
			}
			records := result["domains"].([]any)
			if calls != 1 || len(records) != tc.count {
				t.Fatal("incorrect result count or extra requests")
			}
			for i := 1; i < len(records); i++ {
				if records[i-1].(map[string]any)["name"].(string) > records[i].(map[string]any)["name"].(string) {
					t.Fatal("unstable ordering")
				}
			}
		})
	}
}
func TestRDAPDomainSearchResponses(t *testing.T) {
	child := domainSearchRecord("1.2.0.192.in-addr.arpa.")
	for name, tc := range map[string]struct {
		relation, body string
		status         int
		ok             bool
	}{
		"empty":                 {"down", `{"domainSearchResults":[]}`, 200, true},
		"no_parent":             {"up", `{"errorCode":404}`, 404, true},
		"empty_404":             {"bottom", `{"domainSearchResults":[]}`, 404, true},
		"coded_empty_404":       {"down", `{"errorCode":404,"domainSearchResults":[]}`, 404, true},
		"404_nonempty":          {"down", `{"errorCode":404,"domainSearchResults":[` + child + `]}`, 404, false},
		"404_wrong_code":        {"bottom", `{"errorCode":500,"domainSearchResults":[]}`, 404, false},
		"404_plain":             {"down", `Not Found`, 404, false},
		"404_null":              {"down", `{"domainSearchResults":null}`, 404, false},
		"404_truncated":         {"bottom", `{"domainSearchResults":[],"notices":[{"type":"truncated"}]}`, 404, false},
		"404_array_for_single":  {"top", `{"domainSearchResults":[]}`, 404, false},
		"missing_array":         {"down", `{}`, 200, false},
		"null_array":            {"bottom", `{"domainSearchResults":null}`, 200, false},
		"invalid_json":          {"down", `{`, 200, false},
		"single_for_collection": {"down", child, 200, false},
		"collection_for_single": {"up", `{"domainSearchResults":[]}`, 200, false},
		"same_parent":           {"up", domainSearchRecord("2.0.192.in-addr.arpa."), 200, false},
		"wrong_parent":          {"top", domainSearchRecord("2.0.198.in-addr.arpa."), 200, false},
		"wrong_child":           {"down", `{"domainSearchResults":[` + domainSearchRecord("2.0.198.in-addr.arpa.") + `]}`, 200, false},
		"wrong_bottom":          {"bottom", `{"domainSearchResults":[` + domainSearchRecord("2.0.198.in-addr.arpa.") + `]}`, 200, false},
		"duplicate":             {"down", `{"domainSearchResults":[` + child + `,` + strings.ReplaceAll(child, "in-addr.arpa", "IN-ADDR.ARPA") + `]}`, 200, false},
		"partial":               {"down", `{"domainSearchResults":[],"notices":[{"title":"Incomplete"}]}`, 200, false},
		"next":                  {"down", `{"domainSearchResults":[],"links":[{"rel":"next"}]}`, 200, false},
		"nested_partial":        {"up", `{"objectClassName":"domain","ldhName":"192.in-addr.arpa.","nameservers":[{"objectClassName":"nameserver","ldhName":"ns.example.net","notices":[{"title":"truncated"}]}]}`, 200, false},
		"forbidden":             {"up", `{"errorCode":404}`, 403, false},
		"throttled":             {"down", `{}`, 429, false},
		"server_failure":        {"down", `{}`, 500, false},
		"unsupported":           {"top", `{}`, 501, false},
		"referral":              {"top", `{}`, 302, false},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			c, _ := New(Config{RDAPBaseURL: server.URL})
			_, err := c.searchRDAPDomains(context.Background(), "2.0.192.in-addr.arpa.", tc.relation, false)
			if (err == nil) != tc.ok {
				t.Fatalf("success=%v, expected %v", err == nil, tc.ok)
			}
		})
	}
}
func TestRDAPDomainSearchValidation(t *testing.T) {
	for _, tc := range []struct {
		relation      string
		active, valid bool
	}{{"top", true, true}, {"up", true, true}, {"down", false, true}, {"bottom", false, true}, {"bottom", true, false}, {"down", true, false}, {"../up", false, false}, {"other", false, false}} {
		if (validateRDAPDomainRelation(tc.relation, tc.active) == nil) != tc.valid {
			t.Errorf("incorrect validation for %s", tc.relation)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid query reached server") }))
	defer server.Close()
	c, _ := New(Config{RDAPBaseURL: server.URL})
	if _, err := c.searchRDAPDomains(context.Background(), "2.0.192.in-addr.arpa.", "down", true); err == nil {
		t.Fatal("accepted unsupported status filter")
	}
	if _, err := c.searchRDAPDomains(context.Background(), "example.net", "top", false); err == nil {
		t.Fatal("accepted forward domain")
	}
}
