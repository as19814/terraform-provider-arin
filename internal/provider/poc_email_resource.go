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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &pocEmailResource{}
	_ resource.ResourceWithConfigure      = &pocEmailResource{}
	_ resource.ResourceWithValidateConfig = &pocEmailResource{}
	_ resource.ResourceWithImportState    = &pocEmailResource{}
)

type pocEmailResource struct{ client *arin.Client }
type pocEmailModel struct {
	ID     types.String `tfsdk:"id"`
	Handle types.String `tfsdk:"poc_handle"`
	Email  types.String `tfsdk:"email"`
}

func NewPOCEmailResource() resource.Resource { return &pocEmailResource{} }
func (r *pocEmailResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_poc_email"
}
func (r *pocEmailResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{MarkdownDescription: "Manage one email on an existing ARIN POC, preserving other contact details. Import an existing record before managing it. Destroy removes only this record. Do not overlap management of this email with arin_poc or another resource. Contact values remain in Terraform state. ARIN may reject removal of required contact details.", Attributes: map[string]schema.Attribute{
		"id":         schema.StringAttribute{Computed: true, Sensitive: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "POC-HANDLE/EMAIL."},
		"poc_handle": schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "Existing POC handle. Changes require replacement."},
		"email":      schema.StringAttribute{Required: true, Sensitive: true, PlanModifiers: replace, MarkdownDescription: "Plain email address. Changes require replacement."},
	}}
}
func (r *pocEmailResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *pocEmailResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m pocEmailModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Handle.IsUnknown() {
		if err := arin.ValidatePOCHandle(m.Handle.ValueString()); err != nil {
			resp.Diagnostics.AddError("Invalid POC handle", err.Error())
		}
	}
	if !m.Email.IsUnknown() {
		if err := arin.ValidatePOCEmail(m.Email.ValueString()); err != nil {
			resp.Diagnostics.AddError("Invalid email", err.Error())
		}
	}
}
func (m *pocEmailModel) exists(out *arin.POC) bool {
	for _, e := range out.Emails {
		if e == m.Email.ValueString() {
			return true
		}
	}
	return false
}
func (m *pocEmailModel) identity() {
	m.ID = types.StringValue(m.Handle.ValueString() + "/" + m.Email.ValueString())
}
func (r *pocEmailResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m pocEmailModel
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
	out, err := r.client.AddPOCEmail(ctx, m.Handle.ValueString(), m.Email.ValueString())
	if err != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		resp.Diagnostics.AddError("Could not add email", err.Error()+". The contact identity was retained for refresh and recovery.")
		return
	}
	if !m.exists(out) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		resp.Diagnostics.AddError("Contact addition not confirmed", "The record is missing from the resulting POC.")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *pocEmailResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m pocEmailModel
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
func (r *pocEmailResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Contact replacement required", "Changes to contact records require replacement.")
}
func (r *pocEmailResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m pocEmailModel
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
	_, err = r.client.DeletePOCEmail(ctx, m.Handle.ValueString(), m.Email.ValueString())
	if err != nil && !arin.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not remove email", err.Error())
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
func (r *pocEmailResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	handle, email, ok := strings.Cut(req.ID, "/")
	if !ok || arin.ValidatePOCHandle(handle) != nil || arin.ValidatePOCEmail(email) != nil {
		resp.Diagnostics.AddError("Invalid email import ID", "Use POC-HANDLE/EMAIL, for example EXAMPLE-ARIN/noc@example.net.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("poc_handle"), handle)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("email"), email)...)

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
