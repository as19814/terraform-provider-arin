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

type fakeASSetAPI struct {
	mu      sync.Mutex
	objects map[string]string
	writes  map[string]int
}

func (f *fakeASSetAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "ApiKey acceptance-test-key" || r.Header.Get("Content-Type") != "application/xml" || r.Header.Get("Accept") != "application/xml" {
		http.Error(w, "bad headers", 400)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	name := strings.TrimPrefix(r.URL.Path, "/rest/irr/as-set/")
	switch r.Method {
	case "GET":
		if s, ok := f.objects[name]; ok {
			fmt.Fprint(w, s)
		} else {
			w.WriteHeader(404)
		}
	case "POST", "PUT":
		b, _ := io.ReadAll(r.Body)
		var p struct {
			XMLName xml.Name `xml:"http://www.arin.net/regrws/core/v1 asSet"`
			Name    string   `xml:"name"`
			Org     string   `xml:"orgHandle"`
			Source  string   `xml:"source"`
		}
		if xml.Unmarshal(b, &p) != nil || p.Name == "" || p.Org != "EXAMPLE-1" || p.Source != "ARIN" {
			http.Error(w, "bad payload", 400)
			return
		}
		if r.Method == "POST" {
			if r.URL.Path != "/rest/irr/as-set" || r.URL.Query().Get("orgHandle") != p.Org {
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
		s := strings.ReplaceAll(string(b), "</asSet>", fmt.Sprintf("<creationDate>2026-01-01T00:00:00Z</creationDate><lastModifiedDate>2026-01-%02dT00:00:00Z</lastModifiedDate></asSet>", f.writes["POST"]+f.writes["PUT"]))
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
func setupASSetFake(t *testing.T) *fakeASSetAPI {
	t.Helper()
	f := &fakeASSetAPI{objects: map[string]string{}, writes: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	return f
}
func asSetConfig(name, members, extra string) string {
	return fmt.Sprintf(`provider "arin" {}
resource "arin_irr_as_set" "test" {
 name = %q
 org_handle = "EXAMPLE-1"
 description = ["Example <routing> & peers", "Second line"]
 members = %s
 %s
}`, name, members, extra)
}
func TestAccASSetResourceLifecycle(t *testing.T) {
	f := setupASSetFake(t)
	base := asSetConfig("AS-EXAMPLE", `["AS64496","AS64497"]`, `remarks = ["Some remarks"]
 members_by_ref = ["MNT-EXAMPLE-1"]
 poc_links = [{handle="ADMIN-1",function="AD"},{handle="TECH-1",function="T"}]`)
	empty := asSetConfig("AS-EXAMPLE", `[]`, "")
	replaced := asSetConfig("AS-REPLACED", `["AS64496"]`, "")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.objects) != 0 {
				return fmt.Errorf("AS set remains after destroy")
			}
			if f.writes["POST"] != 3 || f.writes["PUT"] != 2 || f.writes["DELETE"] != 2 {
				return fmt.Errorf("unexpected writes: %v", f.writes)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: base, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("arin_irr_as_set.test", "id", "AS-EXAMPLE"),
				resource.TestCheckResourceAttr("arin_irr_as_set.test", "members.#", "2"),
				resource.TestCheckResourceAttr("arin_irr_as_set.test", "poc_links.#", "2"),
				resource.TestCheckResourceAttr("arin_irr_as_set.test", "description.0", "Example <routing> & peers"),
			)},
			{ResourceName: "arin_irr_as_set.test", ImportState: true, ImportStateVerify: true},
			// External drift is detected and corrected by one PUT.
			{PreConfig: func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.objects["AS-EXAMPLE"] = strings.ReplaceAll(f.objects["AS-EXAMPLE"], "AS64497", "AS64498")
			}, Config: base, Check: resource.TestCheckTypeSetElemAttr("arin_irr_as_set.test", "members.*", "AS64497")},
			// Omitted optional fields and explicit empty membership clear remote values.
			{Config: empty, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("arin_irr_as_set.test", "members.#", "0"),
				resource.TestCheckResourceAttr("arin_irr_as_set.test", "remarks.#", "0"),
				resource.TestCheckResourceAttr("arin_irr_as_set.test", "members_by_ref.#", "0"),
				resource.TestCheckResourceAttr("arin_irr_as_set.test", "poc_links.#", "0"),
			)},
			// A remotely removed object is recreated.
			{PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.objects, "AS-EXAMPLE") }, Config: empty},
			{Config: replaced, Check: resource.TestCheckResourceAttr("arin_irr_as_set.test", "id", "AS-REPLACED")},
		},
	})
}
func TestAccASSetResourceExistingObject(t *testing.T) {
	f := setupASSetFake(t)
	f.objects["AS-EXAMPLE"] = `<asSet xmlns="http://www.arin.net/regrws/core/v1"><name>AS-EXAMPLE</name><orgHandle>EXAMPLE-1</orgHandle><source>ARIN</source></asSet>`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps:                    []resource.TestStep{{Config: asSetConfig("AS-EXAMPLE", `[]`, ""), ExpectError: regexp.MustCompile("already exists; import")}},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 0 {
		t.Fatalf("existing object was mutated: %v", f.writes)
	}
}
func TestAccASSetResourceInvalid(t *testing.T) {
	f := setupASSetFake(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps:                    []resource.TestStep{{Config: asSetConfig("../../bad", `[]`, ""), ExpectError: regexp.MustCompile("Invalid AS set configuration")}},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.writes) != 0 {
		t.Fatal("invalid config caused writes")
	}
}
