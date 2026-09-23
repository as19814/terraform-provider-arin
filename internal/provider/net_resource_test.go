package provider

import (
	"context"
	"encoding/xml"
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
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type fakeNetAPI struct {
	mu                                                         sync.Mutex
	objects                                                    map[string]string
	writes                                                     map[string]int
	createPending, deletePending, lostCreate, denied, resolved bool
	pendingHandle, pendingBody                                 string
	reuseHandle                                                bool
	removalPayload                                             string
	lostRemove                                                 bool
}

func (f *fakeNetAPI) ticket(w http.ResponseWriter) {
	status, resolution := "PENDING_REVIEW", ""
	if f.resolved {
		status, resolution = "RESOLVED", "PROCESSED"
	}
	if f.denied {
		status, resolution = "RESOLVED", "DENIED"
	}
	fmt.Fprintf(w, `<ticket xmlns="http://www.arin.net/regrws/core/v1"><ticketNo>20260922-X1</ticketNo><webTicketStatus>%s</webTicketStatus><webTicketResolution>%s</webTicketResolution></ticket>`, status, resolution)
}
func (f *fakeNetAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "ApiKey acceptance-test-key" {
		w.WriteHeader(403)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	handle := strings.TrimPrefix(r.URL.Path, "/rest/net/")
	if strings.HasPrefix(r.URL.Path, "/rest/ticket/") {
		f.ticket(w)
		return
	}
	switch r.Method {
	case "GET":
		if strings.HasPrefix(handle, "mostSpecificNet/") {
			parts := strings.Split(handle, "/")
			for _, body := range f.objects {
				if strings.Contains(body, "<startAddress>"+parts[1]+"</startAddress>") {
					fmt.Fprint(w, body)
					return
				}
			}
			w.WriteHeader(404)
			return
		}
		if body, ok := f.objects[handle]; ok {
			fmt.Fprint(w, body)
		} else {
			w.WriteHeader(404)
		}
	case "PUT":
		b, _ := io.ReadAll(r.Body)
		body := fakePOCDescriptions(strings.ReplaceAll(string(b), "<originAS>AS", "<originAS>"))
		var identity struct {
			Handle string `xml:"handle"`
			Date   string `xml:"registrationDate"`
		}
		if xml.Unmarshal(b, &identity) != nil {
			w.WriteHeader(400)
			return
		}
		if strings.HasSuffix(handle, "/remove") {
			handle = strings.TrimSuffix(handle, "/remove")
			if identity.Handle != handle || f.objects[handle] == "" {
				w.WriteHeader(400)
				return
			}
			f.writes["remove"]++
			f.removalPayload = string(b)
			if f.lostRemove {
				w.WriteHeader(500)
				return
			}
			fmt.Fprint(w, `<ticketedRequest xmlns="http://www.arin.net/regrws/core/v1">`)
			if f.deletePending {
				f.ticket(w)
			} else {
				fmt.Fprint(w, f.objects[handle])
				delete(f.objects, handle)
			}
			fmt.Fprint(w, "</ticketedRequest>")
			return
		}
		create := strings.HasSuffix(handle, "/reassign") || strings.HasSuffix(handle, "/reallocate")
		if create {
			f.writes["create"]++
			if identity.Handle != "" || identity.Date != "" {
				http.Error(w, "client assigned server identity", 400)
				return
			}
			handle = fmt.Sprintf("NET-192-0-2-0-%d", f.writes["create"]+1)
			if f.reuseHandle {
				handle = "NET-192-0-2-0-2"
			}
			body = strings.Replace(body, "</net>", "<handle>"+handle+"</handle><registrationDate>2026-01-01T00:00:00Z</registrationDate></net>", 1)
			if f.createPending {
				f.pendingBody = body
				f.pendingHandle = handle
				fmt.Fprint(w, `<ticketedRequest xmlns="http://www.arin.net/regrws/core/v1">`)
				f.ticket(w)
				fmt.Fprint(w, "</ticketedRequest>")
				return
			}
			f.objects[handle] = body
			if f.lostCreate {
				w.WriteHeader(500)
				return
			}
			fmt.Fprintf(w, `<ticketedRequest xmlns="http://www.arin.net/regrws/core/v1">%s</ticketedRequest>`, body)
		} else {
			f.writes["update"]++
			if identity.Handle != handle || identity.Date != "2026-01-01T00:00:00Z" {
				http.Error(w, "identity lost on update", 400)
				return
			}
			f.objects[handle] = body
			fmt.Fprint(w, body)
		}
	case "DELETE":
		f.writes["delete"]++
		fmt.Fprint(w, `<ticketedRequest xmlns="http://www.arin.net/regrws/core/v1">`)
		if f.deletePending {
			f.ticket(w)
		} else {
			fmt.Fprint(w, f.objects[handle])
			delete(f.objects, handle)
		}
		fmt.Fprint(w, "</ticketedRequest>")
	default:
		w.WriteHeader(405)
	}
}
func setupNetFake(t *testing.T) (*fakeNetAPI, *arin.Client) {
	t.Helper()
	f := &fakeNetAPI{objects: map[string]string{}, writes: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	c, err := arin.New(arin.Config{APIKey: "acceptance-test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}
func netConfig(extra string) string {
	return `provider "arin" {}
resource "arin_net" "test" {
 parent_net_handle = "NET-192-0-2-0-1"
 name = "EXAMPLE-NET"
 prefixes = ["192.0.2.0/29"]
 ` + extra + "\n}"
}
func TestAccNetResourceLifecycle(t *testing.T) {
	f, _ := setupNetFake(t)
	base := netConfig(`customer_handle = "C123"
comments = ["Operational comment"]`)
	cleared := netConfig(`customer_handle = "C123"`)
	replaced := netConfig(`org_handle = "EXAMPLE-1"
reallocate = true`)
	tfresource.Test(t, tfresource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.objects) != 0 || f.writes["create"] != 3 || f.writes["update"] != 2 || f.writes["delete"] != 2 {
				return fmt.Errorf("unexpected lifecycle: %v, remaining=%d", f.writes, len(f.objects))
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{Config: base, Check: tfresource.TestCheckResourceAttr("arin_net.test", "id", "NET-192-0-2-0-2")},
			{ResourceName: "arin_net.test", ImportState: true, ImportStateVerify: true},
			{PreConfig: func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.objects["NET-192-0-2-0-2"] = strings.Replace(f.objects["NET-192-0-2-0-2"], "EXAMPLE-NET", "EXTERNAL-NAME", 1)
			}, Config: base},
			{Config: cleared, Check: tfresource.ComposeAggregateTestCheckFunc(tfresource.TestCheckResourceAttr("arin_net.test", "comments.#", "0"))},
			{Config: cleared, PlanOnly: true, ExpectNonEmptyPlan: false},
			{PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.objects, "NET-192-0-2-0-2") }, Config: cleared},
			{Config: replaced, Check: tfresource.TestCheckResourceAttr("arin_net.test", "reallocate", "true")},
		},
	})
}
func netTestPlan(t *testing.T, r *netResource) (tfsdk.State, tfsdk.Plan) {
	t.Helper()
	ctx := context.Background()
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	m := netModel{RemovalMessages: types.ListNull(netRemovalMessageType), ID: types.StringUnknown(), Name: types.StringValue("EXAMPLE-NET"), Parent: types.StringValue("NET-192-0-2-0-1"), Customer: types.StringValue("C123"), Org: types.StringValue(""), Reallocate: types.BoolValue(false), Date: types.StringUnknown(), Version: types.Int64Unknown(), PendingOperation: types.StringUnknown(), PendingTicket: types.StringUnknown()}
	m.Prefixes, _ = types.SetValueFrom(ctx, types.StringType, []string{"192.0.2.0/29"})
	m.Comments, _ = types.ListValueFrom(ctx, types.StringType, []string{})
	state := tfsdk.State{Schema: sr.Schema}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	return state, tfsdk.Plan{Schema: sr.Schema, Raw: state.Raw}
}
func TestNetPendingCreationRecovery(t *testing.T) {
	for _, mode := range []string{"pending", "denied", "lost-response"} {
		t.Run(mode, func(t *testing.T) {
			f, c := setupNetFake(t)
			f.createPending = mode != "lost-response"
			f.lostCreate = mode == "lost-response"
			r := &netResource{client: c}
			ctx := context.Background()
			state, plan := netTestPlan(t, r)
			created := resource.CreateResponse{State: state}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			if !created.Diagnostics.HasError() {
				t.Fatal("unconfirmed creation reported success")
			}
			var m netModel
			if d := created.State.Get(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			if m.PendingOperation.ValueString() != "create" || !strings.HasPrefix(m.ID.ValueString(), "pending:") {
				t.Fatal("recovery state missing")
			}
			if mode != "lost-response" && m.PendingTicket.ValueString() != "20260922-X1" {
				t.Fatal("ticket not saved")
			}
			if mode == "pending" {
				del := resource.DeleteResponse{State: created.State}
				r.Delete(ctx, resource.DeleteRequest{State: created.State}, &del)
				if !del.Diagnostics.HasError() || !del.State.Raw.Equal(created.State.Raw) {
					t.Fatal("allowed removal of unresolved creation")
				}
			}
			f.mu.Lock()
			if mode == "denied" {
				f.denied = true
			} else if mode == "pending" {
				f.objects[f.pendingHandle] = f.pendingBody
			}
			f.mu.Unlock()
			read := resource.ReadResponse{State: created.State}
			r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
			if read.Diagnostics.HasError() {
				t.Fatal(read.Diagnostics)
			}
			if mode == "denied" {
				if !read.State.Raw.IsNull() {
					t.Fatal("failed request retained")
				}
			} else {
				if d := read.State.Get(ctx, &m); d.HasError() {
					t.Fatal(d)
				}
				if m.ID.ValueString() != "NET-192-0-2-0-2" || m.PendingOperation.ValueString() != "" {
					t.Fatal("network not recovered")
				}
			}
			if f.writes["create"] != 1 || f.writes["delete"] != 0 {
				t.Fatal("recovery repeated a mutation")
			}
		})
	}
}
func TestNetPendingDeletionRecovery(t *testing.T) {
	f, c := setupNetFake(t)
	r := &netResource{client: c}
	ctx := context.Background()
	state, plan := netTestPlan(t, r)
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	f.deletePending = true
	del := resource.DeleteResponse{State: created.State}
	r.Delete(ctx, resource.DeleteRequest{State: created.State}, &del)
	if !del.Diagnostics.HasError() {
		t.Fatal("pending deletion reported success")
	}
	var m netModel
	if d := del.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if m.PendingOperation.ValueString() != "delete" || m.PendingTicket.ValueString() != "20260922-X1" {
		t.Fatal("deletion ticket lost")
	}
	again := resource.DeleteResponse{State: del.State}
	r.Delete(ctx, resource.DeleteRequest{State: del.State}, &again)
	if !again.Diagnostics.HasError() || f.writes["delete"] != 1 {
		t.Fatal("pending deletion resubmitted")
	}
	f.mu.Lock()
	delete(f.objects, m.ID.ValueString())
	f.mu.Unlock()
	read := resource.ReadResponse{State: again.State}
	r.Read(ctx, resource.ReadRequest{State: again.State}, &read)
	if !read.Diagnostics.HasError() || read.State.Raw.IsNull() {
		t.Fatal("open deletion ticket discarded")
	}
	f.mu.Lock()
	f.resolved = true
	f.mu.Unlock()
	read = resource.ReadResponse{State: again.State}
	r.Read(ctx, resource.ReadRequest{State: again.State}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
		t.Fatal("confirmed deletion not removed")
	}
}

// Exercise persistence through Terraform itself, not just framework response
// structs. Terraform taints failed creates and replaces the recovered object.
func TestAccNetPendingCreationState(t *testing.T) {
	f, _ := setupNetFake(t)
	f.createPending = true
	config := netConfig(`customer_handle = "C123"`)
	tfresource.Test(t, tfresource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.objects) != 0 || f.writes["create"] != 2 || f.writes["delete"] != 2 {
				return fmt.Errorf("unexpected recovery lifecycle: %v remaining=%d", f.writes, len(f.objects))
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{Config: config, ExpectError: regexp.MustCompile("Network creation requires reconciliation")},
			{PreConfig: func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.objects[f.pendingHandle] = f.pendingBody
				f.createPending = false
			}, Config: config},
		},
	})
}
func TestNetReadErrorsPreserveState(t *testing.T) {
	for _, status := range []int{403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f, c := setupNetFake(t)
			r := &netResource{client: c}
			ctx := context.Background()
			state, plan := netTestPlan(t, r)
			created := resource.CreateResponse{State: state}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			if created.Diagnostics.HasError() {
				t.Fatal(created.Diagnostics)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			r.client, _ = arin.New(arin.Config{BaseURL: server.URL, APIKey: "acceptance-test-key"})
			read := resource.ReadResponse{State: created.State}
			r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
			if !read.Diagnostics.HasError() || !read.State.Raw.Equal(created.State.Raw) {
				t.Fatal("read error lost state")
			}
			del := resource.DeleteResponse{State: created.State}
			r.Delete(ctx, resource.DeleteRequest{State: created.State}, &del)
			if !del.Diagnostics.HasError() || !del.State.Raw.Equal(created.State.Raw) {
				t.Fatal("delete preflight error lost state")
			}
			if f.writes["delete"] != 0 {
				t.Fatal("unexpected mutation")
			}
		})
	}
}
func TestNetRejectsDirectAllocationState(t *testing.T) {
	n := &arin.RegisteredNet{Handle: "NET-192-0-2-0-1", Blocks: []arin.RegisteredNetBlock{{Type: "DA", StartAddress: "192.0.2.0", EndAddress: "192.0.2.255", CIDRLength: 24}}}
	if _, d := netState(context.Background(), n); !d.HasError() {
		t.Fatal("accepted direct allocation")
	}
}

func TestAccNetPendingDeletionState(t *testing.T) {
	f, _ := setupNetFake(t)
	config := netConfig(`customer_handle = "C123"`)
	tfresource.Test(t, tfresource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.objects) != 0 || f.writes["create"] != 1 || f.writes["delete"] != 1 {
				return fmt.Errorf("unexpected deletion recovery: %v remaining=%d", f.writes, len(f.objects))
			}
			return nil
		},
		Steps: []tfresource.TestStep{
			{Config: config},
			{PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.deletePending = true }, Config: config, Destroy: true, ExpectError: regexp.MustCompile("Network deletion requires reconciliation")},
			{PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.objects, "NET-192-0-2-0-2"); f.resolved = true }, Config: config, Destroy: true},
		},
	})
}
