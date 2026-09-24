# Report and ticket implementation evidence

Report requests create server-side jobs and return ticket identities. They must
not be exposed as data sources that submit work on refresh, even though ARIN uses
GET for these operations.

## Terraform report receipts

`arin_report_request` submits once and stores the generated ticket number.
`report_type` and `target` are immutable: changing them or explicitly replacing the
resource submits a new request. Normal refresh only reads ticket metadata.
Creation succeeds when ARIN accepts the job, including an IN_PROGRESS ticket;
`status` and `resolution` update on later refreshes. Use existing ticket, message
and attachment data sources to retrieve the eventual report.

ARIN can delete report tickets after expiry. A 404 for a previously confirmed
receipt sets `ticket_available=false`, retains its last known metadata and never
requests a replacement. Other read failures preserve state and return an error.
Destroy forgets only the Terraform receipt. It does not cancel, close or delete
the server ticket. These semantics apply to a submitted request, not a promise to
keep a report downloadable forever.

Imports use `associations/TICKET` or `REPORT-TYPE/TARGET/TICKET`. Provide the
original target: ticket summary metadata confirms the report category but does
not independently expose or verify its original target. Import requires a
readable ticket with the matching category. An already-expired ticket cannot be
newly imported, although an existing receipt can survive expiry.

If a submission returns a ticket number but incomplete metadata, create tries a
summary read to confirm it without resubmitting. If that succeeds, no error taints
the new resource. Local precondition failures and definite rejection responses
leave no pending receipt because no accepted request needs recovery.

Otherwise `pending_submission=true` preserves an uncertain request. No ticket
number means manual reconciliation against the ARIN ticket list is necessary.
Even when a later refresh confirms a known ticket, it does not silently clear the
pending marker: Terraform may have tainted the failed creation and would replace
it on the next apply, submitting a duplicate report. Back up state, identify the
correct report, remove only the pending receipt from state, and import that ticket
using its original report type and target. Refresh and destroy block while this
recovery remains outstanding. Do not discard pending state to force a retry.

Mock Terraform tests verify both supported report types, import, request replacement,
clean plans, expiry without resubmission, and destroy without server mutations.
Recovery tests cover both a partial response and a completely lost response. They
prove an ordinary second apply cannot duplicate the request even with
`create_before_destroy=true`, then verify manual import recovery with only one
submission. Unit tests also cover missing credentials, partial-response
confirmation, unresolved identities and read failures.

The OT&E Terraform lifecycle creates an associations report, imports its ticket,
checks a clean plan, and destroys only the local receipt while the server ticket
remains available. It retains a separate private recovery file at
`terraform-provider-arin/ote-report-resource-<org-hash>.json`. Later test runs import
that saved ticket instead of creating another report. A second live run verified
this import-only path and observed the completed ticket as CLOSED. An empty ticket
identity in that file blocks another run until the first submission is reconciled.

## Terraform ticket status

`arin_ticket_status` manages `status="CLOSED"` on an existing ticket. Its
`ticket_number` is the import identity and changing it requires replacement.
Creation reads the current ticket first and refuses unresolved tickets before
any write. A resolved ticket is closed and verified through a fresh summary.
An already closed ticket needs no PUT. The `resolution`, `ticket_type` and
`closed_date` fields remain server-owned.

The optional `update_method` selects `status` (the existing status-only endpoint)
or `payload` (the full-ticket PUT endpoint). The latter fetches fresh ticket
XML with `msgRefs=true`, changes only the direct status element's text, and
preserves every other byte, including dates, namespaces, sharing metadata and
message references. It refuses missing/duplicate/foreign status elements, nested
status content and embedded messages. It does not construct a PUT from the
summary-only model or append correspondence. Both methods verify closure through
a fresh summary. Import defaults to `status`; selecting `payload` afterwards on
an already closed ticket requires no write.

Client mocks verify exact preservation, scoped paths, closed-ticket no-op,
unresolved rejection, lost responses, mismatched identities and failed read-back.
Terraform mocks cover full-payload closure, import, clean plans, method changes
and an accepted closure whose response is lost, with exactly one PUT.

Closure recovery retains the existing ticket identity after an uncertain write.
Refresh can discover that the transition succeeded. If Terraform tainted the
failed creation, replacement observes the already closed ticket and does not
send another PUT. Read failures preserve state. A 404 for a previously observed
ticket sets `ticket_available=false` and preserves its last known status; this
resource never creates tickets. Import requires an available ticket. Destroy
forgets local management without reopening or deleting the server ticket.

Mock Terraform coverage includes closure, import, clean plans, switching to
another existing ticket, drift to RESOLVED, ticket expiry, invalid desired
statuses, and an accepted closure whose response is lost. Unit tests also verify
that open tickets are not mutated, read errors preserve state, and ignored writes
retain identity with an error.

The OT&E Terraform lifecycle uses the disposable report saved by the report
resource test, verifies configuration/import/refresh/destroy, and confirms zero
writes when it starts CLOSED. It also switches to `update_method="payload"`,
verifying the full detail read and a clean plan without any PUT. This proves the
native read/no-op path, not a successful full-ticket mutation. Its transport is
pinned to OT&E and rejects report
submission and unrelated mutations. A separate native probe against the client's
disposable report confirms that sending PUT to an already closed ticket returns
HTTP 400 `E_BAD_REQUEST` and leaves its metadata unchanged. The normal client
avoids that rejected request. A successful live RESOLVED-to-CLOSED transition is
still unverified because these disposable reports close automatically.

## Client coverage

`ReportRequest` supports two types:

| Type | Target | Endpoint |
| --- | --- | --- |
| `associations` | Empty | `/rest/report/associations` |
| `reassignment` | Uppercase NET handle | `/rest/report/reassignment/NETHANDLE` |

`RequestReport` validates its input, submits once, and checks the returned ticket
metadata and report type. It does not poll or resubmit. A decoded ticket number
is retained alongside errors in later fields so callers can save recovery state.
A lost response without a ticket number remains an uncertain submission, not
proof that no report was created.

`GetTicket` reads metadata from the summary endpoint without loading messages or
attachments. Its typed model is intentionally not a full-record PUT payload.
`CloseTicket` permits the documented RESOLVED-to-CLOSED transition, treats an
already closed ticket as a no-op, and verifies a fresh summary after writing.
It cannot withdraw open requests or reopen tickets. Mock tests cover premature
closure, mismatched response identities, ignored updates, lost responses and
verification failures. Full-ticket PUT is implemented by `CloseTicketWithPayload`. The append-only message
client and Terraform receipt resource are implemented as described below.

## Ticket-message submission is excluded

The append-only ticket-message resource and client submission method were removed
after an approved OT&E PUT returned HTTP 500 with an uncertain outcome. Existing
message and attachment data sources remain supported. This is no longer a release
blocker. Private intent receipts from the historical attempt remain intact for
manual reconciliation; removal does not prove whether ARIN processed that request.

## Preventing report replay

An application making only one `http.Client.Do` call is not sufficient to prevent
a report from being submitted twice. Go's HTTP transport may replay a normal GET
when a reused connection loses the response. Report requests therefore carry a
non-rewindable empty body internally, with no `GetBody` function. The wire request
still contains no body bytes. This prevents the standard transport from treating
the request as replayable after it has been sent.

The regression test warms a connection, accepts a report request, then closes the
connection without a response. Its control case proves an ordinary GET is sent
twice; `RequestReport` sends it only once and returns an uncertain error. A separate
HTTP/2 test verifies the report request still has an empty body. Redirects remain
disabled, and there is no operation-level retry loop. Custom transports supplied
by callers must also honor these non-replayable request semantics.

## Sandbox results and durable receipts

With the current OT&E account:

- Associations reports were accepted, completed and automatically closed.
- Reassignment reports for an owned direct allocation were accepted, completed
  and automatically closed.
- Historical WhoWas probes returned access-denied responses. WhoWas request
  support has since been removed from scope, including its live probes.
- A successful RESOLVED-to-CLOSED write has not yet been observed live because the
  report tickets closed automatically. Existing unrelated tickets are not closed
  merely to exercise this operation.

Reg-RWS has no report-ticket deletion endpoint. The live client test creates each
request at most once and retains a private mode-0600 receipt in the user cache:
`terraform-provider-arin/ote-report-<request-hash>.json`. The hash includes the
organization and complete request. The file is created and synced before the
request, then replaced atomically with the returned ticket or a definite rejection.
It contains account metadata, not API keys, and must stay outside Git.

Subsequent runs read the saved ticket and never submit a replacement. Ticket
status saved in the receipt is only the original response; fresh reads determine
current status. An active report is polled for at most 20 seconds per run. If it
is still active, the receipt remains available for a later run. Automatic ticket
expiry produces a read error rather than silently requesting another report.

An uncertain receipt without a ticket number blocks resubmission. Inspect the
account's ticket list and reconcile the result before editing or removing that
receipt. Do not remove receipts for accepted or uncertain requests to force a retry.
Historical WhoWas rejection receipts may remain in the private cache; current tests
do not read or replay them.

ARIN documents report-ticket deletion 90 days after closure unless retention is
selected in ARIN Online. The API does not expose that retention setting.

## Read endpoint and payload audit

The collected methods guide defines six ticket read operations. All are mapped
to data sources and exercised together by `TestOTEReportReadLifecycle` using the
existing saved associations-report receipt:

| Operation | Data source | Native assertions |
| --- | --- | --- |
| Ticket details, `msgRefs=true` | `arin_ticket` | Saved ticket identity and populated message references |
| Ticket details, `msgRefs=false` | `arin_ticket` | Same message count; embedded report attachment matches direct download |
| Ticket summary | `arin_ticket_summary` | Same saved ticket identity |
| Ticket payload list | `arin_tickets` | Saved ticket included under ASSOCIATIONS_REPORT/CLOSED filters |
| Ticket summary list | `arin_ticket_summaries` | Same filtered ticket identity |
| Individual message | `arin_ticket_message` data source | Message ID matches its reference; attachment references are populated |
| Attachment bytes | `arin_ticket_attachment` | Nonempty decoded bytes, correct size and SHA-256, reference filename and application/octet-stream headers |

The test also reads the message through the typed client and checks a subsequent
clean Terraform plan. Its transport accepts only GETs on sandbox ticket paths;
it cannot generate reports or submit messages. Requests are spaced at least 1.1
seconds apart because concurrent Terraform refreshes hit OT&E's HTTP 429 limit
in the initial unpaced audit. Failed requests are not replayed by the test.
Account content stays in temporary test state and is not logged or committed.

`TicketPayload.rnc` fields are represented by ticket number, four dates, type,
status, resolution, sharing metadata, `flagged`, and message/reference collections.
`flagged` is an optional string in ARIN's schema, so the data sources preserve it
as a string rather than guessing a Boolean type. It remains null when absent.
The mock fixture and decoder tests cover populated, absent and non-Boolean values.
Full-ticket PUT preserves this field along with all other unchanged payload bytes.

`MessagePayload.rnc` fields map to generated identity/date, subject, ordered text,
category, embedded filename/base64 attachments and attachment references. Reference
IDs select fixed authenticated endpoints; response URLs are never followed.
These checks establish the associations report's attachment path. WhoWas submission is deliberately excluded.

## Remaining verification

Ticket detail, message and attachment reads have native evidence through
`TestOTEReportReadLifecycle`. Both ticket-closing methods still need a disposable
RESOLVED ticket to verify a real transition. Mock closure and already-closed native
reads do not prove RESOLVED-to-CLOSED interoperability.
