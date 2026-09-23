package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type oteOrgReadTransport struct{ writes int }

func (t *oteOrgReadTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		t.writes++
		return nil, errors.New("organization recovery validation forbids mutations")
	}
	return http.DefaultTransport.RoundTrip(req)
}
func TestOTEOrgResourceRecovery(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E test opt-in")
	}
	key, handle := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || handle == "" {
		t.Fatal("requires sandbox key and org handle")
	}
	transport := &oteOrgReadTransport{}
	c, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL, HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r := &orgResource{client: c}
	var schema resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schema)
	state := tfsdk.State{Schema: schema.Schema}
	initial := orgModel{Street: types.ListNull(types.StringType), Comments: types.ListNull(types.StringType), POCs: types.SetNull(irrPOCType), ID: types.StringValue(handle), Handle: types.StringValue(handle), PendingOperation: types.StringValue(""), PendingTicket: types.StringValue("")}
	if d := state.Set(ctx, &initial); d.HasError() {
		t.Fatal(d)
	}
	read := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	var current orgModel
	if d := read.State.Get(ctx, &current); d.HasError() {
		t.Fatal(d)
	}
	if current.Handle.ValueString() != handle || current.Name.ValueString() == "" || current.POCs.IsNull() {
		t.Fatal("resource refresh did not populate complete organization state")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(handle))
	journal := filepath.Join(cache, "terraform-provider-arin", fmt.Sprintf("ote-org-create-%x.json", hash[:8]))
	data, err := os.ReadFile(journal)
	if os.IsNotExist(err) {
		t.Log("existing organization refresh passed; no pending creation journal to reconcile")
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Input  arin.RegisteredOrganization
		Result *arin.OrganizationWriteResult
	}
	if err = json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.Result == nil || record.Result.TicketNumber == "" || record.Result.Organization != nil {
		t.Skip("journal does not describe a ticket-only creation")
	}
	model, d := orgState(ctx, &record.Input)
	if d.HasError() {
		t.Fatal(d)
	}
	model = pendingOrg(model, "create", record.Result)
	pending := tfsdk.State{Schema: schema.Schema}
	if d := pending.Set(ctx, &model); d.HasError() {
		t.Fatal(d)
	}
	recovery := resource.ReadResponse{State: pending}
	r.Read(ctx, resource.ReadRequest{State: pending}, &recovery)
	if !recovery.Diagnostics.HasError() || !recovery.State.Raw.Equal(pending.Raw) {
		t.Fatal("ticket-only creation state was not retained")
	}
	del := resource.DeleteResponse{State: pending}
	r.Delete(ctx, resource.DeleteRequest{State: pending}, &del)
	if !del.Diagnostics.HasError() || !del.State.Raw.Equal(pending.Raw) {
		t.Fatal("unresolved creation discarded during destroy")
	}
	if transport.writes != 0 {
		t.Fatal("resource tried to mutate during recovery validation")
	}
	t.Log("existing organization refresh and persisted ticket recovery passed without mutation")
}
