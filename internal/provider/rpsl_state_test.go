package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRPSLStateGuards(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		pending bool
		org     string
	}{
		{"missing", 404, false, ""}, {"forbidden", 403, false, ""}, {"limited", 429, false, ""}, {"server", 500, false, ""}, {"pending_missing", 404, true, ""}, {"pending_found", 200, true, "EXAMPLE-1"}, {"ownership", 200, false, "OTHER"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
				}
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					fmt.Fprintf(w, "as-set: AS-EXAMPLE\nmnt-by: MNT-%s\nsource: ARIN\n", tc.org)
				}
			}))
			defer server.Close()
			client, _ := arin.New(arin.Config{APIKey: "test-key", BaseURL: server.URL})
			r := &rpslResource{client: client}
			ctx := context.Background()
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			state := tfsdk.State{Schema: schema.Schema}
			model := rpslModel{ID: types.StringValue("as-set/AS-EXAMPLE"), Kind: types.StringValue("as-set"), Name: types.StringValue("AS-EXAMPLE"), Origin: types.StringValue(""), Org: types.StringValue("EXAMPLE-1"), Text: types.StringValue("as-set: AS-EXAMPLE\nmnt-by: MNT-EXAMPLE-1\nsource: ARIN\n"), Remote: types.StringNull(), Pending: types.BoolValue(tc.pending)}
			if d := state.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			response := resource.ReadResponse{State: state}
			r.Read(ctx, resource.ReadRequest{State: state}, &response)
			if tc.name == "missing" {
				if response.Diagnostics.HasError() || !response.State.Raw.IsNull() {
					t.Fatal("confirmed absence not removed")
				}
				return
			}
			if !response.Diagnostics.HasError() || response.State.Raw.IsNull() {
				t.Fatal("unsafe refresh discarded identity or accepted drift")
			}
			if tc.name != "pending_found" && !response.State.Raw.Equal(state.Raw) {
				t.Fatal("failed read changed state")
			}
			if tc.pending {
				del := resource.DeleteResponse{State: state}
				r.Delete(ctx, resource.DeleteRequest{State: state}, &del)
				if !del.Diagnostics.HasError() || !del.State.Raw.Equal(state.Raw) || writes != 0 {
					t.Fatal("pending creation was deleted")
				}
			}
		})
	}
}
