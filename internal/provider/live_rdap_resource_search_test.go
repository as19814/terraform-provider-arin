package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveRDAPResourceSearches(t *testing.T) {
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
	specs := map[string]arin.ReadSpec{}
	for _, s := range arin.PublicReads() {
		specs[s.Name] = s
	}
	for _, origin := range []string{arin.RDAPOTEURL, arin.RDAPProductionURL} {
		t.Run(origin, func(t *testing.T) {
			c, err := arin.New(arin.Config{RDAPBaseURL: origin})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			networks, err := c.ListOrganizationNetworks(ctx, org)
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := c.ReadRegistration(ctx, specs["asns"], map[string]string{"org_handle": org})
			if err != nil {
				t.Fatal(err)
			}
			asns := inventory["asns"].([]any)
			if len(asns) == 0 {
				t.Fatal("no ASN registration available")
			}
			entity, err := c.ReadRegistration(ctx, specs["rdap_entity"], map[string]string{"handle": org})
			if err != nil {
				t.Fatal(err)
			}
			orgNames := entity["names"].([]any)
			if len(orgNames) == 0 {
				t.Fatal("no organization name available")
			}
			var contact map[string]any
			contactRole := ""
			for _, raw := range entity["entities"].([]any) {
				ref := raw.(map[string]any)
				roles := []string{}
				for _, value := range ref["roles"].([]any) {
					roles = append(roles, value.(string))
				}
				for _, role := range []string{"technical", "noc", "abuse"} {
					if !slices.Contains(roles, role) {
						continue
					}
					poc, err := c.ReadRegistration(ctx, specs["rdap_entity"], map[string]string{"handle": ref["handle"].(string)})
					if err != nil {
						t.Fatal(err)
					}
					if len(poc["names"].([]any)) > 0 && len(poc["emails"].([]any)) > 0 {
						contact = poc
						contactRole = role
						break
					}
				}
				if contact != nil {
					break
				}
			}
			if contact == nil {
				t.Fatal("no public contact with a searchable name and email")
			}
			config := fmt.Sprintf("provider \"arin\" { rdap_base_url=%q }\n", origin)
			checks := []resource.TestCheckFunc{}
			partialSteps := []resource.TestStep{}
			previous := ""
			add := func(label, specName, by, query, role, wantHandle string, allowPartial bool) {
				spec := specs[specName]
				expected, err := c.ReadRegistration(ctx, spec, map[string]string{"search_by": by, "query": query, "role": role})
				block := fmt.Sprintf("data \"arin_%s\" %q {\n search_by=%q\n query=%q\n role=%q\n", specName, label, by, query, role)
				if err != nil {
					if allowPartial && strings.Contains(err.Error(), "truncated") {
						partialSteps = append(partialSteps, resource.TestStep{Config: fmt.Sprintf("provider \"arin\" { rdap_base_url=%q }\n", origin) + block + "}\n", ExpectError: regexp.MustCompile("truncated")})
						t.Logf("%s produced a partial result; checking Terraform rejects it", label)
						return
					}
					t.Fatalf("%s failed: %v", label, err)
				}
				if previous != "" {
					block += " depends_on=[" + previous + "]\n"
				}
				config += block + "}\n"
				address := "data.arin_" + specName + "." + label
				previous = address
				records := expected[spec.Output].([]any)
				checks = append(checks, resource.TestCheckResourceAttr(address, spec.Output+".#", strconv.Itoa(len(records))))
				found := wantHandle == ""
				for i, raw := range records {
					record := raw.(map[string]any)
					handle := record["handle"].(string)
					checks = append(checks, resource.TestCheckResourceAttr(address, fmt.Sprintf("%s.%d.handle", spec.Output, i), handle))
					if strings.EqualFold(handle, wantHandle) {
						found = true
					}
				}
				if !found {
					t.Fatalf("%s omitted the independently discovered registration", label)
				}
			}
			firstNetwork := ""
			for _, family := range []string{"v4", "v6"} {
				found := false
				for _, network := range networks {
					if network.IPVersion != family {
						continue
					}
					found = true
					if firstNetwork == "" {
						firstNetwork = network.Handle
					}
					if network.Name == "" {
						t.Fatal("network has no searchable name")
					}
					for _, by := range []string{"handle", "name"} {
						term := network.Handle
						if by == "name" {
							term = network.Name
						}
						for i, suffix := range []string{"", "*"} {
							add(family+"_"+by+strconv.Itoa(i), "rdap_networks", by, term+suffix, "any", network.Handle, false)
						}
					}
					break
				}
				if !found {
					t.Fatalf("no %s network for search", family)
				}
			}
			asn := asns[0].(map[string]any)
			for _, by := range []string{"handle", "name"} {
				term := asn[by].(string)
				if term == "" {
					t.Fatal("ASN lacks searchable identity")
				}
				for i, suffix := range []string{"", "*"} {
					add("asn_"+by+strconv.Itoa(i), "rdap_asns", by, term+suffix, "any", asn["handle"].(string), false)
				}
			}
			for _, family := range []struct{ spec, handle string }{{"rdap_networks", firstNetwork}, {"rdap_asns", asn["handle"].(string)}} {
				add(family.spec+"_org_handle", family.spec, "entity_handle", org, "any", family.handle, false)
				add(family.spec+"_org_name", family.spec, "entity_name", orgNames[0].(string), "any", family.handle, false)
				for _, by := range []string{"entity_handle", "entity_name", "entity_email"} {
					term := contact["handle"].(string)
					if by == "entity_name" {
						term = contact["names"].([]any)[0].(string)
					}
					if by == "entity_email" {
						term = contact["emails"].([]any)[0].(string)
					}
					for _, role := range []string{"any", contactRole} {
						add(family.spec+"_"+by+"_"+role, family.spec, by, term, role, "", true)
					}
				}
				for _, role := range []string{"abuse", "noc", "technical"} {
					if role != contactRole {
						add(family.spec+"_contact_"+role, family.spec, "entity_handle", contact["handle"].(string), role, "", false)
					}
				}
				add(family.spec+"_missing", family.spec, "handle", "CODEX-NO-SUCH-RESOURCE-20260923", "any", "", false)
			}
			steps := []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}
			steps = append(steps, partialSteps...)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: steps})
		})
	}
}
