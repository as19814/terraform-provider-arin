package provider

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveRDAPNetworkHierarchy(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires read-only live opt-in")
	}
	org := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if org == "" {
		t.Fatal("requires organization handle")
	}
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	var spec arin.ReadSpec
	for _, s := range arin.PublicReads() {
		if s.Name == "rdap_network_hierarchy" {
			spec = s
		}
	}
	for _, origin := range []string{arin.RDAPOTEURL, arin.RDAPProductionURL} {
		t.Run(origin, func(t *testing.T) {
			c, err := arin.New(arin.Config{RDAPBaseURL: origin})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			networks, err := c.ListOrganizationNetworks(ctx, org)
			if err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf("provider \"arin\" { rdap_base_url=%q }\n", origin)
			checks := []resource.TestCheckFunc{}
			previous := ""
			add := func(label, query, relation string, active bool) []any {
				expected, err := c.ReadRegistration(ctx, spec, map[string]string{"query": query, "relation": relation, "active_only": strconv.FormatBool(active)})
				if err != nil {
					t.Fatalf("%s: %v", label, err)
				}
				config += fmt.Sprintf("data \"arin_rdap_network_hierarchy\" %q {\n query=%q\n relation=%q\n active_only=%t\n", label, query, relation, active)
				if previous != "" {
					config += " depends_on=[data.arin_rdap_network_hierarchy." + previous + "]\n"
				}
				config += "}\n"
				previous = label
				records := expected["networks"].([]any)
				address := "data.arin_rdap_network_hierarchy." + label
				checks = append(checks, resource.TestCheckResourceAttr(address, "networks.#", strconv.Itoa(len(records))))
				for i, raw := range records {
					record := raw.(map[string]any)
					for _, field := range []string{"handle", "start_address", "end_address", "ip_version"} {
						checks = append(checks, resource.TestCheckResourceAttr(address, fmt.Sprintf("networks.%d.%s", i, field), record[field].(string)))
					}
					checks = append(checks, resource.TestCheckResourceAttrSet(address, fmt.Sprintf("networks.%d.rdap_json", i)))
				}
				t.Logf("%s returned %d networks", label, len(records))
				return records
			}
			for _, family := range []string{"v4", "v6"} {
				query, handle := "", ""
				for _, network := range networks {
					if network.IPVersion == family && len(network.CIDRs) > 0 {
						query = network.CIDRs[0]
						handle = network.Handle
						break
					}
				}
				if query == "" {
					t.Fatalf("no %s network for hierarchy test", family)
				}
				for _, relation := range []string{"top", "up", "down", "bottom"} {
					records := add(family+"_prefix_"+relation, query, relation, false)
					if (relation == "top" || relation == "up") && len(records) != 1 {
						t.Fatalf("missing %s ancestor", family)
					}
				}
				active := add(family+"_active_top", query, "top", true)
				if len(active) != 1 || active[0].(map[string]any)["handle"] != handle {
					t.Fatal("active top does not match independently discovered registrant network")
				}
				add(family+"_active_up", query, "up", true)
				prefix, err := netip.ParsePrefix(query)
				if err != nil {
					t.Fatal(err)
				}
				address := prefix.Addr().Next().String()
				for _, relation := range []string{"top", "up", "down", "bottom"} {
					records := add(family+"_address_"+relation, address, relation, false)
					if relation == "down" && len(records) != 0 {
						t.Fatal("point address has children")
					}
					if relation != "down" && len(records) == 0 {
						t.Fatal("point address has no covering network")
					}
				}
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
		})
	}
}
