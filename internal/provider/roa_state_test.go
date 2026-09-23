package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func roaStateFixture(t *testing.T) (*roaFake, *roaResource, tfsdk.State, tfsdk.Plan) {
	t.Helper()
	f, c := setupROAFake(t)
	r := &roaResource{client: c}
	ctx := context.Background()
	prefixes, d := types.MapValueFrom(ctx, types.Int64Type, map[string]int64{"192.0.2.0/24": 28})
	if d.HasError() {
		t.Fatal(d)
	}
	m := roaModel{ID: types.StringUnknown(), Handle: types.StringUnknown(), Org: types.StringValue("EXAMPLE-1"), Name: types.StringValue("Example"), ASN: types.Int64Value(64496), Prefixes: prefixes, AutoLink: types.BoolValue(false), DeleteLinked: types.BoolValue(false), Linked: types.SetUnknown(types.StringType), Before: types.StringUnknown(), After: types.StringUnknown(), Renewed: types.BoolUnknown(), Recovery: types.StringUnknown()}
	var schema resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schema)
	state := tfsdk.State{Schema: schema.Schema}
	plan := tfsdk.Plan{Schema: schema.Schema}
	if d := plan.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	return f, r, state, plan
}
func TestROAUncertainCreationRecovery(t *testing.T) {
	for _, apply := range []bool{false, true} {
		t.Run(fmt.Sprint(apply), func(t *testing.T) {
			f, r, state, plan := roaStateFixture(t)
			f.writeStatus = 500
			f.applyBeforeError = apply
			ctx := context.Background()
			created := resource.CreateResponse{State: state}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			var m roaModel
			if d := created.State.Get(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			if !created.Diagnostics.HasError() || m.Recovery.ValueString() == "" || m.ID.ValueString() == "" || f.posts != 1 {
				t.Fatal("uncertain creation lost recovery state or retried")
			}
			read := resource.ReadResponse{State: created.State}
			r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
			if apply {
				if read.Diagnostics.HasError() {
					t.Fatal(read.Diagnostics)
				}
				if d := read.State.Get(ctx, &m); d.HasError() {
					t.Fatal(d)
				}
				if m.Handle.ValueString() != "roa1" || m.Recovery.ValueString() != "" {
					t.Fatal("accepted creation not recovered")
				}
			} else {
				if !read.Diagnostics.HasError() || !read.State.Raw.Equal(created.State.Raw) {
					t.Fatal("unconfirmed absence discarded recovery state")
				}
				deleted := resource.DeleteResponse{State: created.State}
				r.Delete(ctx, resource.DeleteRequest{State: created.State}, &deleted)
				if !deleted.Diagnostics.HasError() || f.posts != 1 {
					t.Fatal("unresolved transaction allowed destroy")
				}
			}
		})
	}
}
func TestROAUncertainUpdateRecovery(t *testing.T) {
	f, r, state, plan := roaStateFixture(t)
	ctx := context.Background()
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	var m roaModel
	created.State.Get(ctx, &m)
	m.Name = types.StringValue("Updated")
	plan.Set(ctx, &m)
	f.writeStatus = 500
	f.applyBeforeError = true
	updated := resource.UpdateResponse{State: created.State}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: created.State}, &updated)
	if !updated.Diagnostics.HasError() {
		t.Fatal("lost update response accepted")
	}
	var pending roaModel
	updated.State.Get(ctx, &pending)
	if pending.Recovery.ValueString() == "" {
		t.Fatal("update lost journal")
	}
	retried := resource.UpdateResponse{State: updated.State}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: updated.State}, &retried)
	if !retried.Diagnostics.HasError() || f.posts != 2 {
		t.Fatal("pending update replayed")
	}
	read := resource.ReadResponse{State: updated.State}
	r.Read(ctx, resource.ReadRequest{State: updated.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	read.State.Get(ctx, &m)
	if m.Name.ValueString() != "Updated" || m.Handle.ValueString() != "roa2" || m.Recovery.ValueString() != "" {
		t.Fatal("replacement handle not recovered")
	}
}
func TestROAAmbiguousRecoveryPreservesState(t *testing.T) {
	f, r, state, plan := roaStateFixture(t)
	ctx := context.Background()
	f.writeStatus = 500
	f.applyBeforeError = true
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	duplicate := f.objects["roa1"]
	duplicate.Handle = "ambiguous"
	f.objects["ambiguous"] = duplicate
	read := resource.ReadResponse{State: created.State}
	r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
	if !read.Diagnostics.HasError() || !read.State.Raw.Equal(created.State.Raw) || f.posts != 1 {
		t.Fatal("ambiguous recovery adopted an arbitrary ROA")
	}
}
func TestROAExistingRequiresImport(t *testing.T) {
	f, r, state, plan := roaStateFixture(t)
	ctx := context.Background()
	var m roaModel
	plan.Get(ctx, &m)
	a, _ := m.api(ctx)
	f.objects["existing"] = testROAFromRequest("existing", a)
	resp := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() || f.posts != 0 {
		t.Fatal("existing ROA adopted")
	}
}
func TestROAReadAndDeleteFailures(t *testing.T) {
	f, r, state, plan := roaStateFixture(t)
	ctx := context.Background()
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	for _, status := range []int{403, 429, 500} {
		f.readStatus = status
		read := resource.ReadResponse{State: created.State}
		r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
		if !read.Diagnostics.HasError() || !read.State.Raw.Equal(created.State.Raw) {
			t.Fatal("read failure discarded state")
		}
	}
	f.readStatus = 0
	f.ignoreDelete = true
	deleted := resource.DeleteResponse{State: created.State}
	r.Delete(ctx, resource.DeleteRequest{State: created.State}, &deleted)
	if !deleted.Diagnostics.HasError() {
		t.Fatal("unconfirmed deletion accepted")
	}
}
func TestROAVerificationForbiddenRetainsRecovery(t *testing.T) {
	f, r, state, plan := roaStateFixture(t)
	f.afterWriteReadStatus = 403
	ctx := context.Background()
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	var m roaModel
	if d := created.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if !created.Diagnostics.HasError() || m.Recovery.ValueString() == "" {
		t.Fatal("verification 403 discarded an accepted ROA")
	}
	f.readStatus = 0
	read := resource.ReadResponse{State: created.State}
	r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
}
func TestROAPendingRecoveryFollowedByUncertainDelete(t *testing.T) {
	f, r, state, plan := roaStateFixture(t)
	f.writeStatus = 500
	f.applyBeforeError = true
	ctx := context.Background()
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if !created.Diagnostics.HasError() {
		t.Fatal("expected uncertain creation")
	}
	deleted := resource.DeleteResponse{State: created.State}
	r.Delete(ctx, resource.DeleteRequest{State: created.State}, &deleted)
	var m roaModel
	if d := deleted.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if !deleted.Diagnostics.HasError() || m.Handle.ValueString() != "roa1" || m.Recovery.ValueString() != "" {
		t.Fatal("uncertain deletion retained stale recovery journal")
	}
	read := resource.ReadResponse{State: deleted.State}
	r.Read(ctx, resource.ReadRequest{State: deleted.State}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
		t.Fatal("deleted recovered ROA did not leave state")
	}
}
