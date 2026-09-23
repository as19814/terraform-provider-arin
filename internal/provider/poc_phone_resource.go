package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &pocPhoneResource{}
	_ resource.ResourceWithConfigure      = &pocPhoneResource{}
	_ resource.ResourceWithValidateConfig = &pocPhoneResource{}
	_ resource.ResourceWithImportState    = &pocPhoneResource{}
)

type pocPhoneResource struct{ client *arin.Client }
type pocPhoneResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Handle    types.String `tfsdk:"poc_handle"`
	Number    types.String `tfsdk:"number"`
	Type      types.String `tfsdk:"type"`
	Extension types.String `tfsdk:"extension"`
}

func NewPOCPhoneResource() resource.Resource { return &pocPhoneResource{} }
func (r *pocPhoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_poc_phone"
}
func (r *pocPhoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{MarkdownDescription: "Manage one phone on an existing ARIN POC, preserving other contact details. Import an existing record before managing it. Destroy removes only this record. Do not overlap management of this phone with arin_poc or another resource. Contact values remain in Terraform state. ARIN may reject removal of required contact details. Extension changes require replacement; do not use create_before_destroy for the same type/number because ARIN treats duplicate additions as a no-op.", Attributes: map[string]schema.Attribute{
		"id":         schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "POC-HANDLE/TYPE/NUMBER."},
		"poc_handle": schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "Existing POC handle. Changes require replacement."},
		"number":     schema.StringAttribute{Required: true, Sensitive: true, PlanModifiers: replace, MarkdownDescription: "Phone number including its international dialing prefix, for example +1-202-555-0101. Changes require replacement."},
		"type":       schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "O (office), F (fax), or M (mobile). Changes require replacement."},
		"extension":  schema.StringAttribute{Optional: true, Computed: true, Sensitive: true, Default: stringdefault.StaticString(""), PlanModifiers: replace, MarkdownDescription: "Phone extension. Omission means no extension. Changes require delete and re-add because ARIN's individual endpoint cannot update extensions."},
	}}
}
func (r *pocPhoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pocPhoneResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m pocPhoneResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Handle.IsUnknown() {
		if err := arin.ValidatePOCHandle(m.Handle.ValueString()); err != nil {
			resp.Diagnostics.AddError("Invalid POC handle", err.Error())
		}
	}
	if !m.Number.IsUnknown() && !m.Type.IsUnknown() && !m.Extension.IsUnknown() {
		if err := (arin.POCPhone{Type: m.Type.ValueString(), Number: m.Number.ValueString(), Extension: m.Extension.ValueString()}).Validate(); err != nil {
			resp.Diagnostics.AddError("Invalid phone", err.Error())
		}
	}
}
func (m *pocPhoneResourceModel) exists(out *arin.POC) bool {
	for _, p := range out.Phones {
		if p.Type == m.Type.ValueString() && p.Number == m.Number.ValueString() {
			m.Extension = types.StringValue(p.Extension)
			return true
		}
	}
	return false
}
func (m *pocPhoneResourceModel) identity() {
	m.ID = types.StringValue(m.Handle.ValueString() + "/" + m.Type.ValueString() + "/" + m.Number.ValueString())
}
func (r *pocPhoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m pocPhoneResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client.GetPOC(ctx, m.Handle.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read POC", err.Error())
		return
	}
	check := m
	if check.exists(current) {
		resp.Diagnostics.AddError("Contact record already exists", "Import the existing record before managing it.")
		return
	}
	m.identity()
	out, err := r.client.AddPOCPhone(ctx, m.Handle.ValueString(), arin.POCPhone{Type: m.Type.ValueString(), Number: m.Number.ValueString(), Extension: m.Extension.ValueString()})
	if err != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		resp.Diagnostics.AddError("Could not add phone", err.Error()+". The contact identity was retained for refresh and recovery.")
		return
	}
	if !m.exists(out) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		resp.Diagnostics.AddError("Contact addition not confirmed", "The record is missing from the resulting POC.")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *pocPhoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m pocPhoneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.GetPOC(ctx, m.Handle.ValueString())
	if arin.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read contact", err.Error())
		return
	}
	if !m.exists(out) {
		resp.State.RemoveResource(ctx)
		return
	}
	m.identity()
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *pocPhoneResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Contact replacement required", "Changes to contact records require replacement.")
}
func (r *pocPhoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m pocPhoneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.GetPOC(ctx, m.Handle.ValueString())
	if arin.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read POC", err.Error())
		return
	}
	if !m.exists(out) {
		return
	}
	_, err = r.client.DeletePOCPhones(ctx, m.Handle.ValueString(), m.Number.ValueString(), m.Type.ValueString())
	if err != nil && !arin.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not remove phone", err.Error())
		return
	}
	out, err = r.client.GetPOC(ctx, m.Handle.ValueString())
	if arin.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not verify contact removal", err.Error())
		return
	}
	if m.exists(out) {
		resp.Diagnostics.AddError("Contact removal is incomplete", "The contact remains present. Terraform retained its state.")
	}
}
func (r *pocPhoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	handle, rest, ok := strings.Cut(req.ID, "/")
	typ, number, hasType := strings.Cut(rest, "/")
	if !ok || !hasType || arin.ValidatePOCHandle(handle) != nil || (arin.POCPhone{Type: typ, Number: number}).Validate() != nil {
		resp.Diagnostics.AddError("Invalid phone import ID", "Use POC-HANDLE/TYPE/NUMBER, for example EXAMPLE-ARIN/F/+1-202-555-0101.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("poc_handle"), handle)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), typ)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("number"), number)...)

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
