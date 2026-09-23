# IRR route membership

The `arin_irr_route` resource owns descriptions, remarks and `member_of`, a set
of route-set names. Omitting membership clears it on the next apply. Prefix,
origin ASN and organization remain replacement fields. Membership changes update
the route in place. The individual `arin_irr_route` data source returns membership
as `member_of` as well.

ARIN's RoutePayload schema defines a `memberOf` container holding zero or more
`routeSetRef` elements with a `name` attribute. This differs from aut-num XML,
which uses repeated `memberOf` elements with a name attribute directly. The route
client validates route-set names and refuses unsupported membership fields rather
than dropping data during an update.

A route set's `members_by_ref` must authorize the route maintainer, for example
`MNT-EXAMPLE-1`, to include the referencing route. Terraform expressions linking
`member_of` to a managed route-set name also establish creation/deletion ordering.
See the generated resource example.

## Verification

Fake-server Terraform tests cover IPv4 and IPv6 routes with multiple memberships,
set ordering, import, updates, external drift, omission-based clearing, clean plans
and deletion. Client tests check the actual nested XML shape, invalid names and
unsupported representations. Data-source acceptance coverage checks membership
returned by the individual route endpoint.

OT&E tests passed for both a disposable IPv4 /32 and IPv6 /128 using AS64496:

- Confirm prefix ownership and absence of the route before creation.
- Create two randomly named route sets permitting the test organization's maintainer.
- Create a route referring to both sets, then update it to refer to only one.
- Import it and verify a clean plan with membership present.
- Clear membership, verify a clean plan, and delete the route.
- Delete both helper route sets and confirm every test object is absent.

No production writes are used. The test does not claim that expanded set membership
has propagated to external IRR mirrors. Advanced RPSL has separate native coverage. The linked-route client now supports
scoped metadata changes and independent deletion with IPv4/IPv6 OT&E evidence;
the metadata resource and owning-ROA graph pass Terraform mocks and IPv4/IPv6
OT&E with membership, replacement and restoration. Imported full-route
ownership with independent linked-route deletion also passes IPv4/IPv6 OT&E. See [linked-route requirements](irr-route-metadata.md).

References: [ARIN IRR API guide](https://www.arin.net/resources/manage/irr/irr-restful/),
[RoutePayload schema](arin-api/schemas/extracted/RoutePayload.rnc),
[routeSetRef schema](arin-api/schemas/extracted/utils/routeSetRef.rnc).

## Simple XML endpoint and field audit

Both IP families use `/rest/irr/route/IP/PREFIXLENGTH/ORIGINAS` for POST, GET,
PUT and DELETE. `arin_irr_route` implements the unlinked lifecycle;
`arin_irr_route_metadata` manages metadata on existing routes, while
`arin_irr_linked_route` explicitly imports ownership of linked routes and supports
independent deletion. Linked creation belongs to hosted RPKI. Advanced RPSL
objects use their separate resource and data source.

The remaining list endpoints map to `arin_irr_routes` (organization routes) and
`arin_net_routes` (network routes). The latter exposes `include_reassignments`
for the documented `reassignments` query parameter. Collection entries preserve
entry type, organization, origin ASN and prefix. Path tests cover these endpoints;
the existing live Terraform read suite exercises organization and direct network
lists. `TestOTENetAssignmentClientLifecycle` now creates a route under a disposable
organization-recipient reassignment for each IP family and directly verifies:

- The child's direct list contains the route.
- The parent's direct list excludes the child route.
- The parent's list with `reassignments=true` includes the child route.
- The organization list includes the route with matching prefix, origin,
  organization and `SIMPLE` entry type.

Both IPv4 /32 and IPv6 /64 cases passed. The route identity is durably recorded
before POST, and cleanup verifies route absence before the fixture deletes its
NET and customer. A leftover route receipt blocks another probe until reconciled.
This is native client coverage of list semantics; it does not claim a separate
Terraform acceptance run for downstream inclusion.

| Simple payload field | Provider behavior and evidence |
| --- | --- |
| prefix, originAS, orgHandle | Managed identity; resource replacement fields; both IP families pass native lifecycle and import |
| description, remarks, memberOf | Writable metadata; native updates, clearing and route-set membership verified |
| source | Fixed to ARIN, as required by the guide |
| creationDate, lastModifiedDate, version | Guide explicitly marks inputs ignored; dates exposed as computed metadata |
| netHandle | Derived network; computed output; native POST and PUT ignore a nonexistent supplied handle |
| pocLinks | Generated POCs; computed output; native POST and PUT reject partial baseline POC sets as changes to system-generated fields |
| autoLinkedRoaHandle | Computed linkage controlled through hosted RPKI; scoped metadata and independent deletion have separate native evidence |

`TestOTERPKIClientLifecycle` now audits generated route fields inside its
journaled disposable IPv4/IPv6 fixture. Unlinked POST accepts complete or empty
POC containers and a nonexistent network handle, deriving the original POCs and
correct network. Unlinked and linked PUT accept complete or empty POC containers,
and echoed, empty or nonexistent network handles, preserving the derived fields.
Partial POC sets return HTTP 400 with a system-generated-value validation error
for both methods and both IP families. Rejected POST leaves the route absent;
the fixture recreates it normally before continuing. Linked POST is not inferred
from these tests: linked routes are created through RPKI.

The audit rereads identity, organization, network, POCs, description, remarks,
membership and ROA linkage after each probe. Generated timestamps are excluded
from equality checks because the POST probes recreate objects. The enclosing
fixture restores the original RPKI inventories and removes disposable routes.
These results resolve the guide's ambiguity about writable `pocLinks` and
`netHandle` without relying on behavior from other IRR object types.
