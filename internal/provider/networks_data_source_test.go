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

const testNetwork = `{"objectClassName":"ip network","handle":"NET-192-0-2-0-1","name":"EXAMPLE","startAddress":"192.0.2.0","endAddress":"192.0.2.255","ipVersion":"v4","type":"DIRECT ALLOCATION","entities":[{"handle":"EXAMPLE-1","roles":["registrant"]}],"cidr0_cidrs":[{"v4prefix":"192.0.2.0","length":24}]}`

func TestAccNetworksDataSource(t *testing.T) {
	var empty atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("public lookup sent authorization")
		}
		if r.URL.Path != "/registry/ips/reverse_search/entity" || r.URL.Query().Get("handle") != "EXAMPLE-1" {
			t.Error("incorrect RDAP request")
		}
		if empty.Load() {
			fmt.Fprint(w, `{"ipSearchResults":[]}`)
			return
		}
		fmt.Fprintf(w, `{"ipSearchResults":[%s]}`, testNetwork)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	config := `provider "arin" {}
data "arin_networks" "test" { org_handle = "EXAMPLE-1" }`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps: []resource.TestStep{
			{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.arin_networks.test", "networks.%", "1"),
				resource.TestCheckResourceAttr("data.arin_networks.test", "networks.NET-192-0-2-0-1.name", "EXAMPLE"),
				resource.TestCheckResourceAttr("data.arin_networks.test", "networks.NET-192-0-2-0-1.cidrs.0", "192.0.2.0/24"),
			)},
			{PreConfig: func() { empty.Store(true) }, Config: config, Check: resource.TestCheckResourceAttr("data.arin_networks.test", "networks.%", "0")},
		},
	})
}

func TestAccNetworksTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ipSearchResults":[],"notices":[{"type":"result set truncated due to excessive load"}]}`)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps: []resource.TestStep{{Config: `provider "arin" {}
data "arin_networks" "test" { org_handle = "EXAMPLE-1" }`, ExpectError: regexp.MustCompile("truncated the RDAP response; refusing a partial result")}},
	})
}
