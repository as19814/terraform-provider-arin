# Network registration lifecycle

The authenticated client now supports NET creation through simple reassignment,
detailed reassignment, and reallocation. It also supports metadata updates and
deletion of reassigned/reallocated records. The `arin_net` Terraform resource exposes these downstream registration lifecycles
and retains recovery state for pending or uncertain operations.

## API contracts

The official [method reference](https://www.arin.net/resources/registry/regrws/methods/)
and [payload reference](https://www.arin.net/resources/registry/regrws/payloads/)
are stored locally under `arin-api/reg-rws/`.

- `PUT /rest/net/{parent}/reassign`: customer handle for simple reassignment,
  organization handle for detailed reassignment.
- `PUT /rest/net/{parent}/reallocate`: organization handle required.
- `GET /rest/net/{handle}`: authenticated registration record.
- `PUT /rest/net/{handle}`: metadata update. Fetch and preserve current identity,
  registration date, recipient, blocks, version and parent.
- `DELETE /rest/net/{handle}`: deletion of reassigned/reallocated records.
  Direct allocations cannot be deleted with this operation.

Creation and deletion return `ticketedRequest`. A completed response can contain
a NET without a ticket. A ticket-only response is an accepted request, not proof
of completion. The client exposes the ticket number, status and resolution
separately from the network. It does not retry mutation requests. If a NET payload
cannot be decoded, any separately returned ticket is retained for reconciliation.

## Verified in OT&E

`TestOTENetAssignmentClientLifecycle` tests both IPv4 and IPv6:

1. Discover an owned allocation and choose a random unassigned /32 or /64.
2. Verify the covering parent before writing.
3. Create a disposable customer and simple reassignment.
4. Read the network, update its name, and clear comments.
5. Delete the network and verify authenticated GET returns 404.
6. Reuse the freed range for detailed reassignment to the test organization.
7. Delete and verify absence, then create a reallocation to the test organization.
8. Delete and verify absence, then delete the disposable customer.

All six creation paths passed. These requests completed synchronously, returning
embedded NET records without ticket numbers. The Terraform lifecycle, import and clean-plan checks also passed for all six
paths. Pending requests and persisted state are covered by mock acceptance tests; a genuinely asynchronous sandbox lifecycle remains to be exercised.
Organization-recipient tests use the authenticated test organization as recipient,
not an unrelated organization with different acceptance or authorization settings.

The client preserves immutable fields on update and refuses unsupported response
fields rather than silently dropping them. It rejects direct-allocation deletion
before issuing a DELETE.

Run the opt-in test using `make testote` with `ARIN_OTE_API_KEY` and
`ARIN_TEST_ORG_HANDLE` available in the process environment. Both API origins are
hardcoded to OT&E in the live test. Credentials, raw responses and Terraform state
are not fixtures.

## Pending operations and recovery

The resource records `pending_operation` and `pending_ticket` when a write cannot
be confirmed. Before a NET handle is known, its `id` is a `pending:` recovery
identifier. This is not a NET handle and must not be used in dependent API calls.

A subsequent refresh performs exact range lookups and verifies parent, recipient,
creation mode, blocks and name before recovering a created NET. A known failed
creation ticket can release recovery state only when no matching NET exists.
An unknown outcome without a ticket retains state and requires investigation;
removing it blindly could permit a duplicate request.

Terraform marks failed creations as tainted. After the ticket completes, refresh
and verify the registered NET before untainting it if you want to retain it:

```sh
terraform apply -refresh-only
terraform untaint arin_net.example
terraform plan
```

Without untainting, Terraform will normally destroy and replace the recovered
network. Importing the confirmed NET into the correct resource address is another
recovery option after carefully reconciling existing state.

Pending deletion never resubmits DELETE. The NET must be absent, and a known
deletion ticket must reach RESOLVED or CLOSED, before state is released. If the
ticket ends unsuccessfully while the NET remains, an apply clears the failed
ticket marker and reports the failure. Correct the cause before applying again.

A transport failure or malformed response is treated as uncertain, not a failed
write that can be repeated automatically. Credentials and response bodies are not
stored in recovery state. A process killed before it receives the write response
may require manual import or ticket investigation, as Terraform cannot checkpoint
a response it never received.

## Retired NET Origin AS

ARIN [retired the NET Origin AS field on 29 July 2025](https://www.arin.net/announcements/20250729/).
The older payload guide still lists `originASes`, but OT&E tests on 22 September
2026 confirmed that both `AS64496` and `64496` are silently discarded during
creation and metadata update. Fresh GETs return an empty collection.

The new resource therefore has no writable Origin AS setting. The client rejects
nonempty origin inputs instead of claiming they were saved. Legacy read fields
remain readable for older fixtures or responses. Use IRR routes or RPKI to publish
routing information.

## Remaining work

- Exercise a genuinely asynchronous OT&E ticket, including failure/rejection.
- Audit any additional range edge cases beyond the verified minimal CIDR covers.
- Verify message/attachment submission in OT&E; the remove endpoint passes IPv4/IPv6 sandbox tests without correspondence, and message payloads have mock lifecycle coverage.


## Multi-block registration

`TestOTENetMultiBlockLifecycle` passed both IPv4 and IPv6 through simple
reassignment, detailed reassignment and reallocation. Each request uses two
adjacent, unequal-size canonical blocks representing one contiguous range.
ARIN preserves these minimal covers. Terraform update, import, clean-plan and
deletion checks all pass. Exact recovery lookups use the full start/end range,
not separate lookups for individual blocks.

## Existing-NET metadata and POC links

`arin_net_metadata` manages selected metadata on an existing NET. Its destroy
operation removes Terraform management without making an API request, so it
cannot delete a direct allocation or erase its metadata.

Unconfigured name, comments and POC links are preserved from a fresh API read.
Explicit empty comments or POC collections clear those values. When composing it
with `arin_net`, configure only POC links on the metadata resource and let
`arin_net` own name/comments. Mock acceptance tests verify simultaneous changes
to these separate fields without overwriting each other.

NETs support Tech (`T`), NOC (`N`) and Abuse (`AB`) associations. OT&E rejects
Routing (`R`) and DNS (`D`) with E_ENTITY_VALIDATION and the message
“Networks may only have Tech, NOC, and Abuse POCs.” The generic payload table
includes roles for other object types; it is not a list of writable NET roles.
Admin (`AD`) is also invalid. Descriptions are generated by ARIN and exposed as
computed `poc_links.description` values. Description-only drift refreshes state
without a write; a subsequent configured update can refresh those labels again.
Fake acceptance tests cover both cases, and IPv4/IPv6 OT&E lifecycles verify
the returned descriptions.

`TestOTENetMetadataLifecycle` verifies all three supported POC roles on disposable
IPv4 and IPv6 reallocations, including adding, preserving, clearing, importing
and a clean Terraform plan. Metadata destroy leaves the NET and final metadata
intact; test cleanup then deletes the disposable registration. The direct
allocation subtest submits the current metadata unchanged to an owned parent,
then verifies the complete before/after record is identical.

## Managed customer dependency

The combined customer/NET Terraform graph passes IPv4 and IPv6 OT&E creation,
updates, import, recipient replacement and cleanup. See the
[graph lifecycle evidence](customer-network.md) for ordering and reused-handle
behavior, and the [example](../../examples/customer-network/main.tf).

## Removal messages and attachments

`arin_net.removal_messages` is an optional local destroy policy. A nonempty list
selects `PUT /rest/net/HANDLE/remove`; omission or an empty list selects ordinary
DELETE. Each message accepts a subject, ordered text lines, category (`NONE` by
default or `JUSTIFICATION`) and a map of filenames to base64 attachment contents.
The client places these in `net/messages/message`, preserving the freshly read NET
record. This nesting follows the collected `NetPayload.rnc` and `MessagePayload.rnc`
schemas and the official methods/payload guides rechecked on 2026-09-23.

Apply policy changes before removing the NET from configuration. Creation,
refresh and metadata updates never send the configured messages. A change only
to this policy makes no metadata PUT. Imports have no removal messages, since
these are local instructions rather than registered NET fields. The collection
is sensitive, but its text and attachment contents remain in Terraform state.

The client validates XML text, category, filenames and base64 contents before
submission. The encoded NET request has a 4 MiB client limit; this is not a claim
about ARIN's server-side attachment limit. Invalid correspondence never becomes
a removal request. Direct allocations remain ineligible for either deletion path.
A 404 from the remove endpoint is accepted only after a separate NET lookup
confirms absence.

Mock client tests verify payload preservation, multiple messages, binary
attachments, pending tickets, malformed responses that retain a ticket, and
lost connections without replay. Terraform acceptance verifies policy changes,
import behavior, clean plans and destroy through the remove endpoint. Resource
recovery tests prove a second destroy cannot resubmit an uncertain removal, then
reconcile confirmed absence. Existing pending-ticket completion rules also apply.
The existing ordinary NET Terraform lifecycle was rerun in OT&E on 2026-09-23
for IPv4 and IPv6, including import, replacement and verified cleanup; it passed.
`TestOTENetRemoveLifecycle` subsequently passed `PUT /rest/net/HANDLE/remove`
without messages for disposable IPv4 and IPv6 simple reassignments. Both returned
completed NET records without tickets, and fresh GETs confirmed their absence.
The disposable customers were also deleted and their absence verified.

The removal test shares the customer/NET graph guard and writes an exclusive
mode-0600 `ote-net-remove-<org-hash>-<family>.json` receipt in the user cache.
Each mutation is recorded before dispatch. Unknown outcomes or returned tickets
retain the receipt and block further writes, including cleanup. The remove probe
rejects message and message-reference elements before dispatch. Cleanup removes
the receipt only after all tracked NETs and customers are confirmed absent.
`make testote` includes this no-correspondence lifecycle.

Live message/attachment submission remains unverified. No correspondence was
sent to ARIN during this work.
