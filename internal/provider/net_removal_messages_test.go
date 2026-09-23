package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	tfresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const removalMessagesConfig = `removal_messages = [{
 subject = "Remove this reassignment"
 text = ["Supporting evidence", "", "Final line"]
 category = "JUSTIFICATION"
 attachments = { "evidence.txt" = base64encode("test evidence") }
}]`

func TestAccNetRemovalMessages(t *testing.T) {
	f, _ := setupNetFake(t)
	base := netConfig(`customer_handle = "C123"`)
	withMessages := netConfig(`customer_handle = "C123"` + "\n" + removalMessagesConfig)
	changed := strings.Replace(withMessages, "Remove this reassignment", "Final removal request", 1)
	tfresource.Test(t, tfresource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps: []tfresource.TestStep{
			{Config: withMessages, Check: tfresource.TestCheckResourceAttr("arin_net.test", "removal_messages.0.subject", "Remove this reassignment")},
			{ResourceName: "arin_net.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"removal_messages"}},
			{Config: base},
			{Config: changed},
			{Config: changed, PlanOnly: true},
		},
		CheckDestroy: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.objects) != 0 || f.writes["remove"] != 1 || f.writes["delete"] != 0 || f.writes["update"] != 0 || f.writes["create"] != 1 {
				return fmt.Errorf("unexpected removal lifecycle: %v", f.writes)
			}
			for _, field := range []string{"<messages><message>", "Final removal request", "JUSTIFICATION", "evidence.txt", "dGVzdCBldmlkZW5jZQ==", "Final line"} {
				if !strings.Contains(f.removalPayload, field) {
					return fmt.Errorf("missing removal payload field %s", field)
				}
			}
			return nil
		},
	})
}

func TestNetRemovalMessagesRecovery(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			f, c := setupNetFake(t)
			r := &netResource{client: c}
			ctx := context.Background()
			state, plan := netTestPlan(t, r)
			var model netModel
			if d := plan.Get(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			messages, d := types.ListValueFrom(ctx, netRemovalMessageType, []netRemovalMessageModel{{Subject: types.StringValue("Removal evidence"), Text: types.ListNull(types.StringType), Category: types.StringNull(), Attachments: types.MapNull(types.StringType)}})
			if d.HasError() {
				t.Fatal(d)
			}
			model.RemovalMessages = messages
			if d := plan.Set(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			created := resource.CreateResponse{State: state}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
			if created.Diagnostics.HasError() {
				t.Fatal(created.Diagnostics)
			}
			f.deletePending = true
			f.lostRemove = lost
			deleted := resource.DeleteResponse{State: created.State}
			r.Delete(ctx, resource.DeleteRequest{State: created.State}, &deleted)
			if !deleted.Diagnostics.HasError() {
				t.Fatal("unconfirmed removal accepted")
			}
			if d := deleted.State.Get(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			if model.PendingOperation.ValueString() != "delete" || !model.RemovalMessages.Equal(messages) {
				t.Fatal("removal recovery lost state")
			}
			again := resource.DeleteResponse{State: deleted.State}
			r.Delete(ctx, resource.DeleteRequest{State: deleted.State}, &again)
			if !again.Diagnostics.HasError() || f.writes["remove"] != 1 {
				t.Fatal("uncertain removal was resubmitted")
			}
			f.mu.Lock()
			delete(f.objects, model.ID.ValueString())
			f.resolved = true
			f.mu.Unlock()
			read := resource.ReadResponse{State: deleted.State}
			r.Read(ctx, resource.ReadRequest{State: deleted.State}, &read)
			if read.Diagnostics.HasError() || !read.State.Raw.IsNull() {
				t.Fatal("confirmed removal did not reconcile")
			}
		})
	}
}

func TestAccNetRemovalMessagesInvalid(t *testing.T) {
	for _, input := range []string{
		`[{ subject = "x", category = "OTHER" }]`,
		`[{ attachments = { "../file" = "dGVzdA==" } }]`,
		`[{ attachments = { "file" = "not-base64" } }]`,
		`[{}]`,
	} {
		t.Run(input, func(t *testing.T) {
			f, _ := setupNetFake(t)
			tfresource.Test(t, tfresource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []tfresource.TestStep{{Config: netConfig(`customer_handle = "C123"` + "\nremoval_messages = " + input), ExpectError: regexp.MustCompile("Invalid NET removal")}}})
			if len(f.writes) != 0 {
				t.Fatal("invalid removal policy caused a write")
			}
		})
	}
}

func TestNetRemovalMessageInvalidApply(t *testing.T) {
	f, c := setupNetFake(t)
	r := &netResource{client: c}
	ctx := context.Background()
	state, plan := netTestPlan(t, r)
	var model netModel
	if d := plan.Get(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	messages, d := types.ListValueFrom(ctx, netRemovalMessageType, []netRemovalMessageModel{{Subject: types.StringValue("x"), Text: types.ListNull(types.StringType), Category: types.StringValue("OTHER"), Attachments: types.MapNull(types.StringType)}})
	if d.HasError() {
		t.Fatal(d)
	}
	model.RemovalMessages = messages
	if d := plan.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	created := resource.CreateResponse{State: state}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &created)
	if !created.Diagnostics.HasError() || len(f.writes) != 0 {
		t.Fatal("invalid apply-time message allowed creation")
	}
	updated := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, &updated)
	if !updated.Diagnostics.HasError() || len(f.writes) != 0 {
		t.Fatal("invalid apply-time message allowed update")
	}
}

func TestAccNetProposedOTERemovalMessage(t *testing.T) {
	f, _ := setupNetFake(t)
	base := netConfig(`customer_handle = "C123"`)
	configured := netConfig(`customer_handle = "C123"` + "\n" + terraformRemovalMessageConfig)
	tfresource.Test(t, tfresource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.objects) != 0 || f.writes["remove"] != 1 || f.writes["update"] != 0 || f.writes["delete"] != 0 {
			return fmt.Errorf("unexpected proposed removal lifecycle: %v", f.writes)
		}
		return validateApprovedRemovalMessage([]byte(f.removalPayload))
	}, Steps: []tfresource.TestStep{
		{Config: base},
		{Config: configured, Check: tfresource.TestCheckResourceAttr("arin_net.test", "removal_messages.0.subject", approvedRemovalMessage().Subject)},
		{Config: configured, PlanOnly: true},
	}})
}
