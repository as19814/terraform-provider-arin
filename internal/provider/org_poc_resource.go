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
	_ resource.Resource                   = &orgPOCResource{}
	_ resource.ResourceWithConfigure      = &orgPOCResource{}
	_ resource.ResourceWithValidateConfig = &orgPOCResource{}
	_ resource.ResourceWithImportState    = &orgPOCResource{}
)

type orgPOCResource struct{ client *arin.Client }
type orgPOCModel struct {
	ID       types.String `tfsdk:"id"`
	Org      types.String `tfsdk:"org_handle"`
	POC      types.String `tfsdk:"poc_handle"`
	Function types.String `tfsdk:"function"`
}

func NewOrgPOCResource() resource.Resource { return &orgPOCResource{} }
func (r *orgPOCResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_poc"
}
func (r *orgPOCResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{MarkdownDescription: "Manage one non-admin POC association on an existing organization. Both the organization and POC must already exist. Import an existing association first. Destroy removes only this association, preserving both objects and other links. Do not overlap ownership with full organization POC management. ARIN rejects removal of required contacts, including the last Tech POC. Admin changes require a full organization update and are not supported by this endpoint.", Attributes: map[string]schema.Attribute{
		"id":         schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "ORG-HANDLE/POC-HANDLE/FUNCTION."},
		"org_handle": schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "Existing organization handle. Changes require replacement."},
		"poc_handle": schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "Existing POC handle. Changes require replacement."},
		"function":   schema.StringAttribute{Required: true, PlanModifiers: replace, MarkdownDescription: "T (Tech), N (NOC), AB (Abuse), R (Routing), or D (DNS). Changes require replacement."},
	}}
}
func (r *orgPOCResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *orgPOCResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m orgPOCModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, p := range []types.String{m.Org, m.POC} {
		if !p.IsUnknown() {
			if err := arin.ValidatePOCHandle(p.ValueString()); err != nil {
				resp.Diagnostics.AddError("Invalid association handle", err.Error())
			}
		}
	}
	if !m.Function.IsUnknown() {
		if err := arin.ValidateOrgPOCFunction(m.Function.ValueString()); err != nil {
			resp.Diagnostics.AddError("Invalid organization POC role", err.Error())
		}
	}
}
func (m orgPOCModel) exists(links []arin.OrgPOC) bool {
	for _, p := range links {
		if p.Handle == m.POC.ValueString() && p.Function == m.Function.ValueString() {
			return true
		}
	}
	return false
}
func (m *orgPOCModel) identity() {
	m.ID = types.StringValue(m.Org.ValueString() + "/" + m.POC.ValueString() + "/" + m.Function.ValueString())
}
func (r *orgPOCResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m orgPOCModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	links, err := r.client.GetOrganizationPOCs(ctx, m.Org.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read organization POCs", err.Error())
		return
	}
	if m.exists(links) {
		resp.Diagnostics.AddError("Association already exists", "Import the existing association before managing it.")
		return
	}
	m.identity()
	_, err = r.client.AddOrganizationPOC(ctx, m.Org.ValueString(), m.POC.ValueString(), m.Function.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	if err != nil {
		resp.Diagnostics.AddError("Could not add organization POC", err.Error()+". The association identity was retained for refresh and recovery.")
	}
}
func (r *orgPOCResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m orgPOCModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	links, err := r.client.GetOrganizationPOCs(ctx, m.Org.ValueString())
	if arin.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read organization POCs", err.Error())
		return
	}
	if !m.exists(links) {
		resp.State.RemoveResource(ctx)
		return
	}
	m.identity()
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *orgPOCResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Association replacement required", "Changes to organization POC associations require replacement.")
}
func (r *orgPOCResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m orgPOCModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	links, err := r.client.GetOrganizationPOCs(ctx, m.Org.ValueString())
	if arin.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read organization POCs", err.Error())
		return
	}
	if !m.exists(links) {
		return
	}
	_, err = r.client.RemoveOrganizationPOC(ctx, m.Org.ValueString(), m.POC.ValueString(), m.Function.ValueString())
	if err != nil && !arin.IsNotFound(err) {
		resp.Diagnostics.AddError("Could not remove organization POC", err.Error())
		return
	}
	links, err = r.client.GetOrganizationPOCs(ctx, m.Org.ValueString())
	if arin.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not verify association removal", err.Error())
		return
	}
	if m.exists(links) {
		resp.Diagnostics.AddError("Association removal is incomplete", "ARIN still returns the association. Terraform retained its state.")
	}
}
func (r *orgPOCResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || arin.ValidatePOCHandle(parts[0]) != nil || arin.ValidatePOCHandle(parts[1]) != nil || arin.ValidateOrgPOCFunction(parts[2]) != nil {
		resp.Diagnostics.AddError("Invalid association import ID", "Use ORG-HANDLE/POC-HANDLE/FUNCTION, for example EXAMPLE-1/EXAMPLE-ARIN/T.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org_handle"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("poc_handle"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("function"), parts[2])...)
}
