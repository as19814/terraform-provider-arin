package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func aspaStateFixture(t *testing.T) (*aspaFake, *aspaResource, tfsdk.State, tfsdk.Plan) {
	t.Helper()
	f, c := setupASPAFake(t)
	r := &aspaResource{client: c}
	ctx := context.Background()
	p, d := types.SetValueFrom(ctx, types.Int64Type, []int64{64497})
	if d.HasError() {
		t.Fatal(d)
	}
	m := aspaModel{ID: types.StringValue("EXAMPLE-1/64496"), Org: types.StringValue("EXAMPLE-1"), Customer: types.Int64Value(64496), Providers: p}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
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
func TestASPACreateRequiresImport(t *testing.T) {
	f, r, state, plan := aspaStateFixture(t)
	f.objects[64496] = []int64{64497}
	response := resource.CreateResponse{State: state}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() || f.posts != 0 {
		t.Fatal("existing ASPA adopted without import")
	}
}
func TestASPAUncertainCreationRetainsIdentity(t *testing.T) {
	f, r, state, plan := aspaStateFixture(t)
	f.writeStatus = 500
	f.applyBeforeError = true
	ctx := context.Background()
	response := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() {
		t.Fatal("uncertain creation accepted")
	}
	var m aspaModel
	if d := response.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if m.ID.ValueString() != "EXAMPLE-1/64496" || f.posts != 1 {
		t.Fatal("identity lost or transaction retried")
	}
	read := resource.ReadResponse{State: response.State}
	r.Read(ctx, resource.ReadRequest{State: response.State}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.Equal(state.Raw) {
		t.Fatal("uncertain creation did not recover")
	}
}
func TestASPAReadErrorsPreserveState(t *testing.T) {
	for _, code := range []int{403, 404, 429, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			f, r, state, _ := aspaStateFixture(t)
			f.readStatus = code
			resp := resource.ReadResponse{State: state}
			r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
			if code == 404 {
				if resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
					t.Fatal("missing ASPA retained")
				}
			} else if !resp.Diagnostics.HasError() || !resp.State.Raw.Equal(state.Raw) {
				t.Fatal("read failure discarded state")
			}
		})
	}
}
func TestASPAUnconfirmedDeletionPreservesState(t *testing.T) {
	f, r, state, _ := aspaStateFixture(t)
	f.objects[64496] = []int64{64497}
	f.ignoreDelete = true
	resp := resource.DeleteResponse{State: state}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() || !resp.State.Raw.Equal(state.Raw) {
		t.Fatal("unconfirmed deletion lost state")
	}
}
