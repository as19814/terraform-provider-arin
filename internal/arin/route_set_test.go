package arin

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testRouteSet() RouteSet {
	return RouteSet{Name: "RS-EXAMPLE", OrgHandle: "EXAMPLE-1", Description: []string{"Peers & <routing>", "Second line"}, Remarks: []string{"A remark"}, Members: []string{"192.0.2.0/24", "RS-PEERS"}, MembersByRef: []string{"MNT-EXAMPLE-1"}, POCs: []IRRPOC{{Handle: "ADMIN-1", Function: "AD"}, {Handle: "TECH-1", Function: "T"}}}
}
func TestRouteSetPayload(t *testing.T) {
	b, err := testRouteSet().marshal()
	if err != nil {
		t.Fatal(err)
	}
	var root routeSetXML
	if err := xml.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	if root.Source != "ARIN" || root.Name != "RS-EXAMPLE" || root.Description[1].Number != 1 || root.Description[0].Text != "Peers & <routing>" || root.MembersByRef[0].Name != "MNT-EXAMPLE-1" {
		t.Fatalf("unexpected payload: %+v", root)
	}
	if !strings.Contains(string(b), "<membersByRef>") || strings.Contains(string(b), "creationDate") || strings.Contains(string(b), "lastModifiedDate") || strings.Contains(string(b), "pocLinks") {
		t.Fatal("incorrect writable fields")
	}
	decoded, err := decodeRouteSet(b, "RS-EXAMPLE")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Description[0] != testRouteSet().Description[0] || len(decoded.Members) != 2 {
		t.Fatalf("unexpected decoded set: %+v", decoded)
	}
	empty := testRouteSet()
	empty.Remarks = nil
	empty.Members = nil
	empty.MembersByRef = nil
	empty.POCs = nil
	b, err = empty.marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "<remarks") {
		t.Fatal("empty remarks must be omitted to avoid ARIN HTTP 500")
	}
	for _, container := range []string{"members", "membersByRef"} {
		if !strings.Contains(string(b), "<"+container+"></"+container+">") {
			t.Fatalf("missing explicit empty %s container", container)
		}
	}
}
func TestRouteSetValidation(t *testing.T) {
	for _, name := range []string{"RS-EXAMPLE", "AS64496:RS-PEERS", "RS-EXAMPLE:RS-PEERS"} {
		if err := ValidateRouteSetName(name); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*RouteSet)
	}{
		{"name", func(s *RouteSet) { s.Name = "RS-x/../bad" }},
		{"lowercase", func(s *RouteSet) { s.Name = "as-example" }},
		{"org", func(s *RouteSet) { s.OrgHandle = "../BAD" }},
		{"description", func(s *RouteSet) { s.Description = nil }},
		{"newline", func(s *RouteSet) { s.Remarks = []string{"one\ntwo"} }},
		{"member", func(s *RouteSet) { s.Members = []string{"64496"} }},
		{"ref", func(s *RouteSet) { s.MembersByRef = []string{"EXAMPLE-1"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testRouteSet()
			tc.change(&s)
			if _, err := s.marshal(); err == nil {
				t.Fatal("accepted invalid route set")
			}
		})
	}
}
func TestRouteSetWrites(t *testing.T) {
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "ApiKey test-secret" || r.Header.Get("Content-Type") != "application/xml" || r.URL.Query().Has("apikey") {
					t.Error("incorrect authentication or content type")
				}
				if r.Method == "GET" {
					w.WriteHeader(404)
					return
				}
				writes++
				if r.Method != method {
					t.Errorf("unexpected method: %s", r.Method)
				}
				if method == "POST" {
					if r.URL.RequestURI() != "/rest/irr/route-set?orgHandle=EXAMPLE-1" {
						t.Error("wrong POST path")
					}
				} else if r.URL.Path != "/rest/irr/route-set/RS-EXAMPLE" {
					t.Error("wrong object path")
				}
				body, _ := io.ReadAll(r.Body)
				if method == "DELETE" {
					if len(body) != 0 {
						t.Error("DELETE had payload")
					}
					w.WriteHeader(204)
					return
				}
				fmt.Fprint(w, string(body))
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-secret", BaseURL: server.URL})
			var err error
			switch method {
			case "POST":
				_, err = c.CreateRouteSet(context.Background(), testRouteSet())
			case "PUT":
				_, err = c.UpdateRouteSet(context.Background(), testRouteSet())
			case "DELETE":
				err = c.DeleteRouteSet(context.Background(), "RS-EXAMPLE")
			}
			if err != nil {
				t.Fatal(err)
			}
			if writes != 1 {
				t.Fatalf("writes=%d", writes)
			}
		})
	}
}
func TestRouteSetWriteFailuresNotRetried(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"forbidden", 403, `<error><code>E_AUTH</code><message>test-secret</message></error>`},
		{"conflict", 409, `<error><code>E_EXISTS</code></error>`},
		{"rate limit", 429, ``},
		{"server error", 500, ``},
		{"redirect", 307, ``},
		{"async", 202, `<ticket/>`},
		{"malformed", 200, `<routeSet>`},
		{"wrong namespace", 200, `<routeSet xmlns="urn:bad"/>`},
		{"wrong identity", 200, `<routeSet xmlns="http://www.arin.net/regrws/core/v1"><name>RS-OTHER</name><orgHandle>EXAMPLE-1</orgHandle><source>ARIN</source></routeSet>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writes++
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-secret", BaseURL: server.URL})
			_, err := c.UpdateRouteSet(context.Background(), testRouteSet())
			if err == nil || strings.Contains(err.Error(), "test-secret") {
				t.Fatalf("unexpected error: %v", err)
			}
			if writes != 1 {
				t.Fatalf("replayed mutation %d times", writes)
			}
		})
	}
}
func TestRouteSetDeleteStatus(t *testing.T) {
	for _, status := range []int{200, 204, 404, 202, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			err := c.DeleteRouteSet(context.Background(), "RS-EXAMPLE")
			wantError := status != 200 && status != 204 && status != 404
			if (err != nil) != wantError {
				t.Fatalf("status %d: %v", status, err)
			}
		})
	}
}
func TestRouteSetCreatePreflightFailure(t *testing.T) {
	for _, status := range []int{403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("mutation followed failed preflight")
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			if _, err := c.CreateRouteSet(context.Background(), testRouteSet()); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestRouteSetMembers(t *testing.T) {
	for _, member := range []string{"192.0.2.0/24", "192.0.2.0/24^+", "192.0.2.0/24^-", "192.0.2.0/24^25-28", "RS-PEERS", "AS64496:RS-PEERS"} {
		if err := validateRouteSetMember(member, false); err != nil {
			t.Errorf("%s: %v", member, err)
		}
	}
	if err := validateRouteSetMember("2001:db8::/32^48", true); err != nil {
		t.Fatal(err)
	}
	for _, member := range []string{"2001:db8::/32", "192.0.2.1/24", "192.0.2.0/24^23", "192.0.2.0/24^24-33", "192.0.2.0/24^28-25", "192.0.2.0/24^^+", "AS64496"} {
		if err := validateRouteSetMember(member, false); err == nil {
			t.Errorf("accepted %s", member)
		}
	}
	s := testRouteSet()
	s.MPMembers = []string{"2001:db8::/32"}
	b, err := s.marshal()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeRouteSet(b, s.Name)
	if err != nil || len(decoded.MPMembers) != 1 || decoded.MPMembers[0] != s.MPMembers[0] {
		t.Fatalf("MP member roundtrip failed: %v", err)
	}
}
