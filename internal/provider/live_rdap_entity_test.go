package provider

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveRDAPEntity(t *testing.T) {
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
	var spec arin.ReadSpec
	for _, s := range arin.PublicReads() {
		if s.Name == "rdap_entity" {
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
			entity, err := c.ReadRegistration(ctx, spec, map[string]string{"handle": org})
			if err != nil {
				t.Fatal(err)
			}
			refs := entity["entities"].([]any)
			if len(refs) == 0 {
				t.Fatal("no public contacts available")
			}
			contact := refs[0].(map[string]any)["handle"].(string)
			poc, err := c.ReadRegistration(ctx, spec, map[string]string{"handle": contact})
			if err != nil {
				t.Fatal(err)
			}
			if len(entity["names"].([]any)) == 0 || len(poc["names"].([]any)) == 0 || poc["vcard_json"] == nil {
				t.Fatal("missing native contact fields")
			}
			config := fmt.Sprintf(`provider "arin" { rdap_base_url=%q }
data "arin_rdap_entity" "org" { handle=%q }
data "arin_rdap_entity" "poc" { handle=%q }
`, origin, org, contact)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: []resource.TestStep{
				{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_rdap_entity.org", "names.0", entity["names"].([]any)[0].(string)), resource.TestCheckResourceAttr("data.arin_rdap_entity.poc", "names.0", poc["names"].([]any)[0].(string)), resource.TestCheckResourceAttrSet("data.arin_rdap_entity.poc", "vcard_json"), resource.TestCheckResourceAttrSet("data.arin_rdap_entity.org", "rdap_json"))},
				{Config: config, PlanOnly: true},
			}})
		})
	}
}
