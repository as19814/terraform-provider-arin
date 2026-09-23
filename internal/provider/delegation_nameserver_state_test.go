package provider

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestDelegationNameserverReadAndDeleteErrorsPreserveState(t *testing.T) {
	for _, status := range []int{404, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			client, _ := arin.New(arin.Config{APIKey: "test-key", BaseURL: server.URL})
			r := &delegationNameserverResource{client: client}
			ctx := context.Background()
			var schemaResponse resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
			initial := tfsdk.State{Schema: schemaResponse.Schema}
			model := nameserverState(delegationNameserverModel{Delegation: types.StringValue("2.0.192.in-addr.arpa."), Name: types.StringValue("ns.example.net")}, &arin.DelegationNameserver{Name: "ns.example.net"})
			if d := initial.Set(ctx, &model); d.HasError() {
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

func TestDelegationNameserverUnconfirmedDeletePreservesState(t *testing.T) {
	for _, status := range []int{200, 404, 409, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Method == "DELETE" && status != 200 {
					w.WriteHeader(status)
					return
				}
				_ = xml.NewEncoder(w).Encode(fakeDelegation{Name: "2.0.192.in-addr.arpa.", NS: []arin.DelegationNameserver{{Name: "ns.example.net"}}})
			}))
			defer server.Close()
			ctx := context.Background()
			client, err := arin.New(arin.Config{APIKey: "test-key", BaseURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			r := &delegationNameserverResource{client: client}
			var sr resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &sr)
			initial := tfsdk.State{Schema: sr.Schema}
			model := nameserverState(delegationNameserverModel{Delegation: types.StringValue("2.0.192.in-addr.arpa."), Name: types.StringValue("ns.example.net")}, &arin.DelegationNameserver{Name: "ns.example.net"})
			if d := initial.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.DeleteResponse{State: initial}
			r.Delete(ctx, resource.DeleteRequest{State: initial}, &resp)
			if !resp.Diagnostics.HasError() || !resp.State.Raw.Equal(initial.Raw) {
				t.Fatal("unconfirmed removal lost state")
			}
		})
	}
}

func TestDelegationNameserverLostCreateResponseRetainsIdentity(t *testing.T) {
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "POST" {
			created = true
			w.WriteHeader(500)
			return
		}
		d := fakeDelegation{Name: "2.0.192.in-addr.arpa."}
		if created {
			d.NS = []arin.DelegationNameserver{{Name: "ns.example.net"}}
		}
		_ = xml.NewEncoder(w).Encode(d)
	}))
	defer server.Close()
	ctx := context.Background()
	client, err := arin.New(arin.Config{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	r := &delegationNameserverResource{client: client}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	m := delegationNameserverModel{ID: types.StringUnknown(), Delegation: types.StringValue("2.0.192.in-addr.arpa."), Name: types.StringValue("ns.example.net"), TTL: types.Int64Null()}
	plan := tfsdk.Plan{Schema: sr.Schema}
	if d := plan.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sr.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
		t.Fatal("uncertain creation must retain state")
	}
	var saved delegationNameserverModel
	if d := resp.State.Get(ctx, &saved); d.HasError() {
		t.Fatal(d)
	}
	if saved.ID.ValueString() != "2.0.192.in-addr.arpa./ns.example.net" {
		t.Fatal("incorrect recovery identity")
	}
	read := resource.ReadResponse{State: resp.State}
	r.Read(ctx, resource.ReadRequest{State: resp.State}, &read)
	if read.Diagnostics.HasError() || read.State.Raw.IsNull() {
		t.Fatal("refresh did not recover created record")
	}
}
