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

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRDAPResourceSearches(t *testing.T) {
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
			t.Error("unexpected authenticated search")
		}
		if strings.Contains(r.URL.RawQuery, "NO-MATCH") {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"errorCode":404}`)
			return
		}
		body, key, handle := string(network), "ipSearchResults", "NET-192-0-2-0-1"
		if strings.HasPrefix(r.URL.Path, "/registry/autnums") {
			body, key, handle = string(asn), "autnumSearchResults", "AS64496"
		} else if !strings.HasPrefix(r.URL.Path, "/registry/ips") {
			t.Error("unexpected resource endpoint")
		}
		if strings.HasSuffix(r.URL.Path, "/reverse_search/entity") && r.URL.Query().Get("role") != "technical" {
			t.Error("missing role filter")
		}
		if changed.Load() {
			body = strings.ReplaceAll(body, "EXAMPLE-NET", "UPDATED-NET")
			body = strings.ReplaceAll(body, "EXAMPLE-AS", "UPDATED-AS")
		}
		second := strings.Replace(body, `"handle":"`+handle+`"`, `"handle":"`+handle+`-2"`, 1)
		fmt.Fprintf(w, `{%q:[%s,%s]}`, key, second, body)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	config := `provider "arin" {}
`
	checks := []resource.TestCheckFunc{}
	for _, family := range []string{"networks", "asns"} {
		for _, by := range []string{"handle", "name", "entity_handle", "entity_name", "entity_email"} {
			query, role := "Example*", ""
			if by == "handle" {
				query = "NET*"
				if family == "asns" {
					query = "AS*"
				}
			}
			if by == "entity_handle" {
				query = "POC-ARIN"
			}
			if by == "entity_email" {
				query = "noc+test@example.net"
			}
			if strings.HasPrefix(by, "entity_") {
				role = " role=\"technical\"\n"
			}
			config += fmt.Sprintf("data \"arin_rdap_%s\" %q {\n search_by=%q\n query=%q\n%s}\n", family, by, by, query, role)
			address := "data.arin_rdap_" + family + "." + by
			checks = append(checks, resource.TestCheckResourceAttr(address, family+".#", "2"), resource.TestCheckResourceAttrSet(address, family+".0.rdap_json"))
		}
		config += fmt.Sprintf("data \"arin_rdap_%s\" \"missing\" {\n search_by=\"handle\"\n query=\"NO-MATCH\"\n}\n", family)
		checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_"+family+".missing", family+".#", "0"))
	}
	checks = append(checks, resource.TestCheckResourceAttr("data.arin_rdap_networks.handle", "networks.0.handle", "NET-192-0-2-0-1"), resource.TestCheckResourceAttr("data.arin_rdap_networks.entity_email", "networks.0.cidrs.0", "192.0.2.0/24"), resource.TestCheckResourceAttr("data.arin_rdap_asns.handle", "asns.0.start_asn", "64496"))
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)},
		{Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_rdap_networks.name", "networks.0.name", "UPDATED-NET"), resource.TestCheckResourceAttr("data.arin_rdap_asns.name", "asns.0.name", "UPDATED-AS"))},
		{Config: config, PlanOnly: true},
	}})
}
func TestAccRDAPResourceSearchErrors(t *testing.T) {
	for _, family := range []string{"networks", "asns"} {
		t.Run(family, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"notices":[{"type":"result set truncated"}]}`)
			}))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "")
			t.Setenv("ARIN_BASE_URL", "")
			t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
			config := fmt.Sprintf("provider \"arin\" {}\ndata \"arin_rdap_%s\" \"test\" {\n search_by=\"name\"\n query=\"Example*\"\n}\n", family)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, ExpectError: regexp.MustCompile("truncated")}}})
		})
	}
}
