package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &aspaResource{}
	_ resource.ResourceWithConfigure      = &aspaResource{}
	_ resource.ResourceWithImportState    = &aspaResource{}
	_ resource.ResourceWithValidateConfig = &aspaResource{}
)

type aspaResource struct{ client *arin.Client }
type aspaModel struct {
	ID        types.String `tfsdk:"id"`
	Org       types.String `tfsdk:"org_handle"`
	Customer  types.Int64  `tfsdk:"customer_asn"`
	Providers types.Set    `tfsdk:"provider_asns"`
}

func NewASPAResource() resource.Resource { return &aspaResource{} }
func (r *aspaResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_aspa"
}
func (r *aspaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Manage one hosted ASPA and its complete provider-AS set. The customer ASN must belong to the organization and be eligible for hosted RPKI. Import an existing ASPA first. Provider changes use one atomic delete/add transaction; unrelated ASPAs and ROAs are preserved. Destroy removes only this ASPA. ARIN rejects reserved provider ASNs except AS0 as the sole provider.", Attributes: map[string]schema.Attribute{
		"id":            schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "ORG-HANDLE/CUSTOMER-ASN."},
		"org_handle":    schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Existing organization handle. Changes require replacement."},
		"customer_asn":  schema.Int64Attribute{Required: true, PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()}, MarkdownDescription: "Customer AS number owned by the organization. Changes require replacement."},
		"provider_asns": schema.SetAttribute{Required: true, ElementType: types.Int64Type, MarkdownDescription: "Complete nonempty set of provider AS numbers. Must exclude the customer ASN. Use [0] to declare no providers; AS0 cannot be mixed with other ASNs."},
	}}
}
func (r *aspaResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (m aspaModel) api(ctx context.Context) (arin.ASPA, diag.Diagnostics) {
	a := arin.ASPA{CustomerASN: m.Customer.ValueInt64()}
	d := m.Providers.ElementsAs(ctx, &a.ProviderASNs, false)
	return a, d
}
func (r *aspaResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m aspaModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Org.IsUnknown() {
		if err := arin.ValidatePOCHandle(m.Org.ValueString()); err != nil {
			resp.Diagnostics.AddError("Invalid organization handle", err.Error())
			return
		}
	}
	for _, v := range []attr.Value{m.Customer, m.Providers} {
		tv, err := v.ToTerraformValue(ctx)
		if err != nil || !tv.IsFullyKnown() {
			return
		}
	}
	a, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		if err := a.Validate(); err != nil {
			resp.Diagnostics.AddError("Invalid ASPA", err.Error())
		}
	}
}
func (m *aspaModel) identity() {
	m.ID = types.StringValue(m.Org.ValueString() + "/" + strconv.FormatInt(m.Customer.ValueInt64(), 10))
}
func (r *aspaResource) find(ctx context.Context, m aspaModel) (*arin.ASPA, error) {
	values, err := r.client.ListASPAs(ctx, m.Org.ValueString())
	if err != nil {
		return nil, err
	}
	for _, a := range values {
		if a.CustomerASN == m.Customer.ValueInt64() {
			return &a, nil
		}
	}
	return nil, nil
}
func (r *aspaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m aspaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := a.Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid ASPA", err.Error())
		return
	}
	current, err := r.find(ctx, m)
	if err != nil {
		resp.Diagnostics.AddError("Could not read ASPA inventory", err.Error())
		return
	}
	if current != nil {
		resp.Diagnostics.AddError("ASPA already exists", "Import the existing ORG-HANDLE/CUSTOMER-ASN before managing it.")
		return
	}
	m.identity()
	result, err := r.client.ApplyRPKITransaction(ctx, m.Org.ValueString(), arin.RPKITransaction{AddASPAs: []arin.ASPA{a}})
	if result == nil && netDefinitiveFailure(err) {
		resp.Diagnostics.AddError("Could not create ASPA", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	if err != nil {
		resp.Diagnostics.AddError("Could not confirm ASPA creation", err.Error()+". Identity was saved for refresh and recovery; the POST was not replayed.")
	}
}
func (r *aspaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m aspaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.find(ctx, m)
	if arin.IsNotFound(err) || (err == nil && a == nil) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read ASPA", err.Error())
		return
	}
	m.identity()
	p, d := types.SetValueFrom(ctx, types.Int64Type, a.ProviderASNs)
	m.Providers = p
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *aspaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m aspaModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := a.Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid ASPA", err.Error())
		return
	}
	current, err := r.find(ctx, m)
	if err != nil {
		resp.Diagnostics.AddError("Could not read ASPA", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError("ASPA disappeared", "Refresh before recreating the missing ASPA.")
		return
	}
	if _, err = r.client.ApplyRPKITransaction(ctx, m.Org.ValueString(), arin.RPKITransaction{DeleteASPAs: []int64{a.CustomerASN}, AddASPAs: []arin.ASPA{a}}); err != nil {
		resp.Diagnostics.AddError("Could not update ASPA", err.Error()+". The transaction was not replayed; refresh before retrying.")
		return
	}
	m.identity()
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *aspaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m aspaModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.find(ctx, m)
	if arin.IsNotFound(err) || (err == nil && current == nil) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read ASPA", err.Error())
		return
	}
	if _, err = r.client.ApplyRPKITransaction(ctx, m.Org.ValueString(), arin.RPKITransaction{DeleteASPAs: []int64{m.Customer.ValueInt64()}}); err != nil {
		resp.Diagnostics.AddError("Could not remove ASPA", err.Error()+". State was retained; refresh to reconcile the result.")
	}
}
func (r *aspaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || arin.ValidatePOCHandle(parts[0]) != nil {
		resp.Diagnostics.AddError("Invalid ASPA import ID", "Use ORG-HANDLE/CUSTOMER-ASN.")
		return
	}
	asn, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || asn < 1 || asn > 4294967295 || strconv.FormatInt(asn, 10) != parts[1] {
		resp.Diagnostics.AddError("Invalid customer ASN", "Use a canonical decimal ASN between 1 and 4294967295.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org_handle"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("customer_asn"), asn)...)
}
