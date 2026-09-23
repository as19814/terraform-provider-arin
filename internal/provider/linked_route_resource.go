package provider

import (
	"context"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Linked routes are created through RPKI, not the IRR route POST endpoint.
// Import is the explicit transfer of route-existence ownership to this resource.
type irrLinkedRouteResource struct{ irrRouteMetadataResource }

var (
	_ resource.Resource                   = &irrLinkedRouteResource{}
	_ resource.ResourceWithModifyPlan     = &irrLinkedRouteResource{}
	_ resource.ResourceWithConfigure      = &irrLinkedRouteResource{}
	_ resource.ResourceWithImportState    = &irrLinkedRouteResource{}
	_ resource.ResourceWithValidateConfig = &irrLinkedRouteResource{}
)

func NewIRRLinkedRouteResource() resource.Resource { return &irrLinkedRouteResource{} }
func (r *irrLinkedRouteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_irr_linked_route"
}
func (r *irrLinkedRouteResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	r.irrRouteMetadataResource.Schema(ctx, req, resp)
	resp.Schema.MarkdownDescription = "Own an existing ROA-linked IRR route, including its independent deletion. Import by CIDR,AS<number> before applying. ARIN creates links through RPKI, so this resource cannot create or recreate a linked route. Updates manage description, user remarks and membership. Destroy deletes only the IRR route and clears its ROA's per-prefix link flag; the ROA authorization remains. Use arin_irr_route_metadata when Terraform also manages the owning ROA or bundle, since its auto_link setting can otherwise recreate a deleted route. Do not overlap route ownership with other resources. A changed or removed ROA link blocks writes until ownership is reconciled."
	for _, key := range []string{"prefix", "origin_as", "org_handle"} {
		attribute := resp.Schema.Attributes[key].(schema.StringAttribute)
		attribute.MarkdownDescription = "Immutable imported route identity. Keep this value known and unchanged; import a different target into a separate resource."
		resp.Schema.Attributes[key] = attribute
	}
	resp.Schema.Attributes["expected_roa_handle"] = schema.StringAttribute{Required: true, MarkdownDescription: "Exact nonempty ROA handle required for updates and deletion. Import initializes this to the observed link. Changing this field does not create or change an ARIN link."}
}
func (r *irrLinkedRouteResource) Create(_ context.Context, _ resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError("Linked route requires import", "Create the route through an ARIN ROA transaction, then explicitly import its CIDR,AS<number> identity. This resource does not adopt or create linked routes during apply.")
}
func (r *irrLinkedRouteResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	r.irrRouteMetadataResource.ValidateConfig(ctx, req, resp)
	var m irrRouteMetadataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.ExpectedROA.IsUnknown() && arin.ValidatePOCHandle(m.ExpectedROA.ValueString()) != nil {
		resp.Diagnostics.AddError("Expected ROA handle required", "Configure a valid nonempty ROA handle for this linked route.")
	}
}
func (r *irrLinkedRouteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m irrRouteMetadataModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.ExpectedROA.IsUnknown() || arin.ValidatePOCHandle(m.ExpectedROA.ValueString()) != nil {
		resp.Diagnostics.AddError("Expected ROA handle required", "Resolve the exact ROA handle before updating this linked route.")
		return
	}
	r.irrRouteMetadataResource.Update(ctx, req, resp)
}
func (r *irrLinkedRouteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m irrRouteMetadataModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteLinkedIRRRoute(ctx, m.ID.ValueString(), m.OrgHandle.ValueString(), m.ExpectedROA.ValueString()); err != nil {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		resp.Diagnostics.AddError("Could not confirm linked route deletion", err.Error()+". Route ownership was retained for refresh; the DELETE was not replayed.")
	}
}
func (r *irrLinkedRouteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if err := arin.ValidateIRRRouteID(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid linked route import ID", err.Error())
		return
	}
	actual, err := r.client.GetIRRRoute(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Could not import linked route", err.Error())
		return
	}
	if arin.ValidatePOCHandle(actual.AutoLinkedROAHandle) != nil {
		resp.Diagnostics.AddError("Route is not ROA-linked", "Use arin_irr_route to manage an unlinked route.")
		return
	}
	m := irrRouteMetadataModel{ExpectedROA: types.StringValue(actual.AutoLinkedROAHandle)}
	resp.Diagnostics.Append(m.set(ctx, actual)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}

func (r *irrLinkedRouteResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var before, after irrRouteMetadataModel
	resp.Diagnostics.Append(req.State.Get(ctx, &before)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &after)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, pair := range [][2]types.String{{before.Prefix, after.Prefix}, {before.OriginAS, after.OriginAS}, {before.OrgHandle, after.OrgHandle}} {
		if !pair[0].Equal(pair[1]) {
			resp.Diagnostics.AddError("Cannot replace imported linked route", "Prefix, origin and organization identify an imported route. Import a different target into a separate resource; automatic replacement cannot create its RPKI link.")
			return
		}
	}
}
