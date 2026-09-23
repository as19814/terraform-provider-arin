package arin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testCustomer() Customer {
	return Customer{Handle: "C123", Name: "Example & customer", CountryCode: "US", Subdivision: "VA", PostalCode: "20151", StreetAddress: []string{"123 Test Street"}, RegistrationDate: "2026-01-01T00:00:00Z", ParentOrgHandle: "EXAMPLE-1"}
}
func TestCustomerValidation(t *testing.T) {
	for _, change := range []func(*Customer){
		func(c *Customer) { c.Name = "" },
		func(c *Customer) { c.CountryCode = "usa" },
		func(c *Customer) { c.StreetAddress = nil },
		func(c *Customer) { c.StreetAddress = []string{"one\ntwo"} },
		func(c *Customer) { c.PostalCode = "" },
		func(c *Customer) { c.Comments = []string{""} },
	} {
		c := testCustomer()
		change(&c)
		if c.Validate() == nil {
			t.Fatal("accepted invalid customer")
		}
	}
}
func TestCustomerResponseValidation(t *testing.T) {
	b, err := testCustomer().marshal()
	if err != nil {
		t.Fatal(err)
	}
	c, err := decodeCustomer(b, "C123")
	if err != nil || c.Name != testCustomer().Name {
		t.Fatalf("round trip: %v", err)
	}
	for _, body := range []string{
		strings.ReplaceAll(string(b), "</customer>", "<unsupported>value</unsupported></customer>"),
		strings.ReplaceAll(string(b), "<privateCustomer>false</privateCustomer>", ""),
		strings.ReplaceAll(string(b), "C123", "C999"),
		"<customer/>",
	} {
		if _, err := decodeCustomer([]byte(body), "C123"); err == nil {
			t.Fatal("accepted incomplete or mismatched customer")
		}
	}
}
func TestCustomerWriteFailuresAreNotRetried(t *testing.T) {
	for _, status := range []int{202, 400, 403, 409, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(status) }))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			if _, err := c.CreateCustomer(context.Background(), "NET-192-0-2-0-1", testCustomer()); err == nil {
				t.Fatal("accepted failed or pending write")
			}
			if calls != 1 {
				t.Fatalf("retried create: %d", calls)
			}
		})
	}
}
func TestCustomerDeleteMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	if err := c.DeleteCustomer(context.Background(), "C123"); err != nil {
		t.Fatal(err)
	}
}

func TestCustomerUpdateReadFailurePreventsWrite(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" {
			t.Error("wrote without verified current identity")
		}
		w.WriteHeader(403)
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	if _, err := c.UpdateCustomer(context.Background(), testCustomer()); err == nil {
		t.Fatal("ignored failed read")
	}
	if calls != 1 {
		t.Fatalf("unexpected calls: %d", calls)
	}
}
func TestCustomerContextValidation(t *testing.T) {
	for _, pair := range [][2]string{{"../net", "C1"}, {"NET-1", "C1?query"}, {"", "C1"}, {"NET-1", "C1/other"}} {
		if ValidateCustomerContext(pair[0], pair[1]) == nil {
			t.Fatal("accepted invalid identity")
		}
	}
	if err := ValidateCustomerContext("NET-192-0-2-0-1", "C123"); err != nil {
		t.Fatal(err)
	}
}
