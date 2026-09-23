# Linked IRR route metadata and ownership

Status: scoped client update/delete methods are implemented and pass IPv4/IPv6
OT&E tests. `arin_irr_route_metadata` is implemented with fake-server Terraform
lifecycle, import, drift repair, remark clearing and an owning-ROA dependency
graph. Native Terraform graph and linked route-set membership tests now pass
for both IPv4 and IPv6 with complete baseline restoration.

## Native evidence

The current ARIN ROA guide says an IRR route can be deleted independently of its
ROA. The `autoLinkedRoaHandle` field is informational and cannot be used to create
a link through the IRR API. A link is established through a ROA transaction.
Both guides were checked again against the official pages during this audit.

`TestOTERPKIClientLifecycle` now creates disposable linked IPv4 and IPv6 routes
and verifies these operations:

- PUT changes the description and user remarks while retaining the same ROA link.
- ARIN appends `This object is automatically managed by ARIN` as the final remark.
  Sending only user remarks round-trips them with that additional annotation.
- Omitting user remarks clears them while retaining the server annotation.
- DELETE removes the linked IRR route independently. The ROA remains, with its
  per-prefix `autoLinked` flags cleared.

The typed client lifecycle passed in 16.67 seconds. It also runs the existing
combined ROA/ASPA, AS0, deletion-policy, manual-route adoption and duplicate-ROA
checks. Exclusive journal cleanup restored both complete RPKI inventories and
confirmed disposable route absence before removing the journal.

`UpdateLinkedIRRRoute` requires the expected ROA handle and maintaining
organization. It reads the current object before writing, checks both identities,
excludes the generated annotation from the request and validates the response
and a fresh GET. Unknown annotation shapes fail rather than dropping text.
`DeleteLinkedIRRRoute` independently verifies absence after a successful response
or DELETE 404. Both methods make at most one mutation request per call and share the
organization lock used by RPKI transactions. This serializes writes within the
provider process; the API does not offer a conditional link-identity update that
would exclude concurrent changes by other processes.

Mock tests cover IPv4/IPv6, description/remarks and route-set membership, clearing,
wrong organization/link, unlinked routes, reserved annotations, failed reads,
malformed responses, changed response links, unconfirmed metadata and deletion,
and uncertain responses without automatic replay. Native linked route-set
membership changes also pass in the Terraform graph below.

## Terraform contract and remaining verification

`arin_irr_route_metadata` manages metadata on an existing linked or unlinked route.
It must not create or delete the underlying route or change its ROA authorization.
The owning ROA or bundle retains link and route deletion policy. Removing only
this metadata resource releases its state while leaving the route intact.

- Prefix, origin ASN and maintaining organization identify the target.
- An explicit expected ROA handle binds linked-route writes. A Terraform
  expression referencing a ROA or bundle handle establishes operation ordering.
- Description, user remarks and route-set membership are writable. Expose the
  current link and server annotation separately as computed metadata.
- Validate the expected organization and current link immediately before a PUT.
  Changing links must not let a stale planned update mutate another ROA's route.
- Refresh must report current link metadata without blocking a planned update
  from a replaced owning ROA. A missing route removes the metadata state; creating
  it again requires that the owning resource has recreated the route first.
- Import must identify the exact route and populate its current binding. An
  existing standalone route resource must not also own these metadata fields.
- Keep metadata reads and failed writes distinct from confirmed absence. Retain
  the natural route identity after uncertain PUT outcomes and never internally
  replay a mutation.
- Test a full Terraform dependency graph with owning ROA/bundle creation, metadata
  import/update/clearing, owner replacement, clean plans and removal of the
  metadata resource without deleting the route. Repeat against disposable OT&E
  IPv4/IPv6 objects with baseline restoration and route-set membership coverage.

Independent linked-route deletion is a separate supported API operation. The
client exposes it explicitly, but it must not become the destruction behavior of
a metadata-only Terraform resource. `arin_irr_linked_route` now supplies explicit
imported ownership of an existing linked route, including independent deletion.
`arin_irr_route` continues to reject linked objects.

References: [ARIN ROA Auto-Manager](https://www.arin.net/resources/manage/rpki/roas/#irr-auto-manager),
[IRR RESTful API guide](https://www.arin.net/resources/manage/irr/irr-restful/).

The current Terraform tests cover IPv4/IPv6 in linked and unlinked modes,
organization/link guards, uncertain PUT outcomes with retained identity, failed
refreshes and a real Terraform graph replacing the owning ROA. Changing only
the expected handle to a matching new link issues no metadata PUT when fields
already match. Removing metadata ownership sends no DELETE.

## Native Terraform graph

`TestOTEIRRRouteMetadataLifecycle` passed in 18.08 seconds. It creates a ROA
covering one disposable IPv4 /32 and IPv6 /128, two route sets authorizing the
organization maintainer, and metadata resources referencing the generated ROA
handle and route-set names. The test then:

1. Confirms descriptions, user remarks, both memberships and the exact ROA link.
2. Imports each metadata resource and checks a clean plan.
3. Changes user remarks and reduces membership to one route set.
4. Renames the owning ROA, verifies a new handle, and checks that both metadata
   resources follow the new link while retaining their configured fields.
5. Clears remarks and memberships and checks another clean plan.
6. Removes only the metadata resources and proves that the linked routes remain
   with their last configured metadata.
7. Destroys the ROA and route sets and verifies all disposable objects are absent.

The exclusive mode-0600 RPKI journal includes the baseline ROA/ASPA inventories,
disposable route IDs and helper route-set names before any mutation. Cleanup
compares both complete inventories and removes the journal only after restoration.
The graph is included in the serial `make testote` suite. Metadata resources do
not own route existence; the imported full-route resource below does.

## Independent linked-route ownership

`arin_irr_linked_route` requires explicit import of an existing linked route by
`CIDR,AS<number>`. Its `expected_roa_handle` must be nonempty and match the current
link immediately before updates or deletion. Import populates the binding and
separates user remarks from ARIN's generated annotation. It shares the metadata
update/read implementation, but destroy calls the scoped linked-route DELETE
client and verifies absence. Read errors and uncertain DELETE responses retain
state; refresh reconciles accepted deletions without replay.

The IRR API cannot create a ROA link. Accordingly, Create refuses implicit
adoption and instructs the user to create through RPKI and import explicitly.
Automatic identity replacement is rejected during planning. If the route is
removed externally, restore its link through RPKI and import again. A changed or
removed link blocks destructive writes until the ownership binding is reconciled.
For a now-unlinked route, transfer ownership explicitly to `arin_irr_route`.

Use metadata-only ownership when Terraform also maintains the owning ROA or
bundle's `auto_link` policy. Independently deleting a linked route changes its
ROA's per-prefix link state; a separate ROA resource configured to maintain that
link may subsequently recreate it. The full-route resource does not mutate ROA
authorizations or invent a policy to suppress that drift.

Mock Terraform tests cover IPv4/IPv6 import, updates, remark clearing, clean plans,
destroy, import-required creation and refused identity replacement. State tests
cover changed/missing links, foreign organizations, read failures, rejected or
unconfirmed deletion, lost DELETE responses and subsequent absence reconciliation.

`TestOTEIRRLinkedRouteLifecycle` passed in 13.22 seconds. It creates a disposable
ROA with linked IPv4/IPv6 routes outside Terraform, then imports, updates and
destroys each route through `arin_irr_linked_route`. After each deletion it confirms
route absence, the cleared per-prefix link flag, and the unchanged ROA handle and
complete authorization. Exclusive journal cleanup then removes the disposable
ROA, verifies route absence and restores both original inventories. The test is
included in serial `make testote` coverage.
