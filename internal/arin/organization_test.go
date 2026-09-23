package arin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func exampleOrganization() RegisteredOrganization {
	b := true
	return RegisteredOrganization{Handle: "EXAMPLE-1", Name: "Example Organization", RegistrationDate: "2026-09-22T00:00:00Z", CountryCode: "US", City: "Example", Subdivision: "VA", PostalCode: "20151", StreetAddress: []string{"123 Example Street"}, TaxID: "test-tax-id", AcceptReassignments: &b, POCs: []OrgPOC{{Handle: "EXAMPLE-ARIN", Function: "AD"}, {Handle: "EXAMPLE-ARIN", Function: "T"}, {Handle: "EXAMPLE-ARIN", Function: "AB"}}}
}
func organizationBody(t *testing.T, o RegisteredOrganization) []byte {
	t.Helper()
	b, err := o.marshal()
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestRegisteredOrganizationLifecycle(t *testing.T) {
	stored := exampleOrganization()
	exists := false
	writes := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey test-key" {
			t.Error("missing authentication")
		}
		switch r.Method {
		case "GET":
			if !exists {
				w.WriteHeader(404)
				return
			}
		case "POST", "PUT":
			writes++
			b, _ := io.ReadAll(r.Body)
			if r.Method == "POST" {
				if r.URL.Path != "/rest/org" || strings.Contains(string(b), "<handle>") || strings.Contains(string(b), "<registrationDate>") {
					t.Error("invalid create payload")
				}
				b = []byte(strings.Replace(string(b), "</org>", "<handle>EXAMPLE-1</handle><registrationDate>2026-09-22T00:00:00Z</registrationDate></org>", 1))
			}
			o, err := decodeRegisteredOrganization(b, "EXAMPLE-1")
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			stored = *o
			exists = true
		case "DELETE":
			exists = false
		default:
			t.Error("unexpected method")
		}
		_, _ = w.Write(organizationBody(t, stored))
	}))
	defer s.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
	ctx := context.Background()
	out, err := c.CreateOrganization(ctx, exampleOrganization())
	if err != nil || out.Organization == nil {
		t.Fatalf("create: %v", err)
	}
	update := *out.Organization
	update.City = "Changed"
	update.Comments = []string{"Test operations"}
	update.RegistrationDate = "ignored"
	out, err = c.UpdateOrganization(ctx, update)
	if err != nil {
		t.Fatal(err)
	}
	if out.Organization.City != "Changed" || out.Organization.RegistrationDate != exampleOrganization().RegistrationDate {
		t.Fatal("update or generated metadata incorrect")
	}
	update = *out.Organization
	update.Comments = nil
	update.TaxID = ""
	update.RWhoisURL = ""
	b := false
	update.AcceptReassignments = &b
	out, err = c.UpdateOrganization(ctx, update)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Organization.Comments) != 0 || out.Organization.TaxID != "" || *out.Organization.AcceptReassignments {
		t.Fatal("clearing failed")
	}
	for _, field := range []string{"name", "dba"} {
		u := update
		if field == "name" {
			u.Name = "Changed"
		} else {
			u.DBAName = "Changed"
		}
		if _, err = c.UpdateOrganization(ctx, u); err == nil {
			t.Fatal("immutable field accepted")
		}
	}
	if writes != 3 {
		t.Fatal("immutable update sent a mutation")
	}
	if _, err = c.DeleteOrganization(ctx, "EXAMPLE-1"); err != nil {
		t.Fatal(err)
	}
	if _, err = c.GetRegisteredOrganization(ctx, "EXAMPLE-1"); !IsNotFound(err) {
		t.Fatal("organization still present")
	}
}
func TestRegisteredOrganizationDecode(t *testing.T) {
	original := exampleOrganization()
	body := organizationBody(t, original)
	decoded, err := decodeRegisteredOrganization(body, original.Handle)
	original.POCs = sortedOrgPOCs(original.POCs)
	if err != nil || !reflect.DeepEqual(decoded, &original) {
		t.Fatalf("round trip failed: %v; got %#v want %#v", err, decoded, original)
	}
	for _, fragment := range []string{`<unknown/>`, `<city>duplicate</city>`, `<pocLinks/>`, `<messages><message/></messages>`} {
		invalid := strings.Replace(string(body), "</org>", fragment+"</org>", 1)
		if _, err := decodeRegisteredOrganization([]byte(invalid), original.Handle); err == nil {
			t.Errorf("accepted partial representation: %s", fragment)
		}
	}
	for _, body := range []string{strings.Replace(string(body), `function="AD"`, `function="N"`, 1), strings.Replace(string(body), `<city>Example</city>`, `<city extra="x">Example</city>`, 1), strings.Replace(string(body), `<taxId>test-tax-id</taxId>`, `<taxId><unknown/></taxId>`, 1)} {
		if _, err := decodeRegisteredOrganization([]byte(body), original.Handle); err == nil {
			t.Error("accepted invalid org")
		}
	}
	if _, err := decodeRegisteredOrganization(body, "OTHER-1"); err == nil {
		t.Fatal("accepted mismatched identity")
	}
}
func TestOrganizationWriteTickets(t *testing.T) {
	ticket := `<ticket xmlns="http://www.arin.net/regrws/core/v1"><ticketNo>20260922-X1</ticketNo><webTicketStatus>PENDING_REVIEW</webTicketStatus></ticket>`
	for _, status := range []int{200, 201, 202} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(status); fmt.Fprint(w, ticket) }))
			defer s.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
			out, err := c.CreateOrganization(context.Background(), exampleOrganization())
			if err != nil || out.TicketNumber != "20260922-X1" || out.Organization != nil || calls != 1 {
				t.Fatalf("ticket lost or request retried: %v", err)
			}
		})
	}
	for _, children := range []string{`<org/>` + ticket, ticket + `<org/>`, `<unknown/>` + ticket, ticket + `<ticket/>`} {
		out, err := decodeOrganizationWriteResult([]byte(`<ticketedRequest xmlns="http://www.arin.net/regrws/core/v1">`+children+`</ticketedRequest>`), "")
		if err == nil || out == nil || out.TicketNumber != "20260922-X1" {
			t.Fatal("accepted malformed write or lost recovery ticket")
		}
	}
}
func TestOrganizationWriteErrorsAndDeleteVerification(t *testing.T) {
	for _, status := range []int{403, 409, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			writes := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writes++; w.WriteHeader(status) }))
			defer s.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
			if _, err := c.CreateOrganization(context.Background(), exampleOrganization()); err == nil {
				t.Fatal("write error ignored")
			}
			if writes != 1 {
				t.Fatal("write retried")
			}
		})
	}
	for _, status := range []int{200, 202, 204} {
		t.Run("delete"+fmt.Sprint(status), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "DELETE" {
					w.WriteHeader(status)
					return
				}
				w.Write(organizationBody(t, exampleOrganization()))
			}))
			defer s.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
			if _, err := c.DeleteOrganization(context.Background(), "EXAMPLE-1"); err == nil {
				t.Fatal("unconfirmed deletion accepted")
			}
		})
	}
}

func TestOrganizationPendingDelete(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "DELETE" {
			t.Error("unexpected read after pending deletion")
		}
		w.WriteHeader(202)
		fmt.Fprint(w, `<ticket xmlns="http://www.arin.net/regrws/core/v1"><ticketNo>20260922-X1</ticketNo><webTicketStatus>IN_PROGRESS</webTicketStatus></ticket>`)
	}))
	defer s.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
	out, err := c.DeleteOrganization(context.Background(), "EXAMPLE-1")
	if err != nil || out.TicketNumber != "20260922-X1" || calls != 1 {
		t.Fatalf("pending deletion not retained: %v", err)
	}
}
func TestOrganizationAsyncWriteRequiresTicket(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		w.Write(organizationBody(t, exampleOrganization()))
	}))
	defer s.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
	out, err := c.CreateOrganization(context.Background(), exampleOrganization())
	if err == nil || out == nil || out.Organization == nil {
		t.Fatal("async response must report error while retaining known identity")
	}
}
