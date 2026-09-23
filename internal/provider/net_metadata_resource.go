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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &netMetadataResource{}
	_ resource.ResourceWithConfigure      = &netMetadataResource{}
	_ resource.ResourceWithImportState    = &netMetadataResource{}
	_ resource.ResourceWithValidateConfig = &netMetadataResource{}
	_ resource.ResourceWithModifyPlan     = &netMetadataResource{}
)

type netMetadataResource struct{ client *arin.Client }
type netMetadataModel struct {
	ID       types.String `tfsdk:"id"`
	Handle   types.String `tfsdk:"handle"`
	Name     types.String `tfsdk:"name"`
	Comments types.List   `tfsdk:"comments"`
	POCs     types.Set    `tfsdk:"poc_links"`
}

func NewNetMetadataResource() resource.Resource { return &netMetadataResource{} }
func (r *netMetadataResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_net_metadata"
}
func (r *netMetadataResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Manage name, comments and POC links on an existing NET, including direct allocations. This resource never creates or deletes address-space registrations. Destroy leaves the last metadata in ARIN and removes Terraform management. Omitted fields preserve current values; use explicit empty collections to clear them. Do not configure overlapping fields for the same NET in multiple resources. When combining with arin_net, configure only POC links here. Import by NET handle.", Attributes: map[string]schema.Attribute{
		"id":       schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Existing NET handle."},
		"handle":   schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "NET registration to manage. It must already exist."},
		"name":     schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Network name. Omission preserves the current value."},
		"comments": schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType, MarkdownDescription: "Ordered operational comments. Omission preserves current comments; an empty list clears them."},
		"poc_links": schema.SetNestedAttribute{Optional: true, Computed: true, MarkdownDescription: "Explicit NET POC associations. Omission preserves existing associations; an empty set clears them. Organization-inherited contacts are separate. NETs accept Tech, NOC and Abuse POCs only.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"handle":   schema.StringAttribute{Required: true, MarkdownDescription: "POC handle."},
			"function": schema.StringAttribute{Required: true, MarkdownDescription: "One of AB (Abuse), N (NOC) or T (Tech)."},
		}}},
	}}
}
func (r *netMetadataResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (m netMetadataModel) pocs(ctx context.Context) ([]arin.NetPOC, diag.Diagnostics) {
	if m.POCs.IsUnknown() || m.POCs.IsNull() {
		return nil, nil
	}
	var models []irrPOCModel
	d := m.POCs.ElementsAs(ctx, &models, false)
	out := make([]arin.NetPOC, 0, len(models))
	for _, p := range models {
		out = append(out, arin.NetPOC{Handle: p.Handle.ValueString(), Function: p.Function.ValueString()})
	}
	return out, d
}
func (r *netMetadataResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m netMetadataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Handle.IsNull() && !m.Handle.IsUnknown() {
		if err := arin.ValidateCustomerContext(m.Handle.ValueString(), ""); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("handle"), "Invalid NET handle", err.Error())
		}
	}
	for _, v := range []attr.Value{m.Name, m.Comments, m.POCs} {
		tv, err := v.ToTerraformValue(ctx)
		if err != nil || !tv.IsFullyKnown() {
			return
		}
	}
	name := "EXAMPLE"
	if !m.Name.IsNull() {
		name = m.Name.ValueString()
	}
	var comments []string
	resp.Diagnostics.Append(m.Comments.ElementsAs(ctx, &comments, false)...)
	pocs, d := m.pocs(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := arin.ValidateNetMetadata(name, comments, pocs); err != nil {
		resp.Diagnostics.AddError("Invalid NET metadata", err.Error())
	}
}
func netMetadataState(ctx context.Context, n *arin.RegisteredNet) (netMetadataModel, diag.Diagnostics) {
	m := netMetadataModel{ID: types.StringValue(n.Handle), Handle: types.StringValue(n.Handle), Name: types.StringValue(n.Name)}
	var d, next diag.Diagnostics
	m.Comments, next = types.ListValueFrom(ctx, types.StringType, nonNilStrings(n.Comments))
	d.Append(next...)
	pocs := []irrPOCModel{}
	for _, p := range n.POCs {
		pocs = append(pocs, irrPOCModel{Handle: types.StringValue(p.Handle), Function: types.StringValue(p.Function)})
	}
	m.POCs, next = types.SetValueFrom(ctx, irrPOCType, pocs)
	d.Append(next...)
	return m, d
}

// write reads before applying configured fields. Unconfigured metadata remains
// server-owned, including associations changed since Terraform last refreshed.
func (r *netMetadataResource) write(ctx context.Context, m netMetadataModel, config netMetadataModel) (netMetadataModel, diag.Diagnostics) {
	var d diag.Diagnostics
	current, err := r.client.GetRegisteredNet(ctx, m.Handle.ValueString())
	if err != nil {
		d.AddError("Could not read existing NET", err.Error())
		return m, d
	}

	patch := arin.NetMetadataPatch{}
	if !config.Name.IsNull() {
		name := m.Name.ValueString()
		patch.Name = &name
	}
	if !config.Comments.IsNull() {
		var comments []string
		d.Append(m.Comments.ElementsAs(ctx, &comments, false)...)
		patch.Comments = &comments
	}
	if !config.POCs.IsNull() {
		pocs, next := m.pocs(ctx)
		d.Append(next...)
		patch.POCs = &pocs
	}
	if d.HasError() {
		return m, d
	}
	updated, err := r.client.UpdateNetMetadata(ctx, m.Handle.ValueString(), patch)
	if err != nil {
		// Keep a usable identity after an uncertain PUT. This resource only owns
		// metadata on an already-existing NET, so a retry never creates a second NET.
		out, next := netMetadataState(ctx, current)
		d.Append(next...)
		d.AddError("Could not update NET metadata", err.Error())
		return out, d
	}
	return netMetadataState(ctx, updated)
}
func (r *netMetadataResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m, config netMetadataModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, d := r.write(ctx, m, config)
	if !out.ID.IsNull() && !out.ID.IsUnknown() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
	}
	resp.Diagnostics.Append(d...)
}
func (r *netMetadataResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m netMetadataModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	n, err := r.client.GetRegisteredNet(ctx, m.Handle.ValueString())
	if arin.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read NET metadata", err.Error())
		return
	}
	out, d := netMetadataState(ctx, n)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
	}
}
func (r *netMetadataResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, config netMetadataModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, d := r.write(ctx, m, config)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
	}
}
func (r *netMetadataResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
func (r *netMetadataResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if err := arin.ValidateCustomerContext(req.ID, ""); err != nil {
		resp.Diagnostics.AddError("Invalid NET import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("handle"), req.ID)...)
}

// ModifyPlan keeps omitted fields stable when configured fields are unchanged,
// while allowing fresh values during a write that changes other metadata.
// Unconditional UseStateForUnknown would incorrectly freeze fields changed by
// an upstream arin_net resource in the same apply.
func (r *netMetadataResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var prior, plan, config netMetadataModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.Handle.Equal(prior.Handle) {
		return
	}
	if (!config.Name.IsNull() && !plan.Name.Equal(prior.Name)) ||
		(!config.Comments.IsNull() && !plan.Comments.Equal(prior.Comments)) ||
		(!config.POCs.IsNull() && !plan.POCs.Equal(prior.POCs)) {
		return
	}
	if config.Name.IsNull() {
		plan.Name = prior.Name
	}
	if config.Comments.IsNull() {
		plan.Comments = prior.Comments
	}
	if config.POCs.IsNull() {
		plan.POCs = prior.POCs
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}
