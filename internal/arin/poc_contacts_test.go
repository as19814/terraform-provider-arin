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
)

func TestPOCContactOperations(t *testing.T) {
	var mu sync.Mutex
	p := examplePOC()
	p.Handle = "TEST-ARIN"
	p.RegistrationDate = "2026-01-01"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		base := "/rest/poc/TEST-ARIN"
		switch {
		case r.Method == "GET" && r.URL.Path == base:
			body, err := p.marshal()
			if err != nil {
				t.Error(err)
			}
			_, _ = w.Write(body)
			return
		case strings.HasPrefix(r.URL.Path, base+"/email/"):
			email := strings.TrimPrefix(r.URL.Path, base+"/email/")
			if r.Method == "POST" {
				p.Emails = append(p.Emails, email)
			} else if r.Method == "DELETE" {
				out := []string{}
				for _, e := range p.Emails {
					if e != email {
						out = append(out, e)
					}
				}
				p.Emails = out
			} else {
				t.Error("unexpected email method")
			}
		case r.Method == "PUT" && r.URL.Path == base+"/phone":
			var ph pocPhoneXML
			if err := xml.NewDecoder(r.Body).Decode(&ph); err != nil {
				t.Error(err)
			}
			found := false
			for _, old := range p.Phones {
				if old.Type == ph.Type.Code && old.Number == ph.Number {
					found = true
				}
			}
			if !found {
				p.Phones = append(p.Phones, POCPhone{Type: ph.Type.Code, Number: ph.Number, Extension: ph.Extension})
			}
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, base+"/phone/"):
			selector := strings.TrimPrefix(r.URL.Path, base+"/phone/")
			number, kind, _ := strings.Cut(selector, ";type=")
			next := []POCPhone{}
			for _, ph := range p.Phones {
				if (number != "" && ph.Number != number) || (kind != "" && ph.Type != kind) {
					next = append(next, ph)
				}
			}
			p.Phones = next
		default:
			t.Errorf("unexpected operation %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	ctx := context.Background()
	email := "noc+tag/route@example.net"
	if _, err := c.AddPOCEmail(ctx, p.Handle, email); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeletePOCEmail(ctx, p.Handle, email); err != nil {
		t.Fatal(err)
	}
	ph := POCPhone{Type: "F", Number: "+1-202-555-0101", Extension: "42"}
	if _, err := c.AddPOCPhone(ctx, p.Handle, ph); err != nil {
		t.Fatal(err)
	}
	ph.Extension = ""
	if _, err := c.AddPOCPhone(ctx, p.Handle, ph); err == nil {
		t.Fatal("existing extension change accepted")
	}
	if _, err := c.DeletePOCPhones(ctx, p.Handle, ph.Number, ph.Type); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"F", "M"} {
		ph.Type = kind
		if _, err := c.AddPOCPhone(ctx, p.Handle, ph); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.DeletePOCPhones(ctx, p.Handle, ph.Number, ""); err != nil {
		t.Fatal(err)
	}
	ph.Type = "F"
	if _, err := c.AddPOCPhone(ctx, p.Handle, ph); err != nil {
		t.Fatal(err)
	}
	out, err := c.DeletePOCPhones(ctx, p.Handle, "", "F")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Emails) != 1 || out.Emails[0] != "noc@example.net" || len(out.Phones) != 1 || out.Phones[0].Type != "O" {
		t.Fatal("contact operations changed unrelated records")
	}
}
func TestPOCContactValidationBeforeRequest(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer s.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
	ctx := context.Background()
	for _, call := range []func() (*POC, error){func() (*POC, error) { return c.AddPOCEmail(ctx, "TEST-ARIN", "not-email") }, func() (*POC, error) { return c.DeletePOCEmail(ctx, "../bad", "a@example.net") }, func() (*POC, error) { return c.AddPOCPhone(ctx, "TEST-ARIN", POCPhone{Type: "X", Number: "123"}) }, func() (*POC, error) { return c.DeletePOCPhones(ctx, "TEST-ARIN", "", "") }} {
		if _, err := call(); err == nil {
			t.Fatal("invalid contact operation accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid operation reached server")
	}
}

func TestPOCContactMutationErrorsAreNotRetried(t *testing.T) {
	for _, status := range []int{202, 400, 403, 404, 409, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(status) }))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			if _, err := c.AddPOCEmail(context.Background(), "TEST-ARIN", "extra@example.net"); err == nil {
				t.Fatal("unconfirmed mutation accepted")
			}
			if calls != 1 {
				t.Fatalf("expected one mutation attempt, got %d", calls)
			}
		})
	}
}
func TestPOCContactsRequireObservedResult(t *testing.T) {
	p := examplePOC()
	p.Handle = "TEST-ARIN"
	p.RegistrationDate = "2026-01-01"
	p.Phones = append(p.Phones, POCPhone{Type: "F", Number: "+1-202-555-0101"})
	body, err := p.marshal()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = w.Write(body)
		} else {
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	ctx := context.Background()
	calls := []func() (*POC, error){
		func() (*POC, error) { return c.AddPOCEmail(ctx, p.Handle, "missing@example.net") },
		func() (*POC, error) { return c.DeletePOCEmail(ctx, p.Handle, "noc@example.net") },
		func() (*POC, error) {
			return c.AddPOCPhone(ctx, p.Handle, POCPhone{Type: "M", Number: "+1-202-555-0102"})
		},
		func() (*POC, error) { return c.DeletePOCPhones(ctx, p.Handle, "+1-202-555-0101", "F") },
	}
	for _, call := range calls {
		if _, err := call(); err == nil {
			t.Fatal("unobserved result accepted")
		}
	}
}
