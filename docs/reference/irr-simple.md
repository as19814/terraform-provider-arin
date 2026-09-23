# Simple IRR endpoint and field audit

Audited against the [ARIN IRR API guide](https://www.arin.net/resources/manage/irr/irr-restful/)
on 2026-09-23. Advanced objects are covered separately by [RPSL support](irr-rpsl.md).

## AS sets and route sets

| API operation | AS sets | Route sets |
| --- | --- | --- |
| Create | POST `/rest/irr/as-set?orgHandle=ORG` | POST `/rest/irr/route-set?orgHandle=ORG` |
| Read/update/delete | GET/PUT/DELETE `/rest/irr/as-set/NAME` | GET/PUT/DELETE `/rest/irr/route-set/NAME` |
| Org inventory | GET `/rest/org/ORG/as-sets` | GET `/rest/org/ORG/route-sets` |

The individual/list data sources and corresponding resources cover these
endpoints. Both resources represent name, organization, description, remarks,
explicit members and membership-by-reference. Route sets additionally represent
multiprotocol members. Dates, source and complete POC link records are computed.

Native AS-set tests create with `MNT-ORG` membership-by-reference, change to
`ANY`, then clear it. Explicit membership changes and remark clearing also pass.
Route-set tests cover IPv4/IPv6 membership, maintainer references, the `^+` range
operator and clearing. Both lifecycles include import, clean plans and confirmed
deletion. Membership-by-reference configuration round-trips; these tests do not
claim to compute or enumerate the IRR's recursively expanded membership graph.

## Aut-num

POST creates `/rest/irr/aut-num/ASNUMBER`; GET/PUT/DELETE address
the same path; the org inventory is `/rest/org/ORG/aut-nums`. The client, resource and individual/list data sources
implement these endpoints. The resource represents ASN identity, AS name,
organization, description, remarks, AS-set membership and all six routing policy
collections: import, export, default and their multiprotocol counterparts.
Dates, source and POC link records are computed.

The native Terraform lifecycle creates an IRR object only after confirming that
an owned ASN has no existing object. It updates policies, clears collections,
imports, verifies a clean plan and confirms deletion. The ASN registration is
not modified. Both XML and advanced RPSL lifecycle evidence is recorded in the
coverage inventory.

## Computed metadata

All four simple IRR resources now expose `source` and each POC link's
`description`, alongside its handle and function. Client decoders validate that
the returned source is ARIN before producing state. Role descriptions are read
from the response. Outgoing XML continues to omit server-generated POC links.

Fake-server and OT&E lifecycles verify the metadata on AS sets, route sets,
aut-num, IPv4 routes and IPv6 routes, including import and clean plans. Disposable
objects were removed with subsequent reads confirming absence. Writable POC
links on organizations and network metadata retain their existing configuration
schema; this addition concerns computed IRR links.

The AS-set, route-set and aut-num endpoint/field reconciliation is complete.
Route/route6 still needs the coordinated ROA-linked audit recorded in
[route evidence](irr-routes.md). This does not close the provider-wide inventory.
