package provider

import (
	"context"
	"os"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestTicketMessagePendingStateGuards(t *testing.T) {
	for _, mode := range []string{"lost", "partial", "rejected"} {
		t.Run(mode, func(t *testing.T) {
			f := setupTicketMessageFake(t)
			f.lost = mode == "lost"
			f.partial = mode == "partial"
			f.reject = mode == "rejected"
			c, err := arin.New(arin.Config{APIKey: "test-key", BaseURL: os.Getenv("ARIN_BASE_URL")})
			if err != nil {
				t.Fatal(err)
			}
			r := &ticketMessageResource{client: c}
			ctx := context.Background()
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			plan := tfsdk.Plan{Schema: schema.Schema}
			state := tfsdk.State{Schema: schema.Schema}
			m := ticketMessageModel{ID: types.StringUnknown(), Ticket: types.StringValue("20260923-X1"), Message: types.StringUnknown(), Subject: types.StringValue("test"), Category: types.StringValue("NONE"), Text: types.ListValueMust(types.StringType, []attr.Value{}), Attachments: types.MapValueMust(types.StringType, map[string]attr.Value{}), Created: types.StringUnknown(), Pending: types.BoolUnknown(), Available: types.BoolUnknown()}
			if d := plan.Set(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			created := resource.CreateResponse{State: state}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			if !created.Diagnostics.HasError() {
				t.Fatal("expected create failure")
			}
			if mode == "rejected" {
				if !created.State.Raw.IsNull() {
					t.Fatal("definite rejection retained a pending receipt")
				}
				return
			}
			if d := created.State.Get(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			if !m.Pending.ValueBool() || m.ID.ValueString() == "" {
				t.Fatal("missing recovery identity")
			}
			if mode == "partial" && m.Message.ValueString() != "1" {
				t.Fatal("returned message identity lost")
			}
			read := resource.ReadResponse{State: created.State}
			r.Read(ctx, resource.ReadRequest{State: created.State}, &read)
			if !read.Diagnostics.HasError() || !read.State.Raw.Equal(created.State.Raw) {
				t.Fatal("pending refresh did not preserve state")
			}
			deleted := resource.DeleteResponse{State: created.State}
			r.Delete(ctx, resource.DeleteRequest{State: created.State}, &deleted)
			if !deleted.Diagnostics.HasError() {
				t.Fatal("pending receipt discarded")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.submissions != 1 {
				t.Fatal("message replayed")
			}
		})
	}
}
