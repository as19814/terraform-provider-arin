package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

func TestOTERPSLReadLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in for disposable fixtures")
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
	for _, kind := range []string{"as-set", "route-set"} {
		t.Run(kind, func(t *testing.T) {
			suffix := make([]byte, 8)
			if _, err := rand.Read(suffix); err != nil {
				t.Fatal(err)
			}
			prefix, member := "AS-", "AS64496"
			if kind == "route-set" {
				prefix, member = "RS-", "192.0.2.0/24"
			}
			name := prefix + "TF-OTE-RPSL-" + strings.ToUpper(hex.EncodeToString(suffix))
			identity := arin.RPSLKey{Kind: kind, Name: name}
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
			raw := fmt.Sprintf("%s: %s\ndescr: Disposable RPSL data source test\nadmin-c: %s\ntech-c: %s\nmnt-by: MNT-%s\nmembers: %s\nsource: ARIN\n", kind, name, admin, tech, org, member)
			if _, err := c.CreateRPSL(ctx, raw); err != nil {
				t.Fatal(err)
			}
			expected, err := c.GetRPSL(ctx, identity)
			if err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf(`provider "arin" {
 base_url="https://reg.ote.arin.net"
 rdap_base_url="https://rdap.ote.arin.net"
}
data "arin_irr_rpsl" "test" {
 object_type=%q
 name=%q
}`, kind, name)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: []resource.TestStep{
				{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_irr_rpsl.test", "id", kind+"/"+name), resource.TestCheckResourceAttr("data.arin_irr_rpsl.test", "org_handle", org), resource.TestCheckResourceAttr("data.arin_irr_rpsl.test", "rpsl", expected.Text))},
				{Config: config, PlanOnly: true},
			}})
		})
	}
}
