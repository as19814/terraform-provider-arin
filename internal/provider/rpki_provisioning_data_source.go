package provider

import (
	"context"
	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type rpkiProvisioningDataSource struct {
	read func(context.Context, arin.RPKIProvisioningReadConfig) (*arin.RPKIProvisioningInventory, error)
}
type rpkiProvisioningCertificateModel struct {
	URLs   types.String `tfsdk:"certificate_urls"`
	PEM    types.String `tfsdk:"certificate_pem"`
	SHA256 types.String `tfsdk:"sha256"`
	ASN    types.String `tfsdk:"requested_asn"`
	IPv4   types.String `tfsdk:"requested_ipv4"`
	IPv6   types.String `tfsdk:"requested_ipv6"`
}
type rpkiProvisioningClassModel struct {
	Name         types.String                       `tfsdk:"name"`
	URLs         types.String                       `tfsdk:"certificate_urls"`
	ASN          types.String                       `tfsdk:"asn"`
	IPv4         types.String                       `tfsdk:"ipv4"`
	IPv6         types.String                       `tfsdk:"ipv6"`
	SIA          types.String                       `tfsdk:"suggested_sia"`
	NotAfter     types.String                       `tfsdk:"not_after"`
	Issuer       types.String                       `tfsdk:"issuer_pem"`
	Certificates []rpkiProvisioningCertificateModel `tfsdk:"certificates"`
}
type rpkiProvisioningModel struct {
	Endpoint          types.String                 `tfsdk:"endpoint"`
	Child             types.String                 `tfsdk:"child_handle"`
	Parent            types.String                 `tfsdk:"parent_handle"`
	Journal           types.String                 `tfsdk:"journal_directory"`
	KeyFile           types.String                 `tfsdk:"signing_key_file"`
	Certificate       types.String                 `tfsdk:"signing_certificate_pem"`
	Anchor            types.String                 `tfsdk:"signing_ca_pem"`
	Intermediates     types.String                 `tfsdk:"signing_intermediates_pem"`
	CRLs              types.String                 `tfsdk:"signing_crls_pem"`
	Peer              types.String                 `tfsdk:"peer_ca_pem"`
	PeerIntermediates types.String                 `tfsdk:"peer_intermediates_pem"`
	ID                types.String                 `tfsdk:"id"`
	Classes           []rpkiProvisioningClassModel `tfsdk:"classes"`
}

func NewRPKIProvisioningDataSource() datasource.DataSource {
	return &rpkiProvisioningDataSource{read: arin.ReadRPKIProvisioning}
}
func (d *rpkiProvisioningDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rpki_provisioning"
}
func (d *rpkiProvisioningDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var base datasource.SchemaResponse
	(&rpkiPublicationDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &base)
	attrs := base.Schema.Attributes
	delete(attrs, "publisher_handle")
	delete(attrs, "objects")
	attrs["endpoint"] = schema.StringAttribute{Required: true, MarkdownDescription: "Provisioning service URL from a trusted parent response. Requires HTTPS except HTTP on loopback for tests."}
	attrs["child_handle"] = schema.StringAttribute{Required: true, MarkdownDescription: "Assigned child handle from the parent response."}
	attrs["parent_handle"] = schema.StringAttribute{Required: true, MarkdownDescription: "Parent handle from the trusted parent response."}
	attrs["peer_ca_pem"] = schema.StringAttribute{Required: true, MarkdownDescription: "Explicit parent BPKI trust anchor established out of band, in PEM format."}
	attrs["peer_intermediates_pem"] = schema.StringAttribute{Optional: true, MarkdownDescription: "Concatenated parent BPKI intermediate certificates, if needed."}
	attrs["id"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Stable SHA-256 identity of the protocol, endpoint, child/parent handles and BPKI anchors."}
	certAttrs := map[string]schema.Attribute{}
	for name, desc := range map[string]string{"certificate_urls": "Comma-separated publication URLs reported by the parent.", "certificate_pem": "Returned resource certificate, PEM encoded. Its resource trust path is not verified by this inventory read.", "sha256": "SHA-256 of the DER certificate.", "requested_asn": "Echoed ASN request. Null means absent; an empty string requests no ASNs.", "requested_ipv4": "Echoed IPv4 request. Null means absent; an empty string requests no IPv4 resources.", "requested_ipv6": "Echoed IPv6 request. Null means absent; an empty string requests no IPv6 resources."} {
		certAttrs[name] = schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	classAttrs := map[string]schema.Attribute{}
	for name, desc := range map[string]string{"name": "Resource class name.", "certificate_urls": "Comma-separated issuer certificate URLs reported by the parent.", "asn": "Allocated ASN resource set in protocol syntax.", "ipv4": "Allocated IPv4 resource set in protocol syntax.", "ipv6": "Allocated IPv6 resource set in protocol syntax.", "suggested_sia": "Optional suggested publication base, or null.", "not_after": "Allocation expiry in UTC RFC 3339 format.", "issuer_pem": "Returned issuer certificate, PEM encoded. Not an independently authenticated RPKI trust anchor."} {
		classAttrs[name] = schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	classAttrs["certificates"] = schema.ListNestedAttribute{Computed: true, MarkdownDescription: "Issued certificates sorted by publication URLs and DER hash.", NestedObject: schema.NestedAttributeObject{Attributes: certAttrs}}
	attrs["classes"] = schema.ListNestedAttribute{Computed: true, MarkdownDescription: "Resource classes sorted by name. The signed BPKI response is authenticated; resource certificate paths are not validated.", NestedObject: schema.NestedAttributeObject{Attributes: classAttrs}}
	resp.Schema = schema.Schema{MarkdownDescription: "Read an RFC 6492 delegated provisioning inventory using an existing BPKI identity. Authenticates the parent response and checks protocol structure and resource syntax. Does not issue/revoke certificates, validate their RPKI trust paths, enroll accounts, or use ARIN_API_KEY. Updates persistent local exchange journals; uncertain exchanges block automatic retry. Native ARIN delegated sandbox verification remains unavailable.", Attributes: attrs}
}

func (d *rpkiProvisioningDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m rpkiProvisioningModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, v := range []types.String{m.Endpoint, m.Child, m.Parent, m.Journal, m.KeyFile, m.Certificate, m.Anchor, m.CRLs, m.Peer} {
		if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
			resp.Diagnostics.AddError("Unknown RPKI provisioning configuration", "Required BPKI identity and service configuration must be known and nonempty before reading.")
			return
		}
	}
	if m.Intermediates.IsUnknown() || m.PeerIntermediates.IsUnknown() {
		resp.Diagnostics.AddError("Unknown RPKI provisioning configuration", "Intermediate certificates must be known before reading.")
		return
	}
	out, err := d.read(ctx, arin.RPKIProvisioningReadConfig{Child: m.Child.ValueString(), Parent: m.Parent.ValueString(), BPKI: arin.RPKIPublicationReadConfig{Endpoint: m.Endpoint.ValueString(), JournalDirectory: m.Journal.ValueString(), SigningKeyFile: m.KeyFile.ValueString(), SigningCertificatePEM: m.Certificate.ValueString(), SigningAnchorPEM: m.Anchor.ValueString(), SigningIntermediatesPEM: m.Intermediates.ValueString(), SigningCRLsPEM: m.CRLs.ValueString(), PeerAnchorPEM: m.Peer.ValueString(), PeerIntermediatesPEM: m.PeerIntermediates.ValueString()}})
	if err != nil {
		resp.Diagnostics.AddError("Unable to read RPKI provisioning inventory", err.Error())
		return
	}
	m.ID = types.StringValue(out.ID)
	m.Classes = []rpkiProvisioningClassModel{}
	optional := func(p *string) types.String {
		if p == nil {
			return types.StringNull()
		}
		return types.StringValue(*p)
	}
	for _, c := range out.Classes {
		sia := types.StringNull()
		if c.SuggestedSIA != "" {
			sia = types.StringValue(c.SuggestedSIA)
		}
		class := rpkiProvisioningClassModel{Name: types.StringValue(c.Name), URLs: types.StringValue(c.CertificateURLs), ASN: types.StringValue(c.ASN), IPv4: types.StringValue(c.IPv4), IPv6: types.StringValue(c.IPv6), SIA: sia, NotAfter: types.StringValue(c.NotAfter), Issuer: types.StringValue(c.IssuerPEM), Certificates: []rpkiProvisioningCertificateModel{}}
		for _, cert := range c.Certificates {
			class.Certificates = append(class.Certificates, rpkiProvisioningCertificateModel{URLs: types.StringValue(cert.URLs), PEM: types.StringValue(cert.PEM), SHA256: types.StringValue(cert.SHA256), ASN: optional(cert.RequestedASN), IPv4: optional(cert.RequestedIPv4), IPv6: optional(cert.RequestedIPv6)})
		}
		m.Classes = append(m.Classes, class)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
