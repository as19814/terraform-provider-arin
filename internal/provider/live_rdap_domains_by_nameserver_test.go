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

func TestLiveRDAPDomainsByNameserver(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires read-only live opt-in")
	}
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	var spec, lookup arin.ReadSpec
	for _, s := range arin.PublicReads() {
		if s.Name == "rdap_domains_by_nameserver" {
			spec = s
		}
		if s.Name == "rdap_domain" {
			lookup = s
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
			expected, err := c.ReadRegistration(ctx, spec, map[string]string{"nameserver": "ns1.arin.net"})
			if err != nil {
				t.Fatal(err)
			}
			records := expected["domains"].([]any)
			if len(records) == 0 {
				t.Fatal("reference nameserver has no domains")
			}
			// Independently look up one matched reverse domain, rather than relying only on the search parser.
			name := records[0].(map[string]any)["name"].(string)
			domain, err := c.ReadRegistration(ctx, lookup, map[string]string{"name": name})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, raw := range domain["nameservers"].([]any) {
				if raw.(map[string]any)["name"] == "ns1.arin.net" {
					found = true
				}
			}
			if !found {
				t.Fatal("independent lookup does not match searched nameserver")
			}
			checks := []resource.TestCheckFunc{resource.TestCheckResourceAttr("data.arin_rdap_domains_by_nameserver.test", "domains.#", strconv.Itoa(len(records))), resource.TestCheckResourceAttr("data.arin_rdap_domains_by_nameserver.missing", "domains.#", "0")}
			for i, raw := range records {
				checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_domains_by_nameserver.test", fmt.Sprintf("domains.%d.name", i), raw.(map[string]any)["name"].(string)))
			}
			config := fmt.Sprintf(`provider "arin" { rdap_base_url=%q }
data "arin_rdap_domains_by_nameserver" "test" { nameserver="NS1.ARIN.NET." }
data "arin_rdap_domains_by_nameserver" "missing" {
 nameserver="nonexistent-codex-test.invalid"
 depends_on=[data.arin_rdap_domains_by_nameserver.test]
}
`, origin)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
			t.Logf("verified %d reverse domains and no-match query", len(records))
		})
	}
}
