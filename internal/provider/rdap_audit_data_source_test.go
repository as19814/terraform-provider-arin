package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRDAPAudit(t *testing.T) {
	network, err := os.ReadFile("../arin/testdata/rdap_network.json")
	if err != nil {
		t.Fatal(err)
	}
	asn, err := os.ReadFile("../arin/testdata/asn.json")
	if err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected authenticated request")
		}
		value := "initial"
		if changed.Load() {
			value = "updated"
		}
		extend := func(raw []byte) string {
			return strings.TrimSuffix(strings.TrimSpace(string(raw)), "}") + fmt.Sprintf(`,"extension":{"integer":9007199254740993,"value":%q},"type":"DIRECT ALLOCATION"}`, value)
		}
		switch r.URL.RequestURI() {
		case "/registry/help":
			fmt.Fprintf(w, `{"rdapConformance":["reverse_search","rdap_level_0"],"reverse_search_properties":[{"searchableResourceType":"ips","relatedResourceType":"entity","property":"handle"},{"searchableResourceType":"autnums","relatedResourceType":"entity","property":"fn"}],"extension":{"value":%q}}`, value)
		case "/registry/ip/192.0.2.1":
			fmt.Fprint(w, extend(network))
		case "/registry/ips/reverse_search/entity?handle=EXAMPLE-1":
			fmt.Fprintf(w, `{"ipSearchResults":[%s]}`, extend(network))
		case "/registry/autnum/64496":
			fmt.Fprint(w, extend(asn))
		case "/registry/autnums/reverse_search/entity?handle=EXAMPLE-1":
			fmt.Fprintf(w, `{"autnumSearchResults":[%s]}`, extend(asn))
		default:
			t.Error("unexpected RDAP request")
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	config := `provider "arin" {}
data "arin_rdap_help" "test" {}
data "arin_rdap_network" "test" { query="192.0.2.1" }
data "arin_networks" "test" { org_handle="EXAMPLE-1" }
data "arin_asn" "test" { asn=64496 }
data "arin_asns" "test" { org_handle="EXAMPLE-1" }
output "help_extension" { value=jsondecode(data.arin_rdap_help.test.rdap_json).extension.value }
output "network_extension" { value=jsondecode(data.arin_rdap_network.test.rdap_json).extension.value }
output "inventory_extension" { value=jsondecode(data.arin_networks.test.networks["NET-192-0-2-0-1"].rdap_json).extension.value }
output "asn_extension" { value=jsondecode(data.arin_asn.test.rdap_json).extension.value }
output "asns_extension" { value=jsondecode(data.arin_asns.test.asns[0].rdap_json).extension.value }
output "precise_integer" { value=tostring(jsondecode(data.arin_asn.test.rdap_json).extension.integer) }
`
	check := func(value string) resource.TestCheckFunc {
		checks := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr("data.arin_rdap_help.test", "conformance.0", "rdap_level_0"),
			resource.TestCheckResourceAttr("data.arin_rdap_help.test", "reverse_search_properties.0.resource_type", "autnums"),
			resource.TestCheckResourceAttr("data.arin_asn.test", "asn_type", "DIRECT ALLOCATION"),
			resource.TestCheckResourceAttr("data.arin_asns.test", "asns.0.asn_type", "DIRECT ALLOCATION"),
			resource.TestCheckOutput("precise_integer", "9007199254740993"),
		}
		for _, output := range []string{"help_extension", "network_extension", "inventory_extension", "asn_extension", "asns_extension"} {
			checks = append(checks, resource.TestCheckOutput(output, value))
		}
		return resource.ComposeAggregateTestCheckFunc(checks...)
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, Check: check("initial")}, {Config: config, PreConfig: func() { changed.Store(true) }, Check: check("updated")}, {Config: config, PlanOnly: true}}})
}
