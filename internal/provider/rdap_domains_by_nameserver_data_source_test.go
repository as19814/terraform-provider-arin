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

func TestAccRDAPDomainsByNameserver(t *testing.T) {
	fixture, err := os.ReadFile("../arin/testdata/rdap_domain.json")
	if err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected authenticated request")
		}
		switch r.URL.RequestURI() {
		case "/registry/domains?nsLdhName=missing.example.net":
			w.WriteHeader(404)
			fmt.Fprint(w, `{"errorCode":404}`)
		case "/registry/domains?nsLdhName=ns1.example.net":
			body := string(fixture)
			if changed.Load() {
				body = strings.Replace(body, "12345", "12346", 1)
			}
			fmt.Fprintf(w, `{"domainSearchResults":[%s,%s]}`, strings.ReplaceAll(body, "2.0.192.in-addr.arpa.", "3.0.192.in-addr.arpa."), body)
		default:
			t.Error("unexpected path")
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	config := `provider "arin" {}
data "arin_rdap_domains_by_nameserver" "test" { nameserver="NS1.Example.NET." }
data "arin_rdap_domains_by_nameserver" "missing" { nameserver="missing.example.net" }
`
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_rdap_domains_by_nameserver.test", "domains.#", "2"), resource.TestCheckResourceAttr("data.arin_rdap_domains_by_nameserver.test", "domains.0.name", "2.0.192.in-addr.arpa."), resource.TestCheckResourceAttr("data.arin_rdap_domains_by_nameserver.test", "domains.0.ds_records.0.key_tag", "12345"), resource.TestCheckResourceAttr("data.arin_rdap_domains_by_nameserver.missing", "domains.#", "0"))},
		{Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.TestCheckResourceAttr("data.arin_rdap_domains_by_nameserver.test", "domains.0.ds_records.0.key_tag", "12346")},
		{Config: config, PlanOnly: true},
	}})
}
func TestAccRDAPDomainsByNameserverErrors(t *testing.T) {
	for _, tc := range []struct {
		name, query, body, pattern string
		request                    bool
	}{
		{"wildcard", "ns*.example.net", `{}`, "not a valid dns_host", false},
		{"partial", "ns1.example.net", `{"domainSearchResults":[],"notices":[{"title":"truncated"}]}`, "truncated", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !tc.request {
					t.Error("invalid query reached server")
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "")
			t.Setenv("ARIN_BASE_URL", "")
			t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: fmt.Sprintf("provider \"arin\" {}\ndata \"arin_rdap_domains_by_nameserver\" \"test\" { nameserver=%q }", tc.query), ExpectError: regexp.MustCompile(tc.pattern)}}})
		})
	}
}
