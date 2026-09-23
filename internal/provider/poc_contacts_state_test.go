package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func contactStateFixture(t *testing.T, phone bool) (*contactFake, resource.Resource, tfsdk.State, tfsdk.Plan) {
	t.Helper()
	f, c := setupContactFake(t)
	ctx := context.Background()
	var r resource.Resource
	var model any
	if phone {
		r = &pocPhoneResource{client: c}
		model = &pocPhoneResourceModel{ID: types.StringValue("TEST-ARIN/F/+1-202-555-0101"), Handle: types.StringValue("TEST-ARIN"), Type: types.StringValue("F"), Number: types.StringValue("+1-202-555-0101"), Extension: types.StringValue("42")}
	} else {
		r = &pocEmailResource{client: c}
		model = &pocEmailModel{ID: types.StringValue("TEST-ARIN/extra@example.net"), Handle: types.StringValue("TEST-ARIN"), Email: types.StringValue("extra@example.net")}
	}
	var sr resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &sr)
	state := tfsdk.State{Schema: sr.Schema}
	if d := state.Set(ctx, model); d.HasError() {
		t.Fatal(d)
	}
	plan := tfsdk.Plan{Schema: sr.Schema}
	if d := plan.Set(ctx, model); d.HasError() {
		t.Fatal(d)
	}
	return f, r, state, plan
}
func seedContact(f *contactFake, phone bool) {
	if phone {
		f.phones = append(f.phones, arin.POCPhone{Type: "F", Number: "+1-202-555-0101", Extension: "42"})
	} else {
		f.emails = append(f.emails, "extra@example.net")
	}
}
func TestPOCContactReadFailuresPreserveState(t *testing.T) {
	for _, phone := range []bool{false, true} {
		for _, status := range []int{403, 404, 429, 500} {
			t.Run(fmt.Sprintf("phone=%t/%d", phone, status), func(t *testing.T) {
				f, r, state, _ := contactStateFixture(t, phone)
				f.readStatus = status
				ctx := context.Background()
				read := resource.ReadResponse{State: state}
				r.Read(ctx, resource.ReadRequest{State: state}, &read)
				if status == 404 {
					if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
						t.Fatal("missing POC was not removed")
					}
				} else if !read.Diagnostics.HasError() || !read.State.Raw.Equal(state.Raw) {
					t.Fatal("read error lost state")
				}
				del := resource.DeleteResponse{State: state}
				r.Delete(ctx, resource.DeleteRequest{State: state}, &del)
				if (status != 404) != del.Diagnostics.HasError() {
					t.Fatal("wrong delete result")
				}
				if status != 404 && !del.State.Raw.Equal(state.Raw) {
					t.Fatal("failed delete lost state")
				}
				if f.writes != 0 {
					t.Fatal("write after failed read")
				}
			})
		}
	}
}
func TestPOCContactDeleteRequiresConfirmation(t *testing.T) {
	for _, phone := range []bool{false, true} {
		for _, status := range []int{0, 404, 409, 500} {
			t.Run(fmt.Sprintf("phone=%t/%d", phone, status), func(t *testing.T) {
				f, r, state, _ := contactStateFixture(t, phone)
				seedContact(f, phone)
				f.writeStatus = status
				f.ignoreDelete = true
				resp := resource.DeleteResponse{State: state}
				r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
				if !resp.Diagnostics.HasError() || !resp.State.Raw.Equal(state.Raw) {
					t.Fatal("unconfirmed deletion lost state")
				}
			})
		}
	}
}
func TestPOCContactUncertainCreationRetainsIdentity(t *testing.T) {
	for _, phone := range []bool{false, true} {
		t.Run(fmt.Sprint(phone), func(t *testing.T) {
			f, r, state, plan := contactStateFixture(t, phone)
			f.writeStatus = 500
			ctx := context.Background()
			if d := plan.SetAttribute(ctx, path.Root("id"), types.StringUnknown()); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
			if !resp.Diagnostics.HasError() || resp.State.Raw.IsNull() {
				t.Fatal("uncertain creation lost state")
			}
			f.mu.Lock()
			seedContact(f, phone)
			f.writeStatus = 0
			f.mu.Unlock()
			read := resource.ReadResponse{State: resp.State}
			r.Read(ctx, resource.ReadRequest{State: resp.State}, &read)
			if read.Diagnostics.HasError() || !read.State.Raw.Equal(state.Raw) {
				t.Fatal("refresh did not recover contact")
			}
		})
	}
}
func TestPOCContactExistingRecordRequiresImport(t *testing.T) {
	for _, phone := range []bool{false, true} {
		t.Run(fmt.Sprint(phone), func(t *testing.T) {
			f, r, state, plan := contactStateFixture(t, phone)
			seedContact(f, phone)
			resp := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema}}
			r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)
			if !resp.Diagnostics.HasError() || f.writes != 0 {
				t.Fatal("existing record was adopted or modified")
			}
		})
	}
}
