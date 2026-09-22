package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// These acceptance tests run real Terraform against a local fake, never ARIN.
func TestAccOrganizationDataSource(t *testing.T) {
	var updated atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/rest/org/EXAMPLE-1" || r.Header.Get("Authorization") != "ApiKey acceptance-test-key" {
			t.Errorf("unexpected API request")
			w.WriteHeader(400)
			return
		}
		name := "Example Organization"
		if updated.Load() {
			name = "Updated Organization"
		}
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<org xmlns="http://www.arin.net/regrws/core/v1"><handle>EXAMPLE-1</handle><orgName>%s</orgName><registrationDate>2026-01-01</registrationDate></org>`, name)
	}))
	defer server.Close()
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	config := `provider "arin" {}
data "arin_org" "test" { handle = "EXAMPLE-1" }`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps: []resource.TestStep{
			{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_org.test", "handle", "EXAMPLE-1"), resource.TestCheckResourceAttr("data.arin_org.test", "name", "Example Organization"), resource.TestCheckResourceAttr("data.arin_org.test", "registration_date", "2026-01-01"))},
			{PreConfig: func() { updated.Store(true) }, Config: config, Check: resource.TestCheckResourceAttr("data.arin_org.test", "name", "Updated Organization")},
		},
	})
}

func TestAccOrganizationNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, `<error><code>E_OBJECT_NOT_FOUND</code><message>Organization does not exist</message></error>`)
	}))
	defer server.Close()
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps: []resource.TestStep{{Config: `provider "arin" {}
data "arin_org" "test" { handle = "MISSING-1" }`, ExpectError: regexp.MustCompile("E_OBJECT_NOT_FOUND")}},
	})
}
