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

func testASSet() ASSet {
	return ASSet{Name: "AS-EXAMPLE", OrgHandle: "EXAMPLE-1", Description: []string{"Peers & <routing>", "Second line"}, Remarks: []string{"A remark"}, Members: []string{"AS64496", "AS-PEERS"}, MembersByRef: []string{"MNT-EXAMPLE-1"}, POCs: []IRRPOC{{"ADMIN-1", "AD"}, {"TECH-1", "T"}}}
}
func TestASSetPayload(t *testing.T) {
	b, err := testASSet().marshal()
	if err != nil {
		t.Fatal(err)
	}
	var root asSetXML
	if err := xml.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	if root.Source != "ARIN" || root.Name != "AS-EXAMPLE" || root.Description[1].Number != 1 || root.Description[0].Text != "Peers & <routing>" || root.MembersByRef[0].Name != "MNT-EXAMPLE-1" {
		t.Fatalf("unexpected payload: %+v", root)
	}
	if !strings.Contains(string(b), "<membersByRef>") || strings.Contains(string(b), "creationDate") || strings.Contains(string(b), "lastModifiedDate") || strings.Contains(string(b), "pocLinks") {
		t.Fatal("incorrect writable fields")
	}
	decoded, err := decodeASSet(b, "AS-EXAMPLE")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Description[0] != testASSet().Description[0] || len(decoded.Members) != 2 {
		t.Fatalf("unexpected decoded set: %+v", decoded)
	}
	empty := testASSet()
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
func TestASSetValidation(t *testing.T) {
	for _, name := range []string{"AS-EXAMPLE", "AS64496:AS-PEERS", "AS-EXAMPLE:AS-PEERS"} {
		if err := ValidateASSetName(name); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*ASSet)
	}{
		{"name", func(s *ASSet) { s.Name = "AS-x/../bad" }},
		{"lowercase", func(s *ASSet) { s.Name = "as-example" }},
		{"org", func(s *ASSet) { s.OrgHandle = "../BAD" }},
		{"description", func(s *ASSet) { s.Description = nil }},
		{"newline", func(s *ASSet) { s.Remarks = []string{"one\ntwo"} }},
		{"member", func(s *ASSet) { s.Members = []string{"64496"} }},
		{"ref", func(s *ASSet) { s.MembersByRef = []string{"EXAMPLE-1"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testASSet()
			tc.change(&s)
			if _, err := s.marshal(); err == nil {
				t.Fatal("accepted invalid AS set")
			}
		})
	}
}
func TestASSetWrites(t *testing.T) {
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
					if r.URL.RequestURI() != "/rest/irr/as-set?orgHandle=EXAMPLE-1" {
						t.Error("wrong POST path")
					}
				} else if r.URL.Path != "/rest/irr/as-set/AS-EXAMPLE" {
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
				_, err = c.CreateASSet(context.Background(), testASSet())
			case "PUT":
				_, err = c.UpdateASSet(context.Background(), testASSet())
			case "DELETE":
				err = c.DeleteASSet(context.Background(), "AS-EXAMPLE")
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
func TestASSetWriteFailuresNotRetried(t *testing.T) {
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
		{"malformed", 200, `<asSet>`},
		{"wrong namespace", 200, `<asSet xmlns="urn:bad"/>`},
		{"wrong identity", 200, `<asSet xmlns="http://www.arin.net/regrws/core/v1"><name>AS-OTHER</name><orgHandle>EXAMPLE-1</orgHandle><source>ARIN</source></asSet>`},
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
			_, err := c.UpdateASSet(context.Background(), testASSet())
			if err == nil || strings.Contains(err.Error(), "test-secret") {
				t.Fatalf("unexpected error: %v", err)
			}
			if writes != 1 {
				t.Fatalf("replayed mutation %d times", writes)
			}
		})
	}
}
func TestASSetDeleteStatus(t *testing.T) {
	for _, status := range []int{200, 204, 404, 202, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			err := c.DeleteASSet(context.Background(), "AS-EXAMPLE")
			wantError := status != 200 && status != 204 && status != 404
			if (err != nil) != wantError {
				t.Fatalf("status %d: %v", status, err)
			}
		})
	}
}
func TestASSetCreatePreflightFailure(t *testing.T) {
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
			if _, err := c.CreateASSet(context.Background(), testASSet()); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
