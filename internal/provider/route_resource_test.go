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

type fakeRouteAPI struct {
	mu      sync.Mutex
	objects map[string]string
	writes  map[string]int
}

func (f *fakeRouteAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "ApiKey acceptance-test-key" || r.Header.Get("Content-Type") != "application/xml" || r.Header.Get("Accept") != "application/xml" {
		http.Error(w, "bad headers", 400)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/rest/irr/route/")
	last := strings.LastIndex(p, "/")
	if last < 0 {
		http.Error(w, "bad path", 400)
		return
	}
	id := p[:last] + "," + p[last+1:]
	w.Header().Set("Content-Type", "application/xml")
	switch r.Method {
	case "GET":
		if s, ok := f.objects[id]; ok {
			fmt.Fprint(w, s)
		} else {
			w.WriteHeader(404)
		}
	case "POST", "PUT":
		b, _ := io.ReadAll(r.Body)
		var v struct {
			XMLName xml.Name `xml:"http://www.arin.net/regrws/core/v1 route"`
			Prefix  string   `xml:"prefix"`
			Origin  string   `xml:"originAS"`
			Org     string   `xml:"orgHandle"`
			Source  string   `xml:"source"`
		}
		if xml.Unmarshal(b, &v) != nil || v.Prefix+","+v.Origin != id || v.Org != "EXAMPLE-1" || v.Source != "ARIN" || strings.Contains(string(b), "<pocLinks") || strings.Contains(string(b), "<remarks></remarks>") {
			http.Error(w, "bad payload", 400)
			return
		}
		_, exists := f.objects[id]
		if r.Method == "POST" && exists {
			w.WriteHeader(409)
			return
		}
		if r.Method == "PUT" && !exists {
			w.WriteHeader(404)
			return
		}
		f.writes[r.Method]++
		s := strings.ReplaceAll(string(b), "</route>", fmt.Sprintf(`<netHandle>NET-EXAMPLE-1</netHandle><pocLinks><pocLinkRef handle="TECH-1" function="T" description="Tech"/></pocLinks><creationDate>2026-01-01</creationDate><lastModifiedDate>2026-01-%02d</lastModifiedDate></route>`, f.writes["POST"]+f.writes["PUT"]))
		f.objects[id] = s
		fmt.Fprint(w, s)
	case "DELETE":
		f.writes["DELETE"]++
		if s, ok := f.objects[id]; ok {
			delete(f.objects, id)
			fmt.Fprint(w, s)
		} else {
			w.WriteHeader(404)
		}
	default:
		w.WriteHeader(405)
	}
}
func setupRouteFake(t *testing.T) *fakeRouteAPI {
	t.Helper()
	f := &fakeRouteAPI{objects: map[string]string{}, writes: map[string]int{}}
	s := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(s.Close)
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", s.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	return f
}
func routeConfig(prefix, origin, remarks string) string {
	return fmt.Sprintf(`provider "arin" {}
resource "arin_irr_route" "test" {
 prefix = %q
 origin_as = %q
 org_handle = "EXAMPLE-1"
 description = ["Example <route> & peers"]
 remarks = %s
}`, prefix, origin, remarks)
}
func TestAccIRRRouteLifecycle(t *testing.T) {
	for _, prefix := range []string{"192.0.2.0/24", "2001:db8::/48"} {
		t.Run(prefix, func(t *testing.T) {
			f := setupRouteFake(t)
			id := prefix + ",AS64496"
			initial := routeConfig(prefix, "AS64496", `["Initial remark"]`)
			cleared := routeConfig(prefix, "AS64496", `[]`)
			replacement := routeConfig(prefix, "AS64497", `[]`)
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
				CheckDestroy: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if len(f.objects) > 0 {
						return fmt.Errorf("route remains")
					}
					if f.writes["POST"] != 3 || f.writes["PUT"] != 2 || f.writes["DELETE"] != 2 {
						return fmt.Errorf("unexpected writes: %v", f.writes)
					}
					return nil
				},
				Steps: []resource.TestStep{
					{Config: initial, Check: resource.ComposeAggregateTestCheckFunc(checkIRRResponseMetadata("arin_irr_route.test"), resource.TestCheckResourceAttr("arin_irr_route.test", "id", id), resource.TestCheckResourceAttr("arin_irr_route.test", "net_handle", "NET-EXAMPLE-1"), resource.TestCheckResourceAttr("arin_irr_route.test", "poc_links.#", "1"))},
					{ResourceName: "arin_irr_route.test", ImportState: true, ImportStateVerify: true},
					{PreConfig: func() {
						f.mu.Lock()
						defer f.mu.Unlock()
						f.objects[id] = strings.ReplaceAll(f.objects[id], "Initial remark", "External drift")
					}, Config: initial, Check: resource.TestCheckResourceAttr("arin_irr_route.test", "remarks.0", "Initial remark")},
					{Config: cleared, Check: resource.TestCheckResourceAttr("arin_irr_route.test", "remarks.#", "0")},
					{PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.objects, id) }, Config: cleared},
					{Config: replacement, Check: resource.TestCheckResourceAttr("arin_irr_route.test", "id", prefix+",AS64497")},
				},
			})
		})
	}
}
func TestAccIRRRouteInvalid(t *testing.T) {
	setupRouteFake(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps:                    []resource.TestStep{{Config: routeConfig("192.0.2.1/24", "AS64496", `[]`), ExpectError: regexp.MustCompile("without host bits")}},
	})
}

func TestAccIRRRouteMembership(t *testing.T) {
	for _, prefix := range []string{"192.0.2.0/24", "2001:db8::/48"} {
		t.Run(prefix, func(t *testing.T) {
			f := setupRouteFake(t)
			base := routeConfig(prefix, "AS64496", `[]`)
			config := func(members string) string {
				return strings.Replace(base, "remarks = []", "remarks = []\n member_of = "+members, 1)
			}
			initial := config(`["RS-ONE", "AS64496:RS-TWO"]`)
			updated := config(`["RS-THREE"]`)
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
				Steps: []resource.TestStep{
					{Config: initial, Check: resource.TestCheckResourceAttr("arin_irr_route.test", "member_of.#", "2")},
					{ResourceName: "arin_irr_route.test", ImportState: true, ImportStateVerify: true},
					{Config: config(`["AS64496:RS-TWO", "RS-ONE"]`), PlanOnly: true},
					{Config: updated, Check: resource.TestCheckTypeSetElemAttr("arin_irr_route.test", "member_of.*", "RS-THREE")},
					{Config: updated, PreConfig: func() {
						f.mu.Lock()
						defer f.mu.Unlock()
						id := prefix + ",AS64496"
						f.objects[id] = strings.ReplaceAll(f.objects[id], "RS-THREE", "RS-DRIFT")
					}},
					{Config: base, Check: resource.TestCheckResourceAttr("arin_irr_route.test", "member_of.#", "0")},
					{Config: base, PlanOnly: true},
				},
				CheckDestroy: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if len(f.objects) != 0 || f.writes["POST"] != 1 || f.writes["PUT"] != 3 || f.writes["DELETE"] != 1 {
						return fmt.Errorf("unexpected lifecycle writes: %v", f.writes)
					}
					return nil
				},
			})
		})
	}
}
