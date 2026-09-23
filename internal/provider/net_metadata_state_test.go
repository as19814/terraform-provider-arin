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

func TestNetMetadataReadErrorsAndForget(t *testing.T) {
	for _, status := range []int{404, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(status) }))
			defer server.Close()
			client, _ := arin.New(arin.Config{APIKey: "test-key", BaseURL: server.URL})
			r := &netMetadataResource{client: client}
			ctx := context.Background()
			var sr resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &sr)
			initial := tfsdk.State{Schema: sr.Schema}
			m, d := netMetadataState(ctx, &arin.RegisteredNet{Handle: "NET-192-0-2-0-1", Name: "EXAMPLE"})
			if d.HasError() {
				t.Fatal(d)
			}
			if d = initial.Set(ctx, &m); d.HasError() {
				t.Fatal(d)
			}
			read := resource.ReadResponse{State: initial}
			r.Read(ctx, resource.ReadRequest{State: initial}, &read)
			if status == 404 {
				if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
					t.Fatal("missing NET not removed")
				}
			} else if !read.Diagnostics.HasError() || !read.State.Raw.Equal(initial.Raw) {
				t.Fatal("read error lost metadata state")
			}
			del := resource.DeleteResponse{State: initial}
			r.Delete(ctx, resource.DeleteRequest{State: initial}, &del)
			if del.Diagnostics.HasError() || calls != 1 {
				t.Fatal("forgetting metadata made an API request")
			}
		})
	}
}
