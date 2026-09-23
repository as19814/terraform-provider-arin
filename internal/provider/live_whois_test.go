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

func TestLiveWhoisLookups(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires read-only live opt-in")
	}
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	t.Setenv("ARIN_WHOIS_BASE_URL", "")
	specs := map[string]arin.ReadSpec{}
	for _, s := range arin.WhoisReads() {
		specs[s.Name] = s
	}
	for _, origin := range []string{arin.WhoisOTEURL, arin.WhoisProductionURL} {
		t.Run(origin, func(t *testing.T) {
			c, err := arin.New(arin.Config{WhoisBaseURL: origin})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			config := fmt.Sprintf("provider \"arin\" { whois_base_url=%q }\n", origin)
			previous := ""
			checks := []resource.TestCheckFunc{}
			for _, tc := range []struct {
				kind, label, identity string
				details               bool
			}{
				{"org", "org", "FT-684", false}, {"org", "org_details", "FT-684", true}, {"asn", "asn", "AS19814", false},
				{"net", "v4", "NET-23-189-120-0-1", false}, {"net", "v6", "NET6-2602-F805-1", false},
				{"poc", "poc", "KOSTE-ARIN", false}, {"customer", "customer", "C00000055", false},
				{"delegation", "v4_domain", "120.189.23.in-addr.arpa.", false}, {"delegation", "v6_domain", "0.5.0.8.f.2.0.6.2.ip6.arpa.", false}, {"delegation", "signed_domain", "3.112.149.in-addr.arpa.", false},
			} {
				spec := specs["whois_"+tc.kind]
				param := spec.Inputs[0].Name
				expected, err := c.ReadRegistration(ctx, spec, map[string]string{param: tc.identity, "show_details": fmt.Sprint(tc.details)})
				if err != nil {
					t.Fatalf("%s: %v", tc.label, err)
				}
				config += fmt.Sprintf("data %q %q {\n %s=%q\n show_details=%t\n", "arin_"+spec.Name, tc.label, param, tc.identity, tc.details)
				if previous != "" {
					config += " depends_on=[" + previous + "]\n"
				}
				config += "}\n"
				address := "data.arin_" + spec.Name + "." + tc.label
				previous = address
				checks = append(checks, resource.TestCheckResourceAttrSet(address, "whois_xml"))
				for _, field := range []string{"name", "first_name", "start_address", "end_address", "parent_org_handle"} {
					if value, ok := expected[field].(string); ok {
						checks = append(checks, resource.TestCheckResourceAttr(address, field, value))
					}
				}
				if tc.label == "signed_domain" {
					if len(expected["ds_records"].([]any)) == 0 {
						t.Fatal("native DNSSEC reference has no DS records")
					}
					checks = append(checks, resource.TestCheckResourceAttrSet(address, "ds_records.0.digest"))
				}
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
			t.Log("verified all six Whois record types on " + strings.TrimPrefix(origin, "https://"))
		})
	}
}
