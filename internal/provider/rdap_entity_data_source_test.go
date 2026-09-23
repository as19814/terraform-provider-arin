package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRDAPEntity(t *testing.T) {
	fixture, err := os.ReadFile("../arin/testdata/rdap_entity.json")
	if err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Method != "GET" || (r.URL.Path != "/registry/entity/EXAMPLE-1") {
			t.Error("unexpected public lookup")
			w.WriteHeader(400)
			return
		}
		body := string(fixture)
		if changed.Load() {
			body = strings.Replace(body, "Example Organization", "Updated Organization", 1)
		}
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	config := `provider "arin" {}
data "arin_rdap_entity" "test" { handle="EXAMPLE-1" }
`
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_rdap_entity.test", "kind", "org"), resource.TestCheckResourceAttr("data.arin_rdap_entity.test", "names.0", "Example Organization"), resource.TestCheckResourceAttr("data.arin_rdap_entity.test", "emails.0", "noc@example.net"), resource.TestCheckResourceAttr("data.arin_rdap_entity.test", "entities.0.handle", "EXAMPLE-ARIN"))},
		{Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.TestCheckResourceAttr("data.arin_rdap_entity.test", "names.1", "Updated Organization")},
		{Config: config, PlanOnly: true},
	}})
}
func TestAccRDAPEntityMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: `provider "arin" {}
data "arin_rdap_entity" "test" { handle="EXAMPLE-1" }`, ExpectError: regexp.MustCompile("HTTP 404")}}})
}
