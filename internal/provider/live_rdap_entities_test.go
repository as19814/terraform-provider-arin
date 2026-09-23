package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveRDAPEntities(t *testing.T) {
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
		case "rdap_entity":
			lookup = s
		case "rdap_entities":
			search = s
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
			entity, err := c.ReadRegistration(ctx, lookup, map[string]string{"handle": org})
			if err != nil {
				t.Fatal(err)
			}
			refs := entity["entities"].([]any)
			if len(refs) == 0 {
				t.Fatal("no contacts available")
			}
			pocHandle := refs[0].(map[string]any)["handle"].(string)
			poc, err := c.ReadRegistration(ctx, lookup, map[string]string{"handle": pocHandle})
			if err != nil {
				t.Fatal(err)
			}
			config := fmt.Sprintf("provider \"arin\" { rdap_base_url=%q }\n", origin)
			checks := []resource.TestCheckFunc{}
			// Serialize native queries to avoid bursts against the public service.
			previous := ""
			partialSteps := []resource.TestStep{}
			for i, record := range []map[string]any{entity, poc} {
				names := record["names"].([]any)
				if len(names) == 0 {
					t.Fatal("no public name available")
				}
				for _, by := range []string{"handle", "name"} {
					term := record["handle"].(string)
					if by == "name" {
						term = names[0].(string)
					}
					for j, suffix := range []string{"", "*"} {
						name := fmt.Sprintf("entity_%d_%s_%d", i, by, j)
						result, err := c.ReadRegistration(ctx, search, map[string]string{"search_by": by, "query": term + suffix})
						if err != nil {
							if i == 1 && by == "name" && strings.Contains(err.Error(), "truncated") {
								partialConfig := fmt.Sprintf("provider \"arin\" { rdap_base_url=%q }\ndata \"arin_rdap_entities\" \"partial\" {\n search_by=%q\n query=%q\n}\n", origin, by, term+suffix)
								partialSteps = append(partialSteps, resource.TestStep{Config: partialConfig, ExpectError: regexp.MustCompile("truncated")})
								t.Log("common POC name produced a truncated response; checking Terraform rejects it")
								continue
							}
							t.Fatalf("%s search failed: %v", name, err)
						}
						records := result["entities"].([]any)
						found := -1
						for n, v := range records {
							if strings.EqualFold(v.(map[string]any)["handle"].(string), record["handle"].(string)) {
								found = n
								break
							}
						}
						if found < 0 {
							t.Fatalf("%s did not return the source entity", name)
						}
						dependency := ""
						if previous != "" {
							dependency = " depends_on=[data.arin_rdap_entities." + previous + "]\n"
						}
						config += fmt.Sprintf("data \"arin_rdap_entities\" %q {\n search_by=%q\n query=%q\n%s}\n", name, by, term+suffix, dependency)
						previous = name
						checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_entities."+name, fmt.Sprintf("entities.%d.handle", found), records[found].(map[string]any)["handle"].(string)))
					}
				}
			}
			for _, by := range []string{"handle", "name"} {
				name := "missing_" + by
				config += fmt.Sprintf("data \"arin_rdap_entities\" %q {\n search_by=%q\n query=\"CODEX-NO-SUCH-ENTITY-20260923\"\n depends_on=[data.arin_rdap_entities.%s]\n}\n", name, by, previous)
				previous = name
				checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_entities."+name, "entities.#", "0"))
			}
			steps := []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}
			steps = append(steps, partialSteps...)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: steps})
		})
	}
}
