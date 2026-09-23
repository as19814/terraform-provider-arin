package arin

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func examplePOC() POC {
	return POC{ContactType: "ROLE", CompanyName: "Example Networks", LastName: "Test NOC", CountryCode: "US", Subdivision: "VA", City: "Chantilly", PostalCode: "20151", StreetAddress: []string{"123 Example Street"}, Emails: []string{"noc@example.net"}, Phones: []POCPhone{{Type: "O", Number: "+1-202-555-0100"}}, Comments: []string{"Test contact"}}
}
func TestPOCClientLifecycle(t *testing.T) {
	var stored []byte
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey test-key" {
			w.WriteHeader(403)
			return
		}
		switch r.Method {
		case "POST", "PUT":
			var p pocXML
			if err := xml.NewDecoder(r.Body).Decode(&p); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if r.Method == "POST" {
				if r.URL.Path != "/rest/poc;makeLink=true" || p.Handle != "" || p.RegistrationDate != "" {
					t.Error("invalid create identity or linking")
				}
				p.Handle = "TEST-ARIN"
				p.RegistrationDate = "2026-01-01"
			} else if p.Handle != "TEST-ARIN" || p.RegistrationDate != "2026-01-01" {
				t.Error("update lost server identity")
			}
			stored, _ = xml.Marshal(p)
			writes++
			_, _ = w.Write(stored)
		case "GET":
			if stored == nil {
				w.WriteHeader(404)
			} else {
				_, _ = w.Write(stored)
			}
		case "DELETE":
			stored = nil
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	ctx := context.Background()
	p := examplePOC()
	p.Handle = "IGNORED"
	p.RegistrationDate = "ignored"
	created, err := c.CreatePOC(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if created.Handle != "TEST-ARIN" {
		t.Fatal("missing identity")
	}
	changed := *created
	changed.City = "Herndon"
	changed.Comments = nil
	changed.Emails = append(changed.Emails, "abuse@example.net")
	changed.Phones = append(changed.Phones, POCPhone{Type: "F", Number: "+1-202-555-0101", Extension: "42"})
	out, err := c.UpdatePOC(ctx, changed)
	if err != nil {
		t.Fatal(err)
	}
	if out.City != "Herndon" || len(out.Comments) != 0 || len(out.Phones) != 2 || len(out.Emails) != 2 {
		t.Fatal("update lost configured fields")
	}
	changed.LastName = "Changed"
	if _, err = c.UpdatePOC(ctx, changed); err == nil {
		t.Fatal("immutable rename accepted")
	}
	if writes != 2 {
		t.Fatal("immutable rename sent a write")
	}
	if err = c.DeletePOC(ctx, created.Handle); err != nil {
		t.Fatal(err)
	}
	if err = c.DeletePOC(ctx, created.Handle); err != nil {
		t.Fatal(err)
	}
}
func TestPOCValidation(t *testing.T) {
	for _, mutate := range []func(*POC){func(p *POC) { p.ContactType = "X" }, func(p *POC) { p.Emails = nil }, func(p *POC) { p.Emails = []string{"Name <a@example.net>"} }, func(p *POC) { p.Phones = nil }, func(p *POC) { p.FirstName = "Invalid role" }, func(p *POC) { p.Phones = append(p.Phones, p.Phones[0]) }, func(p *POC) { p.CountryCode = "us" }} {
		p := examplePOC()
		mutate(&p)
		if err := p.Validate(); err == nil {
			t.Fatal("invalid POC accepted")
		}
	}
	p := examplePOC()
	p.ContactType = "PERSON"
	p.FirstName = "Example"
	p.LastName = "Person"
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestPOCStrictResponse(t *testing.T) {
	p := examplePOC()
	p.Handle = "TEST-ARIN"
	p.RegistrationDate = "2026-01-01"
	body, err := p.marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "<firstName>") || strings.Contains(string(body), "<middleName>") {
		t.Fatal("empty immutable names must be omitted")
	}
	if _, err = decodePOC(body, p.Handle); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{strings.Replace(string(body), "</poc>", "<unknown>value</unknown></poc>", 1), strings.Replace(string(body), "</phones>", "<unknown/></phones>", 1), strings.Replace(string(body), "</poc>", "<lastName>duplicate</lastName></poc>", 1), strings.Replace(string(body), "<city>", "<city unknown=\"x\">", 1)} {
		if _, err := decodePOC([]byte(body), p.Handle); err == nil {
			t.Fatal("unsupported fields accepted")
		}
	}
	if _, err = decodePOC(body, "OTHER-ARIN"); err == nil {
		t.Fatal("mismatched handle accepted")
	}
}
func TestPOCReadFailurePreventsWrite(t *testing.T) {
	for _, status := range []int{403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			writes := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				w.WriteHeader(status)
			}))
			defer s.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
			p := examplePOC()
			p.Handle = "TEST-ARIN"
			if _, err := c.UpdatePOC(context.Background(), p); err == nil {
				t.Fatal("failed read accepted")
			}
			if writes != 0 {
				t.Fatal("mutation after failed preflight")
			}
		})
	}
}
