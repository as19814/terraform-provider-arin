package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveOrganization reads an existing organization. It never creates, updates,
// or deletes records. Live tests require a separate opt-in from fake acceptance tests.
func TestLiveOrganization(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" {
		t.Skip("set ARIN_LIVE_TESTS=1 and TF_ACC=1 to run read-only live tests")
	}
	if os.Getenv("TF_ACC") != "1" {
		t.Fatal("live tests also require TF_ACC=1")
	}
	if os.Getenv("ARIN_API_KEY") == "" {
		t.Fatal("ARIN_API_KEY is required")
	}
	handle := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if handle == "" {
		t.Fatal("ARIN_TEST_ORG_HANDLE is required")
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"arin": providerserver.NewProtocol6WithError(New("live-test")()),
		},
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf("provider \"arin\" {}\ndata \"arin_org\" \"live\" { handle = %q }", handle),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.arin_org.live", "handle", handle),
				resource.TestCheckResourceAttrSet("data.arin_org.live", "name"),
			),
		}},
	})
}

// TestLiveNetworks discovers the organization's public registrations without
// transmitting the API key. It intentionally does not pin a changing inventory count.
func TestLiveNetworks(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" {
		t.Skip("set ARIN_LIVE_TESTS=1 and TF_ACC=1 to run read-only live tests")
	}
	if os.Getenv("TF_ACC") != "1" {
		t.Fatal("live tests also require TF_ACC=1")
	}
	handle := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if handle == "" {
		t.Fatal("ARIN_TEST_ORG_HANDLE is required")
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())},
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf("provider \"arin\" {}\ndata \"arin_networks\" \"live\" { org_handle = %q }", handle),
			Check:  resource.TestCheckResourceAttrSet("data.arin_networks.live", "networks.%"),
		}},
	})
}

// TestLiveRegistrationCatalog chains discovery into individual reads. Optional
// object families are read only when existing records are discovered.
func TestLiveRegistrationCatalog(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" {
		t.Skip("set ARIN_LIVE_TESTS=1 and TF_ACC=1 for read-only live tests")
	}
	if os.Getenv("TF_ACC") != "1" || os.Getenv("ARIN_API_KEY") == "" {
		t.Fatal("TF_ACC=1 and ARIN_API_KEY are required")
	}
	handle := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if handle == "" {
		t.Fatal("ARIN_TEST_ORG_HANDLE is required")
	}
	config := fmt.Sprintf(`
provider "arin" {}
data "arin_org" "live" { handle = %[1]q }
data "arin_networks" "live" { org_handle = %[1]q }
data "arin_asns" "live" { org_handle = %[1]q }
data "arin_org_pocs" "live" { org_handle = %[1]q }
data "arin_asn" "live" {
 count = length(data.arin_asns.live.asns) > 0 ? 1 : 0
 asn = data.arin_asns.live.asns[0].start_asn
}
data "arin_net" "live" { handle = keys(data.arin_networks.live.networks)[0] }
data "arin_parent_net" "live" {
 start_address = data.arin_net.live.net_blocks[0].start_address
 end_address = data.arin_net.live.net_blocks[0].end_address
}
data "arin_most_specific_net" "live" {
 start_address = data.arin_net.live.net_blocks[0].start_address
 end_address = data.arin_net.live.net_blocks[0].end_address
}
data "arin_nets_by_ip_range" "live" {
 start_address = data.arin_net.live.net_blocks[0].start_address
 end_address = data.arin_net.live.net_blocks[0].end_address
}
data "arin_net_delegations" "live" { net_handle = data.arin_net.live.handle }
data "arin_delegation" "live" {
 count = length(data.arin_net_delegations.live.delegations) > 0 ? 1 : 0
 name = data.arin_net_delegations.live.delegations[0].name
}
data "arin_poc" "live" {
 count = length(data.arin_org.live.poc_links) > 0 ? 1 : 0
 handle = data.arin_org.live.poc_links[0].handle
}
data "arin_customer" "live" {
 count = data.arin_net.live.customer_handle != null && data.arin_net.live.customer_handle != "" ? 1 : 0
 handle = data.arin_net.live.customer_handle
}
data "arin_net_routes" "live" { net_handle = data.arin_net.live.handle }
data "arin_irr_routes" "live" { org_handle = %[1]q }
data "arin_irr_route" "live" {
 count = length(data.arin_irr_routes.live.routes) > 0 ? 1 : 0
 prefix = data.arin_irr_routes.live.routes[0].prefix
 asn = tonumber(trimprefix(data.arin_irr_routes.live.routes[0].origin_as, "AS"))
}
data "arin_irr_aut_nums" "live" { org_handle = %[1]q }
data "arin_irr_aut_num" "live" {
 count = length(data.arin_irr_aut_nums.live.aut_nums) > 0 ? 1 : 0
 asn = tonumber(trimprefix(data.arin_irr_aut_nums.live.aut_nums[0].as_number, "AS"))
}
data "arin_irr_as_sets" "live" { org_handle = %[1]q }
data "arin_irr_as_set" "live" {
 count = length(data.arin_irr_as_sets.live.as_sets) > 0 ? 1 : 0
 name = data.arin_irr_as_sets.live.as_sets[0].name
}
data "arin_irr_route_sets" "live" { org_handle = %[1]q }
data "arin_irr_route_set" "live" {
 count = length(data.arin_irr_route_sets.live.route_sets) > 0 ? 1 : 0
 name = data.arin_irr_route_sets.live.route_sets[0].name
}
data "arin_roas" "live" { org_handle = %[1]q }
data "arin_roa" "live" {
 count = length(data.arin_roas.live.roas) > 0 ? 1 : 0
 org_handle = %[1]q
 handle = data.arin_roas.live.roas[0].handle
}
data "arin_aspas" "live" { org_handle = %[1]q }
data "arin_aspa" "live" {
 count = length(data.arin_aspas.live.aspas) > 0 ? 1 : 0
 org_handle = %[1]q
 customer_asn = data.arin_aspas.live.aspas[0].customer_asn
}
`, handle)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())},
		Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttrSet("data.arin_net.live", "handle"),
			resource.TestCheckResourceAttrSet("data.arin_net.live", "net_blocks.0.start_address"),
			resource.TestCheckResourceAttrSet("data.arin_aspas.live", "aspas.#"),
			resource.TestCheckResourceAttrSet("data.arin_roas.live", "roas.#"),
		)}},
	})
}
