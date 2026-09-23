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

func TestAccRDAPNetwork(t *testing.T) {
	fixture, err := os.ReadFile("../arin/testdata/rdap_network.json")
	if err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Method != "GET" || (r.URL.Path != "/registry/ip/192.0.2.1" && r.URL.Path != "/registry/ip/192.0.2.0/24") {
			t.Error("unexpected public lookup")
			w.WriteHeader(400)
			return
		}
		body := string(fixture)
		if changed.Load() {
			body = strings.Replace(body, "EXAMPLE-NET", "UPDATED-NET", 1)
		}
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	config := `provider "arin" {}
data "arin_rdap_network" "address" { query="192.0.2.1" }
data "arin_rdap_network" "prefix" { query="192.0.2.0/24" }
`
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_rdap_network.address", "handle", "NET-192-0-2-0-1"), resource.TestCheckResourceAttr("data.arin_rdap_network.prefix", "cidrs.0", "192.0.2.0/24"), resource.TestCheckResourceAttr("data.arin_rdap_network.prefix", "org_handles.0", "EXAMPLE-1"), resource.TestCheckResourceAttr("data.arin_rdap_network.prefix", "events.0.action", "registration"))},
		{Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.TestCheckResourceAttr("data.arin_rdap_network.address", "name", "UPDATED-NET")},
		{Config: config, PlanOnly: true},
	}})
}
func TestAccRDAPNetworkMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: `provider "arin" {}
data "arin_rdap_network" "test" { query="192.0.2.1" }`, ExpectError: regexp.MustCompile("HTTP 404")}}})
}
