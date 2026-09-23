package provider

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ticketMessageResource struct{ client *arin.Client }
type ticketMessageModel struct {
	ID          types.String `tfsdk:"id"`
	Ticket      types.String `tfsdk:"ticket_number"`
	Message     types.String `tfsdk:"message_id"`
	Subject     types.String `tfsdk:"subject"`
	Text        types.List   `tfsdk:"text"`
	Category    types.String `tfsdk:"category"`
	Attachments types.Map    `tfsdk:"attachments"`
	Created     types.String `tfsdk:"created_date"`
	Pending     types.Bool   `tfsdk:"pending_submission"`
	Available   types.Bool   `tfsdk:"message_available"`
}

var (
	_ resource.Resource                   = &ticketMessageResource{}
	_ resource.ResourceWithConfigure      = &ticketMessageResource{}
	_ resource.ResourceWithValidateConfig = &ticketMessageResource{}
	_ resource.ResourceWithImportState    = &ticketMessageResource{}
)

func NewTicketMessageResource() resource.Resource { return &ticketMessageResource{} }
func (r *ticketMessageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ticket_message"
}
func (r *ticketMessageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optional := func(defaultValue, description string, sensitive bool) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(defaultValue), Sensitive: sensitive, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: description}
	}
	resp.Schema = schema.Schema{MarkdownDescription: "Append one message to an existing ARIN ticket and retain its receipt. Changing content or replacing this resource sends another message. Refresh never resubmits, including after expiry. Destroy only forgets the receipt; ARIN exposes no message update or delete operation. Uncertain submissions block refresh and destroy until manually reconciled and imported. Message content and attachments are stored in Terraform state.", Attributes: map[string]schema.Attribute{
		"id":                 schema.StringAttribute{Computed: true, MarkdownDescription: "TICKET/MESSAGE, or a temporary recovery identity."},
		"ticket_number":      schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Existing non-closed ticket receiving the message."},
		"message_id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Generated message ID within the ticket."},
		"subject":            optional("", "Message subject. Changes send a new message.", true),
		"category":           optional("NONE", "NONE or JUSTIFICATION. Changes send a new message.", false),
		"text":               schema.ListAttribute{Optional: true, Computed: true, Sensitive: true, ElementType: types.StringType, Default: listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})), PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()}, MarkdownDescription: "Ordered text lines. Changes send a new message."},
		"attachments":        schema.MapAttribute{Optional: true, Computed: true, Sensitive: true, ElementType: types.StringType, Default: mapdefault.StaticValue(types.MapValueMust(types.StringType, map[string]attr.Value{})), PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()}, MarkdownDescription: "Filename to base64 content. Changes send a new message. The encoded request and each downloaded attachment are limited to 4 MiB."},
		"created_date":       schema.StringAttribute{Computed: true, MarkdownDescription: "Server message creation date."},
		"pending_submission": schema.BoolAttribute{Computed: true, MarkdownDescription: "An uncertain POST requires reconciliation and import before retries."},
		"message_available":  schema.BoolAttribute{Computed: true, MarkdownDescription: "False after a previously confirmed message returns 404. The receipt remains and is not resubmitted."},
	}}
}
func (r *ticketMessageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*arin.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider configuration", fmt.Sprintf("Expected *arin.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}
func (m ticketMessageModel) api(ctx context.Context) (arin.RegistrationMessage, diag.Diagnostics) {
	var d diag.Diagnostics
	message := arin.RegistrationMessage{Subject: m.Subject.ValueString(), Category: m.Category.ValueString()}
	d.Append(m.Text.ElementsAs(ctx, &message.Text, false)...)
	var encoded map[string]string
	d.Append(m.Attachments.ElementsAs(ctx, &encoded, false)...)
	names := make([]string, 0, len(encoded))
	for name := range encoded {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data, err := base64.StdEncoding.DecodeString(encoded[name])
		if err != nil {
			d.AddError("Invalid attachment encoding", "Each attachment must contain valid base64.")
			continue
		}
		message.Attachments = append(message.Attachments, arin.RegistrationAttachment{Filename: name, Data: data})
	}
	if err := arin.ValidateRegistrationMessages([]arin.RegistrationMessage{message}); err != nil {
		d.AddError("Invalid message", err.Error())
	}
	return message, d
}
func (r *ticketMessageResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m ticketMessageModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Ticket.IsUnknown() {
		if err := arin.ValidateTicketNumber(m.Ticket.ValueString()); err != nil {
			resp.Diagnostics.AddError("Invalid ticket", err.Error())
		}
	}
	if m.Subject.IsUnknown() || m.Text.IsUnknown() || m.Category.IsUnknown() || m.Attachments.IsUnknown() {
		return
	}
	for _, v := range m.Text.Elements() {
		if v.IsUnknown() {
			return
		}
	}
	for _, v := range m.Attachments.Elements() {
		if v.IsUnknown() {
			return
		}
	}
	_, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
}
func (m *ticketMessageModel) set(message *arin.TicketMessage) {
	m.ID = types.StringValue(m.Ticket.ValueString() + "/" + message.ID)
	m.Message = types.StringValue(message.ID)
	m.Created = types.StringValue(message.CreatedDate)
	m.Available = types.BoolValue(true)
}
func (r *ticketMessageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m ticketMessageModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	message, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		resp.Diagnostics.AddError("Could not prepare message recovery", err.Error())
		return
	}
	receipt, err := r.client.AddTicketMessage(ctx, m.Ticket.ValueString(), message)
	if receipt == nil || (receipt.Message == nil && netDefinitiveFailure(err)) {
		if err != nil {
			resp.Diagnostics.AddError("Could not submit message", err.Error())
		}
		return
	}
	m.ID = types.StringValue("pending-" + hex.EncodeToString(nonce))
	m.Message = types.StringNull()
	m.Created = types.StringNull()
	m.Available = types.BoolNull()
	if receipt.Message != nil {
		m.set(receipt.Message)
	}
	m.Pending = types.BoolValue(!receipt.Confirmed)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	if err != nil {
		resp.Diagnostics.AddError("Message submission requires reconciliation", err.Error()+". Preserve this receipt. Identify the message in ARIN, back up state, remove only this pending receipt, and import TICKET/MESSAGE. Do not resubmit an uncertain POST.")
	}
}
func (r *ticketMessageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m ticketMessageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Pending.ValueBool() {
		resp.Diagnostics.AddError("Message submission requires import recovery", "Reconcile this submission and import TICKET/MESSAGE before retrying. A failed creation may be tainted and automatic replacement would send another message.")
		return
	}
	message, err := r.client.GetTicketMessage(ctx, m.Ticket.ValueString(), m.Message.ValueString())
	if arin.IsNotFound(err) {
		m.Available = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read message", err.Error())
		return
	}
	m.set(message)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *ticketMessageResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Messages are immutable", "Replace the receipt to send another message.")
}
func (r *ticketMessageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m ticketMessageModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if m.Pending.ValueBool() {
		resp.Diagnostics.AddError("Message submission requires import recovery", "Reconcile and import the existing message before forgetting this pending receipt.")
	}
}
func (r *ticketMessageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid message import ID", "Use TICKET/MESSAGE.")
		return
	}
	message, err := r.client.GetTicketMessage(ctx, parts[0], parts[1])
	if err != nil {
		resp.Diagnostics.AddError("Could not import message", err.Error())
		return
	}
	m := ticketMessageModel{Ticket: types.StringValue(parts[0]), Subject: types.StringValue(message.Subject), Category: types.StringValue(message.Category), Pending: types.BoolValue(false)}
	m.set(message)
	var d diag.Diagnostics
	lines := message.Text
	if lines == nil {
		lines = []string{}
	}
	m.Text, d = types.ListValueFrom(ctx, types.StringType, lines)
	resp.Diagnostics.Append(d...)
	attachments := map[string]string{}
	for _, ref := range message.Attachments {
		if _, exists := attachments[ref.Filename]; exists {
			resp.Diagnostics.AddError("Ambiguous attachment filenames", "Messages with duplicate attachment filenames cannot be represented by the attachments map.")
			return
		}
		var spec arin.ReadSpec
		for _, s := range arin.RegistrationReads() {
			if s.Name == "ticket_attachment" {
				spec = s
				break
			}
		}
		values, err := r.client.ReadRegistration(ctx, spec, map[string]string{"ticket_number": parts[0], "message_id": parts[1], "attachment_id": ref.ID})
		if err != nil {
			resp.Diagnostics.AddError("Could not import attachment", err.Error())
			return
		}
		attachments[ref.Filename] = values["content_base64"].(string)
	}
	m.Attachments, d = types.MapValueFrom(ctx, types.StringType, attachments)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
