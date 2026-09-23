package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type rpkiPublicationBundleResource struct {
	read  func(context.Context, arin.RPKIPublicationReadConfig) (*arin.RPKIPublicationInventory, error)
	apply func(context.Context, arin.RPKIPublicationReadConfig, map[string]string, map[string]string) error
}
type rpkiPublicationBundleModel struct {
	Endpoint          types.String `tfsdk:"endpoint"`
	Publisher         types.String `tfsdk:"publisher_handle"`
	Journal           types.String `tfsdk:"journal_directory"`
	KeyFile           types.String `tfsdk:"signing_key_file"`
	Certificate       types.String `tfsdk:"signing_certificate_pem"`
	Anchor            types.String `tfsdk:"signing_ca_pem"`
	Intermediates     types.String `tfsdk:"signing_intermediates_pem"`
	CRLs              types.String `tfsdk:"signing_crls_pem"`
	Peer              types.String `tfsdk:"peer_ca_pem"`
	PeerIntermediates types.String `tfsdk:"peer_intermediates_pem"`
	ID                types.String `tfsdk:"id"`
	Objects           types.Map    `tfsdk:"objects"`
	Hashes            types.Map    `tfsdk:"hashes"`
}

func NewRPKIPublicationBundleResource() resource.Resource {
	return &rpkiPublicationBundleResource{read: arin.ReadRPKIPublication, apply: arin.ApplyRPKIPublicationBundle}
}
func (r *rpkiPublicationBundleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rpki_publication_bundle"
}
func (r *rpkiPublicationBundleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	var base datasource.SchemaResponse
	(&rpkiPublicationDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &base)
	attrs := map[string]schema.Attribute{}
	for name, a := range base.Schema.Attributes {
		if name == "objects" || name == "id" {
			continue
		}
		v := schema.StringAttribute{Required: a.IsRequired(), Optional: a.IsOptional(), MarkdownDescription: a.GetMarkdownDescription()}
		switch name {
		case "endpoint", "publisher_handle", "signing_ca_pem", "peer_ca_pem":
			v.PlanModifiers = []planmodifier.String{stringplanmodifier.RequiresReplace()}
		}
		attrs[name] = v
	}
	attrs["id"] = schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Stable identity assigned to this managed bundle."}
	attrs["objects"] = schema.MapAttribute{Required: true, ElementType: types.StringType, MarkdownDescription: "Map of owned rsync object URLs to canonical base64 content. Supply already-signed objects and their matching manifest together. Up to 10,000 objects and 3 MiB total decoded data; the encoded mutation request must fit 4 MiB. Contents remain in Terraform state."}
	attrs["hashes"] = schema.MapAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Last observed hashes for owned objects. Plans compute desired hashes so out-of-band changes and missing objects produce reconciliation updates."}
	resp.Schema = schema.Schema{MarkdownDescription: "Manage a set of caller-supplied RPKI publication objects with atomic RFC 8181 batches. Create requires absent URLs; update and destroy use hashes from the last refresh as server preconditions. Other repository objects are preserved. Do not overlap ownership with another resource or publisher. Does not generate or validate signed RPKI objects. Uses existing BPKI enrollment, private key files and persistent journals; uncertain mutations block further exchanges and require reconciliation. Import uses an absolute path to a private JSON manifest with exact object contents and BPKI configuration; see the [import format](../reference/delegated-rpki.md#publication-bundle-import). Use the [publication recovery CLI](../reference/delegated-rpki.md#publication-recovery-cli) to reconcile an exact pending digest against an explicitly selected inventory outcome. Native ARIN delegated sandbox verification remains unavailable.", Attributes: attrs}
}
func (m rpkiPublicationBundleModel) config() arin.RPKIPublicationReadConfig {
	return arin.RPKIPublicationReadConfig{Endpoint: m.Endpoint.ValueString(), Publisher: m.Publisher.ValueString(), JournalDirectory: m.Journal.ValueString(), SigningKeyFile: m.KeyFile.ValueString(), SigningCertificatePEM: m.Certificate.ValueString(), SigningAnchorPEM: m.Anchor.ValueString(), SigningIntermediatesPEM: m.Intermediates.ValueString(), SigningCRLsPEM: m.CRLs.ValueString(), PeerAnchorPEM: m.Peer.ValueString(), PeerIntermediatesPEM: m.PeerIntermediates.ValueString()}
}
func (r *rpkiPublicationBundleResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var m rpkiPublicationBundleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Objects.IsUnknown() || m.Objects.IsNull() {
		return
	}
	for _, v := range m.Objects.Elements() {
		if v.IsUnknown() {
			return
		}
	}
	var desired map[string]string
	resp.Diagnostics.Append(m.Objects.ElementsAs(ctx, &desired, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, hashes, err := arin.DecodeRPKIPublicationObjects(desired)
	if err != nil {
		resp.Diagnostics.AddError("Invalid publication bundle", err.Error())
		return
	}
	value, diags := types.MapValueFrom(ctx, types.StringType, hashes)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("hashes"), value)...)
}
func (r *rpkiPublicationBundleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m rpkiPublicationBundleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var desired map[string]string
	resp.Diagnostics.Append(m.Objects.ElementsAs(ctx, &desired, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, m.config(), desired, nil); err != nil {
		resp.Diagnostics.AddError("Could not publish bundle", err.Error())
		return
	}
	m.assignID(desired)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *rpkiPublicationBundleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m rpkiPublicationBundleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	inventory, err := r.read(ctx, m.config())
	if err != nil {
		resp.Diagnostics.AddError("Could not read publication bundle", err.Error())
		return
	}
	owned := m.Objects.Elements()
	hashes := map[string]attr.Value{}
	for _, object := range inventory.Objects {
		if _, ok := owned[object.URI]; ok {
			hashes[object.URI] = types.StringValue(object.SHA256)
		}
	}
	m.Hashes = types.MapValueMust(types.StringType, hashes)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *rpkiPublicationBundleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var planned, previous rpkiPublicationBundleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planned)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &previous)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var desired, prior map[string]string
	resp.Diagnostics.Append(planned.Objects.ElementsAs(ctx, &desired, false)...)
	resp.Diagnostics.Append(previous.Hashes.ElementsAs(ctx, &prior, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, planned.config(), desired, prior); err != nil {
		resp.Diagnostics.AddError("Could not update publication bundle", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &planned)...)
}
func (r *rpkiPublicationBundleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m rpkiPublicationBundleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var prior map[string]string
	resp.Diagnostics.Append(m.Hashes.ElementsAs(ctx, &prior, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, m.config(), nil, prior); err != nil {
		resp.Diagnostics.AddError("Could not withdraw publication bundle", err.Error())
	}
}

func (m *rpkiPublicationBundleModel) assignID(desired map[string]string) {
	uris := make([]string, 0, len(desired))
	for uri := range desired {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	raw, _ := json.Marshal([]any{m.Endpoint.ValueString(), m.Publisher.ValueString(), uris})
	m.ID = types.StringValue(fmt.Sprintf("%x", sha256.Sum256(raw)))
}
