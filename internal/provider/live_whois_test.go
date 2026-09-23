package provider

import (
	"context"
	"encoding/json"
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

func TestLiveWhoisLookups(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires read-only live opt-in")
	}
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	t.Setenv("ARIN_WHOIS_BASE_URL", "")
	specs := map[string]arin.ReadSpec{}
	for _, s := range arin.WhoisRecordReads() {
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
				for _, field := range []string{"name", "first_name", "start_address", "end_address", "parent_org_handle", "country_name", "country_code3", "country_calling_code"} {
					if value, ok := expected[field].(string); ok {
						checks = append(checks, resource.TestCheckResourceAttr(address, field, value))
					}
				}
				if tc.kind == "org" || tc.kind == "poc" || tc.kind == "customer" {
					for _, field := range []string{"country_name", "country_code3", "country_calling_code"} {
						checks = append(checks, resource.TestCheckResourceAttrSet(address, field))
					}
				}
				if tc.kind == "poc" {
					for _, field := range []string{"poc_type_description", "status_description"} {
						value, ok := expected[field].(string)
						if !ok || value == "" {
							t.Fatal("native POC description missing")
						}
						checks = append(checks, resource.TestCheckResourceAttr(address, field, value))
					}
					phones := expected["phones"].([]any)
					if len(phones) == 0 {
						t.Fatal("native POC has no phone metadata")
					}
					for i, phone := range phones {
						value, ok := phone.(map[string]any)["description"].(string)
						if !ok || value == "" {
							t.Fatal("native phone description missing")
						}
						checks = append(checks, resource.TestCheckResourceAttr(address, fmt.Sprintf("phones.%d.description", i), value))
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

func TestLiveWhoisRelationships(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires read-only live opt-in")
	}
	for _, key := range []string{"ARIN_API_KEY", "ARIN_BASE_URL", "ARIN_RDAP_BASE_URL", "ARIN_WHOIS_BASE_URL"} {
		t.Setenv(key, "")
	}
	specs := map[string]arin.ReadSpec{}
	for _, spec := range arin.WhoisRelationshipReads() {
		specs[spec.Name] = spec
	}
	for _, origin := range []string{arin.WhoisOTEURL, arin.WhoisProductionURL} {
		t.Run(origin, func(t *testing.T) {
			config := fmt.Sprintf("provider \"arin\" { whois_base_url=%q }\n", origin)
			previous := ""
			checks := []resource.TestCheckFunc{}
			for _, tc := range []struct {
				kind, label, identity string
				empty                 bool
			}{
				{"poc_orgs", "orgs", "ADMIN8834-ARIN", false},
				{"poc_asns", "asns", "ZG39-ARIN", false},
				{"poc_nets", "nets", "ZG39-ARIN", false},
				{"org_pocs", "pocs", "FT-684", false},
				{"org_asns", "asns", "FT-684", false},
				{"org_nets", "nets", "FT-684", false},
				{"asn_pocs", "pocs", "AS15169", false},
				{"net_pocs", "pocs", "NET-216-239-32-0-1", false},
				{"net_parent", "parent", "NET-23-189-120-0-1", false},
				{"net_children", "children", "NET6-2602-F805-1", false},
				{"net_delegations", "v4", "NET-23-189-120-0-1", false},
				{"net_delegations", "v6", "NET6-2602-F805-1", false},
				{"delegation_nets", "v4", "120.189.23.in-addr.arpa.", false},
				{"delegation_nets", "v6", "0.5.0.8.f.2.0.6.2.ip6.arpa.", false},
				{"poc_asns", "empty", "ADMIN8834-ARIN", true},
				{"poc_nets", "empty", "ADMIN8834-ARIN", true},
				{"asn_pocs", "empty", "AS19814", true},
				{"net_pocs", "empty", "NET-23-189-120-0-1", true},
				{"net_children", "empty", "NET-23-189-120-0-1", true},
			} {
				for _, details := range []bool{false, true} {
					spec := specs["whois_"+tc.kind]
					label := fmt.Sprintf("%s_%t", tc.label, details)
					address := "data.arin_" + spec.Name + "." + label
					config += fmt.Sprintf("data %q %q {\n %s=%q\n show_details=%t\n", "arin_"+spec.Name, label, spec.Inputs[0].Name, tc.identity, details)
					if previous != "" {
						config += " depends_on=[" + previous + "]\n"
					}
					config += "}\n"
					previous = address
					if tc.empty {
						checks = append(checks, resource.TestCheckResourceAttr(address, spec.Output+".#", "0"), resource.TestCheckNoResourceAttr(address, "whois_xml"))
					} else {
						key := "handle"
						if tc.kind == "net_delegations" {
							key = "name"
						}
						checks = append(checks, resource.TestCheckResourceAttrSet(address, spec.Output+".0."+key), resource.TestCheckResourceAttrSet(address, "whois_xml"))
						if strings.HasPrefix(tc.kind, "poc_") || strings.HasSuffix(tc.kind, "_pocs") {
							checks = append(checks, resource.TestCheckResourceAttrSet(address, spec.Output+".0.poc_functions.0"))
						}
					}
				}
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
		})
	}
}

func TestLiveWhoisSearches(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires read-only live opt-in")
	}
	for _, key := range []string{"ARIN_API_KEY", "ARIN_BASE_URL", "ARIN_RDAP_BASE_URL", "ARIN_WHOIS_BASE_URL"} {
		t.Setenv(key, "")
	}
	for _, origin := range []string{arin.WhoisOTEURL, arin.WhoisProductionURL} {
		t.Run(origin, func(t *testing.T) {
			c, err := arin.New(arin.Config{WhoisBaseURL: origin})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			config := fmt.Sprintf("provider \"arin\" {whois_base_url=%q}\n", origin)
			previous := ""
			checks := []resource.TestCheckFunc{}
			for _, spec := range arin.WhoisSearchReads() {
				kind := strings.TrimSuffix(strings.TrimPrefix(spec.Name, "whois_"), "s")
				handle := map[string]string{"org": "FT-684", "customer": "C00000055", "poc": "KOSTE-ARIN", "asn": "AS19814", "net": "NET-23-189-120-0-1"}[kind]
				var individual arin.ReadSpec
				for _, r := range arin.WhoisRecordReads() {
					if r.Name == "whois_"+kind {
						individual = r
					}
				}
				record, err := c.ReadRegistration(ctx, individual, map[string]string{"handle": handle, "show_details": "false"})
				if err != nil {
					t.Fatal(err)
				}
				name := record["name"]
				nameKey := "name"
				if kind == "poc" {
					name = record["last_name"]
					nameKey = "last"
				}
				if name == nil {
					t.Fatal("missing native search fixture name")
				}
				// Verify every documented secondary filter is honored, rather than silently ignored.
				keys := map[string][]string{"org": {"name", "dba"}, "customer": {"name"}, "poc": {"domain", "first", "middle", "last", "company", "city"}, "asn": {"name"}, "net": {"name"}}[kind]
				for _, key := range keys {
					encoded, _ := json.Marshal(map[string]string{"handle": handle, key: "ZZZ-CODEX-NO-MATCH-19814"})
					result, err := c.ReadRegistration(ctx, spec, map[string]string{"filters": string(encoded), "show_details": "false"})
					if err != nil {
						t.Fatalf("%s %s filter: %v", kind, key, err)
					}
					if len(result[spec.Output].([]any)) != 0 {
						t.Fatalf("%s %s filter was ignored", kind, key)
					}
				}
				for _, mode := range []string{"exact", "prefix", "combined", "empty"} {
					filters := map[string]string{"handle": handle}
					if mode == "prefix" {
						filters["handle"] += "*"
					}
					if mode == "combined" {
						filters[nameKey] = name.(string)
					}
					if mode == "empty" {
						filters[nameKey] = "ZZZ-CODEX-NO-MATCH-19814"
					}
					encoded, _ := json.Marshal(filters)
					for _, details := range []bool{false, true} {
						label := fmt.Sprintf("%s_%t", mode, details)
						address := "data.arin_" + spec.Name + "." + label
						config += fmt.Sprintf("data %q %q {\n filters=%s\n show_details=%t\n", "arin_"+spec.Name, label, encoded, details)
						if previous != "" {
							config += " depends_on=[" + previous + "]\n"
						}
						config += "}\n"
						previous = address
						if mode == "empty" {
							checks = append(checks, resource.TestCheckResourceAttr(address, spec.Output+".#", "0"), resource.TestCheckNoResourceAttr(address, "whois_xml"))
						} else {
							checks = append(checks, resource.TestCheckResourceAttr(address, spec.Output+".#", "1"), resource.TestCheckResourceAttr(address, spec.Output+".0.handle", handle), resource.TestCheckResourceAttrSet(address, "whois_xml"))
							if details && kind != "poc" {
								checks = append(checks, resource.TestCheckResourceAttr(address, spec.Output+".0.name", name.(string)))
							}
						}
					}
				}
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
			// Broad searches must fail rather than commit ARIN's capped inventory to state.
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())}, Steps: []resource.TestStep{{Config: fmt.Sprintf("provider \"arin\" {whois_base_url=%q}\ndata \"arin_whois_orgs\" \"partial\" {filters={handle=\"A*\"}}", origin), ExpectError: regexp.MustCompile("truncated")}}})
		})
	}
}
