package arin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func hierarchyNetwork(handle, start, end string) string {
	family := "v4"
	if strings.Contains(start, ":") {
		family = "v6"
	}
	return fmt.Sprintf(`{"objectClassName":"ip network","handle":%q,"ipVersion":%q,"startAddress":%q,"endAddress":%q,"extension":9007199254740993}`, handle, family, start, end)
}
func hierarchyCollection(records ...string) string {
	return `{"ipSearchResults":[` + strings.Join(records, ",") + `]}`
}

func TestRDAPNetworkHierarchy(t *testing.T) {
	parent := hierarchyNetwork("PARENT", "192.0.0.0", "192.0.255.255")
	self := hierarchyNetwork("SELF", "192.0.2.0", "192.0.2.255")
	child := hierarchyNetwork("CHILD", "192.0.2.0", "192.0.2.127")
	for _, tc := range []struct {
		relation, query, body string
		active                bool
		count                 int
	}{
		{"top", "192.0.2.0/24", parent, false, 1},
		{"top", "192.0.2.0/24", self, true, 1},
		{"up", "192.0.2.0/24", parent, true, 1},
		{"up", "192.0.2.1", child, false, 1},
		{"down", "192.0.2.0/24", hierarchyCollection(child), false, 1},
		{"bottom", "192.0.2.0/24", hierarchyCollection(self, child), false, 2},
		{"bottom", "192.0.2.0/24", hierarchyCollection(hierarchyNetwork("OVERLAP", "192.0.1.0", "192.0.2.127")), false, 1},
		{"bottom", "192.0.2.1", hierarchyCollection(child), false, 1},
		{"top", "2001:db8::/48", hierarchyNetwork("V6", "2001:db8::", "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"), false, 1},
		{"down", "2001:db8::/48", hierarchyCollection(hierarchyNetwork("V6", "2001:db8::", "2001:db8::ffff:ffff:ffff:ffff")), false, 1},
	} {
		t.Run(tc.relation+tc.query+fmt.Sprint(tc.active), func(t *testing.T) {
			calls := 0
			path := "/registry/ips/rirSearch1/rdap-" + tc.relation + "/" + tc.query
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
			result, err := c.searchRDAPNetworkHierarchy(context.Background(), tc.query, tc.relation, tc.active)
			if err != nil {
				t.Fatal(err)
			}
			records := result["networks"].([]any)
			if calls != 1 || len(records) != tc.count {
				t.Fatal("incorrect request or record count")
			}
			previous := ""
			for _, raw := range records {
				record := raw.(map[string]any)
				handle := record["handle"].(string)
				if handle < previous {
					t.Fatal("unstable ordering")
				}
				previous = handle
				if !strings.Contains(record["rdap_json"].(string), "9007199254740993") {
					t.Fatal("lost raw JSON number precision")
				}
			}
		})
	}
}
func TestRDAPNetworkHierarchyResponses(t *testing.T) {
	self := hierarchyNetwork("SELF", "192.0.2.0", "192.0.2.255")
	child := hierarchyNetwork("CHILD", "192.0.2.0", "192.0.2.127")
	wrong := hierarchyNetwork("WRONG", "198.51.100.0", "198.51.100.255")
	for name, tc := range map[string]struct {
		relation, body string
		status         int
		ok             bool
	}{
		"empty":                 {"down", hierarchyCollection(), 200, true},
		"no_parent":             {"up", `{"errorCode":404}`, 404, true},
		"empty_404":             {"bottom", hierarchyCollection(), 404, true},
		"coded_empty_404":       {"down", `{"errorCode":404,"ipSearchResults":[]}`, 404, true},
		"mixed_error":           {"bottom", `{"errorCode":404,"ipSearchResults":[],"domainSearchResults":[{}]}`, 404, false},
		"nonempty_404":          {"down", `{"errorCode":404,"ipSearchResults":[` + child + `]}`, 404, false},
		"wrong_code":            {"bottom", `{"errorCode":500,"ipSearchResults":[]}`, 404, false},
		"plain_404":             {"bottom", `Not found`, 404, false},
		"null_404":              {"bottom", `{"ipSearchResults":null}`, 404, false},
		"array_for_single_404":  {"top", hierarchyCollection(), 404, false},
		"partial_404":           {"bottom", `{"ipSearchResults":[],"notices":[{"title":"truncated"}]}`, 404, false},
		"null_array":            {"down", `{"ipSearchResults":null}`, 200, false},
		"missing_array":         {"down", `{}`, 200, false},
		"bad_json":              {"down", `{`, 200, false},
		"success_error":         {"bottom", `{"errorCode":500,"ipSearchResults":[]}`, 200, false},
		"success_null_error":    {"bottom", `{"errorCode":null,"ipSearchResults":[]}`, 200, false},
		"same_parent":           {"up", self, 200, false},
		"same_child":            {"down", hierarchyCollection(self), 200, false},
		"wrong_top":             {"top", wrong, 200, false},
		"wrong_bottom":          {"bottom", hierarchyCollection(wrong), 200, false},
		"wrong_child":           {"down", hierarchyCollection(wrong), 200, false},
		"wrong_family":          {"top", hierarchyNetwork("V6", "::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"), 200, false},
		"single_for_collection": {"down", child, 200, false},
		"collection_for_single": {"up", hierarchyCollection(child), 200, false},
		"duplicate":             {"down", hierarchyCollection(child, strings.Replace(child, "CHILD", "child", 1)), 200, false},
		"partial":               {"down", `{"ipSearchResults":[],"notices":[{"title":"truncated"}]}`, 200, false},
		"next":                  {"down", `{"ipSearchResults":[],"links":[{"rel":"next"}]}`, 200, false},
		"nested_partial":        {"top", strings.TrimSuffix(self, "}") + `,"entities":[{"notices":[{"title":"truncated"}]}]}`, 200, false},
		"forbidden":             {"up", `{"errorCode":404}`, 403, false},
		"throttled":             {"down", `{}`, 429, false},
		"unsupported":           {"top", `{}`, 501, false},
		"referral":              {"top", `{}`, 302, false},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			c, _ := New(Config{RDAPBaseURL: server.URL})
			_, err := c.searchRDAPNetworkHierarchy(context.Background(), "192.0.2.0/24", tc.relation, false)
			if (err == nil) != tc.ok {
				t.Fatalf("success=%v, expected %v: %v", err == nil, tc.ok, err)
			}
		})
	}
}
func TestRDAPNetworkHierarchyValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid config reached server") }))
	defer server.Close()
	c, _ := New(Config{RDAPBaseURL: server.URL})
	for _, tc := range []struct {
		query, relation string
		active          bool
	}{
		{"192.0.2.1/24", "up", false}, {"192.0.2.0/24", "down", true}, {"192.0.2.0/24", "bottom", true}, {"192.0.2.0/24", "../up", false}, {"192.0.2.0/24?x=y", "up", false},
	} {
		if _, err := c.searchRDAPNetworkHierarchy(context.Background(), tc.query, tc.relation, tc.active); err == nil {
			t.Fatal("accepted invalid input")
		}
	}
}
