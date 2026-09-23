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

func TestAccRDAPDomains(t *testing.T) {
	fixture, err := os.ReadFile("../arin/testdata/rdap_domain.json")
	if err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected authenticated hierarchy request")
		}
		if r.URL.Query().Get("status") == "active" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"errorCode":404}`)
			return
		}
		switch r.URL.Path {
		case "/registry/domains/rirSearch1/rdap-top/2.0.192.in-addr.arpa.", "/registry/domains/rirSearch1/rdap-up/2.0.192.in-addr.arpa.":
			fmt.Fprint(w, `{"objectClassName":"domain","ldhName":"192.in-addr.arpa."}`)
		case "/registry/domains/rirSearch1/rdap-down/0.192.in-addr.arpa.", "/registry/domains/rirSearch1/rdap-bottom/0.192.in-addr.arpa.":
			body := string(fixture)
			if changed.Load() {
				body = strings.Replace(body, "NS2.EXAMPLE.NET.", "NS3.EXAMPLE.NET.", 1)
			}
			fmt.Fprintf(w, `{"domainSearchResults":[%s,%s]}`, strings.ReplaceAll(body, "2.0.192.in-addr.arpa.", "3.0.192.in-addr.arpa."), body)
		default:
			t.Error("unexpected hierarchy URL")
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	config := `provider "arin" {}
`
	for _, relation := range []string{"top", "up", "down", "bottom"} {
		name := "2.0.192.IN-ADDR.ARPA"
		if relation == "down" || relation == "bottom" {
			name = "0.192.in-addr.arpa."
		}
		config += fmt.Sprintf("data \"arin_rdap_domains\" %q {\n name=%q\n relation=%q\n}\n", relation, name, relation)
	}
	config += `data "arin_rdap_domains" "active" {
 name="2.0.192.in-addr.arpa."
 relation="up"
 active_only=true
}`
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_rdap_domains.top", "domains.#", "1"), resource.TestCheckResourceAttr("data.arin_rdap_domains.up", "domains.0.name", "192.in-addr.arpa."), resource.TestCheckResourceAttr("data.arin_rdap_domains.down", "domains.#", "2"), resource.TestCheckResourceAttr("data.arin_rdap_domains.bottom", "domains.0.name", "2.0.192.in-addr.arpa."), resource.TestCheckResourceAttr("data.arin_rdap_domains.down", "domains.0.ds_records.0.key_tag", "12345"), resource.TestCheckResourceAttr("data.arin_rdap_domains.active", "domains.#", "0"))},
		{Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.TestCheckResourceAttr("data.arin_rdap_domains.down", "domains.0.nameservers.1.name", "ns3.example.net")},
		{Config: config, PlanOnly: true},
	}})
}
func TestAccRDAPDomainsErrors(t *testing.T) {
	for _, tc := range []struct {
		name, config, body, pattern string
		requests                    bool
	}{
		{"partial", `relation="down"`, `{"domainSearchResults":[],"notices":[{"title":"truncated"}]}`, "truncated", true},
		{"active_children", "relation=\"down\"\n active_only=true", `{}`, "active_only only", false},
		{"relation", `relation="invalid"`, `{}`, "relation must", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !tc.requests {
					t.Error("invalid config made request")
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "")
			t.Setenv("ARIN_BASE_URL", "")
			t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
			config := "provider \"arin\" {}\ndata \"arin_rdap_domains\" \"test\" {\n name=\"2.0.192.in-addr.arpa.\"\n" + tc.config + "\n}\n"
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, ExpectError: regexp.MustCompile(tc.pattern)}}})
		})
	}
}
