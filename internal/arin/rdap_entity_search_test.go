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

func TestRDAPEntitySearch(t *testing.T) {
	fixture, err := os.ReadFile("testdata/rdap_entity.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ by, query, param string }{
		{"handle", "example-*", "handle"},
		{"name", "Organization*", "fn"},
		{"name", "Réseau A&B + /?#%", "fn"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != "/registry/entities" || r.Header.Get("Authorization") != "" || len(r.URL.Query()) != 1 || r.URL.Query().Get(tc.param) != tc.query {
					t.Error("unexpected search request")
				}
				fmt.Fprintf(w, `{"entitySearchResults":[%s,%s]}`, strings.Replace(string(fixture), `"handle":"EXAMPLE-1"`, `"handle":"EXAMPLE-2"`, 1), fixture)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "must-not-send", RDAPBaseURL: server.URL})
			got, err := c.searchRDAPEntities(context.Background(), tc.by, tc.query)
			if err != nil {
				t.Fatal(err)
			}
			records := got["entities"].([]any)
			if calls != 1 || len(records) != 2 || records[0].(map[string]any)["handle"] != "EXAMPLE-1" || records[1].(map[string]any)["handle"] != "EXAMPLE-2" || records[0].(map[string]any)["vcard_json"] == nil {
				t.Fatal("invalid sorted result or unexpected follow-up lookup")
			}
		})
	}
}

func TestRDAPEntitySearchResponses(t *testing.T) {
	entity := `{"objectClassName":"entity","handle":"EXAMPLE-1"}`
	for name, tc := range map[string]struct {
		status int
		body   string
		ok     bool
		count  int
	}{
		"exact":            {200, `{"entitySearchResults":[` + entity + `]}`, true, 1},
		"empty":            {200, `{"entitySearchResults":[]}`, true, 0},
		"not_found":        {404, `{"errorCode":404,"title":"Not Found"}`, true, 0},
		"plain_404":        {404, `Not Found`, false, 0},
		"wrong_error":      {404, `{"errorCode":500}`, false, 0},
		"empty_error":      {404, `{}`, false, 0},
		"truncated_error":  {404, `{"errorCode":404,"notices":[{"type":"result set truncated"}]}`, false, 0},
		"pagination_error": {404, `{"errorCode":404,"links":[{"rel":"next"}]}`, false, 0},
		"forbidden":        {403, `{"errorCode":404}`, false, 0},
		"rate_limit":       {429, `{}`, false, 0},
		"server_error":     {500, `{}`, false, 0},
		"redirect":         {302, `{}`, false, 0},
		"missing_array":    {200, `{}`, false, 0},
		"null_array":       {200, `{"entitySearchResults":null}`, false, 0},
		"wrong_array":      {200, `{"entitySearchResults":{}}`, false, 0},
		"invalid_json":     {200, `{`, false, 0},
		"truncated":        {200, `{"entitySearchResults":[],"remarks":[{"title":"Incomplete"}]}`, false, 0},
		"paginated":        {200, `{"entitySearchResults":[],"links":[{"rel":"Next"}]}`, false, 0},
		"duplicate":        {200, `{"entitySearchResults":[` + entity + `,` + strings.Replace(entity, "EXAMPLE-1", "example-1", 1) + `]}`, false, 0},
		"wrong_handle":     {200, `{"entitySearchResults":[` + strings.Replace(entity, "EXAMPLE-1", "OTHER", 1) + `]}`, false, 0},
		"wrong_class":      {200, `{"entitySearchResults":[{"objectClassName":"autnum","handle":"EXAMPLE-1"}]}`, false, 0},
		"partial_entity":   {200, `{"entitySearchResults":[{"objectClassName":"entity","handle":"EXAMPLE-1","entities":[{"handle":"POC","notices":[{"type":"object truncated"}]}]}]}`, false, 0},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			c, _ := New(Config{RDAPBaseURL: server.URL})
			result, err := c.searchRDAPEntities(context.Background(), "handle", "EXAMPLE-1")
			if (err == nil) != tc.ok {
				t.Fatalf("success=%v, expected %v", err == nil, tc.ok)
			}
			if tc.ok && len(result["entities"].([]any)) != tc.count {
				t.Fatal("wrong result count")
			}
		})
	}
}

func TestRDAPEntitySearchValidation(t *testing.T) {
	var spec ReadSpec
	for _, s := range PublicReads() {
		if s.Name == "rdap_entities" {
			spec = s
		}
	}
	for _, tc := range []struct {
		by, query string
		valid     bool
	}{
		{"handle", "FT-684", true}, {"handle", "FT-*", true}, {"handle", "*", true},
		{"name", "Example Name", true}, {"name", "Example*", true}, {"name", "Réseau & Co.", true},
		{"other", "Example", false}, {"name", "", false}, {"name", "  ", false}, {"name", "*Example", false},
		{"name", "Ex*ample*", false}, {"name", "Example\n", false}, {"name", string([]byte{0xff}), false},
		{"handle", "FT-684&fn=Other", false}, {"handle", "FT 684", false},
	} {
		if (spec.Validate(map[string]string{"search_by": tc.by, "query": tc.query}) == nil) != tc.valid {
			t.Errorf("unexpected validation for %q %q", tc.by, tc.query)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("invalid query reached server") }))
	defer server.Close()
	c, _ := New(Config{RDAPBaseURL: server.URL})
	if _, err := c.searchRDAPEntities(context.Background(), "name", "bad**"); err == nil {
		t.Fatal("invalid query accepted")
	}
}
