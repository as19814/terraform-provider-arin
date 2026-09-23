package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &roaResource{}
	_ resource.ResourceWithConfigure      = &roaResource{}
	_ resource.ResourceWithImportState    = &roaResource{}
	_ resource.ResourceWithValidateConfig = &roaResource{}
)

type roaResource struct{ client *arin.Client }
type roaModel struct {
	ID           types.String `tfsdk:"id"`
	Org          types.String `tfsdk:"org_handle"`
	Handle       types.String `tfsdk:"handle"`
	Name         types.String `tfsdk:"name"`
	ASN          types.Int64  `tfsdk:"asn"`
	Prefixes     types.Map    `tfsdk:"prefixes"`
	AutoLink     types.Bool   `tfsdk:"auto_link"`
	DeleteLinked types.Bool   `tfsdk:"delete_linked_routes"`
	Linked       types.Set    `tfsdk:"linked_prefixes"`
	Before       types.String `tfsdk:"not_valid_before"`
	After        types.String `tfsdk:"not_valid_after"`
	Renewed      types.Bool   `tfsdk:"auto_renewed"`
	Recovery     types.String `tfsdk:"recovery_data"`
}
type roaRecovery struct {
	Request        arin.ROARequest
	Before         []string
	PreviousHandle string
}

func NewROAResource() resource.Resource { return &roaResource{} }
func (r *roaResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_roa"
}
func (r *roaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Manage one hosted ROA, including IPv4, IPv6, AS0 and maximum prefix lengths. Changes use an atomic delete/add transaction and produce a new ARIN handle. Import existing ROAs using ORG-HANDLE/ROA-HANDLE. With auto_link enabled ARIN can adopt existing IRR routes. Updates preserve old routes after unlinking; destroy removes linked routes only when delete_linked_routes is true. Do not also manage linked routes with arin_irr_route. An uncertain write retains recovery state and never automatically repeats the transaction.", Attributes: map[string]schema.Attribute{
		"id":                   schema.StringAttribute{Computed: true, MarkdownDescription: "ORG-HANDLE/ROA-HANDLE. Changes when the authorization is updated."},
		"org_handle":           schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Organization eligible for hosted RPKI. Changes require replacement."},
		"handle":               schema.StringAttribute{Computed: true, MarkdownDescription: "ARIN-generated ROA handle."},
		"name":                 schema.StringAttribute{Required: true, MarkdownDescription: "ROA name."},
		"asn":                  schema.Int64Attribute{Required: true, MarkdownDescription: "Authorized origin AS number, including AS0. AS0 requires auto_link = false."},
		"prefixes":             schema.MapAttribute{Required: true, ElementType: types.Int64Type, MarkdownDescription: "Canonical IPv4/IPv6 CIDRs mapped to maximum prefix lengths. Use the CIDR length to authorize only the exact prefix."},
		"auto_link":            schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Link all prefixes to IRR routes when writing the ROA. Defaults to false; must be false for AS0. On read, true only when all prefixes are linked; linked_prefixes reports individual links."},
		"delete_linked_routes": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Remove linked IRR routes when destroying this ROA. Defaults to false, which unlinks and preserves the routes. Updates always preserve old routes."},
		"linked_prefixes":      schema.SetAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Prefixes currently linked to IRR routes."},
		"not_valid_before":     schema.StringAttribute{Computed: true, MarkdownDescription: "Server-generated validity start."},
		"not_valid_after":      schema.StringAttribute{Computed: true, MarkdownDescription: "Server-generated validity end."},
		"auto_renewed":         schema.BoolAttribute{Computed: true, MarkdownDescription: "Server-reported automatic renewal status."},
		"recovery_data":        schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Provider recovery journal for an uncertain write. Empty after confirmation. Preserve this state until reconciliation succeeds."},
	}}
}
func (r *roaResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*arin.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider configuration", fmt.Sprintf("Expected *arin.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}
func (m roaModel) api(ctx context.Context) (arin.ROARequest, diag.Diagnostics) {
	a := arin.ROARequest{Name: m.Name.ValueString(), ASN: m.ASN.ValueInt64(), AutoLink: m.AutoLink.ValueBool()}
	values := map[string]int64{}
	d := m.Prefixes.ElementsAs(ctx, &values, false)
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		v := values[k]
		a.Resources = append(a.Resources, arin.ROAResource{Prefix: k, MaxLength: &v})
	}
	return a, d
}
func (r *roaResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m roaModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Org.IsUnknown() {
		if err := arin.ValidatePOCHandle(m.Org.ValueString()); err != nil {
			resp.Diagnostics.AddError("Invalid organization handle", err.Error())
		}
	}
	for _, v := range []attr.Value{m.Name, m.ASN, m.Prefixes, m.AutoLink} {
		tv, err := v.ToTerraformValue(ctx)
		if err != nil || !tv.IsFullyKnown() {
			return
		}
	}
	a, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		if err := a.Validate(); err != nil {
			resp.Diagnostics.AddError("Invalid ROA", err.Error())
		}
	}
}
func (m *roaModel) set(ctx context.Context, a arin.ROA) diag.Diagnostics {
	m.Handle = types.StringValue(a.Handle)
	m.ID = types.StringValue(m.Org.ValueString() + "/" + a.Handle)
	m.Name = types.StringValue(a.Name)
	m.ASN = types.Int64Value(a.ASN)
	prefixes := map[string]int64{}
	linked := []string{}
	for _, p := range a.Resources {
		length := int64(netip.MustParsePrefix(p.Prefix).Bits())
		if p.MaxLength != nil {
			length = *p.MaxLength
		}
		prefixes[p.Prefix] = length
		if p.AutoLinked {
			linked = append(linked, p.Prefix)
		}
	}
	var d, more diag.Diagnostics
	m.Prefixes, d = types.MapValueFrom(ctx, types.Int64Type, prefixes)
	m.Linked, more = types.SetValueFrom(ctx, types.StringType, linked)
	d.Append(more...)
	m.AutoLink = types.BoolValue(len(linked) == len(a.Resources))
	m.Before = types.StringValue(a.NotValidBefore)
	m.After = types.StringValue(a.NotValidAfter)
	m.Renewed = types.BoolValue(a.AutoRenewed)
	m.Recovery = types.StringValue("")
	if m.DeleteLinked.IsNull() || m.DeleteLinked.IsUnknown() {
		m.DeleteLinked = types.BoolValue(false)
	}
	return d
}
func (r *roaResource) find(ctx context.Context, m roaModel) (*arin.ROA, error) {
	inventory, err := r.client.ListROAs(ctx, m.Org.ValueString())
	if err != nil {
		return nil, err
	}
	if m.Recovery.ValueString() != "" {
		var recovery roaRecovery
		if err := json.Unmarshal([]byte(m.Recovery.ValueString()), &recovery); err != nil {
			return nil, fmt.Errorf("invalid ROA recovery journal: %w", err)
		}
		if err := recovery.Request.Validate(); err != nil {
			return nil, fmt.Errorf("invalid ROA recovery request: %w", err)
		}
		matches := []arin.ROA{}
		oldPresent := false
		for _, a := range inventory {
			if a.Handle == recovery.PreviousHandle {
				oldPresent = true
			}
			if !slices.Contains(recovery.Before, a.Handle) && arin.ROAMatchesRequest(a, recovery.Request) {
				matches = append(matches, a)
			}
		}
		if len(matches) != 1 || oldPresent {
			return nil, fmt.Errorf("ROA transaction remains uncertain (%d new matching objects, previous handle present: %t); preserve state and reconcile the inventory before retrying", len(matches), oldPresent)
		}
		return &matches[0], nil
	}
	for _, a := range inventory {
		if a.Handle == m.Handle.ValueString() {
			return &a, nil
		}
	}
	return nil, nil
}
func (r *roaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m roaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &m, nil, &resp.Diagnostics)
	if !m.ID.IsNull() && !m.ID.IsUnknown() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *roaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, old roaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if old.Recovery.ValueString() != "" {
		resp.Diagnostics.AddError("ROA recovery required", "Refresh and reconcile the pending transaction before updating.")
		return
	}
	// A local destruction policy change does not require a new ROA.
	a, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	b, d := old.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	if string(aj) == string(bj) {
		old.DeleteLinked = m.DeleteLinked
		resp.Diagnostics.Append(resp.State.Set(ctx, &old)...)
		return
	}
	r.write(ctx, &m, &old, &resp.Diagnostics)
	if !resp.Diagnostics.HasError() || m.Recovery.ValueString() != "" {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *roaResource) write(ctx context.Context, m *roaModel, old *roaModel, d *diag.Diagnostics) {
	a, more := m.api(ctx)
	d.Append(more...)
	if d.HasError() {
		return
	}
	if err := a.Validate(); err != nil {
		d.AddError("Invalid ROA", err.Error())
		return
	}
	inventory, err := r.client.ListROAs(ctx, m.Org.ValueString())
	if err != nil {
		d.AddError("Could not read ROA inventory", err.Error())
		return
	}
	recovery := roaRecovery{Request: a}
	transaction := arin.RPKITransaction{AddROAs: []arin.ROARequest{a}}
	if old != nil {
		recovery.PreviousHandle = old.Handle.ValueString()
		transaction.DeleteROAs = []arin.ROADelete{{Handle: recovery.PreviousHandle}}
	}
	oldPresent := old == nil
	for _, current := range inventory {
		recovery.Before = append(recovery.Before, current.Handle)
		if current.Handle == recovery.PreviousHandle {
			oldPresent = true
			continue
		}
		if arin.ROAMatchesRequest(current, a) {
			d.AddError("ROA already exists", "Import the existing ORG-HANDLE/ROA-HANDLE before managing this authorization.")
			return
		}
	}
	if !oldPresent {
		d.AddError("ROA disappeared", "Refresh before recreating the missing ROA.")
		return
	}
	// Prepare all recovery data before submitting the single, non-retried POST.
	journal, err := json.Marshal(recovery)
	if err != nil {
		d.AddError("Could not prepare recovery", err.Error())
		return
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		d.AddError("Could not prepare recovery", err.Error())
		return
	}
	result, err := r.client.ApplyRPKITransaction(ctx, m.Org.ValueString(), transaction)
	if err == nil {
		d.Append(m.set(ctx, result.ROAs[0])...)
		return
	}
	if result == nil && netDefinitiveFailure(err) {
		d.AddError("Could not write ROA", err.Error())
		return
	}
	if old != nil {
		policy := m.DeleteLinked
		*m = *old
		m.DeleteLinked = policy
	} else {
		m.ID = types.StringValue(m.Org.ValueString() + "/pending-" + hex.EncodeToString(nonce))
		m.Handle = types.StringNull()
		m.Before = types.StringNull()
		m.After = types.StringNull()
		m.Renewed = types.BoolNull()
		m.Linked = types.SetNull(types.StringType)
	}
	m.Recovery = types.StringValue(string(journal))
	d.AddError("Could not confirm ROA transaction", err.Error()+". Recovery state was saved. Refresh reconciles new matching handles against the pre-write inventory; the transaction was not replayed.")
}
func (r *roaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m roaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.find(ctx, m)
	if (arin.IsNotFound(err) && m.Recovery.ValueString() == "") || (err == nil && a == nil) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read ROA", err.Error())
		return
	}
	resp.Diagnostics.Append(m.set(ctx, *a)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *roaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m roaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.find(ctx, m)
	if (arin.IsNotFound(err) && m.Recovery.ValueString() == "") || (err == nil && a == nil) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not reconcile ROA before deletion", err.Error())
		return
	}
	// Resolve pending creation/update state to its concrete handle before delete.
	// If the delete response is lost, refresh must look up that handle rather
	// than search again for an authorization that was just removed.
	resp.Diagnostics.Append(m.set(ctx, *a)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err = r.client.ApplyRPKITransaction(ctx, m.Org.ValueString(), arin.RPKITransaction{DeleteROAs: []arin.ROADelete{{Handle: a.Handle, AutoLink: m.DeleteLinked.ValueBool()}}}); err != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		resp.Diagnostics.AddError("Could not remove ROA", err.Error()+". State was retained; refresh to reconcile the result.")
	}
}
func (r *roaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || arin.ValidatePOCHandle(parts[0]) != nil || !validROAHandle(parts[1]) {
		resp.Diagnostics.AddError("Invalid ROA import ID", "Use ORG-HANDLE/ROA-HANDLE.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org_handle"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("handle"), parts[1])...)
}
func validROAHandle(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}
