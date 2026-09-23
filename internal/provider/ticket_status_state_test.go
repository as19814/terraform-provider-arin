package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func ticketStatusFixture(t *testing.T) (*ticketStatusFake, *ticketStatusResource, tfsdk.State, tfsdk.Plan) {
	t.Helper()
	f, c := setupTicketStatusFake(t)
	r := &ticketStatusResource{client: c}
	var sr resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &sr)
	state := tfsdk.State{Schema: sr.Schema}
	plan := tfsdk.Plan{Schema: sr.Schema}
	m := ticketStatusModel{ID: types.StringUnknown(), Ticket: types.StringValue("20260923-X1"), Status: types.StringValue("CLOSED"), Type: types.StringUnknown(), Resolution: types.StringUnknown(), Closed: types.StringUnknown(), Available: types.BoolUnknown()}
	if d := plan.Set(context.Background(), &m); d.HasError() {
		t.Fatal(d)
	}
	return f, r, state, plan
}
func TestTicketStatusOpenTicketNotChanged(t *testing.T) {
	f, r, state, plan := ticketStatusFixture(t)
	ticket := f.tickets["20260923-X1"]
	ticket.Status = "PENDING_REVIEW"
	f.tickets[ticket.Number] = ticket
	response := resource.CreateResponse{State: state}
	r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &response)
	if !response.Diagnostics.HasError() || !response.State.Raw.IsNull() || f.puts != 0 {
		t.Fatal("unresolved ticket mutated or adopted on create")
	}
}
func TestTicketStatusReadErrors(t *testing.T) {
	for _, status := range []int{403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f, r, state, plan := ticketStatusFixture(t)
			ctx := context.Background()
			created := resource.CreateResponse{State: state}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			if created.Diagnostics.HasError() {
				t.Fatal(created.Diagnostics)
			}
			f.readStatus = status
			read := resource.ReadResponse{State: created.State}
			r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
			if !read.Diagnostics.HasError() || !read.State.Raw.Equal(created.State.Raw) {
				t.Fatal("read error discarded ticket state")
			}
		})
	}
}
func TestTicketStatusUnconfirmedWriteRetainsIdentity(t *testing.T) {
	f, r, state, plan := ticketStatusFixture(t)
	f.ignoreClose = true
	ctx := context.Background()
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	var m ticketStatusModel
	if d := created.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if !created.Diagnostics.HasError() || m.ID.ValueString() != "20260923-X1" || m.Status.ValueString() != "RESOLVED" || f.puts != 1 {
		t.Fatal("unconfirmed write lost natural identity or was accepted")
	}
}
