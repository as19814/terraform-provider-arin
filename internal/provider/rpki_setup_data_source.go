package provider

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type rpkiSetupDataSource struct{}
type rpkiSetupReferralModel struct {
	Referrer      types.String `tfsdk:"referrer"`
	ContactURI    types.String `tfsdk:"contact_uri"`
	Authorization types.String `tfsdk:"authorization_base64"`
}
type rpkiSetupModel struct {
	XML         types.String             `tfsdk:"xml"`
	ID          types.String             `tfsdk:"id"`
	Type        types.String             `tfsdk:"message_type"`
	Tag         types.String             `tfsdk:"tag"`
	Child       types.String             `tfsdk:"child_handle"`
	Parent      types.String             `tfsdk:"parent_handle"`
	Publisher   types.String             `tfsdk:"publisher_handle"`
	Service     types.String             `tfsdk:"service_uri"`
	SIA         types.String             `tfsdk:"sia_base"`
	RRDP        types.String             `tfsdk:"rrdp_notification_uri"`
	Certificate types.String             `tfsdk:"bpki_ta_pem"`
	Fingerprint types.String             `tfsdk:"bpki_ta_sha256"`
	NotBefore   types.String             `tfsdk:"not_before"`
	NotAfter    types.String             `tfsdk:"not_after"`
	Offer       types.Bool               `tfsdk:"publication_offered"`
	Referrals   []rpkiSetupReferralModel `tfsdk:"referrals"`
}

func NewRPKISetupDataSource() datasource.DataSource { return &rpkiSetupDataSource{} }
func (d *rpkiSetupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rpki_setup"
}
func (d *rpkiSetupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"xml":                 schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "RFC 8183 child_request, parent_response, publisher_request or repository_response XML from a trusted out-of-band exchange. Use file() or base64decode() on a ticket attachment. Limited to 4 MiB; retained in state."},
		"publication_offered": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the parent response includes a publication-service offer."},
		"referrals": schema.ListNestedAttribute{Computed: true, MarkdownDescription: "Ordered publication referrals. Authorization tokens are preserved without CMS verification; this data source does not grant or authenticate publication access.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"referrer":             schema.StringAttribute{Computed: true, MarkdownDescription: "Referring publisher handle."},
			"contact_uri":          schema.StringAttribute{Computed: true, MarkdownDescription: "Optional contact URI. Never followed."},
			"authorization_base64": schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Opaque authorization CMS token, normalized as base64. Signature and contents are not verified."},
		}}},
	}
	for name, desc := range map[string]string{
		"id": "SHA-256 of the exact input XML bytes.", "message_type": "Parsed setup message type.", "tag": "Optional exchange tag, with XML token whitespace normalized.",
		"child_handle": "Child handle, if applicable. A parent may assign a handle different from the child's request.", "parent_handle": "Parent handle, if applicable.", "publisher_handle": "Publisher handle, if applicable.",
		"service_uri": "Provisioning or publication HTTP(S) endpoint. No request is made.", "sia_base": "Repository's allocated rsync publication base.", "rrdp_notification_uri": "Optional RRDP notification URI. Never followed.",
		"bpki_ta_pem": "Self-signed BPKI CA certificate in PEM form. The self-signature and CA properties are checked; authenticity must be established out of band.", "bpki_ta_sha256": "SHA-256 fingerprint of the DER certificate.",
		"not_before": "Certificate validity start, in UTC RFC 3339 format. Validity is not checked against the current time.", "not_after": "Certificate validity end, in UTC RFC 3339 format. Expired documents remain readable.",
	} {
		attrs[name] = schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{MarkdownDescription: "Inspect a delegated RPKI or publication setup document locally. Parses the four RFC 8183 exchange types and checks the BPKI CA self-signature. Does not enroll an organization, authenticate the document's source, submit correspondence, generate keys, or execute provisioning/publication operations. No API key or network access is needed.", Attributes: attrs}
}
func (d *rpkiSetupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m rpkiSetupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.XML.IsUnknown() || m.XML.IsNull() {
		resp.Diagnostics.AddError("Unknown RPKI setup XML", "The setup document must be known before it can be parsed.")
		return
	}
	document := []byte(m.XML.ValueString())
	out, err := arin.ParseRPKISetup(document)
	if err != nil {
		resp.Diagnostics.AddError("Invalid RPKI setup document", err.Error())
		return
	}
	optional := func(s string) types.String {
		if s == "" {
			return types.StringNull()
		}
		return types.StringValue(s)
	}
	m.ID = types.StringValue(fmt.Sprintf("%x", sha256.Sum256(document)))
	m.Type = types.StringValue(out.Type)
	m.Tag = types.StringNull()
	if out.Tag != nil {
		m.Tag = types.StringValue(*out.Tag)
	}
	m.Child = optional(out.ChildHandle)
	m.Parent = optional(out.ParentHandle)
	m.Publisher = optional(out.PublisherHandle)
	if out.Type == "child_request" || out.Type == "parent_response" {
		m.Child = types.StringValue(out.ChildHandle)
	}
	if out.Type == "parent_response" {
		m.Parent = types.StringValue(out.ParentHandle)
	}
	if out.Type == "publisher_request" || out.Type == "repository_response" {
		m.Publisher = types.StringValue(out.PublisherHandle)
	}
	m.Service = optional(out.ServiceURI)
	m.SIA = optional(out.SIABase)
	m.RRDP = optional(out.RRDPNotificationURI)
	m.Certificate = types.StringValue(out.CertificatePEM)
	m.Fingerprint = types.StringValue(out.CertificateSHA256)
	m.NotBefore = types.StringValue(out.NotBefore)
	m.NotAfter = types.StringValue(out.NotAfter)
	m.Offer = types.BoolValue(out.Offer)
	m.Referrals = []rpkiSetupReferralModel{}
	for _, r := range out.Referrals {
		m.Referrals = append(m.Referrals, rpkiSetupReferralModel{Referrer: types.StringValue(r.Referrer), ContactURI: optional(r.ContactURI), Authorization: types.StringValue(r.AuthorizationBase64)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
