package provider

import (
	"context"
	"fmt"
	"github.com/as19814/terraform-provider-arin/internal/arin"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func reportStateFixture(t *testing.T) (*reportFake, *reportResource, tfsdk.State, tfsdk.Plan) {
	t.Helper()
	f, c := setupReportFake(t)
	r := &reportResource{client: c}
	ctx := context.Background()
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	state := tfsdk.State{Schema: sr.Schema}
	plan := tfsdk.Plan{Schema: sr.Schema}
	m := reportModel{ID: types.StringUnknown(), Type: types.StringValue("associations"), Target: types.StringValue(""), Ticket: types.StringUnknown(), TicketType: types.StringUnknown(), Status: types.StringUnknown(), Resolution: types.StringUnknown(), Created: types.StringUnknown(), Updated: types.StringUnknown(), Resolved: types.StringUnknown(), Closed: types.StringUnknown(), Available: types.BoolUnknown(), Pending: types.BoolUnknown()}
	if d := plan.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	return f, r, state, plan
}
func TestReportUnknownSubmissionBlocksReadAndDelete(t *testing.T) {
	f, r, state, plan := reportStateFixture(t)
	f.writeStatus = 500
	f.applyBeforeError = true
	ctx := context.Background()
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	var m reportModel
	if d := created.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if !created.Diagnostics.HasError() || !m.Pending.ValueBool() || m.ID.ValueString() == "" || f.submissions != 1 {
		t.Fatal("uncertain submission lost receipt or retried")
	}
	read := resource.ReadResponse{State: created.State}
	r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
	if !read.Diagnostics.HasError() || !read.State.Raw.Equal(created.State.Raw) {
		t.Fatal("unknown ticket silently discarded")
	}
	deleted := resource.DeleteResponse{State: created.State}
	r.Delete(ctx, resource.DeleteRequest{State: created.State}, &deleted)
	if !deleted.Diagnostics.HasError() || f.submissions != 1 {
		t.Fatal("unknown submission forgotten or retried")
	}
}
func TestReportKnownPartialIdentityRecovers(t *testing.T) {
	f, r, state, plan := reportStateFixture(t)
	f.partialResponse = true
	ctx := context.Background()
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	var m reportModel
	created.State.Get(ctx, &m)
	if m.Pending.ValueBool() || m.Ticket.ValueString() != "20260923-X1" || f.submissions != 1 {
		t.Fatal("accepted report was not confirmed before tainting state")
	}
}
func TestReportPendingKnownIdentityStillRequiresImport(t *testing.T) {
	f, r, state, plan := reportStateFixture(t)
	f.partialResponse = true
	f.readStatus = 500
	ctx := context.Background()
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if !created.Diagnostics.HasError() {
		t.Fatal("unconfirmed report accepted")
	}
	f.readStatus = 0
	read := resource.ReadResponse{State: created.State}
	r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
	var m reportModel
	read.State.Get(ctx, &m)
	if !read.Diagnostics.HasError() || !m.Pending.ValueBool() || m.Ticket.ValueString() != "20260923-X1" {
		t.Fatal("refresh allowed a taint replacement to resubmit the report")
	}
	deleted := resource.DeleteResponse{State: read.State}
	r.Delete(ctx, resource.DeleteRequest{State: read.State}, &deleted)
	if !deleted.Diagnostics.HasError() || f.submissions != 1 {
		t.Fatal("pending known receipt forgotten")
	}
}
func TestReportReadErrorsAndExpiry(t *testing.T) {
	for _, status := range []int{403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f, r, state, plan := reportStateFixture(t)
			ctx := context.Background()
			created := resource.CreateResponse{State: state}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			if created.Diagnostics.HasError() {
				t.Fatal(created.Diagnostics)
			}
			f.readStatus = status
			read := resource.ReadResponse{State: created.State}
			r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
			if status == 404 {
				var m reportModel
				read.State.Get(ctx, &m)
				if read.Diagnostics.HasError() || read.State.Raw.IsNull() || m.Available.ValueBool() || m.Ticket.ValueString() != "20260923-X1" {
					t.Fatal("expired report lost its receipt")
				}
			} else if !read.Diagnostics.HasError() || !read.State.Raw.Equal(created.State.Raw) {
				t.Fatal("read error discarded receipt")
			}
			if f.submissions != 1 {
				t.Fatal("read resubmitted report")
			}
		})
	}
}

func TestReportMissingKeyDoesNotLeavePendingState(t *testing.T) {
	_, r, state, plan := reportStateFixture(t)
	client, err := arin.New(arin.Config{BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	r.client = client
	created := resource.CreateResponse{State: state}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &created)
	if !created.Diagnostics.HasError() || !created.State.Raw.IsNull() {
		t.Fatal("missing credentials left an uncertain report receipt")
	}
}
