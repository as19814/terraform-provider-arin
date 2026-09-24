package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &reportResource{}
	_ resource.ResourceWithConfigure      = &reportResource{}
	_ resource.ResourceWithValidateConfig = &reportResource{}
	_ resource.ResourceWithImportState    = &reportResource{}
)

type reportResource struct{ client *arin.Client }
type reportModel struct {
	ID         types.String `tfsdk:"id"`
	Type       types.String `tfsdk:"report_type"`
	Target     types.String `tfsdk:"target"`
	Ticket     types.String `tfsdk:"ticket_number"`
	TicketType types.String `tfsdk:"ticket_type"`
	Status     types.String `tfsdk:"status"`
	Resolution types.String `tfsdk:"resolution"`
	Created    types.String `tfsdk:"created_date"`
	Updated    types.String `tfsdk:"updated_date"`
	Resolved   types.String `tfsdk:"resolved_date"`
	Closed     types.String `tfsdk:"closed_date"`
	Available  types.Bool   `tfsdk:"ticket_available"`
	Pending    types.Bool   `tfsdk:"pending_submission"`
}

func NewReportRequestResource() resource.Resource { return &reportResource{} }
func (r *reportResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_report_request"
}
func (r *reportResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Submit one ARIN report request and retain its ticket receipt. Creation succeeds when ARIN accepts the request; report generation may finish later. Refresh only reads the ticket and never requests another report, including after ticket expiry. Change the request or explicitly replace this resource to submit a new report. Destroy forgets the receipt; it does not cancel, delete or close the ticket. Uncertain submission blocks retries until its ticket is reconciled. WhoWas reports are not supported. Report contents remain available through ticket/message/attachment data sources.", Attributes: map[string]schema.Attribute{
		"id":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Ticket number, or a temporary recovery identity after an uncertain submission."},
		"report_type":        schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "associations or reassignment. Changes submit a new request."},
		"target":             schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Empty for associations; NET handle for reassignment. Changes submit a new request."},
		"ticket_number":      schema.StringAttribute{Computed: true, MarkdownDescription: "ARIN report ticket number."},
		"ticket_type":        schema.StringAttribute{Computed: true, MarkdownDescription: "ARIN ticket category."},
		"status":             schema.StringAttribute{Computed: true, MarkdownDescription: "Most recently read ticket status. Not a guarantee that report attachments are available yet."},
		"resolution":         schema.StringAttribute{Computed: true, MarkdownDescription: "Most recently read ticket resolution."},
		"created_date":       schema.StringAttribute{Computed: true, MarkdownDescription: "Ticket creation date."},
		"updated_date":       schema.StringAttribute{Computed: true, MarkdownDescription: "Ticket update date."},
		"resolved_date":      schema.StringAttribute{Computed: true, MarkdownDescription: "Ticket resolution date, if present."},
		"closed_date":        schema.StringAttribute{Computed: true, MarkdownDescription: "Ticket closure date, if present."},
		"ticket_available":   schema.BoolAttribute{Computed: true, MarkdownDescription: "False when a previously confirmed ticket returns 404. The receipt remains in state and does not create a replacement report."},
		"pending_submission": schema.BoolAttribute{Computed: true, MarkdownDescription: "True when submission could not be confirmed. Preserve state and reconcile the ticket before retrying."},
	}}
}
func (r *reportResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (m reportModel) api() arin.ReportRequest {
	return arin.ReportRequest{Type: m.Type.ValueString(), Target: m.Target.ValueString()}
}
func (r *reportResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m reportModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Type.IsUnknown() || m.Target.IsUnknown() {
		return
	}
	if err := m.api().Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid report request", err.Error())
	}
}
func (m *reportModel) set(t *arin.Ticket) {
	m.ID = types.StringValue(t.Number)
	m.Ticket = types.StringValue(t.Number)
	m.TicketType = types.StringValue(t.Type)
	m.Status = types.StringValue(t.Status)
	m.Resolution = types.StringValue(t.Resolution)
	m.Created = types.StringValue(t.CreatedDate)
	m.Updated = types.StringValue(t.UpdatedDate)
	m.Resolved = types.StringValue(t.ResolvedDate)
	m.Closed = types.StringValue(t.ClosedDate)
	m.Available = types.BoolValue(true)
	m.Pending = types.BoolValue(false)
}
func (r *reportResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m reportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := m.api().Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid report request", err.Error())
		return
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		resp.Diagnostics.AddError("Could not prepare report recovery", err.Error())
		return
	}
	ticket, err := r.client.RequestReport(ctx, m.api())
	if err != nil && ticket != nil {
		// A fresh read may confirm an accepted request despite a partial write
		// response. Recover before returning an error that would taint state.
		verified, readErr := r.client.GetTicket(ctx, ticket.Number)
		if readErr == nil && m.api().MatchesTicket(*verified) {
			ticket, err = verified, nil
		}
	}
	var notSubmitted *arin.ReportNotSubmittedError
	if ticket == nil && (netDefinitiveFailure(err) || errors.As(err, &notSubmitted)) {
		resp.Diagnostics.AddError("Could not request report", err.Error())
		return
	}
	if ticket != nil {
		m.set(ticket)
	} else {
		m.ID = types.StringValue("pending-" + hex.EncodeToString(nonce))
		m.Ticket = types.StringNull()
		m.TicketType = types.StringNull()
		m.Status = types.StringNull()
		m.Resolution = types.StringNull()
		m.Created = types.StringNull()
		m.Updated = types.StringNull()
		m.Resolved = types.StringNull()
		m.Closed = types.StringNull()
		m.Available = types.BoolNull()
	}
	m.Pending = types.BoolValue(err != nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	if err != nil {
		resp.Diagnostics.AddError("Report submission requires reconciliation", err.Error()+". The request was not replayed. Preserve state; if no ticket number was returned, locate the accepted request in ARIN and import its ticket before another submission.")
	}
}
func (r *reportResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m reportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Ticket.ValueString() == "" {
		resp.Diagnostics.AddError("Report submission remains uncertain", "No ticket identity was confirmed. Preserve this receipt and reconcile the ARIN ticket list before removing state or submitting again.")
		return
	}
	ticket, err := r.client.GetTicket(ctx, m.Ticket.ValueString())
	if arin.IsNotFound(err) && !m.Pending.ValueBool() && !m.Available.IsNull() && !m.Available.IsUnknown() {
		m.Available = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read report ticket", err.Error())
		return
	}
	if !m.api().MatchesTicket(*ticket) {
		resp.Diagnostics.AddError("Report ticket type does not match", "Verify the report type and ticket identity before managing this receipt.")
		return
	}
	wasPending := m.Pending.ValueBool()
	m.set(ticket)
	m.Pending = types.BoolValue(wasPending)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	if wasPending {
		resp.Diagnostics.AddError("Report submission confirmed; import required", "The ticket now confirms the request, but the failed creation may have tainted Terraform state. Back up state, remove only this pending receipt from state, and import the verified report ticket. This prevents automatic replacement from submitting a duplicate report.")
	}
}
func (r *reportResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Report requests are immutable", "Replace the resource to submit a new report.")
}
func (r *reportResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m reportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Pending.ValueBool() {
		resp.Diagnostics.AddError("Report submission requires import recovery", "Reconcile the accepted request and import its ticket before forgetting this receipt. A failed creation may be tainted; automatic replacement would submit another report.")
		return
	}
	// ARIN exposes no deletion or cancellation for report requests. Forget only the
	// local receipt, without affecting the server ticket or its attached report.
}
func (r *reportResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	var request arin.ReportRequest
	var number string
	if len(parts) == 2 {
		request.Type = parts[0]
		number = parts[1]
	} else if len(parts) == 3 {
		request.Type = parts[0]
		request.Target = parts[1]
		number = parts[2]
	} else {
		resp.Diagnostics.AddError("Invalid report import ID", "Use associations/TICKET or REPORT-TYPE/TARGET/TICKET.")
		return
	}
	if err := request.Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid report import ID", err.Error())
		return
	}
	if err := arin.ValidateTicketNumber(number); err != nil {
		resp.Diagnostics.AddError("Invalid report ticket", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), number)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ticket_number"), number)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("report_type"), request.Type)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("target"), request.Target)...)
}
