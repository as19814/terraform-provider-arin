package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRPSLDataSource(t *testing.T) {
	for kind, name := range map[string]string{"as-set": "AS-EXAMPLE", "route-set": "RS-EXAMPLE", "aut-num": "AS64496", "route": "192.0.2.0/24", "route6": "2001:db8::/48"} {
		t.Run(kind, func(t *testing.T) {
			origin, configOrigin, path := "", "", "/rest/irr/"+kind+"/"+name
			if strings.HasPrefix(kind, "route") && kind != "route-set" {
				origin = "origin: AS64496\n"
				configOrigin = "origin_as = \"AS64496\""
				path = "/rest/irr/route/" + name + "/AS64496"
			}
			raw := fmt.Sprintf("%s: %s\n%smnt-by: MNT-EXAMPLE-1\nsource: ARIN\nx-preserved: opaque\n", kind, name, origin)
			var changed atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != path || r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Accept") != "application/rpsl" {
					t.Error("unexpected RPSL request")
					w.WriteHeader(400)
					return
				}
				body := raw
				if changed.Load() {
					body = strings.Replace(body, "opaque", "updated", 1)
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "test-key")
			t.Setenv("ARIN_BASE_URL", server.URL)
			t.Setenv("ARIN_RDAP_BASE_URL", "")
			config := fmt.Sprintf("provider \"arin\" {}\ndata \"arin_irr_rpsl\" \"test\" {\nobject_type=%q\nname=%q\n%s\n}\n", kind, name, configOrigin)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
				{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_irr_rpsl.test", "org_handle", "EXAMPLE-1"), resource.TestCheckResourceAttr("data.arin_irr_rpsl.test", "rpsl", raw))},
				{Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.TestCheckResourceAttr("data.arin_irr_rpsl.test", "rpsl", strings.Replace(raw, "opaque", "updated", 1))},
			}})
		})
	}
}
func TestAccRPSLDataSourceRejectsMismatchedIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "as-set: AS-OTHER\nmnt-by: MNT-EXAMPLE-1\nsource: ARIN\n")
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: "provider \"arin\" {}\ndata \"arin_irr_rpsl\" \"test\" {\nobject_type=\"as-set\"\nname=\"AS-EXAMPLE\"\n}", ExpectError: regexp.MustCompile("mismatched RPSL object")}}})
}
