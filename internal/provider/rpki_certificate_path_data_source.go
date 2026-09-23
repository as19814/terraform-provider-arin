package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type rpkiCertificatePathDataSource struct {
	discover func(context.Context, string, arin.RPKICertificateValidation) (string, error)
}
type rpkiCertificatePathModel struct {
	Certificate   types.String `tfsdk:"certificate_pem"`
	Anchor        types.String `tfsdk:"resource_anchor_pem"`
	Notifications types.List   `tfsdk:"rrdp_notifications"`
	Cache         types.String `tfsdk:"rrdp_cache_directory"`
	History       types.String `tfsdk:"manifest_history_directory"`
	Chain         types.String `tfsdk:"issuer_chain_pem"`
	ID            types.String `tfsdk:"id"`
}

func NewRPKICertificatePathDataSource() datasource.DataSource {
	return &rpkiCertificatePathDataSource{discover: arin.DiscoverRPKIIssuerChain}
}
func (d *rpkiCertificatePathDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rpki_certificate_path"
}
func (d *rpkiCertificatePathDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Discover and validate the issuer chain for an existing RPKI resource CA certificate using explicitly configured RRDP repositories and a pinned resource anchor. Current manifests, CRLs, resource authorization and durable manifest history are checked before returning. No API key or BPKI private key is used. Reads update local caches and validation history. AIA URLs are looked up inside the configured repositories, never fetched directly.", Attributes: map[string]schema.Attribute{
		"certificate_pem":            schema.StringAttribute{Required: true, MarkdownDescription: "Exactly one existing resource CA certificate in PEM format, whose path must be published and currently valid."},
		"resource_anchor_pem":        schema.StringAttribute{Required: true, MarkdownDescription: "Exactly one explicitly trusted resource anchor certificate in PEM format. Discovery never selects a replacement anchor."},
		"rrdp_notifications":         schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Ordered HTTPS notification URLs for each issuer, immediate issuer through anchor. One to 31 entries determine path depth; the same repository URL may serve multiple issuers."},
		"rrdp_cache_directory":       schema.StringAttribute{Required: true, MarkdownDescription: "Existing absolute private directory for persistent RRDP caches and polling state."},
		"manifest_history_directory": schema.StringAttribute{Required: true, MarkdownDescription: "Existing absolute private directory for durable manifest rollback protection. Keep it across runs."},
		"issuer_chain_pem":           schema.StringAttribute{Computed: true, MarkdownDescription: "Validated PEM issuer chain, immediate issuer first and configured anchor last. The input certificate is excluded. Can supply issuer_chain_pem on arin_rpki_certificate."},
		"id":                         schema.StringAttribute{Computed: true, MarkdownDescription: "SHA-256 binding the input certificate, anchor, ordered repository URLs and returned chain."},
	}}
}
func (d *rpkiCertificatePathDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m rpkiCertificatePathModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Certificate.IsUnknown() || m.Anchor.IsUnknown() || m.Cache.IsUnknown() || m.History.IsUnknown() || m.Notifications.IsUnknown() {
		resp.Diagnostics.AddError("Unknown RPKI path input", "Certificate path inputs must be known before validation.")
		return
	}
	var notifications []string
	resp.Diagnostics.Append(m.Notifications.ElementsAs(ctx, &notifications, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validation := arin.RPKICertificateValidation{AnchorPEM: m.Anchor.ValueString(), Notifications: notifications, CacheDirectory: m.Cache.ValueString(), HistoryDirectory: m.History.ValueString()}
	chain, err := d.discover(ctx, m.Certificate.ValueString(), validation)
	if err != nil {
		resp.Diagnostics.AddError("RPKI certificate path validation failed", err.Error())
		return
	}
	if chain == "" {
		resp.Diagnostics.AddError("RPKI certificate path validation failed", "Discovery returned an empty issuer chain.")
		return
	}
	m.Chain = types.StringValue(chain)
	identity, _ := json.Marshal([]any{m.Certificate.ValueString(), m.Anchor.ValueString(), notifications, chain})
	m.ID = types.StringValue(fmt.Sprintf("%x", sha256.Sum256(identity)))
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
