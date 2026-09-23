package provider

import (
	"context"
	"fmt"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &ticketStatusResource{}
	_ resource.ResourceWithConfigure      = &ticketStatusResource{}
	_ resource.ResourceWithValidateConfig = &ticketStatusResource{}
	_ resource.ResourceWithImportState    = &ticketStatusResource{}
)

type ticketStatusResource struct{ client *arin.Client }
type ticketStatusModel struct {
	ID         types.String `tfsdk:"id"`
	Ticket     types.String `tfsdk:"ticket_number"`
	Status     types.String `tfsdk:"status"`
	Type       types.String `tfsdk:"ticket_type"`
	Resolution types.String `tfsdk:"resolution"`
	Closed     types.String `tfsdk:"closed_date"`
	Available  types.Bool   `tfsdk:"ticket_available"`
}

func NewTicketStatusResource() resource.Resource { return &ticketStatusResource{} }
func (r *ticketStatusResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ticket_status"
}
func (r *ticketStatusResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Close an existing resolved ARIN ticket. The only configurable status is CLOSED; ARIN does not allow this operation to close unresolved tickets or reopen closed tickets. An already closed ticket requires no write. Destroy forgets local management without reopening or deleting the ticket. A previously observed ticket that becomes unavailable remains in state with its last known status.", Attributes: map[string]schema.Attribute{
		"id":               schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "Ticket number."},
		"ticket_number":    schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Existing ARIN ticket number. Changing it manages another ticket; the previous ticket stays closed."},
		"status":           schema.StringAttribute{Required: true, MarkdownDescription: "Desired status. Only CLOSED is supported; the existing ticket must be RESOLVED or already CLOSED when applying."},
		"ticket_type":      schema.StringAttribute{Computed: true, MarkdownDescription: "ARIN ticket category."},
		"resolution":       schema.StringAttribute{Computed: true, MarkdownDescription: "Server-assigned ticket resolution."},
		"closed_date":      schema.StringAttribute{Computed: true, MarkdownDescription: "Server-assigned closure date, if present."},
		"ticket_available": schema.BoolAttribute{Computed: true, MarkdownDescription: "False if a previously read ticket returns 404. Its last known status is retained; this resource never recreates tickets."},
	}}
}
func (r *ticketStatusResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (r *ticketStatusResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m ticketStatusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Ticket.IsUnknown() {
		if err := arin.ValidateTicketNumber(m.Ticket.ValueString()); err != nil {
			resp.Diagnostics.AddError("Invalid ticket number", err.Error())
		}
	}
	if !m.Status.IsUnknown() && m.Status.ValueString() != "CLOSED" {
		resp.Diagnostics.AddError("Unsupported ticket status", "Only CLOSED can be configured. The existing ticket must first be RESOLVED.")
	}
}
func (m *ticketStatusModel) set(ticket *arin.Ticket) {
	m.ID = types.StringValue(ticket.Number)
	m.Ticket = types.StringValue(ticket.Number)
	m.Status = types.StringValue(ticket.Status)
	m.Type = types.StringValue(ticket.Type)
	m.Resolution = types.StringValue(ticket.Resolution)
	m.Closed = types.StringValue(ticket.ClosedDate)
	m.Available = types.BoolValue(true)
}
func (r *ticketStatusResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m ticketStatusModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Status.ValueString() != "CLOSED" {
		resp.Diagnostics.AddError("Unsupported ticket status", "Only CLOSED can be configured.")
		return
	}
	current, err := r.client.GetTicket(ctx, m.Ticket.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read ticket", err.Error())
		return
	}
	if current.Status != "RESOLVED" && current.Status != "CLOSED" {
		resp.Diagnostics.AddError("Ticket is not resolved", "ARIN only permits closing a RESOLVED ticket. Wait for its resolution before applying.")
		return
	}
	result, err := r.client.CloseTicket(ctx, m.Ticket.ValueString())
	if result != nil {
		m.set(result)
	} else {
		m.set(current)
	}
	// The ticket already exists and closure is idempotent. Keep its natural
	// identity after uncertainty so refresh can recover an accepted transition.
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	if err != nil {
		resp.Diagnostics.AddError("Could not confirm ticket closure", err.Error()+". Ticket identity was retained; refresh before retrying. An already closed ticket is not written again.")
	}
}
func (r *ticketStatusResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m ticketStatusModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ticket, err := r.client.GetTicket(ctx, m.Ticket.ValueString())
	if arin.IsNotFound(err) && !m.Available.IsNull() && !m.Available.IsUnknown() {
		m.Available = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read ticket", err.Error())
		return
	}
	m.set(ticket)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *ticketStatusResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m ticketStatusModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.Status.ValueString() != "CLOSED" {
		resp.Diagnostics.AddError("Unsupported ticket status", "Only CLOSED can be configured.")
		return
	}
	ticket, err := r.client.CloseTicket(ctx, m.Ticket.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not confirm ticket closure", err.Error()+". Refresh before retrying; closed tickets are not written again.")
		return
	}
	m.set(ticket)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
func (r *ticketStatusResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) { /* ARIN cannot reopen or delete this ticket through this resource. */
}
func (r *ticketStatusResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if err := arin.ValidateTicketNumber(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid ticket import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ticket_number"), req.ID)...)
}
