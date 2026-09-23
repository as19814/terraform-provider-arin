package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &orgResource{}
	_ resource.ResourceWithConfigure      = &orgResource{}
	_ resource.ResourceWithImportState    = &orgResource{}
	_ resource.ResourceWithValidateConfig = &orgResource{}
)

type orgResource struct{ client *arin.Client }
type orgModel struct {
	ID               types.String `tfsdk:"id"`
	Handle           types.String `tfsdk:"handle"`
	Name             types.String `tfsdk:"name"`
	DBAName          types.String `tfsdk:"dba_name"`
	Date             types.String `tfsdk:"registration_date"`
	CountryCode      types.String `tfsdk:"country_code"`
	CountryName      types.String `tfsdk:"country_name"`
	City             types.String `tfsdk:"city"`
	Subdivision      types.String `tfsdk:"subdivision"`
	PostalCode       types.String `tfsdk:"postal_code"`
	TaxID            types.String `tfsdk:"tax_id"`
	RWhoisURL        types.String `tfsdk:"rwhois_url"`
	Accept           types.Bool   `tfsdk:"accept_reassignments"`
	Street           types.List   `tfsdk:"street_address"`
	Comments         types.List   `tfsdk:"comments"`
	POCs             types.Set    `tfsdk:"poc_links"`
	PendingOperation types.String `tfsdk:"pending_operation"`
	PendingTicket    types.String `tfsdk:"pending_ticket"`
}

func NewOrgResource() resource.Resource { return &orgResource{} }
func (r *orgResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org"
}
func (r *orgResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optional := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Sensitive: true, Default: stringdefault.StaticString(""), MarkdownDescription: desc}
	}
	dba := optional("Immutable doing-business-as name. Changes require replacement.")
	dba.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{MarkdownDescription: "Manage a complete ARIN organization, including its authoritative POC collection. Creation can require staff review. Pending or uncertain writes retain recovery state and block repeat submission. If creation returns only a ticket, obtain the approved handle from ARIN and import it after reconciling that ticket; see the organization recovery guide. Remove dependent resources before destroy. Do not overlap POC ownership with arin_org_poc. Import existing organizations by handle. Name, address and tax information are sensitive but remain in state.", Attributes: map[string]schema.Attribute{
		"id":                   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Organization handle, or a pending recovery identifier."},
		"handle":               schema.StringAttribute{Computed: true, MarkdownDescription: "ARIN-generated organization handle. Empty until a pending creation is confirmed."},
		"name":                 schema.StringAttribute{Required: true, Sensitive: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Immutable legal organization name. Changes require replacement."},
		"dba_name":             dba,
		"country_code":         schema.StringAttribute{Required: true, MarkdownDescription: "Uppercase two-letter country code."},
		"country_name":         schema.StringAttribute{Computed: true, MarkdownDescription: "Country name returned by ARIN."},
		"city":                 optional("City. Omission sends an empty value."),
		"subdivision":          optional("State or province code, required for US and CA."),
		"postal_code":          optional("Postal code, required for US and CA."),
		"tax_id":               optional("Tax identifier. Omission sends no tax ID element."),
		"rwhois_url":           optional("Referral Whois server hostname and port, not a company website URL."),
		"street_address":       schema.ListAttribute{Required: true, Sensitive: true, ElementType: types.StringType, MarkdownDescription: "Ordered street address lines."},
		"comments":             schema.ListAttribute{Optional: true, Computed: true, Sensitive: true, ElementType: types.StringType, Default: listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})), MarkdownDescription: "Ordered operational comments. Omission clears comments."},
		"accept_reassignments": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Accept incoming reassignments and reallocations. Defaults to true."},
		"poc_links": schema.SetNestedAttribute{Required: true, MarkdownDescription: "Complete POC collection: exactly one AD (Admin), at least one T (Tech), and one AB (Abuse). N, R and D are also supported. Keep a POC linked to the API account to preserve management access.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"handle":   schema.StringAttribute{Required: true, MarkdownDescription: "Existing POC handle."},
			"function": schema.StringAttribute{Required: true, MarkdownDescription: "AD, T, AB, N, R or D."},
		}}},
		"registration_date": schema.StringAttribute{Computed: true, MarkdownDescription: "Generated registration date, preserved during updates."},
		"pending_operation": schema.StringAttribute{Computed: true, MarkdownDescription: "Empty when complete, otherwise create, update or delete requiring reconciliation."},
		"pending_ticket":    schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Unresolved ARIN ticket, or empty for an uncertain outcome without a ticket."},
	}}
}
func (r *orgResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (m orgModel) api(ctx context.Context) (arin.RegisteredOrganization, diag.Diagnostics) {
	out := arin.RegisteredOrganization{Handle: m.Handle.ValueString(), Name: m.Name.ValueString(), DBAName: m.DBAName.ValueString(), CountryCode: m.CountryCode.ValueString(), City: m.City.ValueString(), Subdivision: m.Subdivision.ValueString(), PostalCode: m.PostalCode.ValueString(), TaxID: m.TaxID.ValueString(), RWhoisURL: m.RWhoisURL.ValueString()}
	if !m.Accept.IsNull() && !m.Accept.IsUnknown() {
		b := m.Accept.ValueBool()
		out.AcceptReassignments = &b
	}
	var d diag.Diagnostics
	d.Append(m.Street.ElementsAs(ctx, &out.StreetAddress, false)...)
	d.Append(m.Comments.ElementsAs(ctx, &out.Comments, false)...)
	var pocs []irrPOCModel
	d.Append(m.POCs.ElementsAs(ctx, &pocs, false)...)
	for _, p := range pocs {
		out.POCs = append(out.POCs, arin.OrgPOC{Handle: p.Handle.ValueString(), Function: p.Function.ValueString()})
	}
	return out, d
}
func (r *orgResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m orgModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, v := range []attr.Value{m.Name, m.DBAName, m.CountryCode, m.City, m.Subdivision, m.PostalCode, m.TaxID, m.RWhoisURL, m.Street, m.Comments, m.POCs, m.Accept} {
		tv, err := v.ToTerraformValue(ctx)
		if err != nil || !tv.IsFullyKnown() {
			return
		}
	}
	o, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		if err := o.Validate(); err != nil {
			resp.Diagnostics.AddError("Invalid organization", err.Error())
		}
	}
}
func orgState(ctx context.Context, o *arin.RegisteredOrganization) (orgModel, diag.Diagnostics) {
	m := orgModel{ID: types.StringValue(o.Handle), Handle: types.StringValue(o.Handle), Name: types.StringValue(o.Name), DBAName: types.StringValue(o.DBAName), Date: types.StringValue(o.RegistrationDate), CountryCode: types.StringValue(o.CountryCode), CountryName: types.StringValue(o.CountryName), City: types.StringValue(o.City), Subdivision: types.StringValue(o.Subdivision), PostalCode: types.StringValue(o.PostalCode), TaxID: types.StringValue(o.TaxID), RWhoisURL: types.StringValue(o.RWhoisURL), Accept: types.BoolNull(), PendingOperation: types.StringValue(""), PendingTicket: types.StringValue("")}
	if o.AcceptReassignments != nil {
		m.Accept = types.BoolValue(*o.AcceptReassignments)
	}
	var d, next diag.Diagnostics
	m.Street, next = types.ListValueFrom(ctx, types.StringType, nonNilStrings(o.StreetAddress))
	d.Append(next...)
	m.Comments, next = types.ListValueFrom(ctx, types.StringType, nonNilStrings(o.Comments))
	d.Append(next...)
	pocs := []irrPOCModel{}
	for _, p := range o.POCs {
		pocs = append(pocs, irrPOCModel{Handle: types.StringValue(p.Handle), Function: types.StringValue(p.Function)})
	}
	m.POCs, next = types.SetValueFrom(ctx, irrPOCType, pocs)
	d.Append(next...)
	return m, d
}
func pendingOrg(m orgModel, op string, result *arin.OrganizationWriteResult) orgModel {
	m.PendingOperation = types.StringValue(op)
	m.PendingTicket = types.StringValue("")
	if result != nil {
		m.PendingTicket = types.StringValue(result.TicketNumber)
		if result.Organization != nil {
			m.Handle = types.StringValue(result.Organization.Handle)
		}
	}
	if m.Handle.IsUnknown() || m.Handle.IsNull() {
		m.Handle = types.StringValue("")
	}
	if op == "create" {
		if m.Handle.ValueString() != "" {
			m.ID = m.Handle
		} else {
			data, _ := json.Marshal([]string{m.Name.ValueString(), m.CountryCode.ValueString(), m.DBAName.ValueString()})
			sum := sha256.Sum256(data)
			m.ID = types.StringValue(fmt.Sprintf("pending:%x", sum[:12]))
		}
	}
	if m.CountryName.IsUnknown() || m.CountryName.IsNull() {
		m.CountryName = types.StringValue("")
	}
	if m.Date.IsUnknown() || m.Date.IsNull() {
		m.Date = types.StringValue("")
	}
	if m.Accept.IsUnknown() {
		m.Accept = types.BoolNull()
	}
	return m
}
func orgWriteComplete(result *arin.OrganizationWriteResult, err error) bool {
	return err == nil && result != nil && result.Organization != nil && result.TicketNumber == ""
}
func orgPendingDiagnostic(d *diag.Diagnostics, err error) {
	detail := "Recovery state was saved. No automatic retry will be sent. Reconcile the ticket or uncertain request before another mutation. For ticket-only creation, obtain the approved organization handle and follow the documented import recovery steps."
	if err != nil {
		detail += " " + err.Error()
	}
	d.AddError("Organization operation requires reconciliation", detail)
}
func (r *orgResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m orgModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	o, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := o.Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid organization", err.Error())
		return
	}
	out, err := r.client.CreateOrganization(ctx, o)
	if orgWriteComplete(out, err) {
		m, d = orgState(ctx, out.Organization)
		resp.Diagnostics.Append(d...)
	} else {
		if netDefinitiveFailure(err) {
			resp.Diagnostics.AddError("Could not create organization", err.Error())
			return
		}
		m = pendingOrg(m, "create", out)
		orgPendingDiagnostic(&resp.Diagnostics, err)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// A ticket's shared orgHandle is not proof of the newly created organization's
// identity. Never adopt that organization or guess a handle from message text.
func (r *orgResource) pendingTicket(ctx context.Context, m orgModel) error {
	if m.PendingTicket.ValueString() == "" {
		return nil
	}
	ticket, err := r.client.GetRegistrationTicket(ctx, m.PendingTicket.ValueString())
	if err != nil {
		return err
	}
	if !ticket.Terminal() {
		return fmt.Errorf("ARIN ticket remains %s; no additional mutation was sent", ticket.Status)
	}
	if ticket.Failed() {
		return fmt.Errorf("ARIN ticket ended with %s; verify the outcome before importing or removing recovery state", ticket.Resolution)
	}
	return nil
}
func (r *orgResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m orgModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.PendingOperation.ValueString() != "" {
		if err := r.pendingTicket(ctx, m); err != nil {
			orgPendingDiagnostic(&resp.Diagnostics, err)
			return
		}
		if m.Handle.ValueString() == "" {
			orgPendingDiagnostic(&resp.Diagnostics, errors.New("creation has no confirmed handle; obtain the created handle from ARIN and import after reconciliation"))
			return
		}
	}
	out, err := r.client.GetRegisteredOrganization(ctx, m.Handle.ValueString())
	if arin.IsNotFound(err) {
		if m.PendingOperation.ValueString() == "create" || m.PendingOperation.ValueString() == "update" {
			orgPendingDiagnostic(&resp.Diagnostics, errors.New("organization is absent but the prior write is unresolved"))
			return
		}
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read organization", err.Error())
		return
	}
	actual, d := orgState(ctx, out)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	switch m.PendingOperation.ValueString() {
	case "delete":
		orgPendingDiagnostic(&resp.Diagnostics, errors.New("organization remains present after deletion request"))
		return
	case "update":
		if m.PendingTicket.ValueString() == "" && !orgWritableEqual(m, actual) {
			orgPendingDiagnostic(&resp.Diagnostics, errors.New("uncertain update is not reflected in the organization record"))
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &actual)...)
}
func orgWritableEqual(a, b orgModel) bool {
	return a.Name.Equal(b.Name) && a.DBAName.Equal(b.DBAName) && a.CountryCode.Equal(b.CountryCode) && a.City.Equal(b.City) && a.Subdivision.Equal(b.Subdivision) && a.PostalCode.Equal(b.PostalCode) && a.TaxID.Equal(b.TaxID) && a.RWhoisURL.Equal(b.RWhoisURL) && a.Accept.Equal(b.Accept) && a.Street.Equal(b.Street) && a.Comments.Equal(b.Comments) && a.POCs.Equal(b.POCs)
}
func (r *orgResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, old orgModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if old.PendingOperation.ValueString() != "" {
		orgPendingDiagnostic(&resp.Diagnostics, errors.New("refresh or import must resolve the prior operation first"))
		return
	}
	m.Handle = old.Handle
	m.ID = old.ID
	m.Date = old.Date
	m.CountryName = old.CountryName
	o, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := o.Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid organization", err.Error())
		return
	}
	out, err := r.client.UpdateOrganization(ctx, o)
	if orgWriteComplete(out, err) {
		m, d = orgState(ctx, out.Organization)
		resp.Diagnostics.Append(d...)
	} else {
		if netDefinitiveFailure(err) {
			resp.Diagnostics.AddError("Could not update organization", err.Error())
			return
		}
		m = pendingOrg(m, "update", out)
		orgPendingDiagnostic(&resp.Diagnostics, err)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *orgResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m orgModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.PendingOperation.ValueString() != "" {
		if err := r.pendingTicket(ctx, m); err != nil {
			orgPendingDiagnostic(&resp.Diagnostics, err)
			return
		}
		if m.Handle.ValueString() == "" {
			orgPendingDiagnostic(&resp.Diagnostics, errors.New("creation outcome is unknown; removing recovery state could allow duplicate submission"))
			return
		}
		_, err := r.client.GetRegisteredOrganization(ctx, m.Handle.ValueString())
		if m.PendingOperation.ValueString() == "delete" && arin.IsNotFound(err) {
			return
		}
		orgPendingDiagnostic(&resp.Diagnostics, errors.New("resolve the prior operation through refresh or import before destroy"))
		return
	}
	_, err := r.client.GetRegisteredOrganization(ctx, m.Handle.ValueString())
	if arin.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read organization", err.Error())
		return
	}
	out, err := r.client.DeleteOrganization(ctx, m.Handle.ValueString())
	if err == nil && out != nil && out.TicketNumber == "" {
		return
	}
	if netDefinitiveFailure(err) {
		resp.Diagnostics.AddError("Could not delete organization", err.Error())
		return
	}
	m = pendingOrg(m, "delete", out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	orgPendingDiagnostic(&resp.Diagnostics, err)
}
func (r *orgResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if strings.HasPrefix(req.ID, "pending:") || arin.ValidatePOCHandle(req.ID) != nil {
		resp.Diagnostics.AddError("Invalid organization import ID", "Use the actual ARIN organization handle, after reconciling any pending creation ticket.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("handle"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pending_operation"), "")...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pending_ticket"), "")...)
}
