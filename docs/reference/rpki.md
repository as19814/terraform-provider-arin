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
client exposes creation and deletion autoLink flags; coordinated IRR lifecycle
verification is still outstanding.

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
complete baseline inventories. Tests use `autoLink=false` and do not create IRR
objects.

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
identified by the saved request and restore the recorded customer ASPA. Verify
both complete inventories before removing the snapshot. Do not replace unrelated
objects or submit a second creation merely because the first response was lost.
All test origins are pinned to OT&E; normal CI uses fake servers only.

## Remaining work

The Terraform ROA resource and import, auto-linked IRR ownership, explicit maximum
length cases, AS0 ROA live verification and the final endpoint audit remain in the
[implementation inventory](implementation-status.md). The shared transaction
client already supports combined operations, while the ASPA resource manages a
single customer identity. Any need for a declarative multi-object transaction
resource must be reconciled during that audit.

Reference: [ARIN RPKI RESTful API guide](https://www.arin.net/resources/manage/rpki/rpki-restful/)
and its [collected copy](arin-api/rpki/api.md).
