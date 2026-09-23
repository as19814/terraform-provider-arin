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

func TestAccRDAPNetworkHierarchy(t *testing.T) {
	fixture, err := os.ReadFile("../arin/testdata/rdap_network.json")
	if err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.URL.Query().Get("apikey") != "" {
			t.Error("unexpected authenticated hierarchy request")
		}
		if r.URL.Query().Get("status") == "active" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"errorCode":404}`)
			return
		}
		body := string(fixture)
		if changed.Load() {
			body = strings.ReplaceAll(body, "EXAMPLE-NET", "CHANGED-NET")
		}
		switch r.URL.Path {
		case "/registry/ips/rirSearch1/rdap-top/192.0.2.1", "/registry/ips/rirSearch1/rdap-up/192.0.2.1":
			fmt.Fprint(w, body)
		case "/registry/ips/rirSearch1/rdap-down/192.0.0.0/16", "/registry/ips/rirSearch1/rdap-bottom/192.0.0.0/16":
			fmt.Fprintf(w, `{"ipSearchResults":[%s,%s]}`, strings.NewReplacer("NET-192-0-2-0-1", "NET-192-0-3-0-1", "192.0.2.", "192.0.3.").Replace(body), body)
		default:
			t.Error("unexpected hierarchy path")
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	config := "provider \"arin\" {}\n"
	for _, relation := range []string{"top", "up", "down", "bottom"} {
		query := "192.0.2.1"
		if relation == "down" || relation == "bottom" {
			query = "192.0.0.0/16"
		}
		config += fmt.Sprintf("data \"arin_rdap_network_hierarchy\" %q {\n query=%q\n relation=%q\n}\n", relation, query, relation)
	}
	config += "data \"arin_rdap_network_hierarchy\" \"active\" {\n query=\"192.0.2.0/24\"\n relation=\"up\"\n active_only=true\n}\n"
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("data.arin_rdap_network_hierarchy.top", "networks.#", "1"),
			resource.TestCheckResourceAttr("data.arin_rdap_network_hierarchy.up", "networks.0.handle", "NET-192-0-2-0-1"),
			resource.TestCheckResourceAttr("data.arin_rdap_network_hierarchy.down", "networks.#", "2"),
			resource.TestCheckResourceAttr("data.arin_rdap_network_hierarchy.bottom", "networks.0.cidrs.0", "192.0.2.0/24"),
			resource.TestCheckResourceAttrSet("data.arin_rdap_network_hierarchy.down", "networks.0.rdap_json"),
			resource.TestCheckResourceAttr("data.arin_rdap_network_hierarchy.active", "networks.#", "0"),
		)},
		{Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.TestCheckResourceAttr("data.arin_rdap_network_hierarchy.down", "networks.0.name", "CHANGED-NET")},
		{Config: config, PlanOnly: true},
	}})
}
func TestAccRDAPNetworkHierarchyErrors(t *testing.T) {
	for _, tc := range []struct {
		name, inputs, body, pattern string
		requests                    bool
	}{
		{"partial", "query=\"192.0.2.0/24\"\n relation=\"down\"", `{"ipSearchResults":[],"notices":[{"title":"truncated"}]}`, "truncated", true},
		{"unsupported_filter", "query=\"192.0.2.0/24\"\n relation=\"down\"\n active_only=true", `{}`, "active_only only", false},
		{"invalid_query", "query=\"192.0.2.1/24\"\n relation=\"up\"", `{}`, "not a valid ip_network", false},
		{"invalid_relation", "query=\"192.0.2.0/24\"\n relation=\"invalid\"", `{}`, "relation must", false},
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
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: "provider \"arin\" {}\ndata \"arin_rdap_network_hierarchy\" \"test\" {\n" + tc.inputs + "\n}\n", ExpectError: regexp.MustCompile(tc.pattern)}}})
		})
	}
}
