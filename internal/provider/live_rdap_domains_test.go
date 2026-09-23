package provider

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveRDAPDomains(t *testing.T) {
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
	var lookup, search arin.ReadSpec
	for _, s := range arin.PublicReads() {
		switch s.Name {
		case "rdap_domain":
			lookup = s
		case "rdap_domains":
			search = s
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
			partialSteps := []resource.TestStep{}
			previous := ""
			add := func(label, name, relation string, active, allowPartial bool) []any {
				params := map[string]string{"name": name, "relation": relation, "active_only": strconv.FormatBool(active)}
				expected, err := c.ReadRegistration(ctx, search, params)
				block := fmt.Sprintf("data \"arin_rdap_domains\" %q {\n name=%q\n relation=%q\n active_only=%t\n", label, name, relation, active)
				if err != nil {
					if allowPartial && strings.Contains(err.Error(), "truncated") {
						partialSteps = append(partialSteps, resource.TestStep{Config: fmt.Sprintf("provider \"arin\" { rdap_base_url=%q }\n", origin) + block + "}\n", ExpectError: regexp.MustCompile("truncated")})
						t.Log("broad native child query was truncated; verifying Terraform rejects it")
						return nil
					}
					t.Fatalf("%s query failed: %v", label, err)
				}
				if previous != "" {
					block += " depends_on=[data.arin_rdap_domains." + previous + "]\n"
				}
				config += block + "}\n"
				previous = label
				records := expected["domains"].([]any)
				checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_domains."+label, "domains.#", strconv.Itoa(len(records))))
				for i, raw := range records {
					checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_domains."+label, fmt.Sprintf("domains.%d.name", i), raw.(map[string]any)["name"].(string)))
				}
				return records
			}
			for _, family := range []string{"v4", "v6"} {
				zone := ""
				for _, network := range networks {
					if network.IPVersion != family {
						continue
					}
					for _, cidr := range network.CIDRs {
						prefix, err := netip.ParsePrefix(cidr)
						if err != nil {
							t.Fatal(err)
						}
						candidate := liveReverseZone(prefix)
						if _, err := c.ReadRegistration(ctx, lookup, map[string]string{"name": candidate}); arin.IsNotFound(err) {
							continue
						} else if err != nil {
							t.Fatal(err)
						}
						zone = candidate
						break
					}
					if zone != "" {
						break
					}
				}
				if zone == "" {
					t.Fatalf("no %s domain for native hierarchy test", family)
				}
				var top []any
				for _, relation := range []string{"top", "up", "down", "bottom"} {
					records := add(family+"_"+relation, zone, relation, false, false)
					if (relation == "top" || relation == "up") && len(records) != 1 {
						t.Fatalf("no native %s ancestor for %s", relation, family)
					}
					if relation == "top" {
						top = records
					}
				}
				for _, relation := range []string{"top", "up"} {
					add(family+"_active_"+relation, zone, relation, true, false)
				}
				_, parent, _ := strings.Cut(zone, ".")
				if len(add(family+"_parent_bottom", parent, "bottom", false, false)) == 0 {
					t.Fatal("no bottom result for containing prefix")
				}
				if family == "v6" {
					add("v6_ancestor_children", top[0].(map[string]any)["name"].(string), "down", false, true)
				}
			}
			// This hierarchy is the child-search example in ARIN's public RDAP guide.
			if len(add("reference_children", "112.149.in-addr.arpa.", "down", false, false)) == 0 {
				t.Fatal("reference hierarchy has no children")
			}
			steps := []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}
			steps = append(steps, partialSteps...)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: steps})
		})
	}
}
