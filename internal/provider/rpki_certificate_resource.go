package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type rpkiCertificateResource struct {
	issue  func(context.Context, arin.RPKIProvisioningReadConfig, arin.RPKICertificateRequest, arin.RPKICertificateValidation) (*arin.RPKIIssuedCertificate, error)
	read   func(context.Context, arin.RPKIProvisioningReadConfig, arin.RPKICertificateRequest, arin.RPKICertificateValidation) (*arin.RPKIIssuedCertificate, error)
	revoke func(context.Context, arin.RPKIProvisioningReadConfig, string, string) error
}
type rpkiCertificateModel struct {
	Endpoint          types.String `tfsdk:"endpoint"`
	Child             types.String `tfsdk:"child_handle"`
	Parent            types.String `tfsdk:"parent_handle"`
	Journal           types.String `tfsdk:"journal_directory"`
	KeyFile           types.String `tfsdk:"signing_key_file"`
	Certificate       types.String `tfsdk:"signing_certificate_pem"`
	Anchor            types.String `tfsdk:"signing_ca_pem"`
	Intermediates     types.String `tfsdk:"signing_intermediates_pem"`
	CRLs              types.String `tfsdk:"signing_crls_pem"`
	Peer              types.String `tfsdk:"peer_ca_pem"`
	PeerIntermediates types.String `tfsdk:"peer_intermediates_pem"`
	ID                types.String `tfsdk:"id"`
	Class             types.String `tfsdk:"class_name"`
	CSR               types.String `tfsdk:"csr_pem"`
	RequestedASN      types.String `tfsdk:"requested_asn"`
	RequestedIPv4     types.String `tfsdk:"requested_ipv4"`
	RequestedIPv6     types.String `tfsdk:"requested_ipv6"`
	ResourceAnchor    types.String `tfsdk:"resource_anchor_pem"`
	Issuers           types.String `tfsdk:"issuer_chain_pem"`
	Notifications     types.List   `tfsdk:"rrdp_notifications"`
	Cache             types.String `tfsdk:"rrdp_cache_directory"`
	History           types.String `tfsdk:"manifest_history_directory"`
	SKI               types.String `tfsdk:"ski"`
	Issued            types.String `tfsdk:"certificate_pem"`
	Issuer            types.String `tfsdk:"issuer_pem"`
	URLs              types.String `tfsdk:"certificate_urls"`
	Expiry            types.String `tfsdk:"not_after"`
}

func NewRPKICertificateResource() resource.Resource {
	return &rpkiCertificateResource{issue: arin.IssueRPKICertificate, read: arin.ReadRPKICertificate, revoke: arin.RevokeRPKICertificate}
}
func (r *rpkiCertificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rpki_certificate"
}
func (r *rpkiCertificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	var base datasource.SchemaResponse
	(&rpkiProvisioningDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &base)
	attrs := map[string]schema.Attribute{}
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	for name, a := range base.Schema.Attributes {
		if name == "classes" || name == "id" {
			continue
		}
		v := schema.StringAttribute{Required: a.IsRequired(), Optional: a.IsOptional(), MarkdownDescription: a.GetMarkdownDescription()}
		switch name {
		case "endpoint", "child_handle", "parent_handle":
			v.PlanModifiers = replace
		}
		attrs[name] = v
	}
	for name, desc := range map[string]string{"class_name": "Assigned resource class. Changing it replaces the resource.", "csr_pem": "Signed CA certificate request using an existing resource key. Changing it replaces the resource; private resource key bytes are never sent to the provider.", "resource_anchor_pem": "Explicit resource trust anchor, separate from the BPKI transport anchors.", "issuer_chain_pem": "Ordered PEM resource issuer chain, immediate issuer through the configured anchor.", "rrdp_cache_directory": "Existing absolute private directory for persistent RRDP cache.", "manifest_history_directory": "Existing absolute private directory for durable manifest history."} {
		v := schema.StringAttribute{Required: true, MarkdownDescription: desc}
		if name == "class_name" || name == "csr_pem" {
			v.PlanModifiers = replace
		}
		attrs[name] = v
	}
	for _, name := range []string{"requested_asn", "requested_ipv4", "requested_ipv6"} {
		attrs[name] = schema.StringAttribute{Optional: true, MarkdownDescription: "Requested resource subset in provisioning protocol syntax. Omit to request the entire allocation; an empty string requests none. Changes request a new certificate for the same key."}
	}
	attrs["rrdp_notifications"] = schema.ListAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "HTTPS RRDP notification URLs in issuer-chain order."}
	attrs["id"] = schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Stable identity of service, handles, class and resource key."}
	for name, desc := range map[string]string{"ski": "RFC 5280 method-1 resource key identifier.", "certificate_pem": "Validated issued resource certificate in PEM format.", "issuer_pem": "Immediate resource issuer certificate in PEM format.", "certificate_urls": "Comma-separated certificate publication URLs.", "not_after": "Issued certificate expiry in UTC RFC 3339 format."} {
		attrs[name] = schema.StringAttribute{Computed: true, MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{Attributes: attrs, MarkdownDescription: "Manage a delegated RPKI CA certificate using an existing CSR and BPKI enrollment. Creation and refresh validate the signed parent response and the manifest-backed resource path through explicitly configured RRDP repositories. Updates request issuance for the same key; destroy revokes every certificate for that key within the class. Do not overlap ownership of a class/key with other resources. A revoked key cannot be reused: replacements require a fresh resource key and CSR. Persistent private exchange, cache and history directories must survive Terraform runs. Uncertain operations remain pending and require operator reconciliation; complete issuance/revocation recovery is not yet implemented. Import accepts an absolute path to a private JSON manifest containing the resource configuration and validates the existing certificate without issuing or revoking; see the [import format](../reference/delegated-rpki.md#certificate-import). Native ARIN delegated sandbox verification remains unavailable."}
}
func (m rpkiCertificateModel) config() arin.RPKIProvisioningReadConfig {
	return arin.RPKIProvisioningReadConfig{Child: m.Child.ValueString(), Parent: m.Parent.ValueString(), BPKI: arin.RPKIPublicationReadConfig{Endpoint: m.Endpoint.ValueString(), JournalDirectory: m.Journal.ValueString(), SigningKeyFile: m.KeyFile.ValueString(), SigningCertificatePEM: m.Certificate.ValueString(), SigningAnchorPEM: m.Anchor.ValueString(), SigningIntermediatesPEM: m.Intermediates.ValueString(), SigningCRLsPEM: m.CRLs.ValueString(), PeerAnchorPEM: m.Peer.ValueString(), PeerIntermediatesPEM: m.PeerIntermediates.ValueString()}}
}

func (m rpkiCertificateModel) input() arin.RPKICertificateRequest {
	optional := func(v types.String) *string {
		if v.IsNull() {
			return nil
		}
		s := v.ValueString()
		return &s
	}
	return arin.RPKICertificateRequest{Class: m.Class.ValueString(), CSRPEM: m.CSR.ValueString(), RequestedASN: optional(m.RequestedASN), RequestedIPv4: optional(m.RequestedIPv4), RequestedIPv6: optional(m.RequestedIPv6)}
}
func (m rpkiCertificateModel) validation(ctx context.Context) (arin.RPKICertificateValidation, diag.Diagnostics) {
	var urls []string
	diags := m.Notifications.ElementsAs(ctx, &urls, false)
	return arin.RPKICertificateValidation{AnchorPEM: m.ResourceAnchor.ValueString(), IssuerChainPEM: m.Issuers.ValueString(), Notifications: urls, CacheDirectory: m.Cache.ValueString(), HistoryDirectory: m.History.ValueString()}, diags
}
func (m *rpkiCertificateModel) setCertificate(c *arin.RPKIIssuedCertificate) {
	m.SKI, m.Issued, m.Issuer, m.URLs, m.Expiry = types.StringValue(c.SKI), types.StringValue(c.CertificatePEM), types.StringValue(c.IssuerPEM), types.StringValue(c.CertificateURLs), types.StringValue(c.NotAfter)
	raw, _ := json.Marshal([]string{m.Endpoint.ValueString(), m.Child.ValueString(), m.Parent.ValueString(), m.Class.ValueString(), c.SKI})
	m.ID = types.StringValue(fmt.Sprintf("%x", sha256.Sum256(raw)))
}
func (r *rpkiCertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m rpkiCertificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, d := m.validation(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := r.issue(ctx, m.config(), m.input(), v)
	if err != nil {
		resp.Diagnostics.AddError("Could not issue resource certificate", err.Error())
		return
	}
	m.setCertificate(c)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *rpkiCertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m rpkiCertificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, d := m.validation(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := r.read(ctx, m.config(), m.input(), v)
	if err != nil {
		resp.Diagnostics.AddError("Could not refresh resource certificate", err.Error())
		return
	}
	if c == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	m.setCertificate(c)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *rpkiCertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m rpkiCertificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, d := m.validation(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := r.issue(ctx, m.config(), m.input(), v)
	if err != nil {
		resp.Diagnostics.AddError("Could not update resource certificate", err.Error())
		return
	}
	m.setCertificate(c)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *rpkiCertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m rpkiCertificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.revoke(ctx, m.config(), m.Class.ValueString(), m.SKI.ValueString()); err != nil {
		resp.Diagnostics.AddError("Could not revoke resource certificate", err.Error())
	}
}
