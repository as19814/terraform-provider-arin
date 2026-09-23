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
Each link exposes the server-generated `description` in resource state.
Descriptions are excluded from write payloads and uncertain-update comparisons,
so a label change cannot prevent recovery of an otherwise confirmed update.
Mock acceptance covers description-only drift followed by an organization update;
OT&E read-only recovery verifies descriptions on the existing organization.
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

## Terraform resource and recovery

`arin_org` owns the complete record and POC collection. Legal and DBA name
changes require replacement. Address, tax ID, referral Whois, comments,
reassignment acceptance and POC links update in place. Do not overlap the POC
collection with `arin_org_poc` ownership. Import existing organizations by handle;
remove dependent networks and other objects before destroying an organization.

Mock Terraform acceptance tests cover create, import, field changes and clearing,
Admin replacement, optional POC roles, POC reduction, drift correction, name
replacement and delete. Real Terraform also exercises pending creation, an
attempted second apply, manual import recovery, and pending deletion completion.
Unit tests cover API read errors, uncertain writes and retained recovery state.

Creation in OT&E returned an `ORG_CREATE` ticket with status `PENDING_REVIEW` and
no organization handle. The provider saves `pending_operation`, `pending_ticket`,
the submitted values, and a temporary `pending:` ID. It reports an error so that
an accepted request is not presented as a completed registration. Terraform can
mark this state tainted and plan a replacement; the delete handler blocks that
replacement while the outcome is unresolved, so it cannot issue a second POST.
Do not remove pending state merely to retry an apply.

A ticket's shared `orgHandle` does not identify the newly created organization.
The resource does not adopt that handle or extract an identity from free-form
messages. If creation returns only a ticket, recovery requires the approved
organization handle from ARIN:

1. Wait for terminal processing and verify the result in ARIN. For an accepted
   request, confirm the handle belongs to the organization you requested.
2. Save a private state backup and retain the pending ticket and request details.
3. With no concurrent applies, remove only the pending placeholder from state:
   `terraform state rm arin_org.example`.
4. Immediately import the approved handle:
   `terraform import arin_org.example ORG-HANDLE`.
5. Run `terraform plan` and compare the imported values with the intended record
   before making changes. Do not run an apply between state removal and import.

If ARIN denied the request, or the response was lost without a ticket, investigate
and confirm whether an organization was created before changing recovery state.
An unresolved pending create cannot be destroyed as if it were a completed object.
For pending updates/deletes with known handles, refresh waits for terminal ticket
processing and verifies the record. An uncertain update without a ticket clears
recovery state only when the desired values are visible. Pending deletion clears
only after absence is confirmed. Errors and unknown outcomes retain state.

The OT&E resource recovery test reads the existing organization and reconstructs
pending state from the saved live creation journal. It verifies refresh and
blocked destroy without mutation; its HTTP transport rejects every non-GET call.

## Live creation probe

`TestOTEOrganizationCreateProbe` has a separate `ARIN_OTE_ORG_CREATE_PROBE=1`
opt-in in addition to the usual OT&E test variables. It is excluded from
`make testote`: a staff-reviewed request must not be submitted on every test run.
The probe writes an exclusive mode-0600 journal under the user cache directory:
`terraform-provider-arin/ote-org-create-<org-hash>.json`. The input is saved and
synced before POST; the result is then saved through a temporary file and rename.
A surviving journal blocks another submission. The probe creates no new POC and
uses the existing sandbox organization's contact associations.

The current live request remains pending review, with its journal retained outside
Git. A successful submission probe establishes ticket behavior, not completed CRUD.
Do not delete its journal or rerun the probe until the request has been reconciled.
Once a disposable organization is confirmed, verify its name against the saved
input, complete the remaining mutation tests, delete only that disposable record,
and confirm absence before removing the journal. The original organization must
not be deleted as part of probe cleanup.

## Remaining work

Live disposable creation completion, mutable-field and Admin replacement testing,
and deletion await review of the existing request. No-change full-record PUT,
association lifecycles, resource refresh and pending recovery already have OT&E
evidence. Message payload support and automatic created-handle discovery, if ARIN
provides a trustworthy structured identity, remain to be investigated. Records
with unsupported message fields fail full-record decoding instead of losing them
on update. The broader [coverage inventory](implementation-status.md) remains open.

Sources: [ARIN organization methods](https://www.arin.net/resources/registry/regrws/methods/#orgs)
and [organization payload](https://www.arin.net/resources/registry/regrws/payloads/#org-payload),
checked on 2026-09-22. The older published schema omits `acceptReassignments`,
which the current payload guide and OT&E response include.
