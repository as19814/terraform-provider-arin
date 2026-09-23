package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &delegationNameserverResource{}
	_ resource.ResourceWithConfigure      = &delegationNameserverResource{}
	_ resource.ResourceWithValidateConfig = &delegationNameserverResource{}
	_ resource.ResourceWithImportState    = &delegationNameserverResource{}
)

type delegationNameserverResource struct{ client *arin.Client }
type delegationNameserverModel struct {
	ID         types.String `tfsdk:"id"`
	Delegation types.String `tfsdk:"delegation"`
	Name       types.String `tfsdk:"name"`
	TTL        types.Int64  `tfsdk:"ttl"`
}

func NewDelegationNameserverResource() resource.Resource { return &delegationNameserverResource{} }
func (r *delegationNameserverResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_delegation_nameserver"
}
func (r *delegationNameserverResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Manage one nameserver on an existing reverse delegation, preserving other nameservers and DNSSEC records. Import an existing nameserver before managing it. Destroy removes only this nameserver. Do not combine with arin_delegation on the same zone or manage the same nameserver in multiple resources. Published DNS may take up to 24 hours to update.", Attributes: map[string]schema.Attribute{
		"id":         schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Delegation and nameserver joined by a slash."},
		"delegation": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Existing lowercase reverse zone, including the trailing dot."},
		"name":       schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Lowercase fully qualified nameserver hostname without a trailing dot."},
		"ttl":        schema.Int64Attribute{Optional: true, MarkdownDescription: "TTL in seconds, 0 through 2147483647. Omission resets the nameserver to inherited TTL."},
	}}
}
func (r *delegationNameserverResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *delegationNameserverResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m delegationNameserverModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Delegation.IsUnknown() {
		if err := arin.ValidateDelegationName(m.Delegation.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("delegation"), "Invalid delegation", err.Error())
		}
	}
	if !m.Name.IsUnknown() && !m.TTL.IsUnknown() {
		if err := (arin.DelegationNameserver{Name: m.Name.ValueString(), TTL: delegationTTL(m.TTL)}).Validate(); err != nil {
			resp.Diagnostics.AddError("Invalid nameserver", err.Error())
		}
	}
}
func findDelegationNameserver(d *arin.Delegation, name string) *arin.DelegationNameserver {
	for _, n := range d.Nameservers {
		if n.Name == name {
			return &n
		}
	}
	return nil
}
func nameserverState(m delegationNameserverModel, n *arin.DelegationNameserver) delegationNameserverModel {
	m.ID = types.StringValue(m.Delegation.ValueString() + "/" + m.Name.ValueString())
	m.TTL = types.Int64PointerValue(n.TTL)
	return m
}
func (r *delegationNameserverResource) write(ctx context.Context, m delegationNameserverModel) (delegationNameserverModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	d, err := r.client.SetDelegationNameserver(ctx, m.Delegation.ValueString(), arin.DelegationNameserver{Name: m.Name.ValueString(), TTL: delegationTTL(m.TTL)})
	if err != nil {
		diags.AddError("Could not configure nameserver", err.Error())
		return m, diags
	}
	n := findDelegationNameserver(d, m.Name.ValueString())
	if n == nil {
		diags.AddError("Nameserver update not confirmed", "ARIN returned a delegation without the requested nameserver.")
		return m, diags
	}
	return nameserverState(m, n), diags
}
func (r *delegationNameserverResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m delegationNameserverModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDelegation(ctx, m.Delegation.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read delegation", err.Error())
		return
	}
	if findDelegationNameserver(d, m.Name.ValueString()) != nil {
		resp.Diagnostics.AddError("Nameserver already exists", "Import "+m.Delegation.ValueString()+"/"+m.Name.ValueString()+" before managing this record.")
		return
	}
	// Keep the exact identity if a write response is lost so refresh can reconcile it.
	m.ID = types.StringValue(m.Delegation.ValueString() + "/" + m.Name.ValueString())
	out, diags := r.write(ctx, m)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}
func (r *delegationNameserverResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m delegationNameserverModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDelegation(ctx, m.Delegation.ValueString())
	if arin.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read nameserver", err.Error())
		return
	}
	n := findDelegationNameserver(d, m.Name.ValueString())
	if n == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	m = nameserverState(m, n)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *delegationNameserverResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m delegationNameserverModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, diags := r.write(ctx, m)
	resp.Diagnostics.Append(diags...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
	}
}
func (r *delegationNameserverResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m delegationNameserverModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDelegation(ctx, m.Delegation.ValueString())
	if arin.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read delegation", err.Error())
		return
	}
	if findDelegationNameserver(d, m.Name.ValueString()) == nil {
		return
	}
	_, err = r.client.DeleteDelegationNameserver(ctx, m.Delegation.ValueString(), m.Name.ValueString())
	if err != nil && !arin.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not remove nameserver", err.Error())
		return
	}
	d, err = r.client.GetDelegation(ctx, m.Delegation.ValueString())
	if arin.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not verify nameserver removal", err.Error())
		return
	}
	if findDelegationNameserver(d, m.Name.ValueString()) != nil {
		resp.Diagnostics.AddError("Nameserver removal is incomplete", "ARIN still returns the nameserver. Terraform retained its state for reconciliation.")
	}
}
func (r *delegationNameserverResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	zone, name, ok := strings.Cut(req.ID, "/")
	if !ok || arin.ValidateDelegationName(zone) != nil || (arin.DelegationNameserver{Name: name}).Validate() != nil {
		resp.Diagnostics.AddError("Invalid nameserver import ID", "Use the lowercase delegation/name form, for example 2.0.192.in-addr.arpa./ns1.example.net.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("delegation"), zone)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}
