package provider

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func otePrefixEnd(p netip.Prefix) netip.Addr {
	raw := p.Masked().Addr().AsSlice()
	for bit := p.Bits(); bit < len(raw)*8; bit++ {
		raw[bit/8] |= byte(1 << uint(7-bit%8))
	}
	address, _ := netip.AddrFromSlice(raw)
	return address
}
func oteNetCandidate(t *testing.T, c *arin.Client, org, family string) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	networks, err := c.ListOrganizationNetworks(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	var parent, prefix string
	for _, network := range networks {
		if network.IPVersion != family || !strings.Contains(strings.ToLower(network.Type), "allocation") {
			continue
		}
		detail, err := c.GetRegisteredNet(ctx, network.Handle)
		if err != nil {
			t.Fatal(err)
		}
		if detail.OrgHandle != org {
			continue
		}
		for _, block := range detail.Blocks {
			if block.Type != "DA" && block.Type != "A" {
				continue
			}
			bits := 64
			if family == "v4" {
				bits = 32
			}
			if block.CIDRLength > bits {
				continue
			}
			start := netip.MustParseAddr(block.StartAddress)
			end := netip.MustParseAddr(block.EndAddress)
			low := new(big.Int).SetBytes(start.AsSlice())
			high := new(big.Int).SetBytes(end.AsSlice())
			width := new(big.Int).Add(new(big.Int).Sub(high, low), big.NewInt(1))
			for attempt := 0; attempt < 10; attempt++ {
				offset, err := rand.Int(rand.Reader, width)
				if err != nil {
					t.Fatal(err)
				}
				raw := new(big.Int).Add(low, offset).FillBytes(make([]byte, start.BitLen()/8))
				address, _ := netip.AddrFromSlice(raw)
				candidate := netip.PrefixFrom(address, bits).Masked()
				var current map[string]any
				for _, spec := range arin.RegistrationReads() {
					if spec.Name == "parent_net" {
						current, err = c.ReadRegistration(ctx, spec, map[string]string{"start_address": candidate.Addr().String(), "end_address": otePrefixEnd(candidate).String()})
						break
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				if current["handle"] == network.Handle && current["org_handle"] == org {
					parent = network.Handle
					prefix = candidate.String()
					break
				}
			}
			if prefix != "" {
				break
			}
		}
		if prefix != "" {
			break
		}
	}
	if prefix == "" {
		t.Fatalf("no unassigned %s candidate under an owned allocation", family)
	}

	return parent, prefix
}
func TestOTENetLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires ARIN_OTE_API_KEY and ARIN_TEST_ORG_HANDLE")
	}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARIN_API_KEY", key)
	for _, family := range []string{"v4", "v6"} {
		t.Run(family, func(t *testing.T) {
			parent, prefix := oteNetCandidate(t, client, org, family)
			var random [8]byte
			if _, err := rand.Read(random[:]); err != nil {
				t.Fatal(err)
			}
			name := fmt.Sprintf("TERRAFORM-OTE-%X", random[:])
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			customer, err := client.CreateCustomer(ctx, parent, arin.Customer{Name: name, CountryCode: "US", Subdivision: "VA", PostalCode: "20151", City: "Chantilly", StreetAddress: []string{"123 Test Street"}, Private: true})
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			handle := ""
			recipientCustomer, recipientOrg, expectedName := customer.Handle, "", name
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				// Exact range and recipient/name checks recover an interrupted apply
				// without deleting a preexisting registration.
				a := arin.NetAssignment{ParentNetHandle: parent, Name: expectedName, CustomerHandle: recipientCustomer, OrgHandle: recipientOrg, Prefixes: []string{prefix}}
				for _, reallocate := range []bool{false, true} {
					a.Reallocate = reallocate
					n, e := client.FindNetAssignment(ctx, a)
					if e == nil && n != nil {
						handle = n.Handle
						break
					}
				}
				if handle != "" {
					if _, e := client.DeleteNetAssignment(ctx, handle); e != nil {
						t.Errorf("NET cleanup %s: %v", handle, e)
						return
					}
					if _, e := client.GetRegisteredNet(ctx, handle); !arin.IsNotFound(e) {
						t.Errorf("NET cleanup unconfirmed %s: %v", handle, e)
						return
					}
				}
				if e := client.DeleteCustomer(ctx, customer.Handle); e != nil {
					t.Errorf("customer cleanup %s: %v", customer.Handle, e)
				}
				if _, e := client.GetCustomer(ctx, customer.Handle); !arin.IsNotFound(e) {
					t.Errorf("customer cleanup unconfirmed: %v", e)
				}
			})
			config := func(mode string, updated bool) string {
				recipient := fmt.Sprintf("customer_handle = %q", customer.Handle)
				reallocate := false
				if mode != "simple" {
					recipient = fmt.Sprintf("org_handle = %q", org)
					reallocate = mode == "reallocation"
				}
				netName := name
				extras := "comments = [\"Disposable sandbox NET\"]"
				if updated {
					netName += "-UPDATED"
					extras = ""
				}
				return fmt.Sprintf(`provider "arin" {
 base_url = "https://reg.ote.arin.net"
 rdap_base_url = "https://rdap.ote.arin.net"
}
resource "arin_net" "test" {
 parent_net_handle = %q
 name = %q
 prefixes = [%q]
 %s
 reallocate = %t
 %s
}`, parent, netName, prefix, recipient, reallocate, extras)
			}
			capture := func(s *terraform.State) error {
				record := s.RootModule().Resources["arin_net.test"]
				if record == nil {
					return fmt.Errorf("missing NET state")
				}
				handle = record.Primary.ID
				expectedName = record.Primary.Attributes["name"]
				recipientOrg = record.Primary.Attributes["org_handle"]
				recipientCustomer = record.Primary.Attributes["customer_handle"]
				t.Logf("Disposable %s Terraform NET: %s", family, handle)
				return nil
			}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())},
				CheckDestroy: func(_ *terraform.State) error {
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
					defer cancel()
					if handle == "" {
						// A failed first apply may never invoke its check callback.
						a := arin.NetAssignment{ParentNetHandle: parent, Name: expectedName, CustomerHandle: recipientCustomer, OrgHandle: recipientOrg, Prefixes: []string{prefix}}
						n, e := client.FindNetAssignment(ctx, a)
						if e != nil || n != nil {
							return fmt.Errorf("NET deletion unconfirmed after failed apply: %v", e)
						}
						return nil
					}
					if _, e := client.GetRegisteredNet(ctx, handle); !arin.IsNotFound(e) {
						return fmt.Errorf("NET deletion unconfirmed: %v", e)
					}
					return nil
				},
				Steps: []resource.TestStep{
					{Config: config("simple", false), Check: capture},
					{Config: config("simple", true), Check: resource.ComposeAggregateTestCheckFunc(capture, resource.TestCheckResourceAttr("arin_net.test", "comments.#", "0"))},
					{ResourceName: "arin_net.test", ImportState: true, ImportStateVerify: true},
					{Config: config("simple", true), PlanOnly: true, ExpectNonEmptyPlan: false},
					{Config: config("detailed", true), Check: capture},
					{ResourceName: "arin_net.test", ImportState: true, ImportStateVerify: true},
					{Config: config("reallocation", true), Check: capture},
					{ResourceName: "arin_net.test", ImportState: true, ImportStateVerify: true},
					{Config: config("reallocation", true), PlanOnly: true, ExpectNonEmptyPlan: false},
				},
			})
		})
	}
}
