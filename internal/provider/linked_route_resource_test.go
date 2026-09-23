package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	framework "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func linkedRouteConfig(prefix, remarks string) string {
	return strings.Replace(metadataRouteConfig(prefix, "roa1", remarks), "arin_irr_route_metadata", "arin_irr_linked_route", 1)
}
func TestAccIRRLinkedRouteLifecycle(t *testing.T) {
	for _, prefix := range []string{"192.0.2.0/24", "2001:db8::/48"} {
		t.Run(prefix, func(t *testing.T) {
			f, _ := setupMetadataRouteFake(t, prefix, "roa1")
			f.allowDelete = true
			first, next := linkedRouteConfig(prefix, `["User remark"]`), linkedRouteConfig(prefix, `[]`)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
				{Config: first, ResourceName: "arin_irr_linked_route.test", ImportState: true, ImportStateId: prefix + ",AS64496", ImportStatePersist: true},
				{Config: first},
				{ResourceName: "arin_irr_linked_route.test", ImportState: true, ImportStateVerify: true},
				{Config: first, PlanOnly: true},
				{Config: strings.Replace(first, `org_handle = "EXAMPLE-1"`, `org_handle = "OTHER-1"`, 1), PlanOnly: true, ExpectError: regexp.MustCompile("Cannot replace imported linked route")},
				{Config: next},
				{Config: next, PlanOnly: true},
			}, CheckDestroy: func(_ *terraform.State) error {
				f.mu.Lock()
				defer f.mu.Unlock()
				if f.body != "" || f.deletes != 1 || f.puts != 2 || f.otherWrites != 0 {
					return fmt.Errorf("linked lifecycle missed deletion or replayed writes")
				}
				return nil
			}})
		})
	}
}
func TestAccIRRLinkedRouteRequiresImport(t *testing.T) {
	f, _ := setupMetadataRouteFake(t, "192.0.2.0/24", "roa1")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: linkedRouteConfig(f.prefix, `[]`), ExpectError: regexp.MustCompile("Linked route requires import")}}})
	if f.puts+f.deletes+f.otherWrites != 0 {
		t.Fatal("create silently adopted a route")
	}
}
func TestIRRLinkedRouteDeletionGuards(t *testing.T) {
	for _, mode := range []string{"lost-response", "rejected", "surviving", "wrong-link", "unlinked", "wrong-org", "read-error", "missing"} {
		t.Run(mode, func(t *testing.T) {
			f, c := setupMetadataRouteFake(t, "192.0.2.0/24", "roa1")
			f.allowDelete = true
			r := &irrLinkedRouteResource{irrRouteMetadataResource: irrRouteMetadataResource{client: c}}
			ctx := context.Background()
			var schema framework.SchemaResponse
			r.Schema(ctx, framework.SchemaRequest{}, &schema)
			imported := framework.ImportStateResponse{State: tfsdk.State{Schema: schema.Schema}}
			r.ImportState(ctx, framework.ImportStateRequest{ID: f.prefix + ",AS64496"}, &imported)
			if imported.Diagnostics.HasError() {
				t.Fatal(imported.Diagnostics)
			}
			wantWrites := 1
			switch mode {
			case "lost-response":
				f.deleteStatus = 500
				f.deleteBeforeError = true
			case "rejected":
				f.deleteStatus = 403
			case "surviving":
				f.ignoreDelete = true
			case "wrong-link":
				f.link = "different"
				wantWrites = 0
			case "unlinked":
				f.link = ""
				wantWrites = 0
			case "wrong-org":
				f.body = strings.ReplaceAll(f.body, "EXAMPLE-1", "OTHER-1")
				wantWrites = 0
			case "read-error":
				f.readStatus = 403
				wantWrites = 0
			case "missing":
				f.body = ""
				wantWrites = 0
			}
			deleted := framework.DeleteResponse{State: imported.State}
			r.Delete(ctx, framework.DeleteRequest{State: imported.State}, &deleted)
			if deleted.Diagnostics.HasError() != (mode != "missing") || f.deletes != wantWrites || !deleted.State.Raw.Equal(imported.State.Raw) {
				t.Fatal("deletion guard lost ownership or replayed")
			}
			if mode == "lost-response" {
				read := framework.ReadResponse{State: deleted.State}
				r.Read(ctx, framework.ReadRequest{State: deleted.State}, &read)
				if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
					t.Fatal("lost-response deletion did not reconcile absence")
				}
			}
		})
	}
}
func TestIRRLinkedRouteImportRejectsUnlinked(t *testing.T) {
	f, c := setupMetadataRouteFake(t, "192.0.2.0/24", "")
	r := &irrLinkedRouteResource{irrRouteMetadataResource: irrRouteMetadataResource{client: c}}
	ctx := context.Background()
	var schema framework.SchemaResponse
	r.Schema(ctx, framework.SchemaRequest{}, &schema)
	imported := framework.ImportStateResponse{State: tfsdk.State{Schema: schema.Schema}}
	r.ImportState(ctx, framework.ImportStateRequest{ID: f.prefix + ",AS64496"}, &imported)
	if !imported.Diagnostics.HasError() || !imported.State.Raw.IsNull() {
		t.Fatal("unlinked route adopted as linked")
	}
}
