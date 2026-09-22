package provider

import (
	"context"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &organizationDataSource{}
var _ datasource.DataSourceWithConfigure = &organizationDataSource{}

type organizationDataSource struct{ client *arin.Client }
type organizationModel struct {
	Handle           types.String `tfsdk:"handle"`
	Name             types.String `tfsdk:"name"`
	RegistrationDate types.String `tfsdk:"registration_date"`
}

func NewOrganizationDataSource() datasource.DataSource { return &organizationDataSource{} }
func (d *organizationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org"
}
func (d *organizationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read an existing ARIN organization by its handle using Reg-RWS. This data source does not modify the organization.",
		Attributes: map[string]schema.Attribute{
			"handle":            schema.StringAttribute{Required: true, MarkdownDescription: "Organization handle, for example `EXAMPLE-1`."},
			"name":              schema.StringAttribute{Computed: true, MarkdownDescription: "Registered organization name."},
			"registration_date": schema.StringAttribute{Computed: true, MarkdownDescription: "Registration date as returned by ARIN; no date format conversion is applied."},
		},
	}
}
func (d *organizationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *organizationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data organizationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Missing ARIN client", "The provider must be configured before reading an organization.")
		return
	}
	org, err := d.client.GetOrganization(ctx, data.Handle.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read ARIN organization", err.Error())
		return
	}
	data.Name = types.StringValue(org.Name)
	data.RegistrationDate = types.StringValue(org.RegistrationDate)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
