# Managed RPKI bundle implementation requirements

Status: `arin_rpki_bundle` is implemented with generated documentation and examples.
Terraform lifecycle and recovery pass fake-server tests. Native Terraform
create/import, combined changes, member removal, linked-route cleanup, clean
plans and destroy pass OT&E with complete baseline restoration.

`PlanRPKIBundle` builds one transaction from explicit prior ownership, desired
members and freshly read inventories. `ReconcileRPKIBundle` verifies the complete
postcondition from a serialized plan without replaying writes. Unit tests cover
mixed changes, unrelated inventory, ownership collisions, incomplete visibility,
ambiguous matches, malformed receipts, policy-only changes and reused owned
handles. The native client lifecycle exercises combined ROA creation and ASPA
replacement, then ROA replacement with the unchanged ASPA retained. It verifies
baseline restoration and linked-route cleanup. These tests establish native engine behavior. Terraform fake-server tests now also
cover multiple ROAs/customers, exact manifest import, atomic updates/removals,
policy-only changes, drift, missing members, single-category bundles, AS0,
delayed visibility and recovery without replay. State tests cover malformed and
partial responses, verification failures, returned identity preservation,
ambiguous matches and uncertain deletion.

`TestOTERPKIBundleLifecycle` imports an existing ASPA explicitly, adds disposable
IPv4/IPv6 ROAs while changing its provider set, imports the full manifest,
replaces the ROAs to enable IRR linking while restoring providers, removes the
IPv6 member while changing providers again, and destroys the remaining members.
A second Terraform lifecycle creates both ROAs and the now-absent customer ASPA
from empty state and destroys them. Both phases check clean plans and preserve
unrelated ROAs. The test uses the exclusive RPKI baseline journal, verifies
linked-route absence, restores the original ASPA, compares both complete
inventories and removes the journal only after successful restoration. The
native run passed in 17.35 seconds. Multiple customer ASPAs are exercised by
mocks; the native test uses one customer owned by the sandbox organization.

The hosted RPKI API supports one transaction containing ROA and ASPA additions
and deletions. `ApplyRPKITransaction` already exercises this behavior in OT&E.
Separate `arin_roa` and `arin_aspa` resources cannot provide this guarantee:
Terraform may apply them separately, even with `depends_on`. Supporting every
operation therefore includes an explicit resource for a managed atomic group.

## Resource contract

`arin_rpki_bundle` uses the following ownership model:

- `org_handle` identifies the hosted RPKI organization and requires replacement.
- `name` is a local bundle identity, also immutable. ARIN has no remote bundle
  object or bundle tag. The provider ID is `ORG-HANDLE/BUNDLE-NAME`.
- `roas` is a map keyed by user labels. Each entry specifies the existing ROA
  resource's name, ASN, prefix-to-maximum-length map, auto-link setting and local
  linked-route deletion policy. Its ARIN handle and generated validity/renewal
  metadata are computed independently of the stable map key.
- `aspas` is a map keyed by canonical decimal customer ASN, with the complete
  provider-AS set as each value.
- At least one ROA or ASPA is required. Unrelated organization inventory is never
  implicitly adopted or deleted. An empty category is permitted.
- A separate sensitive recovery attribute persists uncertain transactions.

Standalone resources and bundles must not own the same ROA handles or ASPA
customer identities. A local label does not prove remote ownership. On create,
existing matching ROAs or existing customer ASPAs require explicit import.

## Planning and writes

Read current inventories before planning a write. Retain handles for unchanged
ROAs. Replace changed ROAs by deleting their current handle and adding the desired
authorization in the same transaction. Add/delete ASPAs by customer identity;
provider changes use delete/add together. Remove only entries already tracked by
this resource. Ignore server-generated validity changes when deciding to write.

Submit all changed ROAs and ASPAs in one POST. A change only to the local bundle
name is replacement; a change only to linked-route deletion policy is state-only.
When replacing a ROA, unlink and preserve its former IRR routes as the standalone
resource does. When removing an entry or destroying the bundle, use that entry's
saved deletion policy. Do not issue an empty transaction.

Do not represent the bundle as a list of imperative transactions or accept a raw
XML write payload. Terraform configuration describes the desired managed objects;
reads, diffs and cleanup must remain meaningful after repeated applies.

## Import and recovery

Because ARIN stores no group identity, import must name exact members. Accept an
explicit JSON manifest containing `org_handle`, local `name`, a `roas` map from
stable labels to ARIN handles, and an `aspas` list of customer ASNs. Fetch every
specified member and reject missing or repeated identities. Import must not infer
a whole organization inventory from an organization handle alone.

Before a write, capture desired objects, prior managed identities, the exact
transaction and baseline ROA handles. An accepted or uncertain write must retain
its useful returned identities even if a later response or inventory check fails.
Reconciliation must establish every intended addition, replacement and deletion
before clearing recovery state. New ROAs must match both the authorization and
the expected post-write identity set; ambiguous matches are errors. An unresolved
write blocks create/update/destroy replay. A read error does not establish absence.

## Required verification

- Mock Terraform create, import, update, removal of one member, clean plan and
  destroy, with unchanged sibling inventory preserved throughout.
- Assert that a change spanning ROAs and ASPAs is a single atomic POST; exercise
  multiple ROAs and multiple customer ASPAs with stable user labels.
- Cover drift, missing members, default maximum lengths, AS0 validation, changed
  computed handles, and changes limited to local deletion policy.
- Exercise rejection before mutation, accepted writes with lost responses,
  partial/malformed responses, verification GET errors, delayed visibility,
  ambiguous recovery and repeated applies without replay.
- Run an OT&E combined lifecycle with disposable IPv4/IPv6 ROAs and a saved ASPA
  baseline. Use the existing exclusive RPKI recovery receipt. Verify complete
  inventory restoration and linked-route cleanup before removing that receipt.

The lifecycle checks above now have Terraform mock and native evidence. Recovery
faults are injected through the fake server; native tests do not intentionally
interrupt live transactions. Coordinated ownership with separately managed IRR
routes now has a metadata resource with native owning-ROA graph evidence.
Imported full-route ownership and independent linked-route deletion also have
native IPv4/IPv6 evidence; see [linked-route coverage](irr-route-metadata.md).
