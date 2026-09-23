package provider

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestOTEIRRRouteLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires ARIN_OTE_WRITE_TESTS=1 and TF_ACC=1")
	}
	key := os.Getenv("ARIN_OTE_API_KEY")
	if key == "" {
		key = os.Getenv("ARIN_API_KEY")
	}
	org := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("set ARIN_OTE_API_KEY (or ARIN_API_KEY) and ARIN_TEST_ORG_HANDLE")
	}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := client.GetOrganization(ctx, org); err != nil {
		t.Fatalf("OT&E access preflight failed: %v", err)
	}
	networks, err := client.ListOrganizationNetworks(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARIN_API_KEY", key)
	for _, family := range []string{"v4", "v6"} {
		t.Run(family, func(t *testing.T) {
			prefix := ""
			for _, network := range networks {
				if network.IPVersion != family {
					continue
				}
				start := netip.MustParseAddr(network.StartAddress)
				end := netip.MustParseAddr(network.EndAddress)
				low := new(big.Int).SetBytes(start.AsSlice())
				high := new(big.Int).SetBytes(end.AsSlice())
				width := new(big.Int).Add(new(big.Int).Sub(high, low), big.NewInt(1))
				offset, err := rand.Int(rand.Reader, width)
				if err != nil {
					t.Fatal(err)
				}
				raw := new(big.Int).Add(low, offset).FillBytes(make([]byte, start.BitLen()/8))
				address, ok := netip.AddrFromSlice(raw)
				if !ok {
					t.Fatal("invalid candidate address")
				}
				candidate := netip.PrefixFrom(address, address.BitLen()).String()
				// Confirm the covering parent registration is still held by this org.
				var detail map[string]any
				for _, spec := range arin.RegistrationReads() {
					if spec.Name == "parent_net" {
						detail, err = client.ReadRegistration(ctx, spec, map[string]string{"start_address": address.String(), "end_address": address.String()})
						break
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				if detail["org_handle"] != org {
					continue
				}
				_, err = client.GetIRRRoute(ctx, candidate+",AS64496")
				if arin.IsNotFound(err) {
					prefix = candidate
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if prefix == "" {
				t.Fatalf("no unused %s test prefix found in OT&E organization registrations", family)
			}
			id := prefix + ",AS64496"
			t.Logf("Disposable OT&E route: %s", id)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				r, err := client.GetIRRRoute(ctx, id)
				if arin.IsNotFound(err) {
					return
				}
				if err != nil {
					t.Errorf("cannot verify cleanup for %s: %v", id, err)
					return
				}
				if r.OrgHandle != org {
					t.Errorf("refusing cleanup of route with mismatched org: %s", id)
					return
				}
				if err := client.DeleteIRRRoute(ctx, id); err != nil {
					t.Errorf("route cleanup failed for %s: %v", id, err)
					return
				}
				if _, err := client.GetIRRRoute(ctx, id); !arin.IsNotFound(err) {
					t.Errorf("route cleanup not confirmed for %s: %v", id, err)
				}
			})
			config := func(description, remarks string) string {
				return fmt.Sprintf(`provider "arin" {
 base_url = "https://reg.ote.arin.net"
 rdap_base_url = "https://rdap.ote.arin.net"
}
resource "arin_irr_route" "test" {
 prefix = %q
 origin_as = "AS64496"
 org_handle = %q
 description = [%q]
 remarks = %s
}`, prefix, org, description, remarks)
			}
			updated := config("Updated disposable Terraform OT&E route", `[]`)
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())},
				CheckDestroy: func(_ *terraform.State) error {
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
					defer cancel()
					if _, err := client.GetIRRRoute(ctx, id); !arin.IsNotFound(err) {
						return fmt.Errorf("route deletion not confirmed for %s: %v", id, err)
					}
					return nil
				},
				Steps: []resource.TestStep{
					{Config: config("Disposable Terraform OT&E route", `["Disposable test remark"]`), Check: resource.TestCheckResourceAttr("arin_irr_route.test", "id", id)},
					{Config: updated, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_irr_route.test", "description.0", "Updated disposable Terraform OT&E route"), resource.TestCheckResourceAttr("arin_irr_route.test", "remarks.#", "0"))},
					{ResourceName: "arin_irr_route.test", ImportState: true, ImportStateVerify: true},
					{Config: updated, PlanOnly: true, ExpectNonEmptyPlan: false},
				},
			})
		})
	}
}
