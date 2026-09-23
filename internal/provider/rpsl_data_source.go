package provider

import (
	"context"
	"fmt"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSourceWithConfigure = &rpslDataSource{}

type rpslDataSource struct{ client *arin.Client }
type rpslDataModel struct {
	ID     types.String `tfsdk:"id"`
	Kind   types.String `tfsdk:"object_type"`
	Name   types.String `tfsdk:"name"`
	Origin types.String `tfsdk:"origin_as"`
	Org    types.String `tfsdk:"org_handle"`
	Text   types.String `tfsdk:"rpsl"`
}

func NewRPSLDataSource() datasource.DataSource { return &rpslDataSource{} }
func (d *rpslDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_irr_rpsl"
}
func (d *rpslDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Read an advanced ARIN IRR object using RPSL. Supports route, route6, as-set, route-set and aut-num objects created through the RPSL interface. ARIN rejects this format for simple XML-created objects; use the corresponding typed data source for those. The complete RPSL text is returned without discarding attributes.", Attributes: map[string]schema.Attribute{
		"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Object type and name separated by a slash, with a comma and origin ASN appended for routes."},
		"object_type": schema.StringAttribute{Required: true, MarkdownDescription: "One of route, route6, as-set, route-set or aut-num."},
		"name":        schema.StringAttribute{Required: true, MarkdownDescription: "Canonical CIDR for routes, uppercase set name for sets, or AS followed by the ASN for aut-num."},
		"origin_as":   schema.StringAttribute{Optional: true, MarkdownDescription: "Required for route and route6, for example AS64496. Omit for other types."},
		"org_handle":  schema.StringAttribute{Computed: true, MarkdownDescription: "Maintaining organization derived from the mnt-by attribute."},
		"rpsl":        schema.StringAttribute{Computed: true, MarkdownDescription: "Complete returned RPSL object, including attributes not modeled by the typed XML data sources. Line endings are normalized to LF with a trailing newline."},
	}}
}
func (d *rpslDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*arin.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider configuration", fmt.Sprintf("Expected *arin.Client, got %T", req.ProviderData))
		return
	}
	d.client = client
}
func (d *rpslDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m rpslDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key := arin.RPSLKey{Kind: m.Kind.ValueString(), Name: m.Name.ValueString(), OriginAS: m.Origin.ValueString()}
	out, err := d.client.GetRPSL(ctx, key)
	if err != nil {
		resp.Diagnostics.AddError("Could not read advanced IRR object", err.Error())
		return
	}
	id := key.Kind + "/" + key.Name
	if key.OriginAS != "" {
		id += "," + key.OriginAS
	}
	m.ID = types.StringValue(id)
	m.Org = types.StringValue(out.OrgHandle)
	m.Text = types.StringValue(out.Text)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
