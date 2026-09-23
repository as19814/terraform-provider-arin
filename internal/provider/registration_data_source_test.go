package provider

import (
	"encoding/base64"
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

func fixtureParams(spec arin.ReadSpec) map[string]string {
	p := map[string]string{}
	for _, in := range spec.Inputs {
		p[in.Name] = in.Example
		if in.Default != "" {
			p[in.Name] = in.Default
		}
	}
	switch spec.Name {
	case "org":
		p["handle"] = "EXAMPLE-1"
	case "net":
		p["handle"] = "NET-192-0-2-0-1"
	case "poc":
		p["handle"] = "EXAMPLE-ARIN"
	case "delegation":
		p["name"] = "2.0.192.in-addr.arpa."
	case "irr_as_set":
		p["name"] = "AS-EXAMPLE"
	case "roa":
		p["handle"] = "abc123"
	}
	return p
}
func registrationConfig(spec arin.ReadSpec, params map[string]string) string {
	text := fmt.Sprintf("data %q %q {\n", "arin_"+spec.Name, "test")
	for _, in := range spec.Inputs {
		if in.Default != "" {
			continue
		}
		value := fmt.Sprintf("%q", params[in.Name])
		if in.Kind == "asn" || in.Kind == "id" || in.Kind == "bool" {
			value = params[in.Name]
		}
		text += fmt.Sprintf("  %s = %s\n", in.Name, value)
	}
	return text + "}\n"
}

func TestAccRegistrationCatalog(t *testing.T) {
	expected := map[string][2]string{
		"org": {"name", "Example Organization"}, "net": {"net_blocks.0.cidr_length", "24"}, "parent_net": {"name", "EXAMPLE-NET"}, "most_specific_net": {"ip_version", "4"}, "nets_by_ip_range": {"networks.0.name", "EXAMPLE-NET"},
		"delegation": {"ds_records.0.key_tag", "12345"}, "net_delegations": {"delegations.0.nameservers.0.ttl", "86400"}, "poc": {"emails.0", "noc@example.net"}, "customer": {"private_customer", "true"},
		"irr_route": {"origin_as", "AS64496"}, "irr_routes": {"routes.0.entry_type", "ADVANCED"}, "net_routes": {"routes.0.prefix", "192.0.2.0/24"}, "irr_aut_num": {"import_policy.0", "from AS64497 accept ANY"}, "irr_aut_nums": {"aut_nums.0.as_number", "AS64496"},
		"irr_as_set": {"members.0", "AS64496"}, "irr_as_sets": {"as_sets.0.name", "AS-EXAMPLE"}, "irr_route_set": {"mp_members.0", "2001:db8::/32"}, "irr_route_sets": {"route_sets.0.name", "RS-EXAMPLE"},
		"roas": {"roas.0.asn", "64496"}, "roa": {"resources.#", "2"}, "aspas": {"aspas.0.provider_asns.0", "64497"}, "aspa": {"provider_asns.#", "2"},
		"ticket": {"shared", "true"}, "ticket_summary": {"ticket_status", "IN_PROGRESS"}, "tickets": {"tickets.0.ticket_number", "20260922-X1"}, "ticket_summaries": {"tickets.0.org_handle", "EXAMPLE-1"}, "ticket_message": {"attachment_references.0.filename", "example.txt"}, "ticket_attachment": {"content_base64", base64.StdEncoding.EncodeToString([]byte{0, 1, 255, 3})},
	}
	if len(expected) != len(arin.RegistrationReads()) {
		t.Fatal("acceptance assertions do not cover the catalog")
	}
	routes := map[string][]byte{}
	config := "provider \"arin\" {}\n"
	checks := []resource.TestCheckFunc{resource.TestCheckResourceAttr("data.arin_irr_route.test", "member_of.0", "RS-EXAMPLE")}
	for _, spec := range arin.RegistrationReads() {
		p := fixtureParams(spec)
		root := spec.Root
		if root == "" {
			root = spec.Item
		}
		body := []byte{0, 1, 255, 3}
		if !spec.Binary {
			var err error
			body, err = os.ReadFile("../arin/testdata/" + root + ".xml")
			if err != nil {
				t.Fatal(err)
			}
		}
		if spec.Collection || spec.SelectInput != "" {
			body = []byte(`<collection xmlns="http://www.arin.net/regrws/core/v1">` + string(body) + `</collection>`)
		}
		routes[spec.Path(p)] = body
		config += registrationConfig(spec, p)
		pair := expected[spec.Name]
		checks = append(checks, resource.TestCheckResourceAttr("data.arin_"+spec.Name+".test", pair[0], pair[1]))
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Content-Type") != "application/xml" {
			t.Error("incorrect request method or headers")
		}
		body, ok := routes[r.URL.RequestURI()]
		if !ok {
			t.Errorf("unexpected read endpoint %s", r.URL.RequestURI())
			w.WriteHeader(404)
			return
		}
		if changed.Load() {
			body = []byte(strings.ReplaceAll(string(body), "EXAMPLE-NET", "UPDATED-NET"))
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps: []resource.TestStep{
			{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)},
			{PreConfig: func() { changed.Store(true) }, Config: config, Check: resource.TestCheckResourceAttr("data.arin_net.test", "name", "UPDATED-NET")},
		},
	})
}

func TestAccRegistrationErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, pattern string
		status              int
	}{
		{"missing", `<error xmlns="http://www.arin.net/regrws/core/v1"><code>E_OBJECT_NOT_FOUND</code></error>`, "E_OBJECT_NOT_FOUND", 404},
		{"mismatch", `<net xmlns="http://www.arin.net/regrws/core/v1"><handle>WRONG</handle></net>`, "Mismatched ARIN object", 200},
		{"bad payload", `<org xmlns="http://www.arin.net/regrws/core/v1"/>`, "unexpected object payload", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "test-key")
			t.Setenv("ARIN_BASE_URL", server.URL)
			t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: `provider "arin" {}
data "arin_net" "test" { handle = "NET-192-0-2-0-1" }`, ExpectError: regexp.MustCompile(tc.pattern)}}})
		})
	}
}
