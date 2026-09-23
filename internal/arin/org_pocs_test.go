package arin

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOrganizationPOCClientLifecycle(t *testing.T) {
	var mu sync.Mutex
	links := []NetPOC{{Handle: "ADMIN-ARIN", Function: "AD"}, {Handle: "TECH-ARIN", Function: "T"}}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		base := "/rest/org/EXAMPLE-1"
		if r.Method == "GET" && r.URL.Path == base {
			p := struct {
				XMLName xml.Name `xml:"http://www.arin.net/regrws/core/v1 org"`
				Handle  string   `xml:"handle"`
				Name    string   `xml:"orgName"`
				Links   []NetPOC `xml:"pocLinks>pocLinkRef"`
			}{Handle: "EXAMPLE-1", Name: "Example", Links: links}
			_ = xml.NewEncoder(w).Encode(p)
			return
		}
		poc, function, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, base+"/poc/"), ";pocFunction=")
		switch r.Method {
		case "PUT":
			links = append(links, NetPOC{Handle: poc, Function: function})
		case "DELETE":
			next := []NetPOC{}
			for _, p := range links {
				if (poc != "" && p.Handle != poc) || (function != "" && p.Function != function) {
					next = append(next, p)
				}
			}
			links = next
		default:
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(405)
			return
		}
		w.WriteHeader(204)
	}))
	defer s.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
	ctx := context.Background()
	for _, function := range []string{"T", "N", "AB"} {
		if _, err := c.AddOrganizationPOC(ctx, "EXAMPLE-1", "TEST-ARIN", function); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.RemoveOrganizationPOC(ctx, "EXAMPLE-1", "TEST-ARIN", "N"); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"T", "AB"} {
		if _, err := c.RemoveOrganizationPOC(ctx, "EXAMPLE-1", "TEST-ARIN", role); err != nil {
			t.Fatal(err)
		}
	}
	out, err := c.GetOrganizationPOCs(ctx, "EXAMPLE-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Handle != "ADMIN-ARIN" || out[1].Handle != "TECH-ARIN" {
		t.Fatal("unrelated links changed")
	}
}
func TestOrganizationPOCErrors(t *testing.T) {
	for _, status := range []int{202, 403, 404, 409, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(status) }))
			defer s.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
			if _, err := c.AddOrganizationPOC(context.Background(), "EXAMPLE-1", "TEST-ARIN", "T"); err == nil {
				t.Fatal("unconfirmed write accepted")
			}
			if calls != 1 {
				t.Fatal("mutation retried")
			}
		})
	}
}

func TestOrganizationPOCReadRejectsPartialCollections(t *testing.T) {
	for _, fragment := range []string{"", `<pocLinks><unexpected/></pocLinks>`, `<pocLinks><pocLinkRef handle="TEST-ARIN"/></pocLinks>`, `<pocLinks><pocLinkRef handle="TEST-ARIN" function="T" extra="x"/></pocLinks>`, `<pocLinks/><pocLinks/>`} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `<org xmlns="http://www.arin.net/regrws/core/v1"><handle>EXAMPLE-1</handle><orgName>Example</orgName>%s</org>`, fragment)
		}))
		c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
		if _, err := c.GetOrganizationPOCs(context.Background(), "EXAMPLE-1"); err == nil {
			t.Error("partial collection accepted")
		}
		s.Close()
	}
}
func TestOrganizationPOCValidationPreventsMutation(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer s.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
	ctx := context.Background()
	for _, f := range []func() ([]OrgPOC, error){func() ([]OrgPOC, error) { return c.AddOrganizationPOC(ctx, "EXAMPLE-1", "TEST-ARIN", "AD") }, func() ([]OrgPOC, error) { return c.RemoveOrganizationPOC(ctx, "EXAMPLE-1", "TEST-ARIN", "") }, func() ([]OrgPOC, error) { return c.RemoveOrganizationPOC(ctx, "EXAMPLE-1", "", "N") }} {
		if _, err := f(); err == nil {
			t.Error("unsupported operation accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid operation reached server")
	}
}

func TestOrganizationPOCConcurrentClientsPreserveLinks(t *testing.T) {
	var mu sync.Mutex
	links := []NetPOC{{Handle: "ADMIN-ARIN", Function: "AD"}, {Handle: "TECH-ARIN", Function: "T"}}
	active, maxActive := 0, 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			_, function, _ := strings.Cut(r.URL.Path, ";pocFunction=")
			mu.Lock()
			snapshot := append([]NetPOC{}, links...)
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()
			// Reproduce the server's read-modify-write behavior: overlapping writes
			// replace the collection from stale snapshots and lose other additions.
			time.Sleep(5 * time.Millisecond)
			mu.Lock()
			links = append(snapshot, NetPOC{Handle: "TEST-ARIN", Function: function})
			active--
			mu.Unlock()
			w.WriteHeader(204)
			return
		}
		mu.Lock()
		snapshot := append([]NetPOC{}, links...)
		mu.Unlock()
		payload := struct {
			XMLName xml.Name `xml:"http://www.arin.net/regrws/core/v1 org"`
			Handle  string   `xml:"handle"`
			Name    string   `xml:"orgName"`
			Links   []NetPOC `xml:"pocLinks>pocLinkRef"`
		}{Handle: "EXAMPLE-1", Name: "Example", Links: snapshot}
		_ = xml.NewEncoder(w).Encode(payload)
	}))
	defer s.Close()
	first, _ := New(Config{APIKey: "first-key", BaseURL: s.URL})
	second, _ := New(Config{APIKey: "second-key", BaseURL: s.URL})
	var wg sync.WaitGroup
	for i, role := range []string{"T", "N", "AB", "R", "D"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := first
			if i%2 == 1 {
				c = second
			}
			if _, err := c.AddOrganizationPOC(context.Background(), "EXAMPLE-1", "TEST-ARIN", role); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	out, err := first.GetOrganizationPOCs(context.Background(), "EXAMPLE-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 7 || maxActive != 1 {
		t.Fatalf("concurrent writes lost associations: links=%d overlap=%d", len(out), maxActive)
	}
}
