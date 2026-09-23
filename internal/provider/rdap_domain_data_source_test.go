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

func TestAccRDAPDomain(t *testing.T) {
	fixture, err := os.ReadFile("../arin/testdata/rdap_domain.json")
	if err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected authenticated request")
		}
		switch r.URL.Path {
		case "/registry/domain/2.0.192.in-addr.arpa.":
			body := string(fixture)
			if changed.Load() {
				body = strings.Replace(body, "NS2.EXAMPLE.NET.", "NS3.EXAMPLE.NET.", 1)
			}
			fmt.Fprint(w, body)
		case "/registry/domain/8.b.d.0.1.0.0.2.ip6.arpa.":
			fmt.Fprint(w, `{"objectClassName":"domain","ldhName":"8.b.d.0.1.0.0.2.ip6.arpa."}`)
		default:
			t.Error("unexpected domain request")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	config := `provider "arin" {}
data "arin_rdap_domain" "v4" { name="2.0.192.IN-ADDR.ARPA" }
data "arin_rdap_domain" "v6" { name="8.b.d.0.1.0.0.2.ip6.arpa." }
`
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_rdap_domain.v4", "name", "2.0.192.IN-ADDR.ARPA"), resource.TestCheckResourceAttr("data.arin_rdap_domain.v4", "nameservers.0.name", "ns1.example.net"), resource.TestCheckResourceAttr("data.arin_rdap_domain.v4", "nameservers.0.ipv6_addresses.0", "2001:db8::1"), resource.TestCheckResourceAttr("data.arin_rdap_domain.v4", "zone_signed", "false"), resource.TestCheckResourceAttr("data.arin_rdap_domain.v4", "delegation_signed", "true"), resource.TestCheckResourceAttr("data.arin_rdap_domain.v4", "ds_records.0.key_tag", "12345"), resource.TestCheckResourceAttr("data.arin_rdap_domain.v4", "key_records.0.protocol", "3"), resource.TestCheckResourceAttr("data.arin_rdap_domain.v4", "network_handle", "NET-192-0-2-0-1"), resource.TestCheckResourceAttrSet("data.arin_rdap_domain.v4", "rdap_json"), resource.TestCheckResourceAttr("data.arin_rdap_domain.v6", "nameservers.#", "0"), resource.TestCheckNoResourceAttr("data.arin_rdap_domain.v6", "zone_signed"))},
		{Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.TestCheckResourceAttr("data.arin_rdap_domain.v4", "nameservers.1.name", "ns3.example.net")},
		{Config: config, PlanOnly: true},
	}})
}
func TestAccRDAPDomainErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, pattern string
		status              int
	}{
		{"missing", `{"errorCode":404}`, "HTTP 404", 404},
		{"partial", `{"objectClassName":"domain","ldhName":"2.0.192.in-addr.arpa.","notices":[{"type":"object truncated"}]}`, "truncated", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "")
			t.Setenv("ARIN_BASE_URL", "")
			t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: `provider "arin" {}
data "arin_rdap_domain" "test" { name="2.0.192.in-addr.arpa." }`, ExpectError: regexp.MustCompile(tc.pattern)}}})
		})
	}
}
