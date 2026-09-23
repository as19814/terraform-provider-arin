# Network registration lifecycle

The authenticated client now supports NET creation through simple reassignment,
detailed reassignment, and reallocation. It also supports metadata updates and
deletion of reassigned/reallocated records. The Terraform resource is still to be
implemented, including persistence and reconciliation of asynchronous tickets.

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
4. Read the network, update its name, and clear comments and origin ASNs.
5. Delete the network and verify authenticated GET returns 404.
6. Reuse the freed range for detailed reassignment to the test organization.
7. Delete and verify absence, then create a reallocation to the test organization.
8. Delete and verify absence, then delete the disposable customer.

All six creation paths passed. These requests completed synchronously, returning
embedded NET records without ticket numbers. Pending requests are covered by mock
tests; a genuinely asynchronous sandbox lifecycle remains to be exercised.
Organization-recipient tests use the authenticated test organization as recipient,
not an unrelated organization with different acceptance or authorization settings.

The client preserves immutable fields on update and refuses unsupported response
fields rather than silently dropping them. It rejects direct-allocation deletion
before issuing a DELETE.

Run the opt-in test using `make testote` with `ARIN_OTE_API_KEY` and
`ARIN_TEST_ORG_HANDLE` available in the process environment. Both API origins are
hardcoded to OT&E in the live test. Credentials, raw responses and Terraform state
are not fixtures.

## Remaining Terraform behavior

- Expose creation modes, recipient, prefixes and mutable metadata with import.
- Persist ticket identifiers before returning an unresolved operation.
- Reconcile ticket status and resulting registration without repeating creation.
- Preserve state until deletion is confirmed, including pending deletion tickets.
- Test interrupted requests, pending/denied tickets and refresh/import recovery.
- Cover network POC editing and direct-allocation metadata separately from the
  lifecycle of child registrations.
- Evaluate the remove-NET workflow with message/attachment payloads.
