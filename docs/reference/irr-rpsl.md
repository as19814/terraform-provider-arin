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

## Remaining work

A Terraform resource for advanced RPSL is still pending. It must handle ARIN's
formatting and server-generated attributes without perpetual diffs, preserve
policy semantics during refresh, support import and protect natural identity
after an uncertain write. Native route/route6 and aut-num lifecycles also remain
unverified. Passing set tests does not establish these other lifecycles.

References: [ARIN object permissions](https://www.arin.net/resources/manage/irr/#object-permissions),
[ARIN IRR REST API](https://www.arin.net/resources/manage/irr/irr-restful/).
