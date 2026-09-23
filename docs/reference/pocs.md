# POC implementation evidence

`arin_poc` manages role and person points of contact. The client implements GET,
POST, PUT and DELETE using authenticated Reg-RWS calls. The existing `arin_poc`
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

## Remaining work

Individual phone/email mutation endpoints and resource ownership boundaries
remain to be implemented. Organization associations will be covered with the
organization resources. The full implementation inventory remains authoritative
for broader outstanding API families.

References: collected [methods](arin-api/reg-rws/methods.md#pocs) and
[payload](arin-api/reg-rws/payloads.md#poc-payload) guides.
