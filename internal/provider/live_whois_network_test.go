package provider

import (
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestLiveWhoisNetworkQueries(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires read-only live opt-in")
	}
	for _, key := range []string{"ARIN_API_KEY", "ARIN_BASE_URL", "ARIN_RDAP_BASE_URL", "ARIN_WHOIS_BASE_URL"} {
		t.Setenv(key, "")
	}
	for _, origin := range []string{arin.WhoisOTEURL, arin.WhoisProductionURL} {
		t.Run(origin, func(t *testing.T) {
			config := fmt.Sprintf("provider \"arin\" {whois_base_url=%q}\n", origin)
			previous := ""
			checks := []resource.TestCheckFunc{}
			add := func(kind, label, inputs string) string {
				address := "data.arin_" + kind + "." + label
				config += fmt.Sprintf("data %q %q {\n%s\n", "arin_"+kind, label, inputs)
				if previous != "" {
					config += "depends_on=[" + previous + "]\n"
				}
				config += "}\n"
				previous = address
				checks = append(checks, resource.TestCheckResourceAttrSet(address, "whois_xml"))
				return address
			}
			for _, tc := range []struct{ label, address, prefix, handle string }{
				{"v4", "23.189.120.1", "23.189.120.0/24", "NET-23-189-120-0-1"},
				{"v6", "2602:F805::1", "2602:f805::/36", "NET6-2602-F805-1"},
			} {
				for _, details := range []bool{false, true} {
					label := fmt.Sprintf("%s_%t", tc.label, details)
					for _, kind := range []string{"whois_ip", "whois_cidr"} {
						input := fmt.Sprintf("address=%q", tc.address)
						if kind == "whois_cidr" {
							input = fmt.Sprintf("prefix=%q", tc.prefix)
						}
						address := add(kind, label, input+fmt.Sprintf("\nshow_details=%t", details))
						expected := tc.handle
						if kind == "whois_ip" && tc.label == "v6" {
							expected = "NET6-2602-F805-2"
						}
						checks = append(checks, resource.TestCheckResourceAttr(address, "handle", expected))
					}
					for _, relation := range []string{"less", "more"} {
						address := add("whois_cidr_networks", label+"_"+relation, fmt.Sprintf("prefix=%q\nrelation=%q\nshow_details=%t", tc.prefix, relation, details))
						checks = append(checks, resource.TestCheckResourceAttrSet(address, "networks.0.handle"))
					}
				}
			}
			address := add("whois_ip", "arin_allocation", `address="260f:ffff::1"`)
			checks = append(checks, resource.TestCheckResourceAttr(address, "handle", "NET6-2600-1"))
			address = add("whois_cidr_networks", "hidden_allocation", `prefix="260f:ffff::/48"
relation="less"
show_arin=false`)
			checks = append(checks, resource.TestCheckResourceAttr(address, "networks.#", "0"))
			for _, details := range []bool{false, true} {
				address = add("whois_org", fmt.Sprintf("pocs_%t", details), fmt.Sprintf("handle=\"FT-684\"\nshow_pocs=true\nshow_details=%t", details))
				checks = append(checks, checkWhoisOrgExpansion(address, details))
			}
			factories := map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
			for _, input := range []string{
				"data \"arin_whois_ip\" \"missing\" {\naddress=\"260f:ffff::1\"\nshow_arin=false\n}",
				"data \"arin_whois_cidr\" \"missing\" {prefix=\"23.189.120.0/25\"}",
			} {
				resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{{Config: fmt.Sprintf("provider \"arin\" {whois_base_url=%q}\n%s", origin, input), ExpectError: regexp.MustCompile("404")}}})
			}
		})
	}
}

func checkWhoisOrgExpansion(address string, details bool) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		r, ok := state.RootModule().Resources[address]
		if !ok || r.Primary == nil {
			return fmt.Errorf("missing Whois org state")
		}
		var record struct {
			POCs *struct {
				Links []struct{} `xml:"pocLinkRef"`
			} `xml:"pocs"`
			Nets *struct{} `xml:"nets"`
			ASNs *struct{} `xml:"asns"`
		}
		if err := xml.Unmarshal([]byte(r.Primary.Attributes["whois_xml"]), &record); err != nil {
			return fmt.Errorf("invalid org XML in state")
		}
		if record.POCs == nil || len(record.POCs.Links) == 0 {
			return fmt.Errorf("show_pocs omitted contact references")
		}
		if (record.Nets != nil) != details || (record.ASNs != nil) != details {
			return fmt.Errorf("unexpected expansion of network or ASN inventory")
		}
		return nil
	}
}
