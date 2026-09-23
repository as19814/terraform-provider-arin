# Organization POC association evidence

`arin_org_poc` manages one association between an existing organization and POC.
It supports T (Tech), N (NOC), AB (Abuse), R (Routing) and D (DNS). Admin changes
require the full organization update endpoint and are excluded from this resource.
The underlying organization and contact are preserved when an association is destroyed.

## Identity and ownership

Import uses `ORG-HANDLE/POC-HANDLE/FUNCTION`. Existing associations must be
imported before management. Every configured change requires replacement.
Do not overlap association ownership with a full organization POC collection.
ARIN enforces required contacts and can reject removal of the last Tech POC.

The client reads the organization and validates its complete POC link collection.
This partial model is never serialized as a full organization update. Add and
remove use PUT and DELETE respectively on
`/rest/org/ORG/poc/POC;pocFunction=ROLE`, then read the organization to verify the
requested result. Mutation failures are not retried automatically. Terraform
retains identity on uncertain creation and state on unconfirmed deletion.

## Sandbox findings

Client and Terraform lifecycles pass for all five supported roles, including
import, collection reduction, clean plans and deletion. Organization Routing
and DNS roles work even though OT&E rejects those roles on NET records.

The method guide documents deletion by POC handle alone or function alone.
OT&E rejected the following probes:

| DELETE route | Observed result |
| --- | --- |
| `/rest/org/ORG/poc/POC` | HTTP 400, E_BAD_REQUEST |
| `/rest/org/ORG/poc/POC;pocFunction=` | HTTP 400, E_BAD_REQUEST |
| `/rest/org/ORG/poc/;pocFunction=ROLE` | HTTP 404, E_UNSPECIFIED |
| `/rest/org/ORG/poc;pocFunction=ROLE` | HTTP 404, E_UNSPECIFIED |

Exact handle/function deletion works. The client exposes that operation only;
it does not silently emulate the documented bulk deletion routes. Tests probe
function-only deletion only when the role is absent from the original links.

Concurrent per-association writes can overwrite each other's changes in OT&E.
The client serializes mutations and verification for each API origin and org,
including different client instances in the same process. A concurrent Terraform
lifecycle with five roles passes with this lock. Separate provider processes and
external writers are not coordinated; avoid simultaneous applies to one org.

## Testing and recovery

Mock tests cover strict decoding, endpoint validation, sibling preservation,
concurrent client writes, import, drift recovery, uncertain creation, read errors
and incomplete deletion. Lock tests cover cancellation and independent orgs/origins.

OT&E tests create a disposable POC and save the original organization links to
an exclusive mode-0600 file in the user cache directory under
`terraform-provider-arin/ote-org-pocs-<org-hash>.json` before changing associations.
Cleanup removes only links to that disposable POC, verifies equality with the
original collection, deletes the POC and removes the snapshot. A failed cleanup
retains the snapshot for recovery. A later test refuses to overwrite it.
The snapshot contains account metadata and must not be committed.

If cleanup fails, inspect the snapshot locally, remove only the recorded
POC's disposable associations by exact handle/function, and compare current
links with the saved original collection. Delete the disposable POC only after
that comparison succeeds. Remove the snapshot only after verified recovery.
Do not replace the entire organization with the snapshot's partial data.

`make testote` runs client and provider packages sequentially because both use
the same organization recovery file. Normal CI runs fake-server tests only.

## Remaining work

Full organization creation, update, deletion, Admin POC replacement and ticket
semantics remain in the [coverage inventory](implementation-status.md).

References: collected [organization methods](arin-api/reg-rws/methods.md#orgs)
and [payloads](arin-api/reg-rws/payloads.md).
