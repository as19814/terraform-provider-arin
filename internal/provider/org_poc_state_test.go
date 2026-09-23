package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func orgPOCStateFixture(t *testing.T) (*orgPOCFake, *orgPOCResource, tfsdk.State, tfsdk.Plan) {
	t.Helper()
	f, c := setupOrgPOCFake(t)
	r := &orgPOCResource{client: c}
	ctx := context.Background()
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	m := orgPOCModel{ID: types.StringValue("EXAMPLE-1/TEST-ARIN/N"), Org: types.StringValue("EXAMPLE-1"), POC: types.StringValue("TEST-ARIN"), Function: types.StringValue("N")}
	state := tfsdk.State{Schema: sr.Schema}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	plan := tfsdk.Plan{Schema: sr.Schema}
	if d := plan.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	return f, r, state, plan
}
func TestOrgPOCReadFailuresPreserveState(t *testing.T) {
	for _, status := range []int{403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f, r, state, _ := orgPOCStateFixture(t)
			f.readStatus = status
			ctx := context.Background()
			resp := resource.ReadResponse{State: state}
			r.Read(ctx, resource.ReadRequest{State: state}, &resp)
			if status == 404 {
				if resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
					t.Fatal("missing org did not remove state")
				}
			} else if !resp.Diagnostics.HasError() || !resp.State.Raw.Equal(state.Raw) {
				t.Fatal("failed read lost state")
			}
			del := resource.DeleteResponse{State: state}
			r.Delete(ctx, resource.DeleteRequest{State: state}, &del)
			if (status != 404) != del.Diagnostics.HasError() {
				t.Fatal("unexpected delete result")
			}
			if status != 404 && !del.State.Raw.Equal(state.Raw) {
				t.Fatal("failed delete lost state")
			}
		})
	}
}
func TestOrgPOCDeleteRequiresConfirmation(t *testing.T) {
	for _, status := range []int{0, 404, 409, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f, r, state, _ := orgPOCStateFixture(t)
			f.links = append(f.links, arin.NetPOC{Handle: "TEST-ARIN", Function: "N"})
			f.writeStatus = status
			f.ignoreDelete = true
			resp := resource.DeleteResponse{State: state}
			r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
			if !resp.Diagnostics.HasError() || !resp.State.Raw.Equal(state.Raw) {
				t.Fatal("unconfirmed deletion lost state")
			}
		})
	}
}
func TestOrgPOCCreationGuardsAndRecovery(t *testing.T) {
	f, r, state, plan := orgPOCStateFixture(t)
	ctx := context.Background()
	f.links = append(f.links, arin.NetPOC{Handle: "TEST-ARIN", Function: "N"})
	existing := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &existing)
	if !existing.Diagnostics.HasError() || f.writes != 0 {
		t.Fatal("existing link was adopted or mutated")
	}
	f.mu.Lock()
	f.links = f.links[:2]
	f.writeStatus = 500
	f.mu.Unlock()
	failed := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &failed)
	if !failed.Diagnostics.HasError() || !failed.State.Raw.Equal(state.Raw) {
		t.Fatal("uncertain create lost identity")
	}
	f.mu.Lock()
	f.links = append(f.links, arin.NetPOC{Handle: "TEST-ARIN", Function: "N"})
	f.mu.Unlock()
	read := resource.ReadResponse{State: failed.State}
	r.Read(ctx, resource.ReadRequest{State: failed.State}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.Equal(state.Raw) {
		t.Fatal("refresh did not reconcile link")
	}
}
