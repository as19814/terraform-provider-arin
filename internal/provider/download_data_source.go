package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"slices"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type downloadDataSource struct {
	kind   string
	client *arin.Client
}

var _ datasource.DataSourceWithConfigure = &downloadDataSource{}

func NewBulkWhoisDataSource() datasource.DataSource { return &downloadDataSource{kind: "bulk_whois"} }
func NewInvalidPOCsDataSource() datasource.DataSource {
	return &downloadDataSource{kind: "invalid_pocs"}
}
func (d *downloadDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.kind
}
func (d *downloadDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id":              schema.StringAttribute{Computed: true, MarkdownDescription: "Canonical artifact selection, independent of changing content."},
		"max_bytes":       schema.Int64Attribute{Optional: true, MarkdownDescription: "Positive maximum download size in bytes. Defaults to 67108864 (64 MiB). Increase for larger approved reports; provider timeout_seconds also applies."},
		"include_content": schema.BoolAttribute{Optional: true, MarkdownDescription: "Store base64 contents in sensitive Terraform state. Defaults to true. Set false to stream and return only metadata; the full artifact is still downloaded to compute its digest."},
		"content_base64":  schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Exact downloaded bytes encoded as base64, or null when include_content=false. ZIPs are not extracted and XML is not transformed. State and memory usage grow with the report size."},
		"filename":        schema.StringAttribute{Computed: true, MarkdownDescription: "Response filename, if supplied. No local file is written."},
		"content_type":    schema.StringAttribute{Computed: true, MarkdownDescription: "Response media type."},
		"size_bytes":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of downloaded bytes."},
		"sha256":          schema.StringAttribute{Computed: true, MarkdownDescription: "SHA-256 of the received bytes. This is not an upstream signature."},
	}
	if d.kind == "bulk_whois" {
		attrs["objects"] = schema.SetAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "Any combination of asns, nets, orgs and pocs. Omitted or empty means all objects."}
		attrs["format"] = schema.StringAttribute{Optional: true, MarkdownDescription: "zip (default), xml or txt."}
	}
	resp.Schema = schema.Schema{Attributes: attrs, MarkdownDescription: "Download an existing " + d.kind + " artifact from ARIN's authenticated account service. Requires approved Bulk Whois access. Every refresh downloads the selected artifact; no report is generated. Access errors are returned as errors. The separate download origin uses ARIN's documented API-key query authentication. No redirects, archive extraction or local file writes occur."}
}
func (d *downloadDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*arin.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider configuration", "Expected ARIN client.")
		return
	}
	d.client = client
}
func (d *downloadDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	request := arin.DownloadRequest{Kind: d.kind, Format: "zip"}
	var limit types.Int64
	var include types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("max_bytes"), &limit)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("include_content"), &include)...)
	if limit.IsUnknown() || include.IsUnknown() {
		resp.Diagnostics.AddError("Unknown download options", "Download limits and content selection must be known before reading.")
		return
	}
	maxBytes := int64(64 << 20)
	if !limit.IsNull() {
		maxBytes = limit.ValueInt64()
	}
	if maxBytes <= 0 {
		resp.Diagnostics.AddError("Invalid download limit", "max_bytes must be positive.")
		return
	}
	if d.kind == "bulk_whois" {
		var format types.String
		var objects types.Set
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("format"), &format)...)
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("objects"), &objects)...)
		if format.IsUnknown() || objects.IsUnknown() {
			resp.Diagnostics.AddError("Unknown download selection", "Format and object selections must be known before reading.")
			return
		}
		if !format.IsNull() {
			request.Format = format.ValueString()
		}
		if !objects.IsNull() {
			resp.Diagnostics.Append(objects.ElementsAs(ctx, &request.Objects, false)...)
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if err := request.Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid download selection", err.Error())
		return
	}
	includeContent := include.IsNull() || include.ValueBool()
	var content bytes.Buffer
	var writer io.Writer = io.Discard
	if includeContent {
		writer = &content
	}
	metadata, err := d.client.DownloadTo(ctx, request, writer, maxBytes)
	if err != nil {
		resp.Diagnostics.AddError("Could not download ARIN artifact", err.Error())
		return
	}
	// Preserve optional input nulls instead of introducing defaults into config.
	resp.State.Raw = req.Config.Raw
	objects := slices.Clone(request.Objects)
	slices.Sort(objects)
	selection := strings.Join(objects, "+")
	if selection == "" {
		selection = "all"
	}
	values := map[string]any{"id": request.Kind + "/" + selection + "." + request.Format, "filename": types.StringValue(metadata.Filename), "content_type": metadata.ContentType, "size_bytes": metadata.SizeBytes, "sha256": metadata.SHA256, "content_base64": types.StringNull()}
	if metadata.Filename == "" {
		values["filename"] = types.StringNull()
	}
	if includeContent {
		values["content_base64"] = types.StringValue(base64.StdEncoding.EncodeToString(content.Bytes()))
	}
	for name, value := range values {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(name), value)...)
	}
}
