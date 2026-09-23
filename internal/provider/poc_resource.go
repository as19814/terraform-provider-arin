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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &pocResource{}
	_ resource.ResourceWithConfigure      = &pocResource{}
	_ resource.ResourceWithValidateConfig = &pocResource{}
	_ resource.ResourceWithImportState    = &pocResource{}
	_ resource.ResourceWithModifyPlan     = &pocResource{}
)

type pocResource struct{ client *arin.Client }
type pocModel struct {
	ID                 types.String `tfsdk:"id"`
	ContactType        types.String `tfsdk:"contact_type"`
	CompanyName        types.String `tfsdk:"company_name"`
	FirstName          types.String `tfsdk:"first_name"`
	MiddleName         types.String `tfsdk:"middle_name"`
	LastName           types.String `tfsdk:"last_name"`
	RegistrationDate   types.String `tfsdk:"registration_date"`
	CountryCode        types.String `tfsdk:"country_code"`
	CountryName        types.String `tfsdk:"country_name"`
	CountryCode3       types.String `tfsdk:"country_code3"`
	CountryCallingCode types.String `tfsdk:"country_calling_code"`
	City               types.String `tfsdk:"city"`
	Subdivision        types.String `tfsdk:"subdivision"`
	PostalCode         types.String `tfsdk:"postal_code"`
	Street             types.List   `tfsdk:"street_address"`
	Comments           types.List   `tfsdk:"comments"`
	Emails             types.Set    `tfsdk:"emails"`
	Phones             types.Set    `tfsdk:"phones"`
}
type pocPhoneModel struct {
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	Number      types.String `tfsdk:"number"`
	Extension   types.String `tfsdk:"extension"`
}

var pocPhoneType = types.ObjectType{AttrTypes: map[string]attr.Type{"type": types.StringType, "number": types.StringType, "extension": types.StringType, "description": types.StringType}}

func NewPOCResource() resource.Resource { return &pocResource{} }
func (r *pocResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_poc"
}
func (r *pocResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optional := func(desc string, replace bool) schema.StringAttribute {
		a := schema.StringAttribute{Optional: true, Computed: true, Sensitive: true, Default: stringdefault.StaticString(""), MarkdownDescription: desc}
		if replace {
			a.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
		}
		return a
	}
	resp.Schema = schema.Schema{MarkdownDescription: "Manage an ARIN role or person point of contact. New POCs are linked to the API key's ARIN Online account. Contact type and first, middle and last names require replacement. This resource owns all email addresses and phone numbers; do not overlap with other writers of those collections. Remove dependent organization and NET associations before destroy. Contact attributes are sensitive in output but remain in Terraform state. Import by POC handle.", Attributes: map[string]schema.Attribute{
		"id":                   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "ARIN-generated POC handle."},
		"contact_type":         schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "ROLE or PERSON. Immutable after creation."},
		"company_name":         optional("Company name. Required for ROLE contacts.", false),
		"first_name":           optional("First name. Required for PERSON and empty for ROLE contacts. Immutable after creation.", true),
		"middle_name":          optional("Middle name. Immutable after creation.", true),
		"last_name":            schema.StringAttribute{Required: true, Sensitive: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Last name for a PERSON, or role name for a ROLE. Immutable after creation."},
		"country_code":         schema.StringAttribute{Required: true, MarkdownDescription: "Uppercase two-letter country code."},
		"country_name":         schema.StringAttribute{Computed: true, MarkdownDescription: "Country name returned by ARIN."},
		"country_code3":        schema.StringAttribute{Computed: true, MarkdownDescription: "Three-letter country code returned by ARIN."},
		"country_calling_code": schema.StringAttribute{Computed: true, MarkdownDescription: "E.164 country calling code returned by ARIN, retained as text."},
		"city":                 optional("City. Omission sends an empty value.", false),
		"subdivision":          optional("State/province code. Required for US and CA contacts.", false),
		"postal_code":          optional("Postal code. Required for US and CA contacts.", false),
		"street_address":       schema.ListAttribute{Required: true, Sensitive: true, ElementType: types.StringType, MarkdownDescription: "Ordered address lines. At least one is required."},
		"comments":             schema.ListAttribute{Optional: true, Computed: true, Sensitive: true, ElementType: types.StringType, Default: listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})), MarkdownDescription: "Ordered operational comments. Omission clears comments."},
		"registration_date":    schema.StringAttribute{Computed: true, MarkdownDescription: "Server-generated registration date, preserved on update."},
		"emails":               schema.SetAttribute{Required: true, Sensitive: true, ElementType: types.StringType, MarkdownDescription: "Complete collection of email addresses. At least one is required."},
		"phones": schema.SetNestedAttribute{Required: true, Sensitive: true, MarkdownDescription: "Complete phone collection. At least one office phone is required; each type/number pair must be unique.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"type":        schema.StringAttribute{Required: true, MarkdownDescription: "O (office), F (fax), or M (mobile)."},
			"number":      schema.StringAttribute{Required: true, MarkdownDescription: "Phone number, including international dialing prefix, for example +1-202-555-0100."},
			"extension":   schema.StringAttribute{Optional: true, Computed: true, Sensitive: true, MarkdownDescription: "Phone extension. Omission clears it."},
			"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Phone type description returned by ARIN."},
		}}},
	}}
}
func (r *pocResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (m pocModel) api(ctx context.Context) (arin.POC, diag.Diagnostics) {
	p := arin.POC{Handle: m.ID.ValueString(), ContactType: m.ContactType.ValueString(), CompanyName: m.CompanyName.ValueString(), FirstName: m.FirstName.ValueString(), MiddleName: m.MiddleName.ValueString(), LastName: m.LastName.ValueString(), CountryCode: m.CountryCode.ValueString(), City: m.City.ValueString(), Subdivision: m.Subdivision.ValueString(), PostalCode: m.PostalCode.ValueString()}
	var d diag.Diagnostics
	d.Append(m.Street.ElementsAs(ctx, &p.StreetAddress, false)...)
	d.Append(m.Comments.ElementsAs(ctx, &p.Comments, false)...)
	d.Append(m.Emails.ElementsAs(ctx, &p.Emails, false)...)
	var phones []pocPhoneModel
	d.Append(m.Phones.ElementsAs(ctx, &phones, false)...)
	for _, ph := range phones {
		p.Phones = append(p.Phones, arin.POCPhone{Type: ph.Type.ValueString(), Number: ph.Number.ValueString(), Extension: ph.Extension.ValueString()})
	}
	return p, d
}
func (r *pocResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m pocModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, v := range []attr.Value{m.ContactType, m.CompanyName, m.FirstName, m.MiddleName, m.LastName, m.CountryCode, m.City, m.Subdivision, m.PostalCode, m.Street, m.Comments, m.Emails, m.Phones} {
		tv, err := v.ToTerraformValue(ctx)
		if err != nil || !tv.IsFullyKnown() {
			return
		}
	}
	p, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		if err := p.Validate(); err != nil {
			resp.Diagnostics.AddError("Invalid POC", err.Error())
		}
	}
}
func pocState(ctx context.Context, p *arin.POC) (pocModel, diag.Diagnostics) {
	m := pocModel{ID: types.StringValue(p.Handle), ContactType: types.StringValue(p.ContactType), CompanyName: types.StringValue(p.CompanyName), FirstName: types.StringValue(p.FirstName), MiddleName: types.StringValue(p.MiddleName), LastName: types.StringValue(p.LastName), CountryCode: types.StringValue(p.CountryCode), CountryName: types.StringValue(p.CountryName), CountryCode3: types.StringValue(p.CountryCode3), CountryCallingCode: types.StringValue(p.CountryCallingCode), City: types.StringValue(p.City), Subdivision: types.StringValue(p.Subdivision), PostalCode: types.StringValue(p.PostalCode), RegistrationDate: types.StringValue(p.RegistrationDate)}
	var d, next diag.Diagnostics
	m.Street, next = types.ListValueFrom(ctx, types.StringType, nonNilStrings(p.StreetAddress))
	d.Append(next...)
	m.Comments, next = types.ListValueFrom(ctx, types.StringType, nonNilStrings(p.Comments))
	d.Append(next...)
	m.Emails, next = types.SetValueFrom(ctx, types.StringType, nonNilStrings(p.Emails))
	d.Append(next...)
	phones := []pocPhoneModel{}
	for _, ph := range p.Phones {
		phones = append(phones, pocPhoneModel{Description: types.StringValue(ph.Description), Type: types.StringValue(ph.Type), Number: types.StringValue(ph.Number), Extension: types.StringValue(ph.Extension)})
	}
	m.Phones, next = types.SetValueFrom(ctx, pocPhoneType, phones)
	d.Append(next...)
	return m, d
}
func (r *pocResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m pocModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.CreatePOC(ctx, s)
	if err != nil {
		resp.Diagnostics.AddError("Could not create POC", err.Error()+". Creation is not automatically retried; if the outcome is uncertain, locate and import the new POC before retrying.")
		return
	}
	m, d = pocState(ctx, out)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *pocResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m pocModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.GetPOC(ctx, m.ID.ValueString())
	if arin.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read POC", err.Error())
		return
	}
	m, d := pocState(ctx, out)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *pocResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m pocModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client.UpdatePOC(ctx, s)
	if err != nil {
		resp.Diagnostics.AddError("Could not update POC", err.Error())
		return
	}
	m, d = pocState(ctx, out)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *pocResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m pocModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeletePOC(ctx, m.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not delete POC", err.Error())
	}
}

func (r *pocResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if err := arin.ValidatePOCHandle(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid POC import ID", "Use an ARIN POC handle.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// ModifyPlan defaults omitted extensions after unknown marking. A nested static
// default changes set element identity before computed descriptions are marked
// unknown, which can erase configured extensions or leave descriptions null.
func (r *pocResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan, config pocModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || plan.Phones.IsNull() || plan.Phones.IsUnknown() || config.Phones.IsNull() || config.Phones.IsUnknown() {
		return
	}
	var phones, configured []pocPhoneModel
	resp.Diagnostics.Append(plan.Phones.ElementsAs(ctx, &phones, false)...)
	resp.Diagnostics.Append(config.Phones.ElementsAs(ctx, &configured, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	changed := false
	for i, phone := range phones {
		if phone.Type.IsUnknown() || phone.Number.IsUnknown() {
			continue
		}
		for _, input := range configured {
			if input.Type.Equal(phone.Type) && input.Number.Equal(phone.Number) && input.Extension.IsNull() {
				phones[i].Extension = types.StringValue("")
				changed = true
				break
			}
		}
	}
	if changed {
		value, d := types.SetValueFrom(ctx, pocPhoneType, phones)
		resp.Diagnostics.Append(d...)
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("phones"), value)...)
		}
	}
}
