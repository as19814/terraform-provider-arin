# Organization registration implementation evidence

The full organization client is separate from the minimal organization discovery
model. `GetRegisteredOrganization` reads names, address, tax ID, referral Whois
URL, comments, reassignment acceptance, generated identity and POC associations.
Unknown fields or attributes, duplicate singleton fields, invalid identities and
incomplete required contacts are rejected instead of silently discarding data
that could be needed for a subsequent full-record update.

Creation clears server-generated identity. Updates read the current record,
preserve its registration date, and reject changes to immutable legal and DBA
names. The complete POC collection is authoritative, including the Admin contact.
`UpdateOrganization` and `DeleteOrganization` share the per-organization mutation
lock with individual association operations. Callers must avoid overlapping
ownership of full collections and individual associations.

`OrganizationWriteResult` represents either a completed record, a pending ticket,
or both. Ticket details survive a malformed accompanying organization response.
HTTP 202 without a recovery ticket is an error, while any available identity is
retained in the returned result. Callers must persist results before reporting
errors and must not resubmit an uncertain creation automatically. Completed
deletion is verified by a fresh 404; an accepted deletion ticket is returned for
later reconciliation. Access and server errors do not establish absence.

## Validation evidence

Mock tests cover CRUD, address changes, clearing optional values, generated
metadata preservation, immutable name rejection, malformed records, pending
creation/deletion, ticket preservation on partial response errors, no retries
on mutation errors, and rejection of unconfirmed deletion. These tests establish
client behavior, not that every field mutation is accepted by OT&E.

`TestOTEOrganizationClientNoChange` passed a full-record GET/PUT/GET in OT&E.
It sends unchanged values, saves an exclusive mode-0600 recovery snapshot outside
the repository, and compares the complete record after the write, normalizing
only POC ordering. It removes the snapshot only after confirmed equality and no
pending ticket. A pending ticket is retained with the snapshot for reconciliation.
The live test does not request address, tax ID, Admin, or other account changes.

Run it through `make testote`, or select it explicitly with
`ARIN_OTE_WRITE_TESTS=1 TF_ACC=1 go test ./internal/arin -run '^TestOTEOrganizationClientNoChange$' -v -count=1`.
It requires `ARIN_OTE_API_KEY` and `ARIN_TEST_ORG_HANDLE`; both API origins are
pinned to OT&E. Recovery files reside under the user cache directory in
`terraform-provider-arin/ote-org-record-<org-hash>.json` and contain private account
data. They must not be committed.

## Remaining work

This client is not yet registered as a Terraform managed organization resource.
The existing organization data source and `arin_org_poc` resource remain available.
Next steps are persisted Terraform ticket state, reconciliation to a generated
organization handle, live disposable creation/deletion, mutable-field and Admin
replacement validation, and any message payloads required by registration workflows.
Records containing unsupported message fields currently fail full-record decoding
rather than losing those fields during an update.

Sources: [ARIN organization methods](https://www.arin.net/resources/registry/regrws/methods/#orgs)
and [organization payload](https://www.arin.net/resources/registry/regrws/payloads/#org-payload),
checked on 2026-09-22. The older published schema omits `acceptReassignments`,
which the current payload guide and OT&E response include.
