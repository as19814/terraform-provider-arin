package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPublicCatalog(t *testing.T) {
	asn, err := os.ReadFile("../arin/testdata/asn.json")
	if err != nil {
		t.Fatal(err)
	}
	entity, err := os.ReadFile("../arin/testdata/entity.json")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("public read sent a key")
		}
		switch r.URL.RequestURI() {
		case "/registry/autnum/64496":
			_, _ = w.Write(asn)
		case "/registry/autnums/reverse_search/entity?handle=EXAMPLE-1":
			fmt.Fprintf(w, `{"autnumSearchResults":[%s]}`, asn)
		case "/registry/entity/EXAMPLE-1":
			_, _ = w.Write(entity)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.RequestURI())
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: `
provider "arin" {}
data "arin_asn" "test" { asn = 64496 }
data "arin_asns" "test" { org_handle = "EXAMPLE-1" }
data "arin_org_pocs" "test" { org_handle = "EXAMPLE-1" }
`, Check: resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr("data.arin_asn.test", "name", "EXAMPLE-AS"),
		resource.TestCheckResourceAttr("data.arin_asns.test", "asns.0.start_asn", "64496"),
		resource.TestCheckResourceAttr("data.arin_org_pocs.test", "pocs.0.roles.0", "abuse"),
	)}}})
}
