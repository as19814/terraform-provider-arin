package provider

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type orgPOCFake struct {
	mu                              sync.Mutex
	links                           []arin.NetPOC
	readStatus, writeStatus, writes int
	ignoreDelete                    bool
}

func (f *orgPOCFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method == "GET" {
		if f.readStatus != 0 {
			w.WriteHeader(f.readStatus)
			return
		}
		p := struct {
			XMLName xml.Name      `xml:"http://www.arin.net/regrws/core/v1 org"`
			Handle  string        `xml:"handle"`
			Name    string        `xml:"orgName"`
			Links   []arin.NetPOC `xml:"pocLinks>pocLinkRef"`
		}{Handle: "EXAMPLE-1", Name: "Example", Links: f.links}
		_ = xml.NewEncoder(w).Encode(p)
		return
	}
	f.writes++
	if f.writeStatus != 0 {
		w.WriteHeader(f.writeStatus)
		return
	}
	handle, function, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/rest/org/EXAMPLE-1/poc/"), ";pocFunction=")
	if !ok || handle == "" || function == "" {
		w.WriteHeader(400)
		return
	}
	switch r.Method {
	case "PUT":
		found := false
		for _, p := range f.links {
			if p.Handle == handle && p.Function == function {
				found = true
			}
		}
		if !found {
			f.links = append(f.links, arin.NetPOC{Handle: handle, Function: function})
		}
	case "DELETE":
		if !f.ignoreDelete {
			next := []arin.NetPOC{}
			for _, p := range f.links {
				if p.Handle != handle || p.Function != function {
					next = append(next, p)
				}
			}
			f.links = next
		}
	default:
		w.WriteHeader(405)
		return
	}
	w.WriteHeader(204)
}
func setupOrgPOCFake(t *testing.T) (*orgPOCFake, *arin.Client) {
	t.Helper()
	f := &orgPOCFake{links: []arin.NetPOC{{Handle: "ADMIN-ARIN", Function: "AD"}, {Handle: "TECH-ARIN", Function: "T"}}}
	s := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(s.Close)
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", s.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	c, err := arin.New(arin.Config{APIKey: "test-key", BaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}
func orgPOCConfig(org, poc string, roles []string) string {
	config := "provider \"arin\" {}\n"
	for _, role := range roles {
		config += fmt.Sprintf("resource \"arin_org_poc\" %q {\n org_handle = %q\n poc_handle = %q\n function = %q\n}\n", "role_"+strings.ToLower(role), org, poc, role)
	}
	return config
}
func orgPOCSteps(org, poc string) []resource.TestStep {
	all := orgPOCConfig(org, poc, []string{"T", "N", "AB", "R", "D"})
	steps := []resource.TestStep{{Config: all}}
	for _, role := range []string{"T", "N", "AB", "R", "D"} {
		steps = append(steps, resource.TestStep{ResourceName: "arin_org_poc.role_" + strings.ToLower(role), ImportState: true, ImportStateId: org + "/" + poc + "/" + role, ImportStateVerify: true})
	}
	reduced := orgPOCConfig(org, poc, []string{"N"})
	return append(steps, resource.TestStep{Config: reduced}, resource.TestStep{Config: reduced, PlanOnly: true})
}
func TestAccOrgPOCLifecycle(t *testing.T) {
	f, _ := setupOrgPOCFake(t)
	steps := orgPOCSteps("EXAMPLE-1", "TEST-ARIN")
	steps = append(steps, resource.TestStep{Config: steps[len(steps)-1].Config, PreConfig: func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		next := []arin.NetPOC{}
		for _, p := range f.links {
			if p.Handle != "TEST-ARIN" {
				next = append(next, p)
			}
		}
		f.links = next
	}})
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: steps, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !reflect.DeepEqual(f.links, []arin.NetPOC{{Handle: "ADMIN-ARIN", Function: "AD"}, {Handle: "TECH-ARIN", Function: "T"}}) {
			return fmt.Errorf("unmanaged links changed")
		}
		return nil
	}})
}
