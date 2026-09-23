package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	framework "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type metadataRouteFake struct {
	mu                                                          sync.Mutex
	prefix, link, body                                          string
	puts, otherWrites, readStatus, writeStatus, afterReadStatus int
	applyBeforeError                                            bool
}

func (f *metadataRouteFake) xml() string {
	extra := `<netHandle>NET-EXAMPLE-1</netHandle>`
	body := f.body
	if f.link != "" {
		extra += `<autoLinkedRoaHandle>` + f.link + `</autoLinkedRoaHandle>`
		annotation := `<line number="99">` + arin.LinkedRouteRemark + `</line>`
		if strings.Contains(body, "</remarks>") {
			body = strings.Replace(body, "</remarks>", annotation+"</remarks>", 1)
		} else {
			extra += "<remarks>" + annotation + "</remarks>"
		}
	}
	return strings.Replace(body, "</route>", extra+"</route>", 1)
}
func (f *metadataRouteFake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != "/rest/irr/route/"+f.prefix+"/AS64496" {
		w.WriteHeader(404)
		return
	}
	if r.Method == "GET" {
		if f.readStatus != 0 {
			w.WriteHeader(f.readStatus)
			return
		}
		if f.body == "" {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, f.xml())
		return
	}
	if r.Method != "PUT" {
		f.otherWrites++
		w.WriteHeader(405)
		return
	}
	f.puts++
	body, _ := io.ReadAll(r.Body)
	if strings.Contains(string(body), arin.LinkedRouteRemark) || strings.Contains(string(body), "autoLinkedRoaHandle") {
		w.WriteHeader(400)
		return
	}
	if f.writeStatus == 0 || f.applyBeforeError {
		f.body = string(body)
	}
	if f.afterReadStatus != 0 {
		f.readStatus = f.afterReadStatus
	}
	if f.writeStatus != 0 {
		w.WriteHeader(f.writeStatus)
		return
	}
	fmt.Fprint(w, f.xml())
}
func setupMetadataRouteFake(t *testing.T, prefix, link string) (*metadataRouteFake, *arin.Client) {
	t.Helper()
	f := &metadataRouteFake{prefix: prefix, link: link, body: fmt.Sprintf(`<route xmlns="http://www.arin.net/regrws/core/v1"><prefix>%s</prefix><originAS>AS64496</originAS><orgHandle>EXAMPLE-1</orgHandle><source>ARIN</source><description><line number="0">Initial</line></description></route>`, prefix)}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	c, err := arin.New(arin.Config{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}
func metadataRouteConfig(prefix, link, remarks string) string {
	return fmt.Sprintf(`provider "arin" {}
resource "arin_irr_route_metadata" "test" {
 prefix = %q
 origin_as = "AS64496"
 org_handle = "EXAMPLE-1"
 expected_roa_handle = %q
 description = ["Updated"]
 remarks = %s
 member_of = ["RS-TEST"]
}`, prefix, link, remarks)
}
func TestAccIRRRouteMetadataLifecycle(t *testing.T) {
	for _, prefix := range []string{"192.0.2.0/24", "2001:db8::/48"} {
		for _, link := range []string{"", "roa1"} {
			t.Run(prefix+link, func(t *testing.T) {
				f, _ := setupMetadataRouteFake(t, prefix, link)
				first := metadataRouteConfig(prefix, link, `["User remark"]`)
				cleared := metadataRouteConfig(prefix, link, `[]`)
				steps := []resource.TestStep{
					{Config: first, Check: resource.TestCheckResourceAttr("arin_irr_route_metadata.test", "remarks.#", "1")},
					{ResourceName: "arin_irr_route_metadata.test", ImportState: true, ImportStateVerify: true},
					{Config: first, PlanOnly: true},
					{Config: first, PreConfig: func() {
						f.mu.Lock()
						defer f.mu.Unlock()
						f.body = strings.Replace(f.body, "User remark", "External drift", 1)
					}},
					{Config: cleared},
				}
				if link != "" {
					steps = append(steps, resource.TestStep{Config: metadataRouteConfig(prefix, "roa2", `[]`), PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.link = "roa2" }, Check: resource.TestCheckResourceAttr("arin_irr_route_metadata.test", "auto_linked_roa_handle", "roa2")})
				}
				resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: steps, CheckDestroy: func(_ *terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if f.body == "" || f.otherWrites != 0 || f.puts != 3 {
						return fmt.Errorf("metadata lifecycle deleted target or sent unnecessary writes: puts=%d other=%d", f.puts, f.otherWrites)
					}
					return nil
				}})
			})
		}
	}
}
func TestAccIRRRouteMetadataWrongBinding(t *testing.T) {
	f, _ := setupMetadataRouteFake(t, "192.0.2.0/24", "roa1")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: metadataRouteConfig(f.prefix, "roa2", `[]`), ExpectError: regexp.MustCompile("IRR route ownership changed")}}})
	if f.puts != 0 || f.otherWrites != 0 {
		t.Fatal("wrong owner allowed write")
	}
}
func TestIRRRouteMetadataUncertainState(t *testing.T) {
	for _, mode := range []string{"lost", "verification", "not-applied"} {
		t.Run(mode, func(t *testing.T) {
			f, c := setupMetadataRouteFake(t, "192.0.2.0/24", "roa1")
			r := &irrRouteMetadataResource{client: c}
			ctx := context.Background()
			actual, err := c.GetIRRRoute(ctx, f.prefix+",AS64496")
			if err != nil {
				t.Fatal(err)
			}
			m := irrRouteMetadataModel{ExpectedROA: types.StringValue("roa1")}
			if d := m.set(ctx, actual); d.HasError() {
				t.Fatal(d)
			}
			m.Description, _ = types.ListValueFrom(ctx, types.StringType, []string{"Updated"})
			var schema framework.SchemaResponse
			r.Schema(ctx, framework.SchemaRequest{}, &schema)
			plan := tfsdk.Plan{Schema: schema.Schema}
			if d := plan.Set(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			if mode == "verification" {
				f.afterReadStatus = 403
			} else {
				f.writeStatus = 500
				f.applyBeforeError = mode == "lost"
			}
			created := framework.CreateResponse{State: tfsdk.State{Schema: schema.Schema}}
			r.Create(ctx, framework.CreateRequest{Plan: plan}, &created)
			if !created.Diagnostics.HasError() {
				t.Fatal("expected uncertain write")
			}
			if d := created.State.Get(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			if m.ID.ValueString() != f.prefix+",AS64496" || f.puts != 1 {
				t.Fatal("lost identity or replayed PUT")
			}
			f.readStatus = 0
			read := framework.ReadResponse{State: created.State}
			r.Read(ctx, framework.ReadRequest{State: created.State}, &read)
			if read.Diagnostics.HasError() {
				t.Fatal(read.Diagnostics)
			}
			if d := read.State.Get(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			want := "Updated"
			if mode == "not-applied" {
				want = "Initial"
			}
			if m.Description.Elements()[0].(types.String).ValueString() != want {
				t.Fatal("refresh did not recover actual outcome")
			}
			f.readStatus = 403
			failed := framework.ReadResponse{State: read.State}
			r.Read(ctx, framework.ReadRequest{State: read.State}, &failed)
			if !failed.Diagnostics.HasError() || !failed.State.Raw.Equal(read.State.Raw) {
				t.Fatal("failed read discarded metadata ownership")
			}
			deleted := framework.DeleteResponse{State: read.State}
			r.Delete(ctx, framework.DeleteRequest{State: read.State}, &deleted)
			if deleted.Diagnostics.HasError() || f.puts != 1 || f.otherWrites != 0 {
				t.Fatal("metadata deletion mutated target")
			}
		})
	}
}

func TestAccIRRRouteMetadataOwnerGraph(t *testing.T) {
	f, _ := setupBundleFake(t)
	prefix := "192.0.2.0/24"
	metadata, _ := setupMetadataRouteFake(t, prefix, "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/irr/route/") {
			f.handler(w, r)
			return
		}
		f.mu.Lock()
		link := ""
		for _, a := range f.roas {
			if a.ASN == 64496 && a.AutoLink {
				for _, p := range a.Resources {
					if p.Start == "192.0.2.0" && p.Length == 24 {
						link = a.Handle
					}
				}
			}
		}
		f.mu.Unlock()
		if link == "" {
			w.WriteHeader(404)
			return
		}
		metadata.mu.Lock()
		metadata.link = link
		metadata.mu.Unlock()
		metadata.serve(w, r)
	}))
	defer server.Close()
	t.Setenv("ARIN_BASE_URL", server.URL)
	config := func(name string, include bool) string {
		text := roaConfig("EXAMPLE-1", name, 64496, map[string]int64{prefix: 24}, true, true)
		if include {
			text += fmt.Sprintf(`
resource "arin_irr_route_metadata" "test" {
 prefix = %q
 origin_as = "AS64496"
 org_handle = "EXAMPLE-1"
 expected_roa_handle = arin_roa.test.handle
 description = ["Owned metadata"]
 remarks = ["User metadata"]
}
`, prefix)
		}
		return text
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config("First", true)},
		{Config: config("First", true), PlanOnly: true},
		{Config: config("Replacement", true), Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("arin_irr_route_metadata.test", "expected_roa_handle", "roa2"), resource.TestCheckResourceAttr("arin_irr_route_metadata.test", "auto_linked_roa_handle", "roa2"))},
		{Config: config("Replacement", true), PlanOnly: true},
		{Config: config("Replacement", false), Check: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if _, ok := f.roas["roa2"]; !ok {
				return fmt.Errorf("metadata removal deleted owning ROA")
			}
			return nil
		}},
	}, CheckDestroy: func(_ *terraform.State) error {
		metadata.mu.Lock()
		defer metadata.mu.Unlock()
		if metadata.puts != 1 || metadata.otherWrites != 0 {
			return fmt.Errorf("owner replacement replayed metadata or metadata removal deleted target")
		}
		return nil
	}})
}

func TestIRRRouteMetadataReadOwnershipAndAbsence(t *testing.T) {
	f, c := setupMetadataRouteFake(t, "192.0.2.0/24", "roa1")
	r := &irrRouteMetadataResource{client: c}
	ctx := context.Background()
	var schema framework.SchemaResponse
	r.Schema(ctx, framework.SchemaRequest{}, &schema)
	imported := framework.ImportStateResponse{State: tfsdk.State{Schema: schema.Schema}}
	r.ImportState(ctx, framework.ImportStateRequest{ID: f.prefix + ",AS64496"}, &imported)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	original := f.body
	f.body = strings.ReplaceAll(f.body, "EXAMPLE-1", "OTHER-1")
	read := framework.ReadResponse{State: imported.State}
	r.Read(ctx, framework.ReadRequest{State: imported.State}, &read)
	if !read.Diagnostics.HasError() || !read.State.Raw.Equal(imported.State.Raw) {
		t.Fatal("foreign organization silently adopted")
	}
	f.body = original
	f.link = ""
	read = framework.ReadResponse{State: imported.State}
	r.Read(ctx, framework.ReadRequest{State: imported.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	var m irrRouteMetadataModel
	if d := read.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if m.ExpectedROA.ValueString() != "roa1" || m.ActualROA.ValueString() != "" {
		t.Fatal("link drift changed configured binding or hid observed link")
	}
	f.body = ""
	missing := framework.ReadResponse{State: read.State}
	r.Read(ctx, framework.ReadRequest{State: read.State}, &missing)
	if missing.Diagnostics.HasError() || !missing.State.Raw.IsNull() {
		t.Fatal("confirmed absence retained metadata state")
	}
	if f.puts != 0 || f.otherWrites != 0 {
		t.Fatal("read or import mutated target")
	}
}
