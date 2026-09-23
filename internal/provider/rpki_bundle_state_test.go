package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func bundleStateFixture(t *testing.T) (*bundleFake, *rpkiBundleResource, tfsdk.State, tfsdk.Plan) {
	t.Helper()
	f, c := setupBundleFake(t)
	r := &rpkiBundleResource{client: c}
	ctx := context.Background()
	max := int64(24)
	m := rpkiBundleModel{Org: types.StringValue("EXAMPLE-1"), Name: types.StringValue("test")}
	d := m.set(ctx, &arin.RPKIBundleState{ROAs: map[string]arin.ROA{"v4": {Handle: "planned", Name: "First", ASN: 64496, Resources: []arin.ROAResource{{Prefix: "192.0.2.0/24", MaxLength: &max}}}}, ASPAs: map[int64]arin.ASPA{64496: {CustomerASN: 64496, ProviderASNs: []int64{64510}}}}, nil)
	if d.HasError() {
		t.Fatal(d)
	}
	var schema resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schema)
	state := tfsdk.State{Schema: schema.Schema}
	plan := tfsdk.Plan{Schema: schema.Schema}
	if d := plan.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	return f, r, state, plan
}
func TestBundleUncertainCreation(t *testing.T) {
	for _, mode := range []string{"lost", "not-applied", "malformed", "partial", "verification", "ambiguous", "returned-identity"} {
		t.Run(mode, func(t *testing.T) {
			f, r, state, plan := bundleStateFixture(t)
			ctx := context.Background()
			switch mode {
			case "lost", "ambiguous":
				f.writeStatus = 500
				f.applyBeforeError = true
			case "not-applied":
				f.writeStatus = 500
			case "malformed":
				f.malformed = true
			case "partial":
				f.partial = true
			case "verification", "returned-identity":
				f.afterWriteReadStatus = 403
			}
			created := resource.CreateResponse{State: state}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			var m rpkiBundleModel
			if d := created.State.Get(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			if !created.Diagnostics.HasError() || m.Recovery.ValueString() == "" || m.ID.ValueString() != "EXAMPLE-1/test" || f.posts != 1 {
				t.Fatal("uncertain creation lost ownership or replayed")
			}
			updated := resource.UpdateResponse{State: created.State}
			r.Update(ctx, resource.UpdateRequest{Plan: plan, State: created.State}, &updated)
			deleted := resource.DeleteResponse{State: created.State}
			r.Delete(ctx, resource.DeleteRequest{State: created.State}, &deleted)
			if !updated.Diagnostics.HasError() || !deleted.Diagnostics.HasError() || f.posts != 1 {
				t.Fatal("pending transaction allowed another write")
			}
			f.readStatus = 0
			if mode == "ambiguous" || mode == "returned-identity" {
				a := f.roas["roa1"]
				a.Handle = "other"
				f.roas[a.Handle] = a
				if mode == "returned-identity" {
					delete(f.roas, "roa1")
				}
			}
			read := resource.ReadResponse{State: created.State}
			r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
			if mode == "not-applied" || mode == "ambiguous" || mode == "returned-identity" {
				if !read.Diagnostics.HasError() || !read.State.Raw.Equal(created.State.Raw) {
					t.Fatal("unconfirmed recovery discarded state")
				}
			} else {
				if read.Diagnostics.HasError() {
					t.Fatal(read.Diagnostics)
				}
				if d := read.State.Get(ctx, &m); d.HasError() {
					t.Fatal(d)
				}
				if m.Recovery.ValueString() != "" || len(m.ROAs.Elements()) != 1 || len(m.ASPAs.Elements()) != 1 {
					t.Fatal("complete transaction failed recovery")
				}
			}
			if mode == "verification" || mode == "returned-identity" {
				var journal bundleRecovery
				var pending rpkiBundleModel
				created.State.Get(ctx, &pending)
				if json.Unmarshal([]byte(pending.Recovery.ValueString()), &journal) != nil || journal.Returned == nil || len(journal.Returned.ROAs) != 1 {
					t.Fatal("parsed identities lost")
				}
			}
			if f.posts != 1 {
				t.Fatal("read replayed a transaction")
			}
		})
	}
}
func TestBundleUncertainUpdateAndDelete(t *testing.T) {
	f, r, state, plan := bundleStateFixture(t)
	ctx := context.Background()
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	var m rpkiBundleModel
	created.State.Get(ctx, &m)
	entries := m.ROAs.Elements()
	v := entries["v4"].(types.Object).Attributes()
	v["name"] = types.StringValue("Updated")
	entries["v4"] = types.ObjectValueMust(bundleROAType().AttrTypes, v)
	m.ROAs = types.MapValueMust(bundleROAType(), entries)
	m.ASPAs = types.MapValueMust(types.SetType{ElemType: types.Int64Type}, map[string]attr.Value{"64496": types.SetValueMust(types.Int64Type, []attr.Value{types.Int64Value(64511)})})
	plan.Set(ctx, &m)
	f.writeStatus = 500
	f.applyBeforeError = true
	updated := resource.UpdateResponse{State: created.State}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: created.State}, &updated)
	if !updated.Diagnostics.HasError() {
		t.Fatal("expected lost response")
	}
	read := resource.ReadResponse{State: updated.State}
	r.Read(ctx, resource.ReadRequest{State: updated.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	if d := read.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if m.ROAs.Elements()["v4"].(types.Object).Attributes()["handle"].(types.String).ValueString() != "roa2" || m.Recovery.ValueString() != "" {
		t.Fatal("replacement failed recovery")
	}
	deleted := resource.DeleteResponse{State: read.State}
	r.Delete(ctx, resource.DeleteRequest{State: read.State}, &deleted)
	if !deleted.Diagnostics.HasError() {
		t.Fatal("expected uncertain deletion")
	}
	retry := resource.DeleteResponse{State: deleted.State}
	r.Delete(ctx, resource.DeleteRequest{State: deleted.State}, &retry)
	if !retry.Diagnostics.HasError() || f.posts != 3 {
		t.Fatal("destroy replayed")
	}
	final := resource.ReadResponse{State: deleted.State}
	r.Read(ctx, resource.ReadRequest{State: deleted.State}, &final)
	if final.Diagnostics.HasError() || !final.State.Raw.IsNull() {
		t.Fatal("confirmed deletion retained bundle")
	}
	if len(f.roas) != 1 || len(f.aspas) != 1 || f.posts != 3 {
		t.Fatal("recovery affected siblings or replayed")
	}
}
func TestBundleReadFailureAndRejectedWrite(t *testing.T) {
	f, r, state, plan := bundleStateFixture(t)
	ctx := context.Background()
	f.writeStatus = 400
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if !created.Diagnostics.HasError() || f.posts != 1 || !created.State.Raw.IsNull() {
		t.Fatal("definitive rejection adopted a bundle")
	}
	f.writeStatus = 0
	created = resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	for _, status := range []int{403, 404, 429, 500} {
		f.readStatus = status
		read := resource.ReadResponse{State: created.State}
		r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
		if !read.Diagnostics.HasError() || !read.State.Raw.Equal(created.State.Raw) {
			t.Fatalf("read %d discarded ownership", status)
		}
	}
	f.readStatus = 0
	f.ignoreDelete = true
	deleted := resource.DeleteResponse{State: created.State}
	r.Delete(ctx, resource.DeleteRequest{State: created.State}, &deleted)
	if !deleted.Diagnostics.HasError() {
		t.Fatal("surviving members accepted as deleted")
	}
	read := resource.ReadResponse{State: deleted.State}
	r.Read(ctx, resource.ReadRequest{State: deleted.State}, &read)
	if !read.Diagnostics.HasError() || !read.State.Raw.Equal(deleted.State.Raw) {
		t.Fatal("incomplete delete lost ownership")
	}
}
func TestBundleImportValidation(t *testing.T) {
	for _, id := range []string{`{"org_handle":"EXAMPLE-1","name":"test","roas":{"a":"sibling","a":"missing"}}`, `EXAMPLE-1/test`, `{}`, `{"org_handle":"EXAMPLE-1","name":"test","roas":{"a":"missing"}}`, `{"org_handle":"EXAMPLE-1","name":"test","roas":{"a":"sibling","b":"sibling"}}`, `{"org_handle":"EXAMPLE-1","name":"test","aspas":[64500,64500]}`, `{"org_handle":"EXAMPLE-1","name":"test","aspas":[64500],"extra":true}`, `{"org_handle":"EXAMPLE-1","name":"test","aspas":[64500]} {}`} {
		t.Run(id, func(t *testing.T) {
			f, r, state, _ := bundleStateFixture(t)
			resp := resource.ImportStateResponse{State: state}
			r.ImportState(context.Background(), resource.ImportStateRequest{ID: id}, &resp)
			if !resp.Diagnostics.HasError() || f.posts != 0 {
				t.Fatal("invalid import accepted or wrote")
			}
		})
	}
}

func TestBundleExistingMembersRequireImport(t *testing.T) {
	for _, kind := range []string{"roa", "aspa"} {
		t.Run(kind, func(t *testing.T) {
			f, r, state, plan := bundleStateFixture(t)
			ctx := context.Background()
			var m rpkiBundleModel
			if d := plan.Get(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			desired, d := m.desired(ctx)
			if d.HasError() {
				t.Fatal(d)
			}
			if kind == "roa" {
				f.roas["existing"] = testROAFromRequest("existing", desired.ROAs["v4"].Request)
			} else {
				f.aspas[64496] = testASPAXML{Customer: 64496, Providers: desired.ASPAs[64496].ProviderASNs}
			}
			created := resource.CreateResponse{State: state}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			if !created.Diagnostics.HasError() || f.posts != 0 || !created.State.Raw.IsNull() {
				t.Fatal("existing member silently adopted")
			}
		})
	}
}
