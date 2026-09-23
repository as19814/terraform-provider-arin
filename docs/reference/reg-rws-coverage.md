# Reg-RWS operation reconciliation

Reviewed on 2026-09-23 against the [current Reg-RWS method guide](https://www.arin.net/resources/registry/regrws/methods/), its [saved copy](arin-api/reg-rws/methods.md), provider registration and client implementations. IRR and hosted RPKI cross-references were checked against their [IRR](https://www.arin.net/resources/manage/irr/irr-restful/) and [RPKI](https://www.arin.net/resources/manage/rpki/rpki-restful/) guides.

A fresh bounded HTTPS fetch confirmed that all 56 operation headings match the saved guide, in order. All 56 operation sections have an implementation mapping below. This establishes operation coverage, not complete native verification. Each evidence link distinguishes mock tests, successful sandbox operations, account restrictions and remaining cases. A client method is identified explicitly when Terraform models the equivalent desired state through another endpoint. Header/query authentication variants are transports for the same operation, not separate resources.

| Documented operation | Terraform surface | Client/catalog | Evidence and limits |
| --- | --- | --- | --- |
| Get Customer Information | [data `arin_customer`](../data-sources/customer.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](customer-network.md) |
| Create Recipient Customer | [resource `arin_customer`](../resources/customer.md) | [customer.go](../../internal/arin/customer.go) | [Audit](customer-network.md) |
| Delete Customer | [resource `arin_customer`](../resources/customer.md) | [customer.go](../../internal/arin/customer.go) | [Audit](customer-network.md) |
| Modify Customer | [resource `arin_customer`](../resources/customer.md) | [customer.go](../../internal/arin/customer.go) | [Audit](customer-network.md) |
| Get Delegation Information | [data `arin_delegation`](../data-sources/delegation.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](delegations.md) |
| Modify Delegation | [resource `arin_delegation`](../resources/delegation.md) | [delegation.go](../../internal/arin/delegation.go) | [Audit](delegations.md) |
| Add or Update Nameserver to Delegation | [resource `arin_delegation_nameserver`](../resources/delegation_nameserver.md) | [delegation.go](../../internal/arin/delegation.go) | [Audit](delegations.md) |
| Delete Nameserver from Delegation | [resource `arin_delegation_nameserver`](../resources/delegation_nameserver.md) | [delegation.go](../../internal/arin/delegation.go) | [Audit](delegations.md) |
| Delete Multiple Nameservers from Delegation | [resource `arin_delegation`](../resources/delegation.md) | [delegation.go](../../internal/arin/delegation.go) | [Audit](delegations.md) Bulk DELETE is a client method; the full-zone resource clears collections with PUT. |
| Get NET Information | [data `arin_net`](../data-sources/net.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](net-registration.md) |
| Delete NET | [resource `arin_net`](../resources/net.md) | [net_registration.go](../../internal/arin/net_registration.go) | [Audit](net-registration.md) |
| Remove NET | [resource `arin_net`](../resources/net.md) | [net_registration.go](../../internal/arin/net_registration.go) | [Audit](net-registration.md) Removal with correspondence has two approved native client tests; no further messages are authorized. |
| Modify NET | [resource `arin_net`](../resources/net.md), [resource `arin_net_metadata`](../resources/net_metadata.md) | [net_registration.go](../../internal/arin/net_registration.go) | [Audit](net-registration.md) |
| Get Delegations of a NET | [data `arin_net_delegations`](../data-sources/net_delegations.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](delegations.md) |
| Get Parent of a NET | [data `arin_parent_net`](../data-sources/parent_net.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](net-registration.md) |
| Get Routes for a NET | [data `arin_net_routes`](../data-sources/net_routes.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](irr-routes.md) include_reassignments selects the documented scope. |
| Get Routes for Reassignments of a NET | [data `arin_net_routes`](../data-sources/net_routes.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](irr-routes.md) include_reassignments selects the documented scope. |
| Get Network Details of a Start and End IP Range | [data `arin_nets_by_ip_range`](../data-sources/nets_by_ip_range.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](net-registration.md) |
| Get Network Details of a Start and End IP Range (Most Specific) | [data `arin_most_specific_net`](../data-sources/most_specific_net.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](net-registration.md) |
| Reassign NET | [resource `arin_net`](../resources/net.md) | [net_registration.go](../../internal/arin/net_registration.go) | [Audit](net-registration.md) Assignment mode selects simple/detailed reassignment or reallocation. |
| Reallocate NET | [resource `arin_net`](../resources/net.md) | [net_registration.go](../../internal/arin/net_registration.go) | [Audit](net-registration.md) Assignment mode selects simple/detailed reassignment or reallocation. |
| Create Org for Direct Allocation | [resource `arin_org`](../resources/org.md) | [organization.go](../../internal/arin/organization.go) | [Audit](organizations.md) Original sandbox creation awaits review; native mutable CRUD is incomplete. |
| Get Org Information | [data `arin_org`](../data-sources/org.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](organizations.md) |
| Get a List of route Objects for an Org ID | [data `arin_irr_routes`](../data-sources/irr_routes.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](irr-routes.md) |
| Get a List of route-set Objects for an Org ID | [data `arin_irr_route_sets`](../data-sources/irr_route_sets.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](irr-simple.md) |
| Get a List of as-set Objects for an Org ID | [data `arin_irr_as_sets`](../data-sources/irr_as_sets.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](irr-simple.md) |
| Get a List of aut-num Objects for an Org ID | [data `arin_irr_aut_nums`](../data-sources/irr_aut_nums.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](irr-simple.md) |
| Delete Org | [resource `arin_org`](../resources/org.md) | [organization.go](../../internal/arin/organization.go) | [Audit](organizations.md) Mock lifecycle covered; disposable native organization still pending review. |
| Modify Org | [resource `arin_org`](../resources/org.md) | [organization.go](../../internal/arin/organization.go) | [Audit](organizations.md) Mock lifecycle covered; disposable native organization still pending review. |
| Remove POC from Org | [resource `arin_org_poc`](../resources/org_poc.md) | [org_pocs.go](../../internal/arin/org_pocs.go) | [Audit](org-pocs.md) Non-admin native roles pass; Admin replacement still needs a disposable organization. |
| Add POC to Org | [resource `arin_org_poc`](../resources/org_poc.md) | [org_pocs.go](../../internal/arin/org_pocs.go) | [Audit](org-pocs.md) Non-admin native roles pass; Admin replacement still needs a disposable organization. |
| Get POC Information | [data `arin_poc`](../data-sources/poc.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](pocs.md) |
| Delete POC | [resource `arin_poc`](../resources/poc.md) | [poc.go](../../internal/arin/poc.go) | [Audit](pocs.md) |
| Create POC | [resource `arin_poc`](../resources/poc.md) | [poc.go](../../internal/arin/poc.go) | [Audit](pocs.md) |
| Modify POC | [resource `arin_poc`](../resources/poc.md) | [poc.go](../../internal/arin/poc.go) | [Audit](pocs.md) |
| Add Phone to POC | [resource `arin_poc_phone`](../resources/poc_phone.md) | [poc_contacts.go](../../internal/arin/poc_contacts.go) | [Audit](pocs.md) |
| Delete Phone from POC | [resource `arin_poc_phone`](../resources/poc_phone.md) | [poc_contacts.go](../../internal/arin/poc_contacts.go) | [Audit](pocs.md) |
| Add Email to POC | [resource `arin_poc_email`](../resources/poc_email.md) | [poc_contacts.go](../../internal/arin/poc_contacts.go) | [Audit](pocs.md) |
| Delete Email from POC | [resource `arin_poc_email`](../resources/poc_email.md) | [poc_contacts.go](../../internal/arin/poc_contacts.go) | [Audit](pocs.md) |
| Request WhoWas ASN Report | [resource `arin_report_request`](../resources/report_request.md) | [report.go](../../internal/arin/report.go) | [Audit](reports-tickets.md) GET submits a report; modeled as a resource. Native WhoWas returns access-denied 401. |
| Request WhoWas NET Report | [resource `arin_report_request`](../resources/report_request.md) | [report.go](../../internal/arin/report.go) | [Audit](reports-tickets.md) GET submits a report; modeled as a resource. Native WhoWas returns access-denied 401. |
| Request Associations Report | [resource `arin_report_request`](../resources/report_request.md) | [report.go](../../internal/arin/report.go) | [Audit](reports-tickets.md) GET submits a report; modeled as a resource, not a read-only data source. |
| Request Reassignment Report | [resource `arin_report_request`](../resources/report_request.md) | [report.go](../../internal/arin/report.go) | [Audit](reports-tickets.md) GET submits a report; modeled as a resource, not a read-only data source. |
| Create and Delete ROAs | [resource `arin_roa`](../resources/roa.md), [resource `arin_rpki_bundle`](../resources/rpki_bundle.md) | [rpki.go](../../internal/arin/rpki.go) | [Audit](rpki-bundle.md) |
| Get a List of ROAs for an Org | [data `arin_roas`](../data-sources/roas.md), [data `arin_roa`](../data-sources/roa.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](rpki.md) Individual selection is performed on the organization inventory. |
| Create & Delete ASPAs | [resource `arin_aspa`](../resources/aspa.md), [resource `arin_rpki_bundle`](../resources/rpki_bundle.md) | [rpki.go](../../internal/arin/rpki.go) | [Audit](rpki-bundle.md) |
| Get a List of ASPAs for an Org | [data `arin_aspas`](../data-sources/aspas.md), [data `arin_aspa`](../data-sources/aspa.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](rpki.md) Individual selection is performed on the organization inventory. |
| Add Message to Ticket | [resource `arin_ticket_message`](../resources/ticket_message.md) | [ticket_message.go](../../internal/arin/ticket_message.go) | [Audit](reports-tickets.md) Append-only receipt; native read-only import passed, submission needs new correspondence approval. |
| Modify Ticket | [resource `arin_ticket_status`](../resources/ticket_status.md) | [ticket_payload.go](../../internal/arin/ticket_payload.go) | [Audit](reports-tickets.md) Full-payload and status-only methods supported. Native RESOLVED-to-CLOSED transition remains unverified. |
| Modify Ticket Status | [resource `arin_ticket_status`](../resources/ticket_status.md) | [ticket.go](../../internal/arin/ticket.go) | [Audit](reports-tickets.md) Full-payload and status-only methods supported. Native RESOLVED-to-CLOSED transition remains unverified. |
| Get Ticket Details | [data `arin_ticket`](../data-sources/ticket.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](reports-tickets.md) |
| Get Ticket Summary | [data `arin_ticket_summary`](../data-sources/ticket_summary.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](reports-tickets.md) |
| Get Ticket Payload List | [data `arin_tickets`](../data-sources/tickets.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](reports-tickets.md) |
| Get Ticket Summaries | [data `arin_ticket_summaries`](../data-sources/ticket_summaries.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](reports-tickets.md) |
| Get Ticket Message | [data `arin_ticket_message`](../data-sources/ticket_message.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](reports-tickets.md) |
| Get Ticket Attachment | [data `arin_ticket_attachment`](../data-sources/ticket_attachment.md) | [read_catalog.go](../../internal/arin/read_catalog.go) | [Audit](reports-tickets.md) |

## Cross-reference reconciliation

The IRR guide has four object families: route/route6, aut-num, as-set and route-set. Their create/read/update/delete operations map to the corresponding `arin_irr_*` resources and individual data sources. Advanced RPSL uses `arin_irr_rpsl`. Organization inventories and both NET route scopes appear in the matrix above. Linked-route management has separate resources to preserve ROA ownership. See [route coverage](irr-routes.md), [simple IRR coverage](irr-simple.md) and [linked-route ownership](irr-route-metadata.md).

The hosted RPKI guide exposes one organization transaction endpoint and organization ROA/ASPA inventories. Individual resources and `arin_rpki_bundle` use that shared transaction endpoint; individual data sources select from the inventories. There is no additional independent PUT/DELETE lifecycle to invent for these objects. [Hosted RPKI evidence](rpki.md) records native transaction, import, update and restoration behavior.

The method guide's header-authenticated phone-add example names the POC root while its query-authenticated example names `/phone`. The client uses the verified `/phone` operation. The header example under ticket payload lists repeats a single-ticket summary URL while the query example gives the ticket collection with filters. The client implements the collection route, with native report/ticket evidence. These documentation inconsistencies do not represent additional missing operations.

## Gates that remain outside this reconciliation

- Organization creation is awaiting review. Successful live mutable CRUD, Admin association replacement and deletion still need that original disposable object.
- Native ticket submission and full Terraform NET-removal correspondence require new explicit correspondence approval. The two approved client removal messages were already consumed; this audit does not authorize repeats.
- A successful live ticket closure requires a disposable RESOLVED ticket. Closed-ticket reads/no-op lifecycle coverage does not prove that transition.
- WhoWas, Bulk Whois and invalid-POC retrieval need account access. Bulk/invalid-POC are separate download services, not missing Reg-RWS methods.
- Delegated provisioning/publication uses separate signed protocols. Repository/path verification now passes OT&E, but signed lifecycles still need enrollment; recovery edge cases and the profile audit remain implementation work.
- Public Whois-RWS and RDAP are separate interfaces. Their dedicated audits remain authoritative. Whois delegation search is still ambiguous: the guide lists a descriptive delegation-name item without an unambiguous matrix key, while the documented exact lookup and relationship operations work. No generic search has been inferred from a server ignoring a predicate.

The objective remains open. This reconciliation removes uncertainty about the documented Reg-RWS operation inventory; it does not substitute endpoint counts for payload, lifecycle or native evidence.
