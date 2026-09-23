package arin

import (
	"net/netip"
	"net/url"
	"strconv"
)

func input(name, kind, example, description string) Input {
	return Input{Name: name, Kind: kind, Example: example, Description: description}
}
func optional(in Input, value string) Input { in.Default = value; return in }

var orgInput = input("org_handle", "handle", "FT-684", "ARIN organization handle.")
var netInput = input("net_handle", "handle", "NET6-2602-F805-1", "ARIN network handle.")
var rangeInputs = []Input{input("start_address", "ip", "192.0.2.0", "First address of the IP range."), input("end_address", "ip", "192.0.2.255", "Last address of the IP range, in the same address family.")}
var ticketInput = input("ticket_number", "handle", "20260922-X1", "Existing ARIN ticket number. The API key must have access to the ticket.")
var messageInput = input("message_id", "id", "1", "Existing message ID within the ticket.")
var filters = []Input{optional(input("ticket_type", "enum", "ANY", "Ticket type filter from ARIN's ticket payload enumeration."), "ANY"), optional(input("ticket_status", "enum", "ANY_OPEN", "Ticket status filter, for example ANY_OPEN, CLOSED, or ANY."), "ANY_OPEN")}

func byInput(prefix, key, suffix string) func(map[string]string) string {
	return func(p map[string]string) string { return prefix + segment(p, key) + suffix }
}
func rangePath(operation string) func(map[string]string) string {
	return func(p map[string]string) string {
		return "/rest/net/" + operation + "/" + segment(p, "start_address") + "/" + segment(p, "end_address")
	}
}
func ticketPath(suffix string) func(map[string]string) string {
	return byInput("/rest/ticket/", "ticket_number", suffix)
}
func messagePath(p map[string]string) string {
	return "/rest/ticket/" + segment(p, "ticket_number") + "/message/" + segment(p, "message_id")
}
func ticketListPath(summary bool) func(map[string]string) string {
	return func(p map[string]string) string {
		path := "/rest/ticket"
		if summary {
			path += "/summary"
		}
		return path + ";ticketType=" + segment(p, "ticket_type") + ";ticketStatus=" + segment(p, "ticket_status")
	}
}

// RegistrationReads covers every non-report read in Reg-RWS, IRR, and RPKI.
// Report requests deliberately do not appear: their GET endpoints create tickets.
func RegistrationReads() []ReadSpec {
	specs := []ReadSpec{
		{Name: "org", Description: "Read an ARIN organization through Reg-RWS, including its address and POC links. The tax ID is sensitive and is stored in Terraform state.", Root: "org", Inputs: []Input{input("handle", "handle", "FT-684", "Organization handle.")}, Fields: orgFields, Path: byInput("/rest/org/", "handle", "")},
		{Name: "net", Description: "Read the full registration details of an existing IPv4 or IPv6 network by handle.", Root: "net", Inputs: []Input{input("handle", "handle", "NET6-2602-F805-1", "Network handle.")}, Fields: netFields, Path: byInput("/rest/net/", "handle", "")},
		{Name: "parent_net", Description: "Find the parent network for an IP address range using ARIN's parentNet lookup.", Root: "net", Inputs: rangeInputs, Fields: netFields, Path: rangePath("parentNet")},
		{Name: "most_specific_net", Description: "Find the most specific network registration covering an IP range.", Root: "net", Inputs: rangeInputs, Fields: netFields, Path: rangePath("mostSpecificNet")},
		{Name: "nets_by_ip_range", Description: "List network registrations for an IP address range. This is an authenticated Reg-RWS lookup, distinct from public organization discovery.", Item: "net", Output: "networks", Collection: true, Inputs: rangeInputs, Fields: netFields, Path: rangePath("netsByIpRange")},
		{Name: "delegation", Description: "Read reverse DNS nameservers, TTLs, and DNSSEC DS records for an existing delegation.", Root: "delegation", Inputs: []Input{input("name", "name", "0.5.0.8.f.2.0.6.2.ip6.arpa.", "Reverse delegation name, such as a name ending in in-addr.arpa or ip6.arpa.")}, Fields: delegationFields, Path: byInput("/rest/delegation/", "name", "")},
		{Name: "net_delegations", Description: "List the reverse DNS delegations attached to a network.", Item: "delegation", Output: "delegations", Collection: true, Inputs: []Input{netInput}, Fields: delegationFields, Path: byInput("/rest/net/", "net_handle", "/delegations")},
		{Name: "poc", Description: "Read a point of contact, including names, addresses, email addresses, and telephone numbers. Contact details are stored in Terraform state.", Root: "poc", Inputs: []Input{input("handle", "handle", "TECH1482-ARIN", "Point of contact handle.")}, Fields: pocFields, Path: byInput("/rest/poc/", "handle", "")},
		{Name: "customer", Description: "Read an existing reassignment customer. Customer data is marked sensitive but remains in Terraform state.", Root: "customer", Sensitive: true, Inputs: []Input{input("handle", "handle", "C00000001", "Customer handle.")}, Fields: customerFields, Path: byInput("/rest/customer/", "handle", "")},
		{Name: "irr_route", Description: "Read one IRR route or route6 object by canonical CIDR and origin ASN. Both families use ARIN's route endpoint.", Root: "route", Inputs: []Input{input("prefix", "cidr", "192.0.2.0/24", "Canonical IPv4 or IPv6 CIDR."), input("asn", "asn", "64496", "Origin ASN as a number without an AS prefix.")}, Fields: routeFields, Path: func(p map[string]string) string {
			prefix, _ := netip.ParsePrefix(p["prefix"])
			return "/rest/irr/route/" + url.PathEscape(prefix.Addr().String()) + "/" + strconv.Itoa(prefix.Bits()) + "/AS" + segment(p, "asn")
		}},
		{Name: "irr_routes", Description: "List IRR route references for an organization. Entry types and references are returned directly; simple objects use arin_irr_route; advanced objects use arin_irr_rpsl.", Item: "routeRef", Output: "routes", Collection: true, Inputs: []Input{orgInput}, Fields: routeRefFields, Path: byInput("/rest/org/", "org_handle", "/routes")},
		{Name: "net_routes", Description: "List IRR route references for a network, optionally including reassignments.", Item: "routeRef", Output: "routes", Collection: true, Inputs: []Input{netInput, optional(input("include_reassignments", "bool", "false", "Include routes for reassignments of the network."), "false")}, Fields: routeRefFields, Path: func(p map[string]string) string {
			return "/rest/net/" + segment(p, "net_handle") + "/routes?reassignments=" + p["include_reassignments"]
		}},
		{Name: "irr_aut_num", Description: "Read an IRR aut-num object and its routing policy. This is not an ASN registration record.", Root: "autnum", Inputs: []Input{input("asn", "asn", "64496", "ASN as a number without an AS prefix.")}, Fields: autnumFields, Path: func(p map[string]string) string { return "/rest/irr/aut-num/AS" + segment(p, "asn") }},
		{Name: "irr_aut_nums", Description: "List IRR aut-num references maintained by an organization.", Item: "autNumRef", Output: "aut_nums", Collection: true, Inputs: []Input{orgInput}, Fields: []Field{required(text("as_number", "asNumber")), text("entry_type", "@entry")}, Path: byInput("/rest/org/", "org_handle", "/aut-nums")},
		{Name: "irr_as_set", Description: "Read an IRR AS set and its membership.", Root: "asSet", Inputs: []Input{input("name", "name", "AS-FOUNDABILITY", "AS set name, including a hierarchical name if applicable.")}, Fields: setFields, Path: byInput("/rest/irr/as-set/", "name", "")},
		{Name: "irr_as_sets", Description: "List IRR AS set references maintained by an organization.", Item: "asSetRef", Output: "as_sets", Collection: true, Inputs: []Input{orgInput}, Fields: []Field{required(text("name", "@name")), text("entry_type", "@entry")}, Path: byInput("/rest/org/", "org_handle", "/as-sets")},
		{Name: "irr_route_set", Description: "Read an IRR route set and its IPv4, multiprotocol, and by-reference membership.", Root: "routeSet", Inputs: []Input{input("name", "name", "RS-EXAMPLE", "Route set name, including a hierarchical name if applicable.")}, Fields: setFields, Path: byInput("/rest/irr/route-set/", "name", "")},
		{Name: "irr_route_sets", Description: "List IRR route set references maintained by an organization.", Item: "routeSetRef", Output: "route_sets", Collection: true, Inputs: []Input{orgInput}, Fields: []Field{required(text("name", "@name")), text("entry_type", "@entry")}, Path: byInput("/rest/org/", "org_handle", "/route-sets")},
		{Name: "roas", Description: "List hosted RPKI ROAs for an organization. Omitted max_length values remain null, rather than inventing a maximum prefix length.", Item: "roaSpec", Output: "roas", Collection: true, Inputs: []Input{orgInput}, Fields: roaFields, Path: byInput("/rest/roa/", "org_handle", "")},
		{Name: "aspas", Description: "List hosted RPKI ASPAs for an organization, including customer and provider ASNs.", Item: "aspa", Output: "aspas", Collection: true, Inputs: []Input{orgInput}, Fields: aspaFields, Path: byInput("/rest/aspa/", "org_handle", "")},
		{Name: "ticket", Description: "Read an existing ticket and its message references. No ticket is created. Ticket content is sensitive and stored in Terraform state.", Root: "ticket", Sensitive: true, Inputs: []Input{ticketInput, optional(input("message_references_only", "bool", "true", "Retrieve references rather than full message bodies."), "true")}, Fields: ticketFields, Path: func(p map[string]string) string {
			return ticketPath("")(p) + "?msgRefs=" + p["message_references_only"]
		}},
		{Name: "ticket_summary", Description: "Read one existing ticket without retrieving message bodies. Ticket metadata is sensitive and stored in Terraform state.", Root: "ticket", Sensitive: true, Inputs: []Input{ticketInput}, Fields: ticketFields, Path: ticketPath("/summary")},
		{Name: "tickets", Description: "List existing tickets associated with the API key, filtered by type and status. Ticket content is sensitive and stored in Terraform state. Prefer arin_ticket_summaries when message content is unnecessary.", Item: "ticket", Output: "tickets", Collection: true, Sensitive: true, Inputs: filters, Fields: ticketFields, Path: ticketListPath(false)},
		{Name: "ticket_summaries", Description: "List existing ticket summaries associated with the API key. No report is requested and no ticket is created. Ticket metadata is sensitive and stored in Terraform state.", Item: "ticket", Output: "tickets", Collection: true, Sensitive: true, Inputs: filters, Fields: ticketFields, Path: ticketListPath(true)},
		{Name: "ticket_message", Description: "Read an existing ticket message and attachment references. Content is sensitive and stored in Terraform state.", Root: "message", Sensitive: true, Inputs: []Input{ticketInput, messageInput}, Fields: messageFields, Path: messagePath},
		{Name: "ticket_attachment", Description: "Read an existing ticket attachment as base64 without writing files. Content is sensitive and stored in Terraform state. The shared 4 MiB response limit applies.", Binary: true, Sensitive: true, Inputs: []Input{ticketInput, messageInput, input("attachment_id", "opaque", "1", "Existing attachment ID within the message.")}, Fields: []Field{text("content_base64", ""), integer("size_bytes", ""), text("filename", ""), text("content_type", ""), text("sha256", "")}, Path: func(p map[string]string) string { return messagePath(p) + "/attachment/" + segment(p, "attachment_id") }},
	}
	// ARIN provides collection endpoints only; individual objects are selected locally.
	specs = append(specs,
		ReadSpec{Name: "roa", Description: "Read one hosted ROA by handle from an organization's ROA collection.", Item: "roaSpec", SelectInput: "handle", SelectField: "handle", Inputs: []Input{orgInput, input("handle", "handle", "0123456789abcdef", "ROA handle returned by arin_roas.")}, Fields: roaFields, Path: byInput("/rest/roa/", "org_handle", "")},
		ReadSpec{Name: "aspa", Description: "Read one hosted ASPA by customer ASN from an organization's ASPA collection.", Item: "aspa", SelectInput: "customer_asn", SelectField: "customer_asn", Inputs: []Input{orgInput, input("customer_asn", "asn", "64496", "Customer ASN as a number.")}, Fields: aspaFields, Path: byInput("/rest/aspa/", "org_handle", "")},
	)
	return specs
}
