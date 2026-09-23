package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveRDAPAudit(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires read-only live opt-in")
	}
	org := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if org == "" {
		t.Fatal("requires org handle")
	}
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	specs := map[string]arin.ReadSpec{}
	for _, s := range arin.PublicReads() {
		specs[s.Name] = s
	}
	for _, origin := range []string{arin.RDAPOTEURL, arin.RDAPProductionURL} {
		t.Run(origin, func(t *testing.T) {
			c, err := arin.New(arin.Config{RDAPBaseURL: origin})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			help, err := c.ReadRegistration(ctx, specs["rdap_help"], nil)
			if err != nil {
				t.Fatal(err)
			}
			networks, err := c.ListOrganizationNetworks(ctx, org)
			if err != nil || len(networks) == 0 {
				t.Fatalf("no networks: %v", err)
			}
			asns, err := c.ReadRegistration(ctx, specs["asns"], map[string]string{"org_handle": org})
			if err != nil {
				t.Fatal(err)
			}
			records := asns["asns"].([]any)
			if len(records) == 0 {
				t.Fatal("no ASNs")
			}
			first := records[0].(map[string]any)
			config := fmt.Sprintf(`provider "arin" { rdap_base_url=%q }
data "arin_rdap_help" "test" {}
data "arin_networks" "test" {
 org_handle=%q
 depends_on=[data.arin_rdap_help.test]
}
data "arin_asns" "test" {
 org_handle=%q
 depends_on=[data.arin_networks.test]
}
data "arin_asn" "test" {
 asn=%d
 depends_on=[data.arin_asns.test]
}
output "asn_handle" { value=jsondecode(data.arin_asn.test.rdap_json).handle }
output "asn_inventory_handle" { value=jsondecode(data.arin_asns.test.asns[0].rdap_json).handle }
`, origin, org, org, first["start_asn"].(int64))
			checks := []resource.TestCheckFunc{
				resource.TestCheckResourceAttr("data.arin_rdap_help.test", "conformance.#", strconv.Itoa(len(help["conformance"].([]any)))),
				resource.TestCheckResourceAttr("data.arin_rdap_help.test", "reverse_search_properties.#", strconv.Itoa(len(help["reverse_search_properties"].([]any)))),
				resource.TestCheckResourceAttrSet("data.arin_rdap_help.test", "rdap_json"),
				resource.TestCheckResourceAttr("data.arin_networks.test", "networks.%", strconv.Itoa(len(networks))),
				resource.TestCheckOutput("asn_handle", first["handle"].(string)),
				resource.TestCheckOutput("asn_inventory_handle", first["handle"].(string)),
			}
			previous := "data.arin_asn.test"
			for _, family := range []string{"v4", "v6"} {
				found := false
				for _, network := range networks {
					if network.IPVersion != family || len(network.CIDRs) == 0 {
						continue
					}
					found = true
					config += fmt.Sprintf("data \"arin_rdap_network\" %q {\n query=%q\n depends_on=[%s]\n}\noutput %q { value=jsondecode(data.arin_rdap_network.%s.rdap_json).handle }\noutput %q { value=jsondecode(data.arin_networks.test.networks[%q].rdap_json).handle }\n", family, network.CIDRs[0], previous, family+"_handle", family, family+"_inventory_handle", network.Handle)
					previous = "data.arin_rdap_network." + family
					checks = append(checks, resource.TestCheckOutput(family+"_handle", network.Handle), resource.TestCheckOutput(family+"_inventory_handle", network.Handle))
					break
				}
				if !found {
					t.Fatalf("missing %s inventory", family)
				}
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
		})
	}
}
