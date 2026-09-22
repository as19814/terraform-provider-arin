package provider

import (
	"context"
	"os"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &ARINProvider{}

type ARINProvider struct{ version string }
type providerModel struct {
	APIKey         types.String `tfsdk:"api_key"`
	BaseURL        types.String `tfsdk:"base_url"`
	TimeoutSeconds types.Int64  `tfsdk:"timeout_seconds"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &ARINProvider{version: version} }
}
func (p *ARINProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "arin"
	resp.Version = p.version
}
func (p *ARINProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage ARIN registration and routing resources. Currently supports organization lookup through Reg-RWS. Configure the API key through `ARIN_API_KEY` when possible.",
		Attributes: map[string]schema.Attribute{
			"api_key":         schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "ARIN API key. Defaults to `ARIN_API_KEY`. Your account must have authority over the requested records."},
			"base_url":        schema.StringAttribute{Optional: true, MarkdownDescription: "API origin. Defaults to `ARIN_BASE_URL`, then `https://reg.arin.net`. Use `https://reg.ote.arin.net` for OT&E. HTTPS is required except for loopback test servers."},
			"timeout_seconds": schema.Int64Attribute{Optional: true, MarkdownDescription: "HTTP request timeout in seconds, from 1 to 300. Defaults to 30."},
		},
	}
}
func (p *ARINProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.APIKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Unknown API key", "The API key must be known before the provider can make requests.")
	}
	if config.BaseURL.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("base_url"), "Unknown API origin", "The API origin must be known before the provider can make requests.")
	}
	if config.TimeoutSeconds.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("timeout_seconds"), "Unknown timeout", "The timeout must be known before the provider can make requests.")
	}
	if resp.Diagnostics.HasError() {
		return
	}
	key, baseURL := os.Getenv("ARIN_API_KEY"), os.Getenv("ARIN_BASE_URL")
	if !config.APIKey.IsNull() {
		key = config.APIKey.ValueString()
	}
	if !config.BaseURL.IsNull() {
		baseURL = config.BaseURL.ValueString()
		if baseURL == "" {
			resp.Diagnostics.AddAttributeError(path.Root("base_url"), "Empty API origin", "Set a valid API origin or omit base_url to use the default.")
			return
		}
	}
	timeout := int64(30)
	if !config.TimeoutSeconds.IsNull() {
		timeout = config.TimeoutSeconds.ValueInt64()
	}
	if timeout < 1 || timeout > 300 {
		resp.Diagnostics.AddAttributeError(path.Root("timeout_seconds"), "Invalid timeout", "timeout_seconds must be between 1 and 300.")
		return
	}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: baseURL, Timeout: time.Duration(timeout) * time.Second, UserAgent: "terraform-provider-arin/" + p.version + " terraform/" + req.TerraformVersion})
	if err != nil {
		resp.Diagnostics.AddError("Invalid ARIN configuration", err.Error())
		return
	}
	resp.DataSourceData = client
	resp.ResourceData = client
}
func (p *ARINProvider) Resources(_ context.Context) []func() resource.Resource { return nil }
func (p *ARINProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewOrganizationDataSource}
}
