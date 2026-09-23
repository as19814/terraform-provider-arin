package arin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestWhoisSearches(t *testing.T) {
	for _, spec := range WhoisSearchReads() {
		search, _ := whoisSearch(spec.Name)
		for _, key := range []string{"handle", "q"} {
			for _, details := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/%t", spec.Name, key, details), func(t *testing.T) {
					record := whoisRecordSpec(search.target)
					content := fmt.Sprintf(`<%sRef handle=%q name="Example"/>`, search.target, record.Inputs[0].Example)
					if details {
						content = whoisFixture(search.target, whoisFixtures()[search.target])
					}
					body := whoisFixture(search.endpoint, `<limitExceeded>false</limitExceeded>`+content)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.URL.Path != "/rest/"+search.endpoint+";"+key+"="+record.Inputs[0].Example || (r.URL.Query().Get("showDetails") == "true") != details {
							t.Error("unexpected search request")
						}
						fmt.Fprint(w, body)
					}))
					defer server.Close()
					c, _ := New(Config{WhoisBaseURL: server.URL, APIKey: "must-not-send"})
					encoded, _ := json.Marshal(map[string]string{key: record.Inputs[0].Example})
					result, err := c.ReadRegistration(context.Background(), spec, map[string]string{"filters": string(encoded), "show_details": fmt.Sprint(details)})
					if err != nil {
						t.Fatal(err)
					}
					rows := result[spec.Output].([]any)
					if len(rows) != 1 || rows[0].(map[string]any)["handle"] != record.Inputs[0].Example || result["whois_xml"] != body {
						t.Fatal("lost search data")
					}
				})
			}
		}
	}
}

func TestWhoisSearchFilters(t *testing.T) {
	for _, search := range whoisSearches {
		for _, key := range search.filters {
			encoded, _ := json.Marshal(map[string]string{key: "Example*"})
			if _, err := whoisSearchFilters("whois_"+search.endpoint, string(encoded)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, filters := range []string{`null`, `{}`, `[]`, `{"handle":null}`, `{"handle":true}`, `{"invalid":"anything"}`, `{"handle":""}`, `{"handle":"  "}`, `{"handle":"A\nB"}`, `{"handle":"A*B"}`, `{"handle":"**"}`, `{"handle":"*A"}`, `{"q":null}`, `{"q":""}`, `{"q":"A*B"}`, `{"q":"A\nB"}`} {
		if _, err := whoisSearchFilters("whois_orgs", filters); err == nil {
			t.Errorf("accepted invalid filters: %s", filters)
		}
	}
}

func TestWhoisSearchEscaping(t *testing.T) {
	value := "Example & café;handle=OTHER/?#%+*"
	encoded, _ := json.Marshal(map[string]string{"q": value, "handle": "EXAMPLE*"})
	want := "/rest/orgs;handle=EXAMPLE%2A;q=" + strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != want || r.URL.RawQuery != "" {
			t.Errorf("query delimiter was not escaped: %s", r.URL.RequestURI())
		}
		fmt.Fprint(w, whoisFixture("orgs", `<limitExceeded>false</limitExceeded>`))
	}))
	defer server.Close()
	c, _ := New(Config{WhoisBaseURL: server.URL})
	_, err := c.ReadRegistration(context.Background(), WhoisSearchReads()[0], map[string]string{"filters": string(encoded), "show_details": "false"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWhoisSearchFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"empty_xml", whoisFixture("orgs", `<limitExceeded>false</limitExceeded>`), 200, true},
		{"empty_404", `<html><title>Whois-RWS</title>Sorry, there were no results.</html>`, 404, true},
		{"plain_404", `Not Found`, 404, false},
		{"unknown_record", `<html><title>Whois-RWS</title>Sorry, no record was found for the handle provided.</html>`, 404, false},
		{"forbidden", `<html><title>Whois-RWS</title>Sorry, there were no results.</html>`, 403, false},
		{"partial", whoisFixture("orgs", `<limitExceeded>true</limitExceeded><orgRef handle="EXAMPLE-1"/>`), 200, false},
		{"nested_partial", whoisFixture("orgs", `<org><handle>EXAMPLE-1</handle><pocs><limitExceeded>true</limitExceeded></pocs></org>`), 200, false},
		{"wrong_root", whoisFixture("nets", ``), 200, false},
		{"foreign_record", whoisFixture("orgs", `<orgRef xmlns="urn:foreign" handle="EXAMPLE-1"/>`), 200, false},
		{"redirect", ``, 302, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c, _ := New(Config{WhoisBaseURL: server.URL})
			result, err := c.ReadRegistration(context.Background(), WhoisSearchReads()[0], map[string]string{"filters": `{"handle":"NO-MATCH"}`, "show_details": "false"})
			if (err == nil) != tc.ok || calls != 1 {
				t.Fatalf("success=%v requests=%d: %v", err == nil, calls, err)
			}
			if tc.name == "empty_404" && (len(result["orgs"].([]any)) != 0 || result["whois_xml"] != nil) {
				t.Fatal("invented empty response data")
			}
		})
	}
}
