package provider

import (
	"context"
	"fmt"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &delegationResource{}
	_ resource.ResourceWithConfigure      = &delegationResource{}
	_ resource.ResourceWithValidateConfig = &delegationResource{}
	_ resource.ResourceWithModifyPlan     = &delegationResource{}
	_ resource.ResourceWithImportState    = &delegationResource{}
)

type delegationResource struct{ client *arin.Client }
type delegationModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Nameservers types.Set    `tfsdk:"nameservers"`
	DS          types.Set    `tfsdk:"ds_records"`
}
type delegationNSModel struct {
	Name types.String `tfsdk:"name"`
	TTL  types.Int64  `tfsdk:"ttl"`
}
type delegationDSModel struct {
	Algorithm  types.Int64  `tfsdk:"algorithm"`
	DigestType types.Int64  `tfsdk:"digest_type"`
	KeyTag     types.Int64  `tfsdk:"key_tag"`
	Digest     types.String `tfsdk:"digest"`
	TTL        types.Int64  `tfsdk:"ttl"`
}

var delegationNSType = types.ObjectType{AttrTypes: map[string]attr.Type{"name": types.StringType, "ttl": types.Int64Type}}
var delegationDSType = types.ObjectType{AttrTypes: map[string]attr.Type{"algorithm": types.Int64Type, "digest_type": types.Int64Type, "key_tag": types.Int64Type, "digest": types.StringType, "ttl": types.Int64Type}}

func NewDelegationResource() resource.Resource { return &delegationResource{} }
func (r *delegationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_delegation"
}
func (r *delegationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Own all nameservers and DNSSEC DS records on an existing ARIN reverse delegation. Import a zone that already has DNS records before managing it. Destroy clears all nameservers and DS records, while the delegation object remains tied to its NET. Do not overlap ownership with other DNS resources. API changes may take up to 24 hours to appear in published DNS.", Attributes: map[string]schema.Attribute{
		"id":   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Delegation name."},
		"name": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Existing lowercase reverse zone ending in .in-addr.arpa. or .ip6.arpa., including the trailing dot."},
		"nameservers": schema.SetNestedAttribute{Required: true, MarkdownDescription: "Complete authoritative nameserver collection. An empty set clears all nameservers.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{Required: true, MarkdownDescription: "Lowercase nameserver hostname without a trailing dot."},
			"ttl":  schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "TTL in seconds. Omission preserves an existing nameserver TTL; new nameservers inherit TTL. To reset an explicit TTL to inheritance, remove this nameserver in one apply and add it without TTL in a subsequent apply."},
		}}},
		"ds_records": schema.SetNestedAttribute{Optional: true, Computed: true, Default: setdefault.StaticValue(types.SetValueMust(delegationDSType, []attr.Value{})), MarkdownDescription: "Complete DS record collection. Omission or an empty set clears all DS records.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"algorithm":   schema.Int64Attribute{Required: true, MarkdownDescription: "DNSSEC algorithm number."},
			"digest_type": schema.Int64Attribute{Required: true, MarkdownDescription: "DS digest type, for example 2 for SHA-256."},
			"key_tag":     schema.Int64Attribute{Required: true, MarkdownDescription: "DNSKEY tag, 0 through 65535."},
			"digest":      schema.StringAttribute{Required: true, MarkdownDescription: "Uppercase hexadecimal DS digest."},
			"ttl":         schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: "TTL in seconds. Omission preserves an existing record's TTL; new records inherit TTL. To reset an existing explicit TTL to inheritance, remove that DS record in one apply and add it without TTL in a subsequent apply."},
		}}},
	}}
}
func (r *delegationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func delegationTTL(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := v.ValueInt64()
	return &n
}
func (m delegationModel) api(ctx context.Context) (arin.Delegation, diag.Diagnostics) {
	d := arin.Delegation{Name: m.Name.ValueString()}
	var diags diag.Diagnostics
	var names []delegationNSModel
	var keys []delegationDSModel
	diags.Append(m.Nameservers.ElementsAs(ctx, &names, false)...)
	diags.Append(m.DS.ElementsAs(ctx, &keys, false)...)
	for _, n := range names {
		d.Nameservers = append(d.Nameservers, arin.DelegationNameserver{Name: n.Name.ValueString(), TTL: delegationTTL(n.TTL)})
	}
	for _, k := range keys {
		d.DSRecords = append(d.DSRecords, arin.DelegationDS{Algorithm: k.Algorithm.ValueInt64(), DigestType: k.DigestType.ValueInt64(), KeyTag: k.KeyTag.ValueInt64(), Digest: k.Digest.ValueString(), TTL: delegationTTL(k.TTL)})
	}
	return d, diags
}
func (r *delegationResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m delegationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, v := range []attr.Value{m.Name, m.Nameservers, m.DS} {
		tv, err := v.ToTerraformValue(ctx)
		if err != nil || !tv.IsFullyKnown() {
			return
		}
	}
	d, diags := m.api(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := d.Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid delegation configuration", err.Error())
	}
}
func dsIdentity(d delegationDSModel) string {
	return fmt.Sprintf("%d/%d/%d/%s", d.Algorithm.ValueInt64(), d.DigestType.ValueInt64(), d.KeyTag.ValueInt64(), d.Digest.ValueString())
}
func (r *delegationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var prior, plan delegationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !plan.Name.Equal(prior.Name) || plan.DS.IsUnknown() {
		return
	}
	var old, keys []delegationDSModel
	resp.Diagnostics.Append(prior.DS.ElementsAs(ctx, &old, false)...)
	resp.Diagnostics.Append(plan.DS.ElementsAs(ctx, &keys, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	known := map[string]types.Int64{}
	for _, k := range old {
		known[dsIdentity(k)] = k.TTL
	}
	changed := false
	for i, k := range keys {
		if k.TTL.IsUnknown() && !k.Algorithm.IsUnknown() && !k.DigestType.IsUnknown() && !k.KeyTag.IsUnknown() && !k.Digest.IsUnknown() {
			if ttl, ok := known[dsIdentity(k)]; ok {
				keys[i].TTL = ttl
				changed = true
			}
		}
	}
	if !plan.Nameservers.IsUnknown() {
		var previous, names []delegationNSModel
		resp.Diagnostics.Append(prior.Nameservers.ElementsAs(ctx, &previous, false)...)
		resp.Diagnostics.Append(plan.Nameservers.ElementsAs(ctx, &names, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for i, n := range names {
			if n.TTL.IsUnknown() && !n.Name.IsUnknown() {
				for _, old := range previous {
					if n.Name.Equal(old.Name) {
						names[i].TTL = old.TTL
						changed = true
						break
					}
				}
			}
		}
		var d diag.Diagnostics
		plan.Nameservers, d = types.SetValueFrom(ctx, delegationNSType, names)
		resp.Diagnostics.Append(d...)
	}
	if changed {
		var d diag.Diagnostics
		plan.DS, d = types.SetValueFrom(ctx, delegationDSType, keys)
		resp.Diagnostics.Append(d...)
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
		}
	}
}
func delegationState(ctx context.Context, d *arin.Delegation) (delegationModel, diag.Diagnostics) {
	m := delegationModel{ID: types.StringValue(d.Name), Name: types.StringValue(d.Name)}
	names := []delegationNSModel{}
	keys := []delegationDSModel{}
	for _, n := range d.Nameservers {
		names = append(names, delegationNSModel{Name: types.StringValue(n.Name), TTL: types.Int64PointerValue(n.TTL)})
	}
	for _, k := range d.DSRecords {
		keys = append(keys, delegationDSModel{Algorithm: types.Int64Value(k.Algorithm), DigestType: types.Int64Value(k.DigestType), KeyTag: types.Int64Value(k.KeyTag), Digest: types.StringValue(k.Digest), TTL: types.Int64PointerValue(k.TTL)})
	}
	var diags, next diag.Diagnostics
	m.Nameservers, next = types.SetValueFrom(ctx, delegationNSType, names)
	diags.Append(next...)
	m.DS, next = types.SetValueFrom(ctx, delegationDSType, keys)
	diags.Append(next...)
	return m, diags
}
func (r *delegationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m delegationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	desired, diags := m.api(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	before, err := r.client.GetDelegation(ctx, desired.Name)
	if err != nil {
		resp.Diagnostics.AddError("Could not read existing delegation", err.Error())
		return
	}
	if len(before.Nameservers) != 0 || len(before.DSRecords) != 0 {
		resp.Diagnostics.AddError("Delegation already has DNS records", "Import "+desired.Name+" to review changes to its existing records before applying.")
		return
	}
	out, err := r.client.UpdateDelegation(ctx, desired)
	if err != nil {
		m, diags = delegationState(ctx, before)
		resp.Diagnostics.Append(diags...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		resp.Diagnostics.AddError("Could not configure delegation", err.Error()+". The existing zone identity was retained for refresh and recovery.")
		return
	}
	m, diags = delegationState(ctx, out)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *delegationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m delegationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDelegation(ctx, m.Name.ValueString())
	if arin.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read delegation", err.Error())
		return
	}
	m, diags := delegationState(ctx, d)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *delegationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m delegationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	desired, diags := m.api(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.UpdateDelegation(ctx, desired)
	if err != nil {
		resp.Diagnostics.AddError("Could not update delegation", err.Error())
		return
	}
	m, diags = delegationState(ctx, out)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *delegationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m delegationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := r.client.UpdateDelegation(ctx, arin.Delegation{Name: m.Name.ValueString()})
	if arin.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not clear delegation", err.Error())
		return
	}
	after, err := r.client.GetDelegation(ctx, m.Name.ValueString())
	if arin.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not verify delegation clearing", err.Error())
		return
	}
	if len(after.Nameservers) != 0 || len(after.DSRecords) != 0 {
		resp.Diagnostics.AddError("Delegation clearing is incomplete", "DNS records remain visible; Terraform retained the resource for reconciliation.")
	}
}
func (r *delegationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if err := arin.ValidateDelegationName(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid delegation import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}
