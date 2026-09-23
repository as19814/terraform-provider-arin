package provider

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Use the first octet/nibble-aligned zone wholly inside a discovered prefix.
func liveReverseZone(prefix netip.Prefix) string {
	if prefix.Addr().Is4() {
		address := prefix.Addr().As4()
		labels := []string{}
		for i := (prefix.Bits()+7)/8 - 1; i >= 0; i-- {
			labels = append(labels, strconv.Itoa(int(address[i])))
		}
		return strings.Join(append(labels, "in-addr", "arpa"), ".") + "."
	}
	address := fmt.Sprintf("%x", prefix.Addr().As16())
	labels := []string{}
	for i := (prefix.Bits()+3)/4 - 1; i >= 0; i-- {
		labels = append(labels, string(address[i]))
	}
	return strings.Join(append(labels, "ip6", "arpa"), ".") + "."
}
func TestLiveRDAPDomain(t *testing.T) {
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
		if s.Name == "rdap_domain" {
			spec = s
		}
	}
	for _, origin := range []string{arin.RDAPOTEURL, arin.RDAPProductionURL} {
		t.Run(origin, func(t *testing.T) {
			c, err := arin.New(arin.Config{RDAPBaseURL: origin})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			networks, err := c.ListOrganizationNetworks(ctx, org)
			if err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf("provider \"arin\" { rdap_base_url=%q }\n", origin)
			checks := []resource.TestCheckFunc{}
			previous := ""
			add := func(label, name string, expected map[string]any) {
				dependency := ""
				if previous != "" {
					dependency = " depends_on=[data.arin_rdap_domain." + previous + "]\n"
				}
				config += fmt.Sprintf("data \"arin_rdap_domain\" %q {\n name=%q\n%s}\n", label, name, dependency)
				previous = label
				checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_domain."+label, "nameservers.#", strconv.Itoa(len(expected["nameservers"].([]any)))), resource.TestCheckResourceAttrSet("data.arin_rdap_domain."+label, "rdap_json"))
				if handle, ok := expected["handle"].(string); ok {
					checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_domain."+label, "handle", handle))
				}
			}
			for _, family := range []string{"v4", "v6"} {
				found := false
				for _, network := range networks {
					if network.IPVersion != family {
						continue
					}
					for _, cidr := range network.CIDRs {
						prefix, err := netip.ParsePrefix(cidr)
						if err != nil {
							t.Fatal(err)
						}
						zone := liveReverseZone(prefix)
						expected, err := c.ReadRegistration(ctx, spec, map[string]string{"name": zone})
						if arin.IsNotFound(err) {
							continue
						}
						if err != nil {
							t.Fatal(err)
						}
						add(family, zone, expected)
						add(family+"_normalized", strings.ToUpper(strings.TrimSuffix(zone, ".")), expected)
						found = true
						break
					}
					if found {
						break
					}
				}
				if !found {
					t.Fatalf("no %s reverse domain available", family)
				}
			}
			// ARIN's published RDAP guide uses this public zone as a signed example.
			signed := "3.112.149.in-addr.arpa."
			expected, err := c.ReadRegistration(ctx, spec, map[string]string{"name": signed})
			if err != nil {
				t.Fatal(err)
			}
			if expected["delegation_signed"] != true || len(expected["ds_records"].([]any)) == 0 {
				t.Fatal("signed reference zone no longer publishes DS records")
			}
			add("signed", signed, expected)
			checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_domain.signed", "delegation_signed", "true"), resource.TestCheckResourceAttr("data.arin_rdap_domain.signed", "ds_records.#", strconv.Itoa(len(expected["ds_records"].([]any)))))
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
		})
	}
}
