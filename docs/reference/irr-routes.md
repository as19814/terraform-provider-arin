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

## Remaining field audit

The simple XML guide lists `pocLinks` and `netHandle` without marking them ignored,
while the current simple route client treats both as server-generated metadata.
Verify their write behavior on disposable routes before closing this family's
field audit. The AS-set POC rejection evidence alone does not establish route or
route6 behavior. The guide explicitly marks creation/modified dates and version
as ignored and fixes source to ARIN. Description, remarks, route-set membership,
identity and ROA-linked lifecycle have the native evidence described above.
