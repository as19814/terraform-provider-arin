package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

func TestOTERPSLResourceLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and organization")
	}
	c, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pocs, err := c.GetOrganizationPOCs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	admin, tech := "", ""
	for _, p := range pocs {
		if p.Function == "AD" {
			admin = p.Handle
		}
		if p.Function == "T" {
			tech = p.Handle
		}
	}
	if admin == "" || tech == "" {
		t.Fatal("requires registered Admin and Tech contacts")
	}
	t.Setenv("ARIN_API_KEY", key)
	for _, kind := range []string{"as-set", "route-set", "route", "route6", "aut-num"} {
		t.Run(kind, func(t *testing.T) {
			identity := rpslOTEIdentity(t, ctx, c, org, kind)
			name := identity.Name

			if _, err := c.GetRPSL(ctx, identity); !arin.IsNotFound(err) {
				t.Fatalf("absence not confirmed: %v", err)
			}
			t.Cleanup(func() {
				cleanup, done := context.WithTimeout(context.Background(), time.Minute)
				defer done()
				if err := c.DeleteRPSL(cleanup, identity, org); err != nil {
					t.Errorf("cleanup %s: %v", name, err)
					return
				}
				if _, err := c.GetRPSL(cleanup, identity); !arin.IsNotFound(err) {
					t.Errorf("cleanup unconfirmed %s: %v", name, err)
				}
			})
			policy := "members: AS64497\nmembers: AS64496\n"
			switch kind {
			case "route-set":
				policy = "members: 198.51.100.0/24\nmembers: 192.0.2.0/24\nmp-members: 2001:db8::/48\n"
			case "route", "route6":
				policy = "origin: " + identity.OriginAS + "\n"
			case "aut-num":
				policy = "as-name: TF-OTE-RPSL\nimport: from AS64496\n  accept ANY\nexport: to AS64496 announce " + name + "\n"
			}
			raw := fmt.Sprintf("%s: %s\ndescr: Disposable RPSL resource test\nadmin-c: %s\ntech-c: %s\nmnt-by: MNT-%s\n%ssource: ARIN\n", kind, name, admin, tech, org, policy)

			config := func(payload string) string {
				return fmt.Sprintf(`provider "arin" {
 base_url="https://reg.ote.arin.net"
 rdap_base_url="https://rdap.ote.arin.net"
}
resource "arin_irr_rpsl" "test" {
 object_type=%q
 name=%q
 org_handle=%q
 origin_as=%q
 rpsl=%q
}
data "arin_irr_rpsl" "test" {
 object_type=arin_irr_rpsl.test.object_type
 name=arin_irr_rpsl.test.name
 origin_as=arin_irr_rpsl.test.origin_as
 depends_on=[arin_irr_rpsl.test]
}`, kind, name, org, identity.OriginAS, payload)
			}
			initial := config(raw)
			updatedRaw := strings.Replace(raw, "Disposable RPSL resource test", "Updated RPSL resource test", 1) + "remarks: Managed by Terraform sandbox test\n"
			updated := config(updatedRaw)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: []resource.TestStep{
				{Config: initial, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_irr_rpsl.test", "pending_creation", "false"), resource.TestCheckResourceAttr("arin_irr_rpsl.test", "rpsl", raw), resource.TestCheckResourceAttrPair("data.arin_irr_rpsl.test", "rpsl", "arin_irr_rpsl.test", "remote_rpsl"))},
				{Config: initial, PlanOnly: true},
				{Config: updated, Check: resource.TestCheckResourceAttr("arin_irr_rpsl.test", "rpsl", updatedRaw)},
				{ResourceName: "arin_irr_rpsl.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"rpsl"}},
				{Config: updated, PlanOnly: true},
			}, CheckDestroy: func(_ *terraform.State) error {
				if _, err := c.GetRPSL(ctx, identity); !arin.IsNotFound(err) {
					return fmt.Errorf("deletion not confirmed: %v", err)
				}
				return nil
			}})
		})
	}
}

func rpslOTEIdentity(t *testing.T, ctx context.Context, c *arin.Client, org, kind string) arin.RPSLKey {
	t.Helper()
	if kind == "as-set" || kind == "route-set" {
		suffix := make([]byte, 8)
		if _, err := rand.Read(suffix); err != nil {
			t.Fatal(err)
		}
		prefix := "AS-"
		if kind == "route-set" {
			prefix = "RS-"
		}
		return arin.RPSLKey{Kind: kind, Name: prefix + "TF-OTE-RPSL-" + strings.ToUpper(hex.EncodeToString(suffix))}
	}
	if kind == "aut-num" {
		for _, spec := range arin.PublicReads() {
			if spec.Name != "asns" {
				continue
			}
			inventory, err := c.ReadRegistration(ctx, spec, map[string]string{"org_handle": org})
			if err != nil {
				t.Fatal(err)
			}
			for _, record := range inventory["asns"].([]any) {
				name := fmt.Sprintf("AS%d", record.(map[string]any)["start_asn"].(int64))
				if _, err := c.GetAutnum(ctx, name); !arin.IsNotFound(err) {
					continue
				}
				key := arin.RPSLKey{Kind: kind, Name: name}
				if _, err := c.GetRPSL(ctx, key); arin.IsNotFound(err) {
					return key
				}
			}
		}
		t.Fatal("no owned ASN with an unused IRR aut-num identity")
	}
	networks, err := c.ListOrganizationNetworks(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	family := "v4"
	if kind == "route6" {
		family = "v6"
	}
	for _, network := range networks {
		if network.IPVersion != family {
			continue
		}
		start, end := netip.MustParseAddr(network.StartAddress), netip.MustParseAddr(network.EndAddress)
		low, high := new(big.Int).SetBytes(start.AsSlice()), new(big.Int).SetBytes(end.AsSlice())
		width := new(big.Int).Add(new(big.Int).Sub(high, low), big.NewInt(1))
		for attempt := 0; attempt < 10; attempt++ {
			offset, err := rand.Int(rand.Reader, width)
			if err != nil {
				t.Fatal(err)
			}
			address, _ := netip.AddrFromSlice(new(big.Int).Add(low, offset).FillBytes(make([]byte, start.BitLen()/8)))
			owned := false
			for _, spec := range arin.RegistrationReads() {
				if spec.Name != "parent_net" {
					continue
				}
				parent, err := c.ReadRegistration(ctx, spec, map[string]string{"start_address": address.String(), "end_address": address.String()})
				if err != nil {
					t.Fatal(err)
				}
				owned = parent["org_handle"] == org
			}
			if !owned {
				continue
			}
			key := arin.RPSLKey{Kind: kind, Name: netip.PrefixFrom(address, address.BitLen()).String(), OriginAS: "AS64496"}
			if _, err := c.GetIRRRoute(ctx, key.Name+","+key.OriginAS); !arin.IsNotFound(err) {
				continue
			}
			if _, err := c.GetRPSL(ctx, key); arin.IsNotFound(err) {
				return key
			}
		}
	}
	t.Fatal("no owned prefix with an unused IRR route identity")
	return arin.RPSLKey{}
}
