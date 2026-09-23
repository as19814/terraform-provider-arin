# Data source coverage

All non-report read endpoints documented in the collected Reg-RWS, IRR, and hosted RPKI guides have data sources. Public RDAP also supplies network and ASN discovery, ASN registration details, and organization contact references.

Report-request endpoints are intentionally excluded: they create tickets even though ARIN exposes them as HTTP GET. These data sources perform no write operations; managed resources are documented separately. Historical Whois-RWS search variations, delegated RPKI protocol exchanges, and account report-generation workflows are not Terraform data sources in this provider.

## Catalog

| Data source | API | Purpose |
| --- | --- | --- |
| [arin_networks](../data-sources/networks.md) | Public RDAP | Discover networks directly registered to an organization. |
| [arin_asn](../data-sources/asn.md) | Public RDAP | Read an ASN registration through public ARIN RDAP. No API key is sent. This is distinct from an IRR aut-num object. |
| [arin_asns](../data-sources/asns.md) | Public RDAP | List ASN registrations where an organization is the direct registrant using public RDAP. No API key is needed. Incomplete results are rejected. |
| [arin_aspa](../data-sources/aspa.md) | Hosted RPKI | Read one hosted ASPA by customer ASN from an organization's ASPA collection. |
| [arin_aspas](../data-sources/aspas.md) | Hosted RPKI | List hosted RPKI ASPAs for an organization, including customer and provider ASNs. |
| [arin_customer](../data-sources/customer.md) | Reg-RWS | Read an existing reassignment customer. Customer data is marked sensitive but remains in Terraform state. |
| [arin_delegation](../data-sources/delegation.md) | Reg-RWS | Read reverse DNS nameservers, TTLs, and DNSSEC DS records for an existing delegation. |
| [arin_irr_as_set](../data-sources/irr_as_set.md) | IRR | Read an IRR AS set and its membership. |
| [arin_irr_as_sets](../data-sources/irr_as_sets.md) | IRR | List IRR AS set references maintained by an organization. |
| [arin_irr_aut_num](../data-sources/irr_aut_num.md) | IRR | Read an IRR aut-num object and its routing policy. This is not an ASN registration record. |
| [arin_irr_aut_nums](../data-sources/irr_aut_nums.md) | IRR | List IRR aut-num references maintained by an organization. |
| [arin_irr_route](../data-sources/irr_route.md) | IRR | Read one IRR route or route6 object by canonical CIDR and origin ASN. Both families use ARIN's route endpoint. |
| [arin_irr_route_set](../data-sources/irr_route_set.md) | IRR | Read an IRR route set and its IPv4, multiprotocol, and by-reference membership. |
| [arin_irr_route_sets](../data-sources/irr_route_sets.md) | IRR | List IRR route set references maintained by an organization. |
| [arin_irr_routes](../data-sources/irr_routes.md) | IRR | List IRR route references for an organization. Entry types and references are returned directly; individual objects can be read with arin_irr_route. |
| [arin_most_specific_net](../data-sources/most_specific_net.md) | Reg-RWS | Find the most specific network registration covering an IP range. |
| [arin_net](../data-sources/net.md) | Reg-RWS | Read the full registration details of an existing IPv4 or IPv6 network by handle. |
| [arin_net_delegations](../data-sources/net_delegations.md) | Reg-RWS | List the reverse DNS delegations attached to a network. |
| [arin_net_routes](../data-sources/net_routes.md) | IRR | List IRR route references for a network, optionally including reassignments. |
| [arin_nets_by_ip_range](../data-sources/nets_by_ip_range.md) | Reg-RWS | List network registrations for an IP address range. This is an authenticated Reg-RWS lookup, distinct from public organization discovery. |
| [arin_org](../data-sources/org.md) | Reg-RWS | Read an ARIN organization through Reg-RWS, including its address and POC links. The tax ID is sensitive and is stored in Terraform state. |
| [arin_org_pocs](../data-sources/org_pocs.md) | Public RDAP | List public contact handles and roles linked directly to an ARIN organization. Use arin_poc to read contact details through Reg-RWS. No API key is needed. |
| [arin_parent_net](../data-sources/parent_net.md) | Reg-RWS | Find the parent network for an IP address range using ARIN's parentNet lookup. |
| [arin_poc](../data-sources/poc.md) | Reg-RWS | Read a point of contact, including names, addresses, email addresses, and telephone numbers. Contact details are stored in Terraform state. |
| [arin_roa](../data-sources/roa.md) | Hosted RPKI | Read one hosted ROA by handle from an organization's ROA collection. |
| [arin_roas](../data-sources/roas.md) | Hosted RPKI | List hosted RPKI ROAs for an organization. Omitted max_length values remain null, rather than inventing a maximum prefix length. |
| [arin_ticket](../data-sources/ticket.md) | Reg-RWS | Read an existing ticket and its message references. No ticket is created. Ticket content is sensitive and stored in Terraform state. |
| [arin_ticket_attachment](../data-sources/ticket_attachment.md) | Reg-RWS | Read an existing ticket attachment as base64 without writing files. Content is sensitive and stored in Terraform state. The shared 4 MiB response limit applies. |
| [arin_ticket_message](../data-sources/ticket_message.md) | Reg-RWS | Read an existing ticket message and attachment references. Content is sensitive and stored in Terraform state. |
| [arin_ticket_summaries](../data-sources/ticket_summaries.md) | Reg-RWS | List existing ticket summaries associated with the API key. No report is requested and no ticket is created. Ticket metadata is sensitive and stored in Terraform state. |
| [arin_ticket_summary](../data-sources/ticket_summary.md) | Reg-RWS | Read one existing ticket without retrieving message bodies. Ticket metadata is sensitive and stored in Terraform state. |
| [arin_tickets](../data-sources/tickets.md) | Reg-RWS | List existing tickets associated with the API key, filtered by type and status. Ticket content is sensitive and stored in Terraform state. Prefer arin_ticket_summaries when message content is unnecessary. |

## Behavior

- Individual lookups return errors for missing records. Collection lookups return empty lists for successful empty collections, never for authentication errors.
- Public RDAP searches reject truncation and pagination rather than returning an incomplete inventory.
- Registration collections reject unexpected record types. No response-provided links are followed.
- IP addresses are normalized, including ARIN's decimal, zero-padded IPv4 format. Optional server fields remain null. Ordered text follows numeric line indices.
- API keys are used only for authenticated registration reads. Public RDAP requests do not carry them.
- Customer and ticket content, attachments, and organization tax IDs are sensitive in Terraform's UI. Sensitive values are still stored in state.
- Attachments remain in memory and are returned as base64, with filename, content type, byte count, and SHA-256. No files are written; the 4 MiB response limit applies.
- Collection-only RPKI APIs are used for individual ROA/ASPA selection. Reads never synthesize RPKI objects.

## Testing and API notes

Mock acceptance tests cover every catalog entry through real Terraform, including typed nested values, identity checks, errors, empty inventories, and refresh. Fixtures use documentation address ranges and synthetic contact/customer/ticket data.

The opt-in live suite follows existing records from the selected organization. Network, contact, ASN, IRR, ROA, and ASPA reads were exercised against FT-684. Live customer, individual aut-num/route-set, and ticket-content reads require suitable existing records; mock coverage does not imply live verification of those cases.

The ASPA endpoint requires Content-Type: application/xml even on GET. The client supplies it for authenticated reads.

ARIN's current "Get Ticket Payload List" header-auth example repeats the single-ticket summary URL. The implemented listing path uses the matrix-filter endpoint documented in the adjacent URL-auth example, with authentication moved to the header. No key is placed in the URL. Both filtered ticket-list endpoints were also checked live and returned valid empty collections for open QUESTION tickets.

The provider does not promise that every account can access every object. Authorization errors are reported without falling back to a different account or service.
