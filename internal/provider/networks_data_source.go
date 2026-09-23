package provider

import (
	"context"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &networksDataSource{}
var _ datasource.DataSourceWithConfigure = &networksDataSource{}

type networksDataSource struct{ client *arin.Client }
type networksModel struct {
	OrgHandle types.String            `tfsdk:"org_handle"`
	Networks  map[string]networkModel `tfsdk:"networks"`
}
type networkModel struct {
	Name         types.String `tfsdk:"name"`
	IPVersion    types.String `tfsdk:"ip_version"`
	Type         types.String `tfsdk:"registration_type"`
	StartAddress types.String `tfsdk:"start_address"`
	EndAddress   types.String `tfsdk:"end_address"`
	CIDRs        types.List   `tfsdk:"cidrs"`
	RDAPJSON     types.String `tfsdk:"rdap_json"`
}

func NewNetworksDataSource() datasource.DataSource { return &networksDataSource{} }
func (d *networksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_networks"
}
func (d *networksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List IPv4 and IPv6 networks registered to an organization using public ARIN RDAP. Includes allocations and assignments where the organization is the direct registrant, not records where it is only a contact. Results reflect public registration data, not API-key authority or BGP announcements. No API key is needed. Truncated or paginated results are rejected instead of returning a partial inventory.",
		Attributes: map[string]schema.Attribute{
			"org_handle": schema.StringAttribute{Required: true, MarkdownDescription: "ARIN organization handle, for example `FT-684`."},
			"networks": schema.MapNestedAttribute{Computed: true, MarkdownDescription: "Networks keyed by ARIN network handle. Includes more-specific registrations held by the same organization, so address ranges can overlap. This does not recursively include networks reassigned to other organizations.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"rdap_json":         schema.StringAttribute{Computed: true, MarkdownDescription: "Complete RDAP network JSON, including nested entities and extensions. No links are followed."},
				"name":              schema.StringAttribute{Computed: true, MarkdownDescription: "Registered network name."},
				"ip_version":        schema.StringAttribute{Computed: true, MarkdownDescription: "Address family: `v4` or `v6`."},
				"registration_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Registration type returned by ARIN, such as `DIRECT ALLOCATION` or `ASSIGNMENT`."},
				"start_address":     schema.StringAttribute{Computed: true, MarkdownDescription: "First IP address in canonical form."},
				"end_address":       schema.StringAttribute{Computed: true, MarkdownDescription: "Last IP address in canonical form."},
				"cidrs":             schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Sorted, canonical CIDRs from ARIN's cidr0 extension. Empty if ARIN omits the extension; start_address and end_address remain available."},
			}}},
		},
	}
}
func (d *networksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*arin.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider client", "The provider did not supply an ARIN API client. Please report this provider bug.")
		return
	}
	d.client = client
}
func (d *networksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data networksModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Missing ARIN client", "The provider must be configured before listing networks.")
		return
	}
	networks, err := d.client.ListOrganizationNetworks(ctx, data.OrgHandle.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to list ARIN networks", err.Error())
		return
	}
	data.Networks = make(map[string]networkModel, len(networks))
	for _, network := range networks {
		cidrs, diags := types.ListValueFrom(ctx, types.StringType, network.CIDRs)
		resp.Diagnostics.Append(diags...)
		data.Networks[network.Handle] = networkModel{Name: types.StringValue(network.Name), IPVersion: types.StringValue(network.IPVersion), Type: types.StringValue(network.Type), StartAddress: types.StringValue(network.StartAddress), EndAddress: types.StringValue(network.EndAddress), CIDRs: cidrs, RDAPJSON: types.StringValue(network.RDAPJSON)}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
