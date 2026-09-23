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

type fakeAutnumAPI struct {
	mu      sync.Mutex
	objects map[string]string
	writes  map[string]int
}

func (f *fakeAutnumAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "ApiKey acceptance-test-key" || r.Header.Get("Content-Type") != "application/xml" || r.Header.Get("Accept") != "application/xml" {
		http.Error(w, "bad headers", 400)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	name := strings.TrimPrefix(r.URL.Path, "/rest/irr/aut-num/")
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
			XMLName xml.Name `xml:"http://www.arin.net/regrws/core/v1 autnum"`
			Name    string   `xml:"asNumber"`
			Org     string   `xml:"orgHandle"`
			Source  string   `xml:"source"`
		}
		if xml.Unmarshal(b, &p) != nil || p.Name == "" || p.Org != "EXAMPLE-1" || p.Source != "ARIN" {
			http.Error(w, "bad payload", 400)
			return
		}
		if r.Method == "POST" {
			if r.URL.Path != "/rest/irr/aut-num/"+p.Name {
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
		s := strings.ReplaceAll(string(b), "</autnum>", fmt.Sprintf("<pocLinks><pocLinkRef handle=\"ADMIN-1\" function=\"AD\" description=\"Admin\"/><pocLinkRef handle=\"TECH-1\" function=\"T\" description=\"Tech\"/></pocLinks><creationDate>2026-01-01T00:00:00Z</creationDate><lastModifiedDate>2026-01-%02dT00:00:00Z</lastModifiedDate></autnum>", f.writes["POST"]+f.writes["PUT"]))
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
func setupAutnumFake(t *testing.T) *fakeAutnumAPI {
	t.Helper()
	f := &fakeAutnumAPI{objects: map[string]string{}, writes: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	return f
}
func autnumConfig(name, members, extra string) string {
	return fmt.Sprintf(`provider "arin" {}
resource "arin_irr_aut_num" "test" {
 as_number = %q
 as_name = "EXAMPLE-AS"
 org_handle = "EXAMPLE-1"
 description = ["Example <routing> & peers", "Second line"]
 member_of = %s
 %s
}`, name, members, extra)
}
func TestAccAutnumResourceLifecycle(t *testing.T) {
	f := setupAutnumFake(t)
	base := autnumConfig("AS64496", `["AS-ONE","AS-TWO"]`, `remarks = ["Some remarks"]
 import_policy = ["from AS64497 accept ANY"]
 export_policy = ["to AS64497 announce AS64496"]
 default_policy = ["to AS64497 networks ANY"]
 mp_import_policy = ["afi ipv6.unicast from AS64497 accept ANY"]
 mp_export_policy = ["afi ipv6.unicast to AS64497 announce AS64496"]
 mp_default_policy = ["afi ipv6.unicast to AS64497 networks ANY"]`)
	empty := autnumConfig("AS64496", `[]`, "")
	replaced := autnumConfig("AS64497", `["AS-ONE"]`, "")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.objects) != 0 {
				return fmt.Errorf("aut-num remains after destroy")
			}
			if f.writes["POST"] != 3 || f.writes["PUT"] != 2 || f.writes["DELETE"] != 2 {
				return fmt.Errorf("unexpected writes: %v", f.writes)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: base, Check: resource.ComposeAggregateTestCheckFunc(checkIRRResponseMetadata("arin_irr_aut_num.test"),
				resource.TestCheckResourceAttr("arin_irr_aut_num.test", "id", "AS64496"),
				resource.TestCheckResourceAttr("arin_irr_aut_num.test", "member_of.#", "2"),
				resource.TestCheckResourceAttr("arin_irr_aut_num.test", "poc_links.#", "2"),
				resource.TestCheckResourceAttr("arin_irr_aut_num.test", "description.0", "Example <routing> & peers"),
			)},
			{ResourceName: "arin_irr_aut_num.test", ImportState: true, ImportStateVerify: true},
			// External drift is detected and corrected by one PUT.
			{PreConfig: func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.objects["AS64496"] = strings.ReplaceAll(f.objects["AS64496"], "AS-TWO", "AS-OTHER")
			}, Config: base, Check: resource.TestCheckTypeSetElemAttr("arin_irr_aut_num.test", "member_of.*", "AS-TWO")},
			// Omitted optional fields and explicit empty membership clear remote values.
			{Config: empty, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("arin_irr_aut_num.test", "member_of.#", "0"),
				resource.TestCheckResourceAttr("arin_irr_aut_num.test", "remarks.#", "0"),
				resource.TestCheckResourceAttr("arin_irr_aut_num.test", "import_policy.#", "0"),
				resource.TestCheckResourceAttr("arin_irr_aut_num.test", "poc_links.#", "2"),
			)},
			// A remotely removed object is recreated.
			{PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.objects, "AS64496") }, Config: empty},
			{Config: replaced, Check: resource.TestCheckResourceAttr("arin_irr_aut_num.test", "id", "AS64497")},
		},
	})
}
func TestAccAutnumResourceExistingObject(t *testing.T) {
	f := setupAutnumFake(t)
	f.objects["AS64496"] = `<autnum xmlns="http://www.arin.net/regrws/core/v1"><asNumber>AS64496</asNumber><asName>EXAMPLE-AS</asName><orgHandle>EXAMPLE-1</orgHandle><source>ARIN</source></autnum>`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps:                    []resource.TestStep{{Config: autnumConfig("AS64496", `[]`, ""), ExpectError: regexp.MustCompile("already exists; import")}},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 0 {
		t.Fatalf("existing object was mutated: %v", f.writes)
	}
}
func TestAccAutnumResourceInvalid(t *testing.T) {
	f := setupAutnumFake(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps:                    []resource.TestStep{{Config: autnumConfig("../../bad", `[]`, ""), ExpectError: regexp.MustCompile("Invalid aut-num configuration")}},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 0 {
		t.Fatal("invalid config caused writes")
	}
}
