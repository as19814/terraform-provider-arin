package provider

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func orgStateFixture(t *testing.T) (*orgFake, *orgResource, tfsdk.State, tfsdk.Plan) {
	t.Helper()
	f, c := setupOrgFake(t)
	ctx := context.Background()
	r := &orgResource{client: c}
	b := true
	o := &arin.RegisteredOrganization{Handle: "ORG-TEST", Name: "Example Organization", RegistrationDate: "2026-09-22T00:00:00Z", CountryCode: "US", City: "Original", Subdivision: "VA", PostalCode: "20151", StreetAddress: []string{"123 Example Street"}, AcceptReassignments: &b, POCs: []arin.OrgPOC{{Handle: "ADMIN-ARIN", Function: "AD"}, {Handle: "TECH-ARIN", Function: "T"}, {Handle: "ABUSE-ARIN", Function: "AB"}}}
	m, d := orgState(ctx, o)
	if d.HasError() {
		t.Fatal(d)
	}
	var schema resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schema)
	state := tfsdk.State{Schema: schema.Schema}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	plan := tfsdk.Plan{Schema: schema.Schema}
	if d := plan.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	out, err := c.CreateOrganization(ctx, *o)
	if err != nil || out.Organization == nil {
		t.Fatal(err)
	}
	f.creates = 0
	return f, r, state, plan
}
func TestOrgPendingCreationNeverResubmitsOrAdoptsSharedOrg(t *testing.T) {
	f, r, state, plan := orgStateFixture(t)
	f.pendingCreate = true
	ctx := context.Background()
	var m orgModel
	state.Get(ctx, &m)
	m.ID = types.StringUnknown()
	m.Handle = types.StringUnknown()
	plan.Set(ctx, &m)
	create := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &create)
	if !create.Diagnostics.HasError() {
		t.Fatal("pending creation reported complete")
	}
	var pending orgModel
	if d := create.State.Get(ctx, &pending); d.HasError() {
		t.Fatal(d)
	}
	if pending.PendingTicket.ValueString() != "20260922-X1" || pending.PendingOperation.ValueString() != "create" || pending.Handle.ValueString() != "" {
		t.Fatal("pending identity lost")
	}
	for _, status := range []string{"PENDING_REVIEW", "RESOLVED", "CLOSED"} {
		f.ticketStatus = status
		f.ticketResolution = "ACCEPTED"
		reads := f.orgReads
		read := resource.ReadResponse{State: create.State}
		r.Read(ctx, resource.ReadRequest{State: create.State}, &read)
		if !read.Diagnostics.HasError() || !read.State.Raw.Equal(create.State.Raw) || f.orgReads != reads {
			t.Fatal("pending creation adopted shared org or lost state")
		}
		del := resource.DeleteResponse{State: create.State}
		r.Delete(ctx, resource.DeleteRequest{State: create.State}, &del)
		if !del.Diagnostics.HasError() || !del.State.Raw.Equal(create.State.Raw) || f.deletes != 0 {
			t.Fatal("destroy discarded unresolved creation")
		}
		update := resource.UpdateResponse{State: create.State}
		r.Update(ctx, resource.UpdateRequest{State: create.State, Plan: plan}, &update)
		if !update.Diagnostics.HasError() || f.updates != 0 {
			t.Fatal("update submitted during unresolved create")
		}
	}
	if f.creates != 1 {
		t.Fatal("creation repeated")
	}
}
func TestOrgPendingUpdateReconcilesWithoutResubmission(t *testing.T) {
	f, r, state, plan := orgStateFixture(t)
	f.pendingUpdate = true
	ctx := context.Background()
	var m orgModel
	plan.Get(ctx, &m)
	m.City = types.StringValue("Changed")
	plan.Set(ctx, &m)
	update := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &update)
	if !update.Diagnostics.HasError() {
		t.Fatal("pending update reported complete")
	}
	read := resource.ReadResponse{State: update.State}
	r.Read(ctx, resource.ReadRequest{State: update.State}, &read)
	if !read.Diagnostics.HasError() {
		t.Fatal("pending ticket treated as complete")
	}
	f.ticketStatus = "RESOLVED"
	f.ticketResolution = "ACCEPTED"
	read = resource.ReadResponse{State: update.State}
	r.Read(ctx, resource.ReadRequest{State: update.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	read.State.Get(ctx, &m)
	if m.PendingOperation.ValueString() != "" || m.City.ValueString() != "Changed" || f.updates != 1 {
		t.Fatal("update did not reconcile")
	}
}
func TestOrgPendingDeleteRequiresTerminalTicketAndAbsence(t *testing.T) {
	f, r, state, _ := orgStateFixture(t)
	f.pendingDelete = true
	ctx := context.Background()
	del := resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &del)
	if !del.Diagnostics.HasError() {
		t.Fatal("pending deletion reported complete")
	}
	f.body = ""
	read := resource.ReadResponse{State: del.State}
	r.Read(ctx, resource.ReadRequest{State: del.State}, &read)
	if !read.Diagnostics.HasError() || read.State.Raw.IsNull() {
		t.Fatal("pending ticket discarded because org was absent")
	}
	f.ticketStatus = "RESOLVED"
	f.ticketResolution = "PROCESSED"
	read = resource.ReadResponse{State: del.State}
	r.Read(ctx, resource.ReadRequest{State: del.State}, &read)
	if read.Diagnostics.HasError() || !read.State.Raw.IsNull() || f.deletes != 1 {
		t.Fatal("completed deletion failed to reconcile")
	}
}
func TestOrgReadErrorsKeepState(t *testing.T) {
	for _, code := range []int{403, 404, 429, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			f, r, state, _ := orgStateFixture(t)
			f.readStatus = code
			read := resource.ReadResponse{State: state}
			r.Read(context.Background(), resource.ReadRequest{State: state}, &read)
			if code == 404 {
				if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
					t.Fatal("missing organization retained")
				}
			} else if !read.Diagnostics.HasError() || !read.State.Raw.Equal(state.Raw) {
				t.Fatal("read failure discarded state")
			}
		})
	}
}
func TestOrgUncertainUpdateKeepsStateUntilDesiredValuesAppear(t *testing.T) {
	f, r, state, plan := orgStateFixture(t)
	f.writeStatus = 500
	ctx := context.Background()
	var m orgModel
	plan.Get(ctx, &m)
	m.City = types.StringValue("Changed")
	plan.Set(ctx, &m)
	update := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &update)
	if !update.Diagnostics.HasError() {
		t.Fatal("uncertain update accepted")
	}
	read := resource.ReadResponse{State: update.State}
	r.Read(ctx, resource.ReadRequest{State: update.State}, &read)
	if !read.Diagnostics.HasError() || !read.State.Raw.Equal(update.State.Raw) {
		t.Fatal("uncertain update state lost")
	}
	// A later server read confirms the requested fields and supplies role labels.
	// Response-only descriptions must not prevent recovery of the uncertain PUT.
	f.body = strings.Replace(f.body, "<city>Original</city>", "<city>Changed</city>", 1)
	recovered := resource.ReadResponse{State: update.State}
	r.Read(ctx, resource.ReadRequest{State: update.State}, &recovered)
	if recovered.Diagnostics.HasError() {
		t.Fatal(recovered.Diagnostics)
	}
	var actual orgModel
	if d := recovered.State.Get(ctx, &actual); d.HasError() {
		t.Fatal(d)
	}
	if actual.PendingOperation.ValueString() != "" || actual.City.ValueString() != "Changed" || f.updates != 0 {
		t.Fatal("confirmed update failed to recover without resubmission")
	}
}

func TestOrgUncertainCreationRetainsRecoveryIdentity(t *testing.T) {
	f, r, state, plan := orgStateFixture(t)
	f.writeStatus = 500
	ctx := context.Background()
	var m orgModel
	plan.Get(ctx, &m)
	m.ID = types.StringUnknown()
	m.Handle = types.StringUnknown()
	plan.Set(ctx, &m)
	create := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &create)
	if !create.Diagnostics.HasError() {
		t.Fatal("uncertain creation reported complete")
	}
	if d := create.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if m.ID.ValueString() == "" || m.PendingOperation.ValueString() != "create" || m.Handle.ValueString() != "" {
		t.Fatal("uncertain creation identity lost")
	}
	del := resource.DeleteResponse{State: create.State}
	r.Delete(ctx, resource.DeleteRequest{State: create.State}, &del)
	if !del.Diagnostics.HasError() || !del.State.Raw.Equal(create.State.Raw) {
		t.Fatal("uncertain creation discarded")
	}
}
func TestOrgUnconfirmedDeleteRetainsPendingState(t *testing.T) {
	f, r, state, _ := orgStateFixture(t)
	f.ignoreDelete = true
	ctx := context.Background()
	del := resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &del)
	if !del.Diagnostics.HasError() {
		t.Fatal("unconfirmed deletion accepted")
	}
	var m orgModel
	if d := del.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if m.PendingOperation.ValueString() != "delete" {
		t.Fatal("deletion recovery state missing")
	}
	retry := resource.DeleteResponse{State: del.State}
	r.Delete(ctx, resource.DeleteRequest{State: del.State}, &retry)
	if !retry.Diagnostics.HasError() || f.deletes != 1 {
		t.Fatal("unconfirmed deletion repeated")
	}
}
