package provider

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type fakeRouteSetAPI struct {
	mu      sync.Mutex
	objects map[string]string
	writes  map[string]int
}

func (f *fakeRouteSetAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "ApiKey acceptance-test-key" || r.Header.Get("Content-Type") != "application/xml" || r.Header.Get("Accept") != "application/xml" {
		http.Error(w, "bad headers", 400)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	name := strings.TrimPrefix(r.URL.Path, "/rest/irr/route-set/")
	switch r.Method {
	case "GET":
		if s, ok := f.objects[name]; ok {
			fmt.Fprint(w, s)
		} else {
			w.WriteHeader(404)
		}
	case "POST", "PUT":
		b, _ := io.ReadAll(r.Body)
		// Match the OT&E validation rules that exposed the original write bugs.
		if strings.Contains(string(b), "<pocLinks") || strings.Contains(string(b), "<remarks></remarks>") {
			http.Error(w, "server-owned POCs or invalid empty remarks", 400)
			return
		}
		var p struct {
			XMLName xml.Name `xml:"http://www.arin.net/regrws/core/v1 routeSet"`
			Name    string   `xml:"name"`
			Org     string   `xml:"orgHandle"`
			Source  string   `xml:"source"`
		}
		if xml.Unmarshal(b, &p) != nil || p.Name == "" || p.Org != "EXAMPLE-1" || p.Source != "ARIN" {
			http.Error(w, "bad payload", 400)
			return
		}
		if r.Method == "POST" {
			if r.URL.Path != "/rest/irr/route-set" || r.URL.Query().Get("orgHandle") != p.Org {
				http.Error(w, "bad create path", 400)
				return
			}
			if _, ok := f.objects[p.Name]; ok {
				w.WriteHeader(409)
				return
			}
		} else if name != p.Name {
			http.Error(w, "bad update path", 400)
			return
		}
		f.writes[r.Method]++
		s := strings.ReplaceAll(string(b), "</routeSet>", fmt.Sprintf("<pocLinks><pocLinkRef handle=\"ADMIN-1\" function=\"AD\" description=\"Admin\"/><pocLinkRef handle=\"TECH-1\" function=\"T\" description=\"Tech\"/></pocLinks><creationDate>2026-01-01T00:00:00Z</creationDate><lastModifiedDate>2026-01-%02dT00:00:00Z</lastModifiedDate></routeSet>", f.writes["POST"]+f.writes["PUT"]))
		f.objects[p.Name] = s
		fmt.Fprint(w, s)
	case "DELETE":
		f.writes["DELETE"]++
		if s, ok := f.objects[name]; ok {
			delete(f.objects, name)
			fmt.Fprint(w, s)
		} else {
			w.WriteHeader(404)
		}
	default:
		w.WriteHeader(405)
	}
}
func setupRouteSetFake(t *testing.T) *fakeRouteSetAPI {
	t.Helper()
	f := &fakeRouteSetAPI{objects: map[string]string{}, writes: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	return f
}
func routeSetConfig(name, members, extra string) string {
	return fmt.Sprintf(`provider "arin" {}
resource "arin_irr_route_set" "test" {
 name = %q
 org_handle = "EXAMPLE-1"
 description = ["Example <routing> & peers", "Second line"]
 members = %s
 %s
}`, name, members, extra)
}
func TestAccRouteSetResourceLifecycle(t *testing.T) {
	f := setupRouteSetFake(t)
	base := routeSetConfig("RS-EXAMPLE", `["192.0.2.0/24","198.51.100.0/24"]`, `remarks = ["Some remarks"]
 mp_members = ["2001:db8::/32"]
 members_by_ref = ["MNT-EXAMPLE-1"]`)
	empty := routeSetConfig("RS-EXAMPLE", `[]`, "")
	replaced := routeSetConfig("RS-REPLACED", `["192.0.2.0/24"]`, "")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.objects) != 0 {
				return fmt.Errorf("route set remains after destroy")
			}
			if f.writes["POST"] != 3 || f.writes["PUT"] != 2 || f.writes["DELETE"] != 2 {
				return fmt.Errorf("unexpected writes: %v", f.writes)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: base, Check: resource.ComposeAggregateTestCheckFunc(checkIRRResponseMetadata("arin_irr_route_set.test"),
				resource.TestCheckResourceAttr("arin_irr_route_set.test", "id", "RS-EXAMPLE"),
				resource.TestCheckResourceAttr("arin_irr_route_set.test", "members.#", "2"),
				resource.TestCheckResourceAttr("arin_irr_route_set.test", "mp_members.#", "1"),
				resource.TestCheckResourceAttr("arin_irr_route_set.test", "poc_links.#", "2"),
				resource.TestCheckResourceAttr("arin_irr_route_set.test", "description.0", "Example <routing> & peers"),
			)},
			{ResourceName: "arin_irr_route_set.test", ImportState: true, ImportStateVerify: true},
			// External drift is detected and corrected by one PUT.
			{PreConfig: func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.objects["RS-EXAMPLE"] = strings.ReplaceAll(f.objects["RS-EXAMPLE"], "198.51.100.0/24", "203.0.113.0/24")
			}, Config: base, Check: resource.TestCheckTypeSetElemAttr("arin_irr_route_set.test", "members.*", "198.51.100.0/24")},
			// Omitted optional fields and explicit empty membership clear remote values.
			{Config: empty, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("arin_irr_route_set.test", "members.#", "0"),
				resource.TestCheckResourceAttr("arin_irr_route_set.test", "mp_members.#", "0"),
				resource.TestCheckResourceAttr("arin_irr_route_set.test", "remarks.#", "0"),
				resource.TestCheckResourceAttr("arin_irr_route_set.test", "members_by_ref.#", "0"),
				resource.TestCheckResourceAttr("arin_irr_route_set.test", "poc_links.#", "2"),
			)},
			// A remotely removed object is recreated.
			{PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.objects, "RS-EXAMPLE") }, Config: empty},
			{Config: replaced, Check: resource.TestCheckResourceAttr("arin_irr_route_set.test", "id", "RS-REPLACED")},
		},
	})
}
func TestAccRouteSetResourceExistingObject(t *testing.T) {
	f := setupRouteSetFake(t)
	f.objects["RS-EXAMPLE"] = `<routeSet xmlns="http://www.arin.net/regrws/core/v1"><name>RS-EXAMPLE</name><orgHandle>EXAMPLE-1</orgHandle><source>ARIN</source></routeSet>`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps:                    []resource.TestStep{{Config: routeSetConfig("RS-EXAMPLE", `[]`, ""), ExpectError: regexp.MustCompile("already exists; import")}},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 0 {
		t.Fatalf("existing object was mutated: %v", f.writes)
	}
}
func TestAccRouteSetResourceInvalid(t *testing.T) {
	f := setupRouteSetFake(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps:                    []resource.TestStep{{Config: routeSetConfig("../../bad", `[]`, ""), ExpectError: regexp.MustCompile("Invalid route set configuration")}},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 0 {
		t.Fatal("invalid config caused writes")
	}
}
