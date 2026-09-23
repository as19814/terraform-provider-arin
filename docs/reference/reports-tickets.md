# Report and ticket implementation evidence

Report requests create server-side jobs and return ticket identities. They must
not be exposed as data sources that submit work on refresh, even though ARIN uses
GET for these operations.

## Client coverage

`ReportRequest` supports four types:

| Type | Target | Endpoint |
| --- | --- | --- |
| `associations` | Empty | `/rest/report/associations` |
| `reassignment` | Uppercase NET handle | `/rest/report/reassignment/NETHANDLE` |
| `who_was_asn` | Canonical decimal ASN | `/rest/report/whoWas/asn/ASNUMBER` |
| `who_was_net` | Canonical IPv4/IPv6 address, not CIDR | `/rest/report/whoWas/net/IPADDRESS` |

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
verification failures. Full-ticket PUT and message submission remain separate
implementation work.

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
- WhoWas ASN and NET requests returned HTTP 401, `E_AUTHENTICATION`, explicitly
  stating that the account lacks WhoWas access. These are account-access limits,
  not evidence that the endpoints are unavailable. Client paths and payloads have
  mock coverage; native successful generation remains unverified.
- An explicit RESOLVED-to-CLOSED write has not yet been observed live because the
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
receipt. Definite WhoWas authorization rejections are retained and reported as
skipped live coverage. After account access is granted, only those definite
rejection receipts may be removed to allow a new authorized attempt. Do not remove
receipts for accepted or uncertain requests to force a retry.

ARIN documents report-ticket deletion 90 days after closure unless retention is
selected in ARIN Online. The API does not expose that retention setting.

## Remaining work

The Terraform report-request resource must preserve request and ticket identity,
import existing receipts, avoid resubmitting on refresh, and retain uncertain
submission state. Report attachment access already exists through ticket data
sources. Message submission, full-ticket modification and native explicit closure
still need lifecycle decisions, implementation and sandbox evidence. See the
[coverage inventory](implementation-status.md).

References: [ARIN Reg-RWS methods](https://www.arin.net/resources/manage/regrws/methods/)
and [payloads](https://www.arin.net/resources/manage/regrws/payloads/), with local
copies in [the API documentation index](arin-api/README.md).
