# Hosted RPKI implementation evidence

The shared client reads typed ROA/ASPA inventories and submits combined changes
through `POST /rest/rpki/ORG`. A transaction can add and delete both object types.
Updates use delete/add in one transaction, avoiding an intermediate missing object.
Writes are serialized per origin/organization within the provider process and
are never automatically replayed after an error or lost response.

The client validates response identities and authorization contents, then checks
fresh inventories for every addition and deletion. Replacing an ASPA keeps the
same customer-AS identity. ROA additions return the full verified inventory
record, including generated dates. Partial errors preserve decoded additions for
recovery. An uncertain POST requires inventory reconciliation before retrying.

## Wire formats and defaults

ASPA GET requires `Content-Type: application/xml` even without a request body.
The existing authenticated client already supplies it. Changing only the Accept
header does not satisfy this requirement; standalone probes without Content-Type
receive HTTP 415 and E_UNSUPPORTED_MEDIA_TYPE.

Inventory collections and their ROA/ASPA entries use the core namespace while
fields use the RPKI namespace. ROA inventory responses repeat flat `resources`
elements; transaction responses use `resources/roaSpecResource` and may include
`autoLink`. The client explicitly supports both shapes and rejects mixed,
unexpected or partial data. Unknown collection attributes are errors rather
than evidence that a managed object disappeared.

An omitted ROA maxLength remains absent in the model. Authorization comparisons
use the prefix length as its effective default. IPv4/IPv6 prefixes must be
canonical and maximum lengths must fit their address families. The transaction
client exposes creation and deletion autoLink flags. Sandbox evidence below
confirms their effects on disposable IPv4 and IPv6 IRR routes.

## ROA resource

`arin_roa` imports as `ORG-HANDLE/ROA-HANDLE`. Its `prefixes` map contains
canonical IPv4/IPv6 CIDRs and their maximum prefix lengths. An API record with
omitted maxLength is read as the CIDR length. The `asn` field accepts AS0 with `auto_link=false`. Native OT&E accepts an AS0
request with autoLink=true but silently creates an unlinked ROA and no IRR
objects. Local validation rejects that combination before any write, preventing
an unsatisfied Terraform plan and an unnecessary recovery journal.
Updates atomically delete the old ROA and add the new authorization, so both
`handle` and `id` change. A matching existing authorization requires import.
A change only to the local deletion policy does not submit a new transaction.

`auto_link` controls linking at creation/replacement. `linked_prefixes` reports
the current links individually. On read, `auto_link` is true only if every prefix
is linked, including for imported objects. Updates always delete with
`autoLink=false`, preserving previously linked routes after unlinking them.
Destroy also preserves routes by default. Set `delete_linked_routes=true` to
remove them instead. This deletion policy is local and imports default to false.
ARIN can adopt existing routes when auto-linking. Avoid managing those same
objects through `arin_irr_route`. OT&E rejects a second ROA with the same origin
and prefixes, even under a different name, with HTTP 400 and a same-origin prefix
validation error. The rejected transaction leaves both inventories and the
original IRR links unchanged. Atomic replacement remains supported because the
old authorization is removed in the same transaction.

Adopting disposable manual IPv4/IPv6 routes preserved their descriptions,
original remarks, POC links, network and organization. Linking added one remark;
unlinking with `autoLink=false` removed that annotation and restored the original
metadata. The provider does not promise to preserve arbitrary advanced RPSL
extensions without further IRR coverage.

An uncertain create or update stores `recovery_data`, including the requested
authorization, the pre-write handles, and the previous handle for updates.
Refresh accepts exactly one new matching authorization and requires the previous
handle to be absent. Zero matches, multiple matches, or a surviving previous
handle retain state and produce a recovery error. Reads never treat an unresolved
write as a missing resource; updates and destroy must reconcile it first.
Transactions are not replayed. A successful POST followed by a failed verification
GET retains recovery state even when that GET returns HTTP 403 or another 4xx.
ASPA creation likewise retains its natural identity in this situation.

If reconciliation cannot establish a unique result, preserve a state backup and
inspect the organization's ROAs. Once the correct handle is established, remove
only the pending Terraform state entry and import that handle. Do not rerun
creation or delete possible matches blindly. `recovery_data` is marked sensitive
to keep the journal out of ordinary plan output, but Terraform state still
contains it and must be protected.

Terraform mock tests cover create/import, atomic updates, drift repair,
missing-object recreation, deletion-policy-only changes, and sibling preservation.
A real Terraform recovery test confirms a second apply cannot resubmit an
unresolved creation, then exercises late inventory discovery and an accepted
update with a lost response. Unit tests additionally cover ambiguous discovery,
read failures, verification failures and unconfirmed deletion.

The Terraform OT&E lifecycle passes IPv4/IPv6 creation and import, AS0 origin
updates, IRR linking, unlinking while preserving routes, re-adopting those routes,
and destroy with linked-route deletion. Cleanup verifies every disposable route
is absent and both original RPKI inventories are unchanged. Its private recovery
snapshot records the disposable name and route IDs before mutation.

## ASPA resource

`arin_aspa` imports as `ORG-HANDLE/CUSTOMER-ASN`. It owns the complete nonempty
provider-AS set for that customer. Existing ASPAs require import before create.
Organization/customer changes replace the resource; provider changes use an
atomic delete/add transaction. Destroy deletes only the managed customer's ASPA.

A customer cannot be its own provider, and provider ASNs must be unique. AS0 is
supported as the sole provider, representing a no-provider declaration. It must
not be mixed with other provider ASNs. This follows the
[IETF ASPA registration description](https://www.ietf.org/archive/id/draft-ietf-sidrops-aspa-verification-19.html#section-5)
and was verified in OT&E. ARIN rejects other reserved provider ASNs, including
the documentation ASN used in its API example. Sandbox tests therefore discover
an allocated ASN through RDAP before adding it temporarily to a provider set.

Creation retains its natural identity on uncertain outcomes so refresh can
recover an accepted transaction. Read failures preserve state; missing objects
are removed from state. Unconfirmed deletion retains state. Failed transactions
are not retried internally.

## Sandbox evidence and recovery

The combined client lifecycle passes creation of one ROA containing an owned
IPv4 /32 and IPv6 /128, an ASPA provider-set replacement in the same transaction,
and atomic ROA replacement. A rejected reserved-provider probe left both original
inventories unchanged. Successful writes also completed cleanup and restored the
complete baseline inventories. The lifecycle also replaces the ROA with an AS0
ROA containing IPv4 /31 with maxLength 32 and IPv6 /127 with maxLength 128.
The expanded ranges are checked against the original owned parent before use.
These non-default maximum lengths survive the transaction and inventory reads.
A separate raw request intentionally bypasses local validation to verify the
native AS0 auto-link normalization described above. It uses the same disposable
prefixes, verifies no AS0 IRR routes exist, and deletes the resulting unlinked
ROA during cleanup.

The test also creates linked IPv4 /32 and IPv6 /128 IRR routes through a ROA,
verifies their `autoLinkedRoaHandle`, and exercises both deletion flags:

- `autoLink=false` deletes the ROA and retains the routes with their ROA links
  cleared. The now-independent routes can be deleted through the IRR API.
- `autoLink=true` deletes the ROA and both linked routes.

Each candidate route is proven absent before any mutation. Cleanup removes only
these disposable routes, verifies their absence, and restores both complete RPKI
inventories. The manual-route adoption and duplicate-link checks described above
cover pre-existing disposable routes and rejected duplicate authorizations as well.

The Terraform ASPA lifecycle imports an existing sandbox record, changes its
providers, verifies import and a clean plan, changes to an AS0-only declaration,
restores configured providers, and destroys the record. Registered cleanup then
recreates the original ASPA and compares all ASPAs and ROAs with their baseline.
Mock tests additionally cover drift correction, missing-object recreation,
sibling preservation, import guards, lost-response recovery, read errors and
unconfirmed deletion.

Both client and provider tests use an exclusive mode-0600 recovery snapshot in
the user cache directory under
`terraform-provider-arin/ote-rpki-<org-hash>.json`. The snapshot is synced before
mutation and removed only after verified restoration. It contains account data
and must not enter Git. A surviving snapshot blocks another test against that
organization. `make testote` runs packages sequentially to avoid overlapping tests.

If cleanup fails, inspect the snapshot locally. Remove only the disposable ROAs
identified by the saved request or `Names` list and restore the recorded customer
ASPA. Delete the disposable ROAs with `autoLink=true`. Check each saved request
prefix and ASN
for a leftover IRR route; remove it only if it is unlinked. The test proved those
route identities absent before mutation. Also verify the saved request prefixes
have no AS0 IRR routes left by the native normalization probe. Verify both complete
inventories and route absence before removing the snapshot. Do not replace unrelated
objects or submit a second creation merely because the first response was lost.
All test origins are pinned to OT&E; normal CI uses fake servers only.

## Endpoint and payload audit

The current official hosted RPKI guide was rechecked on 2026-09-23. Its three
HTTP endpoints map to the provider as follows:

| Endpoint | Implemented coverage |
| --- | --- |
| `GET /rest/roa/ORG` | `arin_roas`, collection-filtered `arin_roa`, managed ROA refresh and transaction verification |
| `GET /rest/aspa/ORG` | `arin_aspas`, collection-filtered `arin_aspa`, managed ASPA refresh and transaction verification |
| `POST /rest/rpki/ORG` | Shared ROA/ASPA additions/deletions, standalone CRUD/replacement, and combined client lifecycle |

ROA requests cover name, origin ASN, auto-link policy, prefixes and optional
maximum lengths. Reads retain generated handles, validity and renewal metadata,
per-prefix link status and the documented flat/wrapped resource shapes. Address
range and IP-family metadata are validated against each CIDR. ASPAs cover customer
ASN and the complete provider set. Deletion supports ROA handles with their
`autoLink` policy and ASPA customer identities. Both standalone objects import
from the organization collections; the guide defines no separate single-object
GET endpoint. Automatic renewal and validity dates are server metadata.

Deletion receipts are now checked against the submitted identities. Unexpected
or repeated ROA/ASPA deletion identities reject the response while retaining
parsed additions for recovery. Omitted deletion echoes still require fresh
inventory confirmation; a receipt alone never proves a requested deletion.
Mock tests cover matching, omitted, duplicated and unrelated receipt identities.
The combined OT&E client lifecycle passed again with these checks, restoring
both complete inventories and confirming disposable IRR route absence.

## Remaining work

The [managed atomic bundle](rpki-bundle.md) is now implemented. Terraform mock
coverage includes multiple ROAs/customer ASPAs, import, combined changes,
policy-only updates, drift, missing members and uncertain-write recovery. Native
Terraform create/import, combined changes, IRR links, member removal and destroy
pass OT&E with both inventory baselines restored.

Coordinated ownership of ROA-linked IRR routes with separately managed IRR
resources remains in the implementation inventory. Bundles and standalone ROAs
currently own their links, and `arin_irr_route` rejects linked objects.

Reference: [ARIN RPKI RESTful API guide](https://www.arin.net/resources/manage/rpki/rpki-restful/)
and its [collected copy](arin-api/rpki/api.md).
