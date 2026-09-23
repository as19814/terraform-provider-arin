package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &rpkiBundleResource{}
	_ resource.ResourceWithConfigure      = &rpkiBundleResource{}
	_ resource.ResourceWithValidateConfig = &rpkiBundleResource{}
	_ resource.ResourceWithImportState    = &rpkiBundleResource{}
)

type rpkiBundleResource struct{ client *arin.Client }
type rpkiBundleModel struct {
	ID       types.String `tfsdk:"id"`
	Org      types.String `tfsdk:"org_handle"`
	Name     types.String `tfsdk:"name"`
	ROAs     types.Map    `tfsdk:"roas"`
	ASPAs    types.Map    `tfsdk:"aspas"`
	Recovery types.String `tfsdk:"recovery_data"`
}
type bundleROAModel struct {
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
}
type bundleRecovery struct {
	Plan     *arin.RPKIBundlePlan
	Returned *arin.RPKITransactionResult
}

func NewRPKIBundleResource() resource.Resource { return &rpkiBundleResource{} }
func (r *rpkiBundleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rpki_bundle"
}
func bundleROAAttributes() map[string]schema.Attribute {
	var resp resource.SchemaResponse
	(&roaResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	a := resp.Schema.Attributes
	for _, key := range []string{"id", "org_handle", "recovery_data"} {
		delete(a, key)
	}
	return a
}
func bundleROAType() types.ObjectType {
	a := map[string]attr.Type{}
	for key, value := range bundleROAAttributes() {
		a[key] = value.GetType()
	}
	return types.ObjectType{AttrTypes: a}
}
func (r *rpkiBundleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Manage an explicit group of hosted ROAs and ASPAs using one atomic transaction per change. ARIN has no remote bundle object; the local name and member identities define ownership. Never manage these members in another bundle or standalone resource. Import requires a JSON manifest with org_handle, name, roas (label to handle), and aspas (customer ASN list). Uncertain writes retain a sensitive recovery journal and block replay until every intended change is confirmed.", Attributes: map[string]schema.Attribute{
		"id":            schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Local identity ORG-HANDLE/BUNDLE-NAME."},
		"org_handle":    schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Hosted RPKI organization handle. Changes require replacement."},
		"name":          schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Local bundle name using letters, digits and hyphens. Changes require replacement."},
		"roas":          schema.MapNestedAttribute{Required: true, NestedObject: schema.NestedAttributeObject{Attributes: bundleROAAttributes()}, MarkdownDescription: "Managed ROAs keyed by stable local labels. Use {} for an ASPA-only bundle. Replacements preserve old linked IRR routes; removals use the prior entry's delete_linked_routes policy."},
		"aspas":         schema.MapAttribute{Required: true, ElementType: types.SetType{ElemType: types.Int64Type}, MarkdownDescription: "Canonical decimal customer ASNs mapped to complete nonempty provider-AS sets. Use {} for a ROA-only bundle."},
		"recovery_data": schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Saved transaction and returned identities for uncertain writes. Preserve until refresh confirms the complete result."},
	}}
}
func (r *rpkiBundleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	base := &roaResource{}
	base.Configure(ctx, req, resp)
	r.client = base.client
}
func (m rpkiBundleModel) identityError() error {
	if err := arin.ValidatePOCHandle(m.Org.ValueString()); err != nil {
		return err
	}
	if !validROAHandle(m.Name.ValueString()) {
		return fmt.Errorf("bundle name must contain only letters, digits and hyphens")
	}
	return nil
}
func (m rpkiBundleModel) desired(ctx context.Context) (arin.RPKIBundleDesired, diag.Diagnostics) {
	out := arin.RPKIBundleDesired{ROAs: map[string]arin.RPKIBundleROA{}, ASPAs: map[int64]arin.ASPA{}}
	roas := map[string]bundleROAModel{}
	d := m.ROAs.ElementsAs(ctx, &roas, false)
	for label, entry := range roas {
		base := roaModel{Name: entry.Name, ASN: entry.ASN, Prefixes: entry.Prefixes, AutoLink: entry.AutoLink}
		request, more := base.api(ctx)
		d.Append(more...)
		out.ROAs[label] = arin.RPKIBundleROA{Request: request, DeleteLinkedRoutes: entry.DeleteLinked.ValueBool()}
	}
	for key, value := range m.ASPAs.Elements() {
		customer, err := strconv.ParseInt(key, 10, 64)
		if err != nil || strconv.FormatInt(customer, 10) != key {
			d.AddError("Invalid ASPA customer key", "Use a canonical decimal customer ASN.")
			continue
		}
		providers := []int64{}
		d.Append(value.(types.Set).ElementsAs(ctx, &providers, false)...)
		out.ASPAs[customer] = arin.ASPA{CustomerASN: customer, ProviderASNs: providers}
	}
	if !d.HasError() {
		if len(out.ROAs)+len(out.ASPAs) == 0 {
			d.AddError("Empty RPKI bundle", "Configure at least one ROA or ASPA.")
		}
		if err := out.Validate(); err != nil {
			d.AddError("Invalid RPKI bundle", err.Error())
		}
	}
	return out, d
}
func (m rpkiBundleModel) ownership(ctx context.Context) (arin.RPKIBundleOwnership, diag.Diagnostics) {
	out := arin.RPKIBundleOwnership{ROAs: map[string]arin.RPKIBundleOwnedROA{}}
	roas := map[string]bundleROAModel{}
	d := m.ROAs.ElementsAs(ctx, &roas, false)
	for label, entry := range roas {
		out.ROAs[label] = arin.RPKIBundleOwnedROA{Handle: entry.Handle.ValueString(), DeleteLinkedRoutes: entry.DeleteLinked.ValueBool()}
	}
	for key := range m.ASPAs.Elements() {
		customer, err := strconv.ParseInt(key, 10, 64)
		if err != nil || strconv.FormatInt(customer, 10) != key {
			d.AddError("Invalid ASPA state", "Customer key is not a canonical ASN.")
			continue
		}
		out.ASPAs = append(out.ASPAs, customer)
	}
	return out, d
}
func (r *rpkiBundleResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m rpkiBundleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Org.IsUnknown() && !m.Name.IsUnknown() {
		if err := m.identityError(); err != nil {
			resp.Diagnostics.AddError("Invalid bundle identity", err.Error())
		}
	}
	// Only configured fields must be known. Computed member metadata is unknown during planning.
	for _, v := range []attr.Value{m.ROAs, m.ASPAs} {
		if v.IsUnknown() || v.IsNull() {
			return
		}
	}
	for _, v := range m.ROAs.Elements() {
		o, ok := v.(types.Object)
		if !ok || o.IsUnknown() || o.IsNull() {
			return
		}
		for _, key := range []string{"name", "asn", "prefixes", "auto_link", "delete_linked_routes"} {
			a := o.Attributes()[key]
			tv, err := a.ToTerraformValue(ctx)
			if err != nil || !tv.IsFullyKnown() {
				return
			}
		}
	}
	for _, v := range m.ASPAs.Elements() {
		tv, err := v.ToTerraformValue(ctx)
		if err != nil || !tv.IsFullyKnown() {
			return
		}
	}
	_, d := m.desired(ctx)
	resp.Diagnostics.Append(d...)
}
func (r *rpkiBundleResource) inventory(ctx context.Context, org string) (arin.RPKIBundleInventory, error) {
	roas, err := r.client.ListROAs(ctx, org)
	if err != nil {
		return arin.RPKIBundleInventory{}, err
	}
	aspas, err := r.client.ListASPAs(ctx, org)
	return arin.RPKIBundleInventory{ROAs: roas, ASPAs: aspas}, err
}
func (m *rpkiBundleModel) set(ctx context.Context, state *arin.RPKIBundleState, policies map[string]arin.RPKIBundleOwnedROA) diag.Diagnostics {
	var d diag.Diagnostics
	roas := map[string]bundleROAModel{}
	for label, a := range state.ROAs {
		base := roaModel{Org: m.Org, DeleteLinked: types.BoolValue(policies[label].DeleteLinkedRoutes)}
		d.Append(base.set(ctx, a)...)
		roas[label] = bundleROAModel{Handle: base.Handle, Name: base.Name, ASN: base.ASN, Prefixes: base.Prefixes, AutoLink: base.AutoLink, DeleteLinked: base.DeleteLinked, Linked: base.Linked, Before: base.Before, After: base.After, Renewed: base.Renewed}
	}
	var more diag.Diagnostics
	m.ROAs, more = types.MapValueFrom(ctx, bundleROAType(), roas)
	d.Append(more...)
	aspas := map[string]attr.Value{}
	for customer, a := range state.ASPAs {
		v, more := types.SetValueFrom(ctx, types.Int64Type, a.ProviderASNs)
		d.Append(more...)
		aspas[strconv.FormatInt(customer, 10)] = v
	}
	m.ASPAs, more = types.MapValue(types.SetType{ElemType: types.Int64Type}, aspas)
	d.Append(more...)
	m.ID = types.StringValue(m.Org.ValueString() + "/" + m.Name.ValueString())
	m.Recovery = types.StringValue("")
	return d
}
func bundlePolicies(plan *arin.RPKIBundlePlan) map[string]arin.RPKIBundleOwnedROA {
	out := map[string]arin.RPKIBundleOwnedROA{}
	for label, entry := range plan.Desired.ROAs {
		out[label] = arin.RPKIBundleOwnedROA{DeleteLinkedRoutes: entry.DeleteLinkedRoutes}
	}
	return out
}
func (r *rpkiBundleResource) write(ctx context.Context, m *rpkiBundleModel, old *rpkiBundleModel, destroy bool, d *diag.Diagnostics) bool {
	if err := m.identityError(); err != nil {
		d.AddError("Invalid bundle identity", err.Error())
		return false
	}
	if old != nil && old.Recovery.ValueString() != "" {
		d.AddError("Bundle recovery required", "Refresh and reconcile the pending transaction before making another change.")
		return false
	}
	desired := arin.RPKIBundleDesired{}
	var more diag.Diagnostics
	if !destroy {
		desired, more = m.desired(ctx)
		d.Append(more...)
	}
	prior := arin.RPKIBundleOwnership{}
	if old != nil {
		prior, more = old.ownership(ctx)
		d.Append(more...)
	}
	if d.HasError() {
		return false
	}
	before, err := r.inventory(ctx, m.Org.ValueString())
	if err != nil {
		d.AddError("Could not read RPKI inventories", err.Error())
		return false
	}
	plan, err := arin.PlanRPKIBundle(prior, desired, before)
	if err != nil {
		d.AddError("Could not plan bundle transaction", err.Error())
		return false
	}
	journal := bundleRecovery{Plan: plan}
	// Serialize before the write so a journal construction failure cannot lose ownership.
	encoded, err := json.Marshal(journal)
	if err != nil {
		d.AddError("Could not prepare bundle recovery", err.Error())
		return false
	}
	after := before
	if !plan.Empty() {
		result, writeErr := r.client.ApplyRPKITransaction(ctx, m.Org.ValueString(), plan.Transaction)
		if result == nil && netDefinitiveFailure(writeErr) {
			d.AddError("Could not write RPKI bundle", writeErr.Error())
			return false
		}
		journal.Returned = result
		if result != nil {
			encoded, _ = json.Marshal(journal)
		}
		if writeErr == nil {
			after, err = r.inventory(ctx, m.Org.ValueString())
		} else {
			err = writeErr
		}
	}
	var state *arin.RPKIBundleState
	if err == nil {
		state, err = reconcileBundle(journal, after)
	}
	if err != nil {
		if old != nil {
			*m = *old
		} else {
			d.Append(m.set(ctx, &arin.RPKIBundleState{}, nil)...)
		}
		m.Recovery = types.StringValue(string(encoded))
		d.AddError("Could not confirm bundle transaction", err.Error()+". Recovery state was saved; refresh must confirm every member before another write. The POST was not replayed.")
		return true
	}
	d.Append(m.set(ctx, state, bundlePolicies(plan))...)
	return true
}
func (r *rpkiBundleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m rpkiBundleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.write(ctx, &m, nil, false, &resp.Diagnostics) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *rpkiBundleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, old rpkiBundleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.write(ctx, &m, &old, false, &resp.Diagnostics) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *rpkiBundleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m rpkiBundleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inventory, err := r.inventory(ctx, m.Org.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read RPKI bundle", err.Error())
		return
	}
	if m.Recovery.ValueString() != "" {
		var journal bundleRecovery
		if err = json.Unmarshal([]byte(m.Recovery.ValueString()), &journal); err != nil {
			resp.Diagnostics.AddError("Invalid bundle recovery journal", err.Error())
			return
		}
		state, err := reconcileBundle(journal, inventory)
		if err != nil {
			resp.Diagnostics.AddError("Bundle recovery remains pending", err.Error())
			return
		}
		if len(journal.Plan.Desired.ROAs)+len(journal.Plan.Desired.ASPAs) == 0 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(m.set(ctx, state, bundlePolicies(journal.Plan))...)
	} else {
		owned, d := m.ownership(ctx)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		// Missing members are removed from state so Terraform can plan their recreation.
		state := &arin.RPKIBundleState{ROAs: map[string]arin.ROA{}, ASPAs: map[int64]arin.ASPA{}}
		for label, entry := range owned.ROAs {
			for _, a := range inventory.ROAs {
				if a.Handle == entry.Handle {
					state.ROAs[label] = a
				}
			}
		}
		for _, customer := range owned.ASPAs {
			for _, a := range inventory.ASPAs {
				if a.CustomerASN == customer {
					state.ASPAs[customer] = a
				}
			}
		}
		resp.Diagnostics.Append(m.set(ctx, state, owned.ROAs)...)
	}
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *rpkiBundleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m rpkiBundleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	old := m
	if r.write(ctx, &m, &old, true, &resp.Diagnostics) && resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *rpkiBundleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var manifest struct {
		Org   string            `json:"org_handle"`
		Name  string            `json:"name"`
		ROAs  map[string]string `json:"roas"`
		ASPAs []int64           `json:"aspas"`
	}
	if err := bundleImportJSON(json.NewDecoder(strings.NewReader(req.ID)), 0); err != nil {
		resp.Diagnostics.AddError("Invalid bundle import manifest", err.Error())
		return
	}
	decoder := json.NewDecoder(strings.NewReader(req.ID))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		resp.Diagnostics.AddError("Invalid bundle import manifest", err.Error())
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		resp.Diagnostics.AddError("Invalid bundle import manifest", "Expected one JSON object.")
		return
	}
	m := rpkiBundleModel{Org: types.StringValue(manifest.Org), Name: types.StringValue(manifest.Name)}
	if err := m.identityError(); err != nil {
		resp.Diagnostics.AddError("Invalid bundle import identity", err.Error())
		return
	}
	owned := arin.RPKIBundleOwnership{ROAs: map[string]arin.RPKIBundleOwnedROA{}, ASPAs: manifest.ASPAs}
	for label, handle := range manifest.ROAs {
		owned.ROAs[label] = arin.RPKIBundleOwnedROA{Handle: handle}
	}
	// Reuse the planner's ownership validation before any remote lookup.
	if _, err := arin.PlanRPKIBundle(owned, arin.RPKIBundleDesired{}, arin.RPKIBundleInventory{}); err != nil {
		resp.Diagnostics.AddError("Invalid bundle import members", err.Error())
		return
	}
	if len(owned.ROAs)+len(owned.ASPAs) == 0 {
		resp.Diagnostics.AddError("Empty bundle import", "Specify at least one exact member identity.")
		return
	}
	inventory, err := r.inventory(ctx, manifest.Org)
	if err != nil {
		resp.Diagnostics.AddError("Could not read bundle import members", err.Error())
		return
	}
	state := &arin.RPKIBundleState{ROAs: map[string]arin.ROA{}, ASPAs: map[int64]arin.ASPA{}}
	for label, entry := range owned.ROAs {
		for _, a := range inventory.ROAs {
			if a.Handle == entry.Handle {
				state.ROAs[label] = a
			}
		}
	}
	for _, customer := range owned.ASPAs {
		for _, a := range inventory.ASPAs {
			if a.CustomerASN == customer {
				state.ASPAs[customer] = a
			}
		}
	}
	if len(state.ROAs) != len(owned.ROAs) || len(state.ASPAs) != len(owned.ASPAs) {
		resp.Diagnostics.AddError("Bundle import member missing", "Every specified ROA handle and ASPA customer must exist in this organization.")
		return
	}
	resp.Diagnostics.Append(m.set(ctx, state, owned.ROAs)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}

// Parsed response handles are additional evidence, even when verification fails.
// A different newly matching handle must not silently replace that evidence.
func reconcileBundle(journal bundleRecovery, inventory arin.RPKIBundleInventory) (*arin.RPKIBundleState, error) {
	state, err := arin.ReconcileRPKIBundle(journal.Plan, inventory)
	if err != nil {
		return nil, err
	}
	if journal.Returned != nil {
		for _, returned := range journal.Returned.ROAs {
			for label, desired := range journal.Plan.Desired.ROAs {
				if arin.ROAMatchesRequest(returned, desired.Request) && state.ROAs[label].Handle != returned.Handle {
					return nil, fmt.Errorf("recovered ROA differs from its returned transaction identity")
				}
			}
		}
	}
	return state, nil
}

// Reject repeated JSON keys rather than silently dropping explicitly named members.
func bundleImportJSON(d *json.Decoder, depth int) error {
	if depth > 16 {
		return fmt.Errorf("bundle import manifest nesting is too deep")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	if delimiter != '{' && delimiter != '[' {
		return fmt.Errorf("invalid JSON structure")
	}
	keys := map[string]bool{}
	for d.More() {
		if delimiter == '{' {
			token, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok || keys[key] {
				return fmt.Errorf("bundle import contains a repeated or invalid object key")
			}
			keys[key] = true
		}
		if err := bundleImportJSON(d, depth+1); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}
