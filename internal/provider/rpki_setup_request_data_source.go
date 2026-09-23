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

type rpkiSetupRequestDataSource struct{}
type rpkiSetupRequestReferralModel struct {
	Referrer      types.String `tfsdk:"referrer"`
	Authorization types.String `tfsdk:"authorization_base64"`
}
type rpkiSetupRequestModel struct {
	Type        types.String                    `tfsdk:"message_type"`
	Handle      types.String                    `tfsdk:"handle"`
	Certificate types.String                    `tfsdk:"bpki_ta_pem"`
	Tag         types.String                    `tfsdk:"tag"`
	Referrals   []rpkiSetupRequestReferralModel `tfsdk:"referrals"`
	XML         types.String                    `tfsdk:"xml"`
	ID          types.String                    `tfsdk:"id"`
}

func NewRPKISetupRequestDataSource() datasource.DataSource { return &rpkiSetupRequestDataSource{} }
func (d *rpkiSetupRequestDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rpki_setup_request"
}
func (d *rpkiSetupRequestDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Generate a local RFC 8183 child or publisher setup request from an existing BPKI CA certificate. The corresponding private key must remain with the CA software. This data source produces XML for an out-of-band exchange; it does not submit the request or enroll the organization. No API key is needed.", Attributes: map[string]schema.Attribute{
		"message_type": schema.StringAttribute{Required: true, MarkdownDescription: "child_request for delegated provisioning or publisher_request for repository publication."},
		"handle":       schema.StringAttribute{Required: true, MarkdownDescription: "Requested child or publisher handle, up to 255 ASCII letters, digits, slashes, underscores or hyphens. The service may assign a different handle in its response."},
		"bpki_ta_pem":  schema.StringAttribute{Required: true, MarkdownDescription: "Exactly one self-signed BPKI CA certificate in PEM form. CA properties and self-signature are checked; RPKI resource extensions are forbidden. Current validity dates and possession of its private key are not checked. Supply only the public certificate."},
		"tag":          schema.StringAttribute{Optional: true, MarkdownDescription: "Optional exchange tag, up to 1024 characters after XML token whitespace normalization. Omission and an explicitly empty tag remain distinct in generated XML."},
		"referrals": schema.ListNestedAttribute{Optional: true, MarkdownDescription: "Ordered publication referrals, allowed only for publisher_request. Copy referrer and authorization_base64 from a trusted parent response. Tokens are base64-validated but their CMS signatures are not verified.", NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"referrer":             schema.StringAttribute{Required: true, MarkdownDescription: "Referring publisher handle."},
			"authorization_base64": schema.StringAttribute{Required: true, Sensitive: true, MarkdownDescription: "Opaque referral CMS token from the parent, up to 512,000 decoded bytes."},
		}}},
		"xml": schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Deterministic generated XML, bounded to 4 MiB. It includes referral tokens when supplied and is stored in Terraform state."},
		"id":  schema.StringAttribute{Computed: true, MarkdownDescription: "SHA-256 of the generated XML bytes."},
	}}
}
func (d *rpkiSetupRequestDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m rpkiSetupRequestModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Type.IsUnknown() || m.Handle.IsUnknown() || m.Certificate.IsUnknown() || m.Tag.IsUnknown() {
		resp.Diagnostics.AddError("Unknown setup request input", "Setup request inputs must be known before XML can be generated.")
		return
	}
	request := arin.RPKISetupRequest{Type: m.Type.ValueString(), Handle: m.Handle.ValueString(), CertificatePEM: m.Certificate.ValueString()}
	if !m.Tag.IsNull() {
		tag := m.Tag.ValueString()
		request.Tag = &tag
	}
	for _, r := range m.Referrals {
		if r.Referrer.IsUnknown() || r.Authorization.IsUnknown() {
			resp.Diagnostics.AddError("Unknown setup referral", "Referral values must be known before XML can be generated.")
			return
		}
		request.Referrals = append(request.Referrals, arin.RPKISetupRequestReferral{Referrer: r.Referrer.ValueString(), AuthorizationBase64: r.Authorization.ValueString()})
	}
	document, err := arin.BuildRPKISetupRequest(request)
	if err != nil {
		resp.Diagnostics.AddError("Invalid RPKI setup request", err.Error())
		return
	}
	m.XML = types.StringValue(string(document))
	m.ID = types.StringValue(fmt.Sprintf("%x", sha256.Sum256(document)))
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
