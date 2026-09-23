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
)

func TestRouteSetReadAndDeleteErrorsPreserveState(t *testing.T) {
	for _, status := range []int{404, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			client, _ := arin.New(arin.Config{APIKey: "test-key", BaseURL: server.URL})
			r := &routeSetResource{client: client}
			ctx := context.Background()
			var schemaResponse resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
			initial := tfsdk.State{Schema: schemaResponse.Schema}
			model, d := routeSetState(ctx, &arin.RouteSet{Name: "RS-EXAMPLE", OrgHandle: "EXAMPLE-1", Description: []string{"Example"}})
			if d.HasError() {
				t.Fatal(d)
			}
			if d = initial.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			read := resource.ReadResponse{State: initial}
			r.Read(ctx, resource.ReadRequest{State: initial}, &read)
			if status == 404 {
				if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
					t.Fatal("missing object was not removed")
				}
			} else if !read.Diagnostics.HasError() || !read.State.Raw.Equal(initial.Raw) {
				t.Fatal("read error failed to preserve state")
			}
			del := resource.DeleteResponse{State: initial}
			r.Delete(ctx, resource.DeleteRequest{State: initial}, &del)
			if (status != 404) != del.Diagnostics.HasError() {
				t.Fatal("incorrect deletion status")
			}
			if status != 404 && !del.State.Raw.Equal(initial.Raw) {
				t.Fatal("delete error failed to preserve state")
			}
		})
	}
}
