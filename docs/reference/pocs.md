# POC implementation evidence

`arin_poc` manages role and person points of contact. The client implements full-record GET, POST, PUT and DELETE plus individual
email and phone operations using authenticated Reg-RWS calls. The existing `arin_poc`
data source remains available for read-only use.

## Ownership and identity

Creation uses `/rest/poc;makeLink=true` so the API key's ARIN Online account is
linked to the new contact and can manage it. The provider does not expose an
unlinked creation mode that would immediately prevent further management.
The server generates the handle and registration date. Import uses the handle.

Contact type, first name, middle name and last name are immutable. Terraform
requires replacement for changes to those fields; the client independently
rejects in-place changes. Company name, address, emails, phones and operational
comments are mutable. The resource owns both complete contact collections.
Email addresses and phones are sets, since response ordering is not stable.
Phone identity within a collection is the type/number pair.

POCs must be detached from dependent organizations and networks before deletion.
The provider does not remove associations implicitly. It verifies a 404 after a
successful delete and preserves state on access errors, conflicts, rate limits
and server failures. An uncertain creation is not retried automatically: locate
and import the created handle before retrying to avoid duplicate contacts.

Names, addresses, emails, phones and comments are marked sensitive in Terraform
output. They remain present in state and are not a substitute for state access
controls. ARIN's registration publication behavior is independent of Terraform's
sensitive flag.

## Sandbox findings

- Role and person create, read, update and delete lifecycles pass using disposable
  records linked to the sandbox API account.
- Role first/middle names that are empty must be omitted from XML on update.
  Sending empty elements returns an immutable-field error. Existing nonempty
  immutable names are retained verbatim.
- Person names reject digits. Synthetic live-test names use alphabetic suffixes.
- Empty comments clear by omitting the comment element.
- Full replacement can add and remove email addresses and phone records.
  Omitting a previously configured phone extension clears it.
- Terraform create, import, contact updates, collection reduction, stable plans,
  role-to-person replacement and destroy pass in OT&E.
- Deletion is verified by a subsequent missing-object response. Earlier failed
  probes also ran registered cleanup and left no disposable POCs behind.

Mock tests cover complete payloads, account linking, preservation of generated
identity, immutable-field rejection, strict response decoding, preflight read
failures, drift correction and state retention on API errors.

## Individual contact endpoint evidence

The client supports email POST/DELETE and phone PUT/DELETE without replacing
unrelated contact collections. Phone deletion accepts an exact number/type pair,
a number across all types, or a type across all numbers; an empty selector is
rejected before a request. All mutations read the resulting POC and verify the
requested addition/removal. Unconfirmed writes are not retried automatically.

OT&E role and person tests verify email addition/removal (including a plus-tag
address), phone addition, exact deletion, number-only deletion, and type-only
deletion. The complete POC is compared with its baseline afterward and the
disposable contact is then deleted and verified missing.

The phone addition URL is `/rest/poc/HANDLE/phone`. The collected method guide's
header-auth example omits `/phone`, while its query-auth example includes it;
OT&E confirms that `/phone` works with header authentication and a phone payload.

An existing phone is identified by type and number. A duplicate phone PUT returns
success but leaves the old extension unchanged, even if a different nonempty
extension is supplied. `AddPOCPhone` returns an existing identical phone without
writing and rejects a changed extension. Use a full POC update to modify an
extension, or remove and re-add the individual record. This differs from the
full POC update, which can clear or replace extensions.

Mock tests cover request paths, validation, unrelated-record preservation,
no retries after mutation errors, and rejection of success responses whose
requested changes cannot be observed.

## Individual Terraform resources

`arin_poc_email` imports as `POC-HANDLE/EMAIL`. `arin_poc_phone` imports as
`POC-HANDLE/TYPE/NUMBER`. Both require an existing POC, refuse to adopt existing
records without import, and remove only their own record on destroy. Multiple
individual resources may share a POC when they manage different records; they
must not overlap the complete collections owned by `arin_poc`.

All configured changes require replacement. For a phone, extension changes use
delete then add, since the native addition endpoint ignores extension updates.
Do not use `create_before_destroy` for an extension change on the same type and
number. ARIN may reject removal of contact details required for a valid POC;
full-POC updates can change an extension without removing the phone first.

The identity is saved even if creation has an uncertain outcome. Refresh can
reconcile a lost response. Access errors, conflicts and unconfirmed deletion
retain state. Identity and contact values are sensitive in Terraform output,
while still stored in state.

Mock acceptance tests cover multiple simultaneous records, import, email and
phone-extension replacement, extension clearing, stable plans, missing-record
recreation, drift and sibling preservation. State tests cover import guards,
uncertain creation, access errors and unconfirmed deletion. The combined OT&E
Terraform lifecycle passes against a disposable POC. After resource destroy,
the complete POC is compared with its baseline; cleanup deletes the POC and
verifies absence.

## Remaining work

Organization associations are covered by `arin_org_poc`; see [evidence](org-pocs.md).
The final API audit remains, including reconciliation of the phone type
description metadata (already exposed by the read data source) with managed
resource state. Country code3 and E.164 calling code metadata now round-trip
through resource state, with native Terraform lifecycle and import coverage. The
full implementation inventory tracks the broader outstanding API families.

References: collected [methods](arin-api/reg-rws/methods.md#pocs) and
[payload](arin-api/reg-rws/payloads.md#poc-payload) guides.
