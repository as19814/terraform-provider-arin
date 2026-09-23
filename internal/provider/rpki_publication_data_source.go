package provider

import (
	"context"
	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type rpkiPublicationDataSource struct {
	read func(context.Context, arin.RPKIPublicationReadConfig) (*arin.RPKIPublicationInventory, error)
}
type rpkiPublicationObjectModel struct {
	URI    types.String `tfsdk:"uri"`
	SHA256 types.String `tfsdk:"sha256"`
}
type rpkiPublicationModel struct {
	Endpoint          types.String                 `tfsdk:"endpoint"`
	Publisher         types.String                 `tfsdk:"publisher_handle"`
	Journal           types.String                 `tfsdk:"journal_directory"`
	KeyFile           types.String                 `tfsdk:"signing_key_file"`
	Certificate       types.String                 `tfsdk:"signing_certificate_pem"`
	Anchor            types.String                 `tfsdk:"signing_ca_pem"`
	Intermediates     types.String                 `tfsdk:"signing_intermediates_pem"`
	CRLs              types.String                 `tfsdk:"signing_crls_pem"`
	Peer              types.String                 `tfsdk:"peer_ca_pem"`
	PeerIntermediates types.String                 `tfsdk:"peer_intermediates_pem"`
	ID                types.String                 `tfsdk:"id"`
	Objects           []rpkiPublicationObjectModel `tfsdk:"objects"`
}

func NewRPKIPublicationDataSource() datasource.DataSource {
	return &rpkiPublicationDataSource{read: arin.ReadRPKIPublication}
}
func (d *rpkiPublicationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rpki_publication"
}
func (d *rpkiPublicationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{}
	for name, description := range map[string]string{
		"endpoint":                "Publication service URL from a trusted repository response. Requires HTTPS, except HTTP on loopback for tests.",
		"publisher_handle":        "Assigned publisher handle. Scopes the durable exchange journal; publication authorization is determined by the BPKI identity and endpoint.",
		"journal_directory":       "Absolute path to an existing private directory for durable signing-time and pending-request journals. Must persist between runs. Failed exchanges can require recovery before further reads.",
		"signing_key_file":        "Local private RSA key file, PKCS#8 or PKCS#1 PEM, with no group or other permissions. Symlinks are rejected. Only the path is retained in Terraform state; key bytes are never returned.",
		"signing_certificate_pem": "Existing BPKI signing EE certificate in PEM format.",
		"signing_ca_pem":          "Explicit local BPKI trust anchor in PEM format.",
		"signing_crls_pem":        "Concatenated current PEM CRLs for the local BPKI signing chain.",
		"peer_ca_pem":             "Explicit publication server BPKI trust anchor established out of band, in PEM format.",
	} {
		attrs[name] = schema.StringAttribute{Required: true, MarkdownDescription: description}
	}
	attrs["signing_intermediates_pem"] = schema.StringAttribute{Optional: true, MarkdownDescription: "Concatenated local BPKI intermediate CA certificates, if needed."}
	attrs["peer_intermediates_pem"] = schema.StringAttribute{Optional: true, MarkdownDescription: "Concatenated publication server BPKI intermediate CA certificates, if needed."}
	attrs["id"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Stable SHA-256 identity of the protocol, endpoint, publisher and BPKI anchors."}
	attrs["objects"] = schema.ListNestedAttribute{Computed: true, MarkdownDescription: "Published objects sorted by URI. These server-reported hashes do not independently validate RPKI resource objects.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
		"uri":    schema.StringAttribute{Computed: true, MarkdownDescription: "Published rsync object URI."},
		"sha256": schema.StringAttribute{Computed: true, MarkdownDescription: "Server-reported lowercase SHA-256 object hash."},
	}}}
	resp.Schema = schema.Schema{MarkdownDescription: "Read an RFC 8181 publication inventory using an existing BPKI signing identity. Sends a signed list request and verifies the response against the configured peer anchor. Does not publish or remove objects, enroll accounts, or use ARIN_API_KEY. Updates local exchange journals; interrupted or unverifiable exchanges remain pending and block automatic retry. No native ARIN delegated sandbox verification is available yet.", Attributes: attrs}
}
func (d *rpkiPublicationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m rpkiPublicationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, v := range []types.String{m.Endpoint, m.Publisher, m.Journal, m.KeyFile, m.Certificate, m.Anchor, m.CRLs, m.Peer} {
		if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
			resp.Diagnostics.AddError("Unknown RPKI publication configuration", "Required BPKI identity and service configuration must be known and nonempty before reading.")
			return
		}
	}
	if m.Intermediates.IsUnknown() || m.PeerIntermediates.IsUnknown() {
		resp.Diagnostics.AddError("Unknown RPKI publication configuration", "Intermediate certificates must be known before reading.")
		return
	}
	out, err := d.read(ctx, arin.RPKIPublicationReadConfig{Endpoint: m.Endpoint.ValueString(), Publisher: m.Publisher.ValueString(), JournalDirectory: m.Journal.ValueString(), SigningKeyFile: m.KeyFile.ValueString(), SigningCertificatePEM: m.Certificate.ValueString(), SigningAnchorPEM: m.Anchor.ValueString(), SigningIntermediatesPEM: m.Intermediates.ValueString(), SigningCRLsPEM: m.CRLs.ValueString(), PeerAnchorPEM: m.Peer.ValueString(), PeerIntermediatesPEM: m.PeerIntermediates.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Unable to read RPKI publication inventory", err.Error())
		return
	}
	m.ID = types.StringValue(out.ID)
	m.Objects = []rpkiPublicationObjectModel{}
	for _, object := range out.Objects {
		m.Objects = append(m.Objects, rpkiPublicationObjectModel{URI: types.StringValue(object.URI), SHA256: types.StringValue(object.SHA256)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
