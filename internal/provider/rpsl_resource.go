package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.ResourceWithConfigure      = &rpslResource{}
	_ resource.ResourceWithImportState    = &rpslResource{}
	_ resource.ResourceWithValidateConfig = &rpslResource{}
)

type rpslResource struct{ client *arin.Client }
type rpslModel struct {
	ID      types.String `tfsdk:"id"`
	Kind    types.String `tfsdk:"object_type"`
	Name    types.String `tfsdk:"name"`
	Origin  types.String `tfsdk:"origin_as"`
	Org     types.String `tfsdk:"org_handle"`
	Text    types.String `tfsdk:"rpsl"`
	Remote  types.String `tfsdk:"remote_rpsl"`
	Pending types.Bool   `tfsdk:"pending_creation"`
}

func NewRPSLResource() resource.Resource { return &rpslResource{} }
func (r *rpslResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_irr_rpsl"
}
func (r *rpslResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{MarkdownDescription: "Manage one advanced ARIN IRR object through RPSL. Supports route, route6, as-set, route-set and aut-num. Owns the complete policy payload; ARIN validates routing-policy syntax and contact authorization. Identity and organization changes require replacement. Alignment, attribute-name ordering, folded continuation lines and server timestamps do not cause drift; repeated attribute ordering and policy values remain significant. Import existing advanced objects before managing them. Destroy deletes the IRR object, not its underlying network or ASN registration. Do not manage the same object through an XML resource.", Attributes: map[string]schema.Attribute{
		"id":               schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Import identity: type/name, with ,AS<number> appended for routes."},
		"object_type":      schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "route, route6, as-set, route-set or aut-num."},
		"name":             schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "Canonical prefix, uppercase set name or AS<number>, matching the first RPSL attribute."},
		"origin_as":        schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), PlanModifiers: replace, MarkdownDescription: "Required for routes; empty for other objects. Must match the origin attribute."},
		"org_handle":       schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "Maintaining organization. The RPSL mnt-by must be MNT- followed by this handle."},
		"rpsl":             schema.StringAttribute{Required: true, MarkdownDescription: "Complete single-object RPSL payload. Include identity, source: ARIN, mnt-by and the required contact/policy attributes. Server-created timestamps are ignored in comparisons. Configured formatting is retained when the remote policy matches."},
		"remote_rpsl":      schema.StringAttribute{Computed: true, MarkdownDescription: "Complete latest server representation, including ARIN formatting and timestamps."},
		"pending_creation": schema.BoolAttribute{Computed: true, MarkdownDescription: "True after an uncertain creation. Blocks deletion and replacement until the object is reconciled and imported."},
	}}
}
func (r *rpslResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (m rpslModel) key() arin.RPSLKey {
	return arin.RPSLKey{Kind: m.Kind.ValueString(), Name: m.Name.ValueString(), OriginAS: m.Origin.ValueString()}
}
func (m rpslModel) validate() error {
	if err := m.key().Validate(); err != nil {
		return err
	}
	object, err := arin.ParseRPSL(m.Text.ValueString())
	if err != nil {
		return err
	}
	if object.Key != m.key() || object.OrgHandle != m.Org.ValueString() {
		return errors.New("RPSL identity and mnt-by must match object_type, name, origin_as and org_handle")
	}
	return nil
}
func (r *rpslResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m rpslModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, v := range []attr.Value{m.Kind, m.Name, m.Origin, m.Org, m.Text} {
		tv, err := v.ToTerraformValue(ctx)
		if err != nil || !tv.IsFullyKnown() {
			return
		}
	}
	if err := m.validate(); err != nil {
		resp.Diagnostics.AddError("Invalid RPSL configuration", err.Error())
	}
}
func (m *rpslModel) observed(out *arin.RPSLObject) {
	m.ID = types.StringValue(out.Key.ID())
	m.Kind = types.StringValue(out.Key.Kind)
	m.Name = types.StringValue(out.Key.Name)
	m.Origin = types.StringValue(out.Key.OriginAS)
	m.Org = types.StringValue(out.OrgHandle)
	if m.Text.IsNull() || m.Text.IsUnknown() || !arin.EqualRPSL(m.Text.ValueString(), out.Text) {
		m.Text = types.StringValue(out.Text)
	}
	m.Remote = types.StringValue(out.Text)
}
func (r *rpslResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m rpslModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := m.validate(); err != nil {
		resp.Diagnostics.AddError("Invalid RPSL configuration", err.Error())
		return
	}
	_, err := r.client.CreateRPSL(ctx, m.Text.ValueString())
	if err != nil {
		var write *arin.RPSLWriteError
		var api *arin.APIError
		if !errors.As(err, &write) || (errors.As(err, &api) && api.StatusCode >= 400 && api.StatusCode < 500 && api.StatusCode != 408) {
			resp.Diagnostics.AddError("Could not create advanced IRR object", err.Error())
			return
		}
	}
	// A fresh GET verifies both ordinary writes and accepted writes with a lost response.
	out, readErr := r.client.GetRPSL(ctx, m.key())
	if readErr == nil && out.OrgHandle == m.Org.ValueString() && arin.EqualRPSL(m.Text.ValueString(), out.Text) {
		m.observed(out)
		m.Pending = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		return
	}
	m.ID = types.StringValue(m.key().ID())
	m.Pending = types.BoolValue(true)
	m.Remote = types.StringNull()
	if readErr == nil && out.OrgHandle == m.Org.ValueString() {
		m.Remote = types.StringValue(out.Text)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	resp.Diagnostics.AddError("Could not confirm advanced IRR creation", "The natural identity was retained and automatic replacement is blocked. Inspect the object and reconcile it using import before retrying. The server did not confirm the requested policy.")
}
func (r *rpslResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m rpslModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.GetRPSL(ctx, m.key())
	if m.Pending.ValueBool() {
		if err == nil && out.OrgHandle == m.Org.ValueString() {
			m.Remote = types.StringValue(out.Text)
			resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		}
		resp.Diagnostics.AddError("Uncertain advanced IRR creation", "Creation is unresolved. Preserve a state backup, inspect the object, then remove this receipt from state and import the confirmed object. Do not recreate it blindly.")
		return
	}
	if arin.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read advanced IRR object", err.Error())
		return
	}
	if !m.Org.IsNull() && !m.Org.IsUnknown() && out.OrgHandle != m.Org.ValueString() {
		resp.Diagnostics.AddError("Advanced IRR ownership changed", "The maintaining organization differs from state; refusing to adopt it.")
		return
	}
	m.observed(out)
	m.Pending = types.BoolValue(false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *rpslResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var prior, m rpslModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if prior.Pending.ValueBool() {
		resp.Diagnostics.AddError("Uncertain advanced IRR creation", "Reconcile and import the object before updating it.")
		return
	}
	if err := m.validate(); err != nil {
		resp.Diagnostics.AddError("Invalid RPSL configuration", err.Error())
		return
	}
	current, err := r.client.GetRPSL(ctx, m.key())
	if err != nil {
		resp.Diagnostics.AddError("Could not read advanced IRR object", err.Error())
		return
	}
	if current.OrgHandle != m.Org.ValueString() {
		resp.Diagnostics.AddError("Advanced IRR ownership changed", "Refusing to update an object with a different maintainer.")
		return
	}
	if !arin.EqualRPSL(m.Text.ValueString(), current.Text) {
		_, writeErr := r.client.UpdateRPSL(ctx, m.Text.ValueString())
		current, err = r.client.GetRPSL(ctx, m.key())
		if err != nil || current.OrgHandle != m.Org.ValueString() || !arin.EqualRPSL(m.Text.ValueString(), current.Text) {
			message := "Refresh before retrying. The server did not confirm the requested policy."
			if writeErr != nil {
				message = writeErr.Error() + ". " + message
			}
			resp.Diagnostics.AddError("Could not confirm advanced IRR update", message)
			return
		}
	}
	m.observed(current)
	m.Pending = types.BoolValue(false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *rpslResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m rpslModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Pending.ValueBool() {
		resp.Diagnostics.AddError("Uncertain advanced IRR creation", "Automatic deletion and replacement are blocked. Inspect and import the confirmed object first.")
		return
	}
	err := r.client.DeleteRPSL(ctx, m.key(), m.Org.ValueString())
	_, readErr := r.client.GetRPSL(ctx, m.key())
	if arin.IsNotFound(readErr) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not delete advanced IRR object", err.Error())
		return
	}
	resp.Diagnostics.AddError("Could not confirm advanced IRR deletion", "The object is still present or could not be read. Refresh before retrying.")
}
func (r *rpslResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	key, err := arin.ParseRPSLID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid RPSL import ID", err.Error())
		return
	}
	for name, value := range map[string]string{"id": key.ID(), "object_type": key.Kind, "name": key.Name, "origin_as": key.OriginAS} {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(name), value)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pending_creation"), false)...)
}
