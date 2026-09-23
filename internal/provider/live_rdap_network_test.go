package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Both origins are read-only. No API key is required or sent.
func TestLiveRDAPNetwork(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires read-only live opt-in")
	}
	org := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if org == "" {
		t.Fatal("requires organization handle for discovery")
	}
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	var spec arin.ReadSpec
	for _, s := range arin.PublicReads() {
		if s.Name == "rdap_network" {
			spec = s
		}
	}
	for _, origin := range []string{arin.RDAPOTEURL, arin.RDAPProductionURL} {
		t.Run(origin, func(t *testing.T) {
			c, err := arin.New(arin.Config{RDAPBaseURL: origin})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			networks, err := c.ListOrganizationNetworks(ctx, org)
			if err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf("provider \"arin\" { rdap_base_url=%q }\n", origin)
			checks := []resource.TestCheckFunc{}
			for _, family := range []string{"v4", "v6"} {
				found := false
				for _, network := range networks {
					if network.IPVersion != family || len(network.CIDRs) == 0 {
						continue
					}
					queries := []string{network.StartAddress, network.CIDRs[0]}
					for i, query := range queries {
						expected, err := c.ReadRegistration(ctx, spec, map[string]string{"query": query})
						if err != nil {
							t.Fatal(err)
						}
						name := fmt.Sprintf("%s_%d", family, i)
						config += fmt.Sprintf("data \"arin_rdap_network\" %q { query=%q }\n", name, query)
						checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_network."+name, "handle", expected["handle"].(string)), resource.TestCheckResourceAttr("data.arin_rdap_network."+name, "ip_version", family))
					}
					found = true
					break
				}
				if !found {
					t.Fatalf("no %s network available for native lookup", family)
				}
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
			t.Logf("verified address and prefix lookups for IPv4/IPv6 on %s", strings.TrimPrefix(origin, "https://"))
		})
	}
}
