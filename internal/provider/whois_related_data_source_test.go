package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccWhoisRelationships(t *testing.T) {
	fixtures := map[string]string{}
	config := "provider \"arin\" {}\n"
	checks := []resource.TestCheckFunc{}
	for _, spec := range arin.WhoisRelationshipReads() {
		owner, relation, _ := strings.Cut(strings.TrimPrefix(spec.Name, "whois_"), "_")
		identity := strings.TrimSuffix(spec.Inputs[0].Example, ".")
		if owner == "delegation" {
			owner = "rdns"
		}
		if relation == "delegations" {
			relation = "rdns"
		}
		for _, detail := range []bool{false, true} {
			label, suffix := "refs", ""
			if detail {
				label = "details"
				suffix = "_details"
			}
			body, err := os.ReadFile("../arin/testdata/" + spec.Name + suffix + ".xml")
			if err != nil {
				t.Fatal(err)
			}
			path := "/rest/" + owner + "/" + identity + "/" + relation
			if detail {
				path += "?showDetails=true"
			}
			fixtures[path] = string(body)
			config += fmt.Sprintf("data %q %q {\n %s=%q\n show_details=%t\n}\n", "arin_"+spec.Name, label, spec.Inputs[0].Name, spec.Inputs[0].Example, detail)
			address := "data.arin_" + spec.Name + "." + label
			checks = append(checks, resource.TestCheckResourceAttr(address, spec.Output+".#", "1"), resource.TestMatchResourceAttr(address, "whois_xml", regexp.MustCompile(`revision="initial"`)))
			if spec.Output == "pocs" && detail {
				checks = append(checks, whoisPOCMetadataChecks(address, "pocs.0.")...)
			}
			if spec.Output == "pocs" || owner == "poc" {
				checks = append(checks, resource.TestCheckResourceAttr(address, spec.Output+".0.poc_functions.0", "T"))
			}
			if spec.Output == "networks" {
				checks = append(checks, resource.TestCheckResourceAttr(address, "networks.0.ip_version", "v4"))
				blocks := "0"
				if detail || relation == "parent" {
					blocks = "1"
				}
				checks = append(checks, resource.TestCheckResourceAttr(address, "networks.0.net_blocks.#", blocks))
			}
		}
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected authenticated request")
		}
		body, ok := fixtures[r.URL.RequestURI()]
		if !ok {
			t.Error("unexpected relationship request")
			w.WriteHeader(400)
			return
		}
		revision := "initial"
		if changed.Load() {
			revision = "updated"
		}
		fmt.Fprint(w, strings.Replace(body, ">", ` revision="`+revision+`">`, 1))
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_WHOIS_BASE_URL", server.URL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.TestMatchResourceAttr("data.arin_whois_org_nets.details", "whois_xml", regexp.MustCompile(`revision="updated"`))}, {Config: config, PlanOnly: true}}})
}
func TestAccWhoisRelationshipsEmptyAndErrors(t *testing.T) {
	owner, err := os.ReadFile("../arin/testdata/whois_org.xml")
	if err != nil {
		t.Fatal(err)
	}
	noMatches := `<html><title>Whois-RWS</title><body>Sorry, no related resources were found for the handle provided.</body></html>`
	for _, tc := range []struct {
		name, body, pattern string
		status              int
	}{
		{"empty", noMatches, "", 404},
		{"plain_404", "Not Found", "HTTP 404", 404},
		{"partial", `<asns xmlns="https://www.arin.net/whoisrws/core/v1"><limitExceeded>true</limitExceeded></asns>`, "truncated", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/rest/org/EXAMPLE-1" {
					w.Write(owner)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "")
			t.Setenv("ARIN_BASE_URL", "")
			t.Setenv("ARIN_WHOIS_BASE_URL", server.URL)
			config := `provider "arin" {}
data "arin_whois_org_asns" "test" { handle="EXAMPLE-1" }
`
			step := resource.TestStep{Config: config}
			if tc.pattern != "" {
				step.ExpectError = regexp.MustCompile(tc.pattern)
			} else {
				step.Check = resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_whois_org_asns.test", "asns.#", "0"), resource.TestCheckNoResourceAttr("data.arin_whois_org_asns.test", "whois_xml"))
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{step}})
		})
	}
}
