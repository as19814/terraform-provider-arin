package provider

import (
	"context"
	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"slices"
)

var (
	_ resource.Resource                   = &irrRouteMetadataResource{}
	_ resource.ResourceWithConfigure      = &irrRouteMetadataResource{}
	_ resource.ResourceWithValidateConfig = &irrRouteMetadataResource{}
	_ resource.ResourceWithImportState    = &irrRouteMetadataResource{}
)

type irrRouteMetadataResource struct{ client *arin.Client }
type irrRouteMetadataModel struct {
	Source           types.String `tfsdk:"source"`
	ID               types.String `tfsdk:"id"`
	Prefix           types.String `tfsdk:"prefix"`
	OriginAS         types.String `tfsdk:"origin_as"`
	NetHandle        types.String `tfsdk:"net_handle"`
	OrgHandle        types.String `tfsdk:"org_handle"`
	Description      types.List   `tfsdk:"description"`
	Remarks          types.List   `tfsdk:"remarks"`
	MemberOf         types.Set    `tfsdk:"member_of"`
	POCs             types.Set    `tfsdk:"poc_links"`
	CreationDate     types.String `tfsdk:"creation_date"`
	LastModifiedDate types.String `tfsdk:"last_modified_date"`
	ExpectedROA      types.String `tfsdk:"expected_roa_handle"`
	ActualROA        types.String `tfsdk:"auto_linked_roa_handle"`
	ServerRemarks    types.List   `tfsdk:"server_remarks"`
}

func NewIRRRouteMetadataResource() resource.Resource { return &irrRouteMetadataResource{} }
func (r *irrRouteMetadataResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_irr_route_metadata"
}
func (r *irrRouteMetadataResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	(&irrRouteResource{}).Schema(ctx, req, resp)
	resp.Schema.MarkdownDescription = "Manage description, user remarks and route-set membership on an existing IRR route. Does not create or delete the route. Removing this resource releases metadata ownership and leaves the remote object intact. For linked routes, reference the owning ROA or bundle handle in expected_roa_handle to establish ordering. Do not also manage these fields with arin_irr_route or arin_irr_rpsl. Import by CIDR,AS<number>."
	resp.Schema.Attributes["expected_roa_handle"] = schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), MarkdownDescription: "Expected ROA link immediately before a write. Empty (default) requires an unlinked route. Reference the owning ROA or bundle handle for linked routes. Import initializes this to the observed link."}
	resp.Schema.Attributes["auto_linked_roa_handle"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Currently observed ROA link, or empty for an unlinked route."}
	resp.Schema.Attributes["server_remarks"] = schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "ARIN-generated link annotation, excluded from writable remarks."}
}
func (r *irrRouteMetadataResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	base := &irrRouteResource{}
	base.Configure(ctx, req, resp)
	r.client = base.client
}
func (m irrRouteMetadataModel) api(ctx context.Context) (arin.IRRRoute, diag.Diagnostics) {
	return (irrRouteModel{Prefix: m.Prefix, OriginAS: m.OriginAS, OrgHandle: m.OrgHandle, Description: m.Description, Remarks: m.Remarks, MemberOf: m.MemberOf}).api(ctx)
}
func (m irrRouteMetadataModel) validate(ctx context.Context) (arin.IRRRoute, diag.Diagnostics) {
	want, d := m.api(ctx)
	if !d.HasError() {
		if err := want.Validate(); err != nil {
			d.AddError("Invalid IRR route metadata", err.Error())
		}
	}
	if m.ExpectedROA.IsUnknown() || (!m.ExpectedROA.IsNull() && m.ExpectedROA.ValueString() != "" && arin.ValidatePOCHandle(m.ExpectedROA.ValueString()) != nil) {
		d.AddError("Invalid expected ROA handle", "Use a known valid ROA handle or an empty string for an unlinked route.")
	}
	if slices.Contains(want.Remarks, arin.LinkedRouteRemark) {
		d.AddError("Reserved route remark", "Exclude ARIN's generated link annotation from user remarks.")
	}
	return want, d
}
func (r *irrRouteMetadataResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m irrRouteMetadataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, v := range []attr.Value{m.Prefix, m.OriginAS, m.OrgHandle, m.Description, m.Remarks, m.MemberOf, m.ExpectedROA} {
		tv, err := v.ToTerraformValue(ctx)
		if err != nil || !tv.IsFullyKnown() {
			return
		}
	}
	_, d := m.validate(ctx)
	resp.Diagnostics.Append(d...)
}
func (m *irrRouteMetadataModel) set(ctx context.Context, actual *arin.IRRRoute) diag.Diagnostics {
	copy := *actual
	annotations := []string{}
	if actual.AutoLinkedROAHandle != "" {
		remarks, err := arin.LinkedRouteUserRemarks(*actual)
		if err != nil {
			var d diag.Diagnostics
			d.AddError("Unrecognized linked-route annotation", err.Error())
			return d
		}
		copy.Remarks = remarks
		annotations = []string{arin.LinkedRouteRemark}
	}
	base, d := irrRouteState(ctx, &copy)
	m.Source = base.Source
	m.ID = base.ID
	m.Prefix = base.Prefix
	m.OriginAS = base.OriginAS
	m.NetHandle = base.NetHandle
	m.OrgHandle = base.OrgHandle
	m.Description = base.Description
	m.Remarks = base.Remarks
	m.MemberOf = base.MemberOf
	m.POCs = base.POCs
	m.CreationDate = base.CreationDate
	m.LastModifiedDate = base.LastModifiedDate
	m.ActualROA = types.StringValue(actual.AutoLinkedROAHandle)
	var more diag.Diagnostics
	m.ServerRemarks, more = types.ListValueFrom(ctx, types.StringType, annotations)
	d.Append(more...)
	return d
}
func metadataMatches(actual, want arin.IRRRoute) bool {
	remarks := actual.Remarks
	if actual.AutoLinkedROAHandle != "" {
		var err error
		remarks, err = arin.LinkedRouteUserRemarks(actual)
		if err != nil {
			return false
		}
	}
	members, desired := slices.Clone(actual.MemberOf), slices.Clone(want.MemberOf)
	slices.Sort(members)
	slices.Sort(desired)
	return actual.ID() == want.ID() && actual.OrgHandle == want.OrgHandle && slices.Equal(actual.Description, want.Description) && slices.Equal(remarks, want.Remarks) && slices.Equal(members, desired)
}
func (r *irrRouteMetadataResource) write(ctx context.Context, m *irrRouteMetadataModel, d *diag.Diagnostics) bool {
	want, more := m.validate(ctx)
	d.Append(more...)
	if d.HasError() {
		return false
	}
	before, err := r.client.GetIRRRoute(ctx, want.ID())
	if err != nil {
		d.AddError("Could not find metadata target", err.Error()+". The route must already exist.")
		return false
	}
	if before.OrgHandle != want.OrgHandle || before.AutoLinkedROAHandle != m.ExpectedROA.ValueString() {
		d.AddError("IRR route ownership changed", "The current organization or ROA link does not match the planned metadata target. Refresh the owning resource before retrying.")
		return false
	}
	saved := *m
	d.Append(saved.set(ctx, before)...)
	if d.HasError() {
		return false
	}
	if metadataMatches(*before, want) {
		*m = saved
		return true
	}
	var actual *arin.IRRRoute
	if m.ExpectedROA.ValueString() == "" {
		actual, err = r.client.UpdateIRRRoute(ctx, want)
	} else {
		actual, err = r.client.UpdateLinkedIRRRoute(ctx, want, m.ExpectedROA.ValueString())
	}
	if err == nil {
		// Also verify the unlinked path through a fresh read.
		actual, err = r.client.GetIRRRoute(ctx, want.ID())
		if err == nil && (actual.AutoLinkedROAHandle != m.ExpectedROA.ValueString() || !metadataMatches(*actual, want)) {
			d.AddError("Could not confirm route metadata", "The fresh route does not match the requested fields and ROA binding.")
			*m = saved
			return true
		}
	}
	if err != nil {
		*m = saved
		d.AddError("Could not confirm route metadata", err.Error()+". The existing route identity was retained for refresh; the PUT was not replayed.")
		return true
	}
	d.Append(m.set(ctx, actual)...)
	return true
}
func (r *irrRouteMetadataResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m irrRouteMetadataModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.write(ctx, &m, &resp.Diagnostics) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *irrRouteMetadataResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m irrRouteMetadataModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.write(ctx, &m, &resp.Diagnostics) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *irrRouteMetadataResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m irrRouteMetadataModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	actual, err := r.client.GetIRRRoute(ctx, m.ID.ValueString())
	if arin.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read route metadata", err.Error())
		return
	}
	if actual.OrgHandle != m.OrgHandle.ValueString() {
		resp.Diagnostics.AddError("IRR route organization changed", "Refusing to adopt metadata from another organization.")
		return
	}
	// Keep the configured expected handle while exposing observed link drift. A
	// replaced owning ROA can then supply its new handle in the next plan.
	resp.Diagnostics.Append(m.set(ctx, actual)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *irrRouteMetadataResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
func (r *irrRouteMetadataResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if err := arin.ValidateIRRRouteID(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid route metadata import ID", err.Error())
		return
	}
	actual, err := r.client.GetIRRRoute(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Could not import route metadata", err.Error())
		return
	}
	m := irrRouteMetadataModel{ExpectedROA: types.StringValue(actual.AutoLinkedROAHandle)}
	resp.Diagnostics.Append(m.set(ctx, actual)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
