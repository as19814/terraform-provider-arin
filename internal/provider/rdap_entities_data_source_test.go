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

func TestAccRDAPEntities(t *testing.T) {
	fixture, err := os.ReadFile("../arin/testdata/rdap_entity.json")
	if err != nil {
		t.Fatal(err)
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/registry/entities" || r.Header.Get("Authorization") != "" || len(r.URL.Query()) != 1 {
			t.Error("unexpected public search")
			w.WriteHeader(400)
			return
		}
		if r.URL.Query().Get("handle") == "MISSING" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"errorCode":404,"title":"Not Found"}`)
			return
		}
		if r.URL.Query().Get("handle") != "EXAMPLE-*" && r.URL.Query().Get("fn") != "Organization*" {
			t.Error("wrong search field or term")
		}
		body := string(fixture)
		if changed.Load() {
			body = strings.Replace(body, "Example Organization", "Updated Organization", 1)
		}
		fmt.Fprintf(w, `{"entitySearchResults":[%s,%s]}`, strings.Replace(body, `"handle":"EXAMPLE-1"`, `"handle":"EXAMPLE-2"`, 1), body)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	config := `provider "arin" {}
data "arin_rdap_entities" "handle" {
 search_by="handle"
 query="EXAMPLE-*"
}
data "arin_rdap_entities" "name" {
 search_by="name"
 query="Organization*"
}
data "arin_rdap_entities" "missing" {
 search_by="handle"
 query="MISSING"
}`
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_rdap_entities.handle", "entities.#", "2"), resource.TestCheckResourceAttr("data.arin_rdap_entities.handle", "entities.0.handle", "EXAMPLE-1"), resource.TestCheckResourceAttr("data.arin_rdap_entities.name", "entities.1.handle", "EXAMPLE-2"), resource.TestCheckResourceAttr("data.arin_rdap_entities.name", "entities.0.emails.0", "noc@example.net"), resource.TestCheckResourceAttr("data.arin_rdap_entities.missing", "entities.#", "0"), resource.TestCheckResourceAttrSet("data.arin_rdap_entities.name", "entities.0.rdap_json"))},
		{Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.TestCheckResourceAttr("data.arin_rdap_entities.handle", "entities.0.names.1", "Updated Organization")},
		{Config: config, PlanOnly: true},
	}})
}

func TestAccRDAPEntitiesPartial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"entitySearchResults":[],"notices":[{"type":"result set truncated"}]}`)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_RDAP_BASE_URL", server.URL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: `provider "arin" {}
data "arin_rdap_entities" "test" {
 search_by="handle"
 query="EXAMPLE-*"
}`, ExpectError: regexp.MustCompile("truncated")}}})
}
