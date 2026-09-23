package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccWhoisNetworkQueries(t *testing.T) {
	net, err := os.ReadFile("../arin/testdata/whois_net.xml")
	if err != nil {
		t.Fatal(err)
	}
	org, err := os.ReadFile("../arin/testdata/whois_org.xml")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected authenticated request")
		}
		switch r.URL.Path {
		case "/rest/org/EXAMPLE-1":
			if r.URL.Query().Get("showPocs") != "true" {
				t.Error("missing showPocs option")
			}
			fmt.Fprint(w, strings.Replace(string(org), "</org>", `<pocs><pocLinkRef handle="EXAMPLE-ARIN"/></pocs></org>`, 1))
		case "/rest/ip/192.0.2.1", "/rest/cidr/192.0.2.0/24":
			fmt.Fprint(w, string(net))
		case "/rest/cidr/192.0.2.0/24/less", "/rest/cidr/192.0.2.0/24/more":
			if r.URL.Query().Get("showARIN") != "false" {
				t.Error("missing showARIN option")
			}
			body := `<netRef handle="NET-192-0-2-0-1" startAddress="192.0.2.0" endAddress="192.0.2.255"/>`
			if r.URL.Query().Get("showDetails") == "true" {
				body = string(net)
			}
			fmt.Fprint(w, `<nets xmlns="https://www.arin.net/whoisrws/core/v1"><limitExceeded>false</limitExceeded>`+body+`</nets>`)
		default:
			t.Error("unexpected network query")
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_WHOIS_BASE_URL", server.URL)
	config := `provider "arin" {}
data "arin_whois_ip" "test" {address="192.0.2.1"}
data "arin_whois_cidr" "test" {prefix="192.0.2.0/24"}
data "arin_whois_org" "test" {
 handle="EXAMPLE-1"
 show_pocs=true
}
`
	checks := []resource.TestCheckFunc{resource.TestCheckResourceAttr("data.arin_whois_ip.test", "handle", "NET-192-0-2-0-1"), resource.TestCheckResourceAttr("data.arin_whois_cidr.test", "start_address", "192.0.2.0"), resource.TestMatchResourceAttr("data.arin_whois_org.test", "whois_xml", regexp.MustCompile("pocLinkRef"))}
	for _, relation := range []string{"less", "more"} {
		for _, details := range []bool{false, true} {
			label := fmt.Sprintf("%s_%t", relation, details)
			config += fmt.Sprintf("data \"arin_whois_cidr_networks\" %q {\n prefix=\"192.0.2.0/24\"\n relation=%q\n show_details=%t\n show_arin=false\n}\n", label, relation, details)
			address := "data.arin_whois_cidr_networks." + label
			checks = append(checks, resource.TestCheckResourceAttr(address, "networks.#", "1"), resource.TestCheckResourceAttr(address, "networks.0.handle", "NET-192-0-2-0-1"), resource.TestCheckResourceAttrSet(address, "whois_xml"))
		}
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
}

func TestAccWhoisNetworkQueryErrors(t *testing.T) {
	for _, tc := range []struct {
		name, config, body, pattern string
		status                      int
		request                     bool
	}{
		{"invalid_relation", `data "arin_whois_cidr_networks" "test" {
 prefix="192.0.2.0/24"
 relation="up"
}`, "", "relation must be less or more", 200, false},
		{"host_bits", `data "arin_whois_cidr" "test" {prefix="192.0.2.1/24"}`, "", "valid cidr", 200, false},
		{"scoped_ip", `data "arin_whois_ip" "test" {address="fe80::1%eth0"}`, "", "valid ip", 200, false},
		{"missing", `data "arin_whois_ip" "test" {address="192.0.2.1"}`, "Not found", "404", 404, true},
		{"empty", `data "arin_whois_cidr_networks" "test" {
 prefix="192.0.2.0/24"
 relation="more"
}`, `<nets xmlns="https://www.arin.net/whoisrws/core/v1"><limitExceeded>false</limitExceeded></nets>`, "", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !tc.request {
					t.Error("invalid input reached ARIN")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "")
			t.Setenv("ARIN_BASE_URL", "")
			t.Setenv("ARIN_WHOIS_BASE_URL", server.URL)
			step := resource.TestStep{Config: "provider \"arin\" {}\n" + tc.config}
			if tc.pattern != "" {
				step.ExpectError = regexp.MustCompile(tc.pattern)
			} else {
				step.Check = resource.TestCheckResourceAttr("data.arin_whois_cidr_networks.test", "networks.#", "0")
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{step}})
		})
	}
}
