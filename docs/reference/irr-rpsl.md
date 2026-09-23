# Advanced IRR RPSL

ARIN separates simple objects (created with XML or the online form) from advanced
objects (created with RPSL). The official permissions table says each API format
can manage only objects created in that format. The sandbox confirmed this for
our advanced AS sets and route sets: XML reads returned HTTP 406 `E_BAD_REQUEST`
requiring `Accept: application/rpsl`.

## Implemented

`arin_irr_rpsl` reads advanced `route`, `route6`, `as-set`, `route-set` and `aut-num`
objects. Inputs are `object_type`, `name`, and `origin_as` for routes. Outputs include
the maintaining organization and the complete RPSL object. Text line endings are
normalized to LF, outer whitespace is trimmed and a trailing newline is added.
Attribute order, continuation lines and attributes unknown to the provider are
preserved. The data source sends only GET requests.

The shared client implements GET, POST, PUT and DELETE using RPSL for both Accept
and Content-Type headers. Set creation uses the collection endpoint with an
organization query parameter. Both route families use the route endpoint with
CIDR and origin ASN. All other authenticated XML requests retain their existing
Content-Type behavior, including bodyless ASPA reads.

The parser validates a single object, canonical identity, one ARIN source and
one MNT-organization maintainer. It rejects ambiguous or multiple identities,
wrong address families, invalid continuations and control characters. It is not
a complete RPSL policy grammar implementation; ARIN validates attribute semantics.
Unknown attributes are preserved instead of being converted into a partial XML
representation. Create requires absence; update and delete check ownership through
a fresh read. Writes are not automatically retried, redirects remain disabled,
and mismatched response identities are errors. Read and response size limits and
credential redaction are shared with the existing client.

## Evidence

- Client mock lifecycles cover all five supported object types, exact endpoint and
  header selection, existing-object protection, ownership guards and idempotent
  deletion of absent objects.
- Parser tests cover preservation of unknown attributes and continuation lines,
  invalid/multiple identities, duplicate source/maintainer fields and family mismatches.
- Terraform fake-server tests read and refresh all five types and reject a response
  for a different identity.
- Native client tests create, read, update and delete randomly named advanced AS
  sets and route sets using the organization's registered Admin/Tech contacts.
  A fresh GET confirms each update and a 404 confirms deletion.
- Native Terraform tests read both types from disposable fixtures, compare the
  complete returned text with an independent client read, and verify a clean plan.
  Cleanup removes the fixtures and confirms absence. No production writes occur.

Native Terraform resource lifecycles also passed for all five object types. Tests
create, read through the data source, update, import, verify clean plans and delete
each object. Route tests select unused host prefixes within owned registrations;
the aut-num test selects an owned ASN with no existing IRR object. Both API formats
are checked for absence before using those identities. Cleanup verifies absence.
A second run also verified repeated set members and a folded aut-num policy line.
The ASN and network registrations are not modified.

## Managed resource

`arin_irr_rpsl` also manages advanced objects. Configure the object type, natural
name, organization, route origin when applicable, and the complete RPSL payload.
The payload identity and maintainer must agree with the explicit attributes.
Identity or organization changes require replacement. Existing objects must be
imported. Simple XML objects are not converted implicitly.

Refresh retains configured text when parsed attributes match. Comparison ignores
alignment after colons, ordering between different attribute names, continuation
folding, `created` and `last-modified`. Repeated attribute order, internal spacing
and every other value remain significant. Unknown attributes are included in the
comparison. This conservative comparison does not claim full RPSL semantic
normalization. `remote_rpsl` always records the full latest server text.

Import uses `type/name`, with `,AS<number>` appended for routes. Import initially
populates `rpsl` from ARIN's representation. If configuration uses different but
equivalent formatting, the next apply stores that formatting locally without a
PUT; subsequent plans are clean. Import verification deliberately excludes the
configured text while comparing identity and the complete remote representation.

Create verifies the result through a fresh GET. If a lost response is followed by
a matching read, the resource recovers immediately. If the result remains uncertain,
it persists its natural identity with `pending_creation=true`. Read and destroy
block automatic replacement while pending, including `create_before_destroy`.
Back up state, inspect the remote object, then use state removal and import to
adopt the confirmed object. If absence is established, remove the receipt only
after ruling out an outstanding write. Do not blindly retry creation.

Update also verifies the requested policy after a write and preserves state on
failure. Delete checks the maintainer and verifies absence afterward. A confirmed
404 removes a normal resource from state; read errors or changed ownership do not.
Destroy removes only the IRR object, never the network or ASN registration.

Mock Terraform lifecycles cover all five object types, import, formatting stability,
policy drift and deletion. Recovery tests cover immediate recovery and failed
read-back followed by blocked replacement and manual import without another POST.
State tests cover read errors, ownership changes and pending deletion protection.

References: [ARIN object permissions](https://www.arin.net/resources/manage/irr/#object-permissions),
[ARIN IRR REST API](https://www.arin.net/resources/manage/irr/irr-restful/).
