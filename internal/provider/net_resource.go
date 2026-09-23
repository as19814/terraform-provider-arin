package provider

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = &netResource{}
	_ resource.ResourceWithConfigure      = &netResource{}
	_ resource.ResourceWithImportState    = &netResource{}
	_ resource.ResourceWithValidateConfig = &netResource{}
)

type netResource struct{ client *arin.Client }
type netModel struct {
	RemovalMessages  types.List   `tfsdk:"removal_messages"`
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Parent           types.String `tfsdk:"parent_net_handle"`
	Customer         types.String `tfsdk:"customer_handle"`
	Org              types.String `tfsdk:"org_handle"`
	Reallocate       types.Bool   `tfsdk:"reallocate"`
	Prefixes         types.Set    `tfsdk:"prefixes"`
	Comments         types.List   `tfsdk:"comments"`
	Version          types.Int64  `tfsdk:"ip_version"`
	Date             types.String `tfsdk:"registration_date"`
	PendingOperation types.String `tfsdk:"pending_operation"`
	PendingTicket    types.String `tfsdk:"pending_ticket"`
}

func NewNetResource() resource.Resource { return &netResource{} }
func (r *netResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_net"
}
func (r *netResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	recipient := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString(""), PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: description}
	}
	resp.Schema = schema.Schema{MarkdownDescription: "Manage a downstream NET registration: simple reassignment to a customer, detailed reassignment to an organization, or reallocation to an organization. Changing the recipient, parent, prefixes or creation mode replaces the registration. Import an existing reassignment or reallocation by NET handle. Direct allocations are not managed by this resource. Pending or uncertain operations retain recovery state and block repeat submissions; see the [network lifecycle guide](../reference/net-registration.md).", Attributes: map[string]schema.Attribute{
		"id":                schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}, MarkdownDescription: "NET handle after completion, or a pending recovery identifier before creation is confirmed."},
		"name":              schema.StringAttribute{Required: true, MarkdownDescription: "Network name containing letters, digits, spaces and hyphens."},
		"parent_net_handle": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, MarkdownDescription: "Parent allocation or reallocation."},
		"customer_handle":   recipient("Recipient customer for a simple reassignment. Mutually exclusive with org_handle."),
		"org_handle":        recipient("Recipient organization for a detailed reassignment or reallocation. Mutually exclusive with customer_handle."),
		"reallocate":        schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}, MarkdownDescription: "Create a reallocation instead of a reassignment. Requires org_handle."},
		"prefixes":          schema.SetAttribute{Required: true, ElementType: types.StringType, PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}, MarkdownDescription: "Minimal canonical CIDRs describing one contiguous range of one address family. IPv6 blocks must be /64 or larger."},
		"removal_messages":  netRemovalMessagesSchema(),
		"comments":          schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType, Default: listdefault.StaticValue(types.ListValueMust(types.StringType, []attr.Value{})), MarkdownDescription: "Ordered operational comments. Omission clears comments."},
		"ip_version":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Address family, 4 or 6."},
		"registration_date": schema.StringAttribute{Computed: true, MarkdownDescription: "ARIN registration date preserved on updates."},
		"pending_operation": schema.StringAttribute{Computed: true, MarkdownDescription: "Empty after completion; create or delete when reconciliation is required."},
		"pending_ticket":    schema.StringAttribute{Computed: true, MarkdownDescription: "ARIN ticket for an unresolved operation, or empty if ARIN did not return a ticket."},
	}}
}
func (r *netResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
func (m netModel) api(ctx context.Context) (arin.NetAssignment, diag.Diagnostics) {
	a := arin.NetAssignment{ParentNetHandle: m.Parent.ValueString(), Name: m.Name.ValueString(), CustomerHandle: m.Customer.ValueString(), OrgHandle: m.Org.ValueString(), Reallocate: m.Reallocate.ValueBool()}
	var d diag.Diagnostics
	d.Append(m.Prefixes.ElementsAs(ctx, &a.Prefixes, false)...)
	d.Append(m.Comments.ElementsAs(ctx, &a.Comments, false)...)
	return a, d
}
func (r *netResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m netModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, v := range []attr.Value{m.Name, m.Parent, m.Customer, m.Org, m.Reallocate, m.Prefixes, m.Comments, m.RemovalMessages} {
		tv, err := v.ToTerraformValue(ctx)
		if err != nil || !tv.IsFullyKnown() {
			return
		}
	}
	a, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := a.Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid network assignment", err.Error())
	}
	_, messagesDiagnostics := netRemovalMessages(ctx, m.RemovalMessages)
	resp.Diagnostics.Append(messagesDiagnostics...)
}
func netState(ctx context.Context, n *arin.RegisteredNet) (netModel, diag.Diagnostics) {
	m := netModel{RemovalMessages: types.ListNull(netRemovalMessageType), ID: types.StringValue(n.Handle), Name: types.StringValue(n.Name), Parent: types.StringValue(n.ParentNetHandle), Customer: types.StringValue(n.CustomerHandle), Org: types.StringValue(n.OrgHandle), Version: types.Int64Value(int64(n.Version)), Date: types.StringValue(n.RegistrationDate), PendingOperation: types.StringValue(""), PendingTicket: types.StringValue("")}
	var d, next diag.Diagnostics
	prefixes := []string{}
	kind := ""
	for _, b := range n.Blocks {
		if b.Type != "A" && b.Type != "S" {
			d.AddError("Unsupported network lifecycle", "arin_net manages reassignments and reallocations, not direct allocations.")
			return m, d
		}
		if kind != "" && kind != b.Type {
			d.AddError("Mixed network types", "A managed registration must have one network type.")
			return m, d
		}
		kind = b.Type
		prefixes = append(prefixes, fmt.Sprintf("%s/%d", b.StartAddress, b.CIDRLength))
	}
	m.Reallocate = types.BoolValue(kind == "A")
	m.Prefixes, next = types.SetValueFrom(ctx, types.StringType, prefixes)
	d.Append(next...)
	m.Comments, next = types.ListValueFrom(ctx, types.StringType, nonNilStrings(n.Comments))
	d.Append(next...)
	return m, d
}
func netDefinitiveFailure(err error) bool {
	var api *arin.APIError
	return errors.As(err, &api) && api.StatusCode >= 400 && api.StatusCode < 500 && api.StatusCode != 408
}
func pendingNetModel(m netModel, a arin.NetAssignment, ticket string) netModel {
	sum := sha256.Sum256([]byte(fmt.Sprint(a.ParentNetHandle, a.Name, a.Prefixes, a.CustomerHandle, a.OrgHandle)))
	m.ID = types.StringValue(fmt.Sprintf("pending:%x", sum[:12]))
	m.Date = types.StringValue("")
	m.PendingOperation = types.StringValue("create")
	m.PendingTicket = types.StringValue(ticket)
	version := int64(4)
	if len(a.Prefixes) > 0 && netip.MustParsePrefix(a.Prefixes[0]).Addr().Is6() {
		version = 6
	}
	m.Version = types.Int64Value(version)
	return m
}
func (r *netResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m netModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	_, messageDiagnostics := netRemovalMessages(ctx, m.RemovalMessages)
	resp.Diagnostics.Append(messageDiagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := a.Validate(); err != nil {
		resp.Diagnostics.AddError("Invalid network assignment", err.Error())
		return
	}
	existing, err := r.client.FindNetAssignment(ctx, a)
	if err != nil {
		resp.Diagnostics.AddError("Could not verify network availability", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError("Network already exists", "Import "+existing.Handle+" before managing it.")
		return
	}
	result, err := r.client.CreateNetAssignment(ctx, a)
	if err == nil && result.Net != nil {
		m, d = netStateWithRemoval(ctx, result.Net, m.RemovalMessages)
		resp.Diagnostics.Append(d...)
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
		}
		return
	}
	if netDefinitiveFailure(err) {
		resp.Diagnostics.AddError("Could not create network", err.Error())
		return
	}
	ticket := ""
	if result != nil {
		ticket = result.TicketNumber
	}
	m = pendingNetModel(m, a, ticket)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	detail := "The request has not been confirmed complete. Recovery state was saved; refresh to reconcile it before retrying. Terraform may mark this failed creation as tainted. After verifying successful creation, import or untaint it to retain the network."
	if err != nil {
		detail += " " + err.Error()
	}
	if ticket != "" {
		detail += " Ticket: " + ticket + "."
	}
	resp.Diagnostics.AddError("Network creation requires reconciliation", detail)
}

// resolveCreate returns failed=true only for a terminal unsuccessful ticket and
// no matching NET. Unknown outcomes never authorize a new submission.
func (r *netResource) resolveCreate(ctx context.Context, m netModel) (n *arin.RegisteredNet, failed bool, err error) {
	a, d := m.api(ctx)
	if d.HasError() {
		return nil, false, errors.New("invalid network recovery state")
	}
	n, err = r.client.FindNetAssignment(ctx, a)
	if err != nil || n != nil {
		return n, false, err
	}
	ticket := m.PendingTicket.ValueString()
	if ticket == "" {
		return nil, false, errors.New("creation outcome is unknown and no NET is visible; investigate the request before removing recovery state")
	}
	status, err := r.client.GetRegistrationTicket(ctx, ticket)
	if err != nil {
		return nil, false, err
	}
	if status.Failed() {
		return nil, true, nil
	}
	return nil, false, fmt.Errorf("creation ticket %s is %s (%s); the network is not yet confirmed", ticket, status.Status, status.Resolution)
}
func (r *netResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m netModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var n *arin.RegisteredNet
	var err error
	if m.PendingOperation.ValueString() == "create" {
		var failed bool
		n, failed, err = r.resolveCreate(ctx, m)
		if failed {
			resp.State.RemoveResource(ctx)
			return
		}
	} else {
		n, err = r.client.GetRegisteredNet(ctx, m.ID.ValueString())
		if arin.IsNotFound(err) {
			if err := r.confirmDeletionTicket(ctx, m); err != nil {
				resp.Diagnostics.AddError("Deletion ticket is unresolved", err.Error())
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not reconcile network", err.Error())
		return
	}
	out, d := netStateWithRemoval(ctx, n, m.RemovalMessages)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.PendingOperation.ValueString() == "delete" {
		out.PendingOperation = m.PendingOperation
		out.PendingTicket = m.PendingTicket
		resp.Diagnostics.AddWarning("Network deletion is unresolved", "The NET still exists. The pending deletion remains in state to prevent another submission.")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}
func (r *netResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var prior, m netModel
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if prior.PendingOperation.ValueString() != "" {
		resp.Diagnostics.AddError("Network operation is unresolved", "Reconcile the pending operation before updating network metadata.")
		return
	}
	_, messageDiagnostics := netRemovalMessages(ctx, m.RemovalMessages)
	resp.Diagnostics.Append(messageDiagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, d := m.api(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	var n *arin.RegisteredNet
	var err error
	if m.Name.Equal(prior.Name) && m.Comments.Equal(prior.Comments) {
		// Changing the destroy policy must not issue a metadata PUT or send messages.
		n, err = r.client.GetRegisteredNet(ctx, prior.ID.ValueString())
	} else {
		n, err = r.client.UpdateRegisteredNet(ctx, prior.ID.ValueString(), a.Name, a.Comments, nil, nil)
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not update network", err.Error())
		return
	}
	m, d = netStateWithRemoval(ctx, n, m.RemovalMessages)
	resp.Diagnostics.Append(d...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	}
}
func (r *netResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m netModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.PendingOperation.ValueString() == "create" {
		n, failed, err := r.resolveCreate(ctx, m)
		if failed {
			return
		}
		if err != nil {
			resp.Diagnostics.AddError("Creation still requires reconciliation", err.Error())
			return
		}
		var d diag.Diagnostics
		m, d = netStateWithRemoval(ctx, n, m.RemovalMessages)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	_, err := r.client.GetRegisteredNet(ctx, m.ID.ValueString())
	if arin.IsNotFound(err) {
		if err := r.confirmDeletionTicket(ctx, m); err != nil {
			resp.Diagnostics.AddError("Deletion ticket is unresolved", err.Error())
		}
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not verify network before deletion", err.Error())
		return
	}
	if m.PendingOperation.ValueString() == "delete" {
		ticket := m.PendingTicket.ValueString()
		if ticket != "" {
			status, e := r.client.GetRegistrationTicket(ctx, ticket)
			if e != nil {
				resp.Diagnostics.AddError("Could not reconcile deletion ticket", e.Error())
				return
			}
			if status.Failed() {
				// A terminal rejected request did not remove this existing NET. Clear the
				// marker, but require a separate apply before submitting another request.
				m.PendingOperation = types.StringValue("")
				m.PendingTicket = types.StringValue("")
				resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
				resp.Diagnostics.AddError("Previous deletion request failed", "Ticket "+ticket+" ended with "+status.Resolution+". Resolve its cause before applying again.")
				return
			}
		}
		resp.Diagnostics.AddError("Network deletion still requires reconciliation", "The NET remains visible. No additional deletion request was sent. Ticket: "+ticket)
		return
	}
	messages, d := netRemovalMessages(ctx, m.RemovalMessages)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result *arin.NetWriteResult
	var writeErr error
	if len(messages) == 0 {
		result, writeErr = r.client.DeleteNetAssignment(ctx, m.ID.ValueString())
	} else {
		result, writeErr = r.client.RemoveNetAssignment(ctx, m.ID.ValueString(), messages)
	}
	if netDefinitiveFailure(writeErr) {
		resp.Diagnostics.AddError("Could not delete network", writeErr.Error())
		return
	}
	m.PendingOperation = types.StringValue("delete")
	m.PendingTicket = types.StringValue("")
	if result != nil {
		m.PendingTicket = types.StringValue(result.TicketNumber)
	}
	_, readErr := r.client.GetRegisteredNet(ctx, m.ID.ValueString())
	if arin.IsNotFound(readErr) {
		readErr = r.confirmDeletionTicket(ctx, m)
		if readErr == nil {
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
	detail := "The NET is not confirmed absent. The pending deletion was saved to prevent another submission."
	if writeErr != nil {
		detail += " " + writeErr.Error()
	}
	if readErr != nil {
		detail += " " + readErr.Error()
	}
	resp.Diagnostics.AddError("Network deletion requires reconciliation", detail)
}
func (r *netResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if strings.HasPrefix(req.ID, "pending:") || arin.ValidateCustomerContext(req.ID, "") != nil {
		resp.Diagnostics.AddError("Invalid network import ID", "Use the ARIN NET handle of a reassignment or reallocation.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pending_operation"), "")...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pending_ticket"), "")...)
}

// An absent record alone is insufficient when a known deletion ticket is still
// open: its later processing could affect a replacement using the same range.
func (r *netResource) confirmDeletionTicket(ctx context.Context, m netModel) error {
	if m.PendingOperation.ValueString() != "delete" || m.PendingTicket.ValueString() == "" {
		return nil
	}
	ticket, err := r.client.GetRegistrationTicket(ctx, m.PendingTicket.ValueString())
	if err != nil {
		return err
	}
	if !ticket.Terminal() {
		return fmt.Errorf("ticket %s is still %s; wait for terminal processing before reissuing the range", ticket.Number, ticket.Status)
	}
	return nil
}
