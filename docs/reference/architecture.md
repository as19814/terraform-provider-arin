# Provider foundation

## Boundaries

The provider layer owns Terraform schemas, diagnostics, and state. The ARIN client owns authentication, HTTP behavior, and XML/JSON decoding. The client does not import Terraform packages, so it can be exercised independently.

A provider instance creates one immutable client shared by its resources and data sources. Provider aliases can use different credentials or origins. Organization handles belong to individual data sources rather than being a provider-global setting; future organization-scoped resources should make that association explicit.

## Current API behavior

Authenticated reads are defined in `internal/arin/read_catalog.go` with explicit endpoints, validated inputs, expected XML roots, collection members, and typed field mappings. A shared Terraform adapter exposes those fields as concrete schemas rather than raw XML. Public ASN and contact reads have a separate RDAP implementation. Network discovery keeps its map keyed by handle.

The read models are not replacement payloads for writes. Do not serialize them for updates: omitted registration fields can carry meaning in Reg-RWS. Report-request endpoints are absent from the catalog because they create tickets.

The XML parser validates document shape and namespaces, limits nesting, rejects unexpected collection members, preserves absent scalars as null, and orders multiline text by numeric line indices. It normalizes ARIN's zero-padded IPv4 addresses and equivalent CIDR representations. Unordered collections are sorted deterministically. Identity checks prevent assigning mismatched records to requested handles, prefixes, or ASNs.

Attachment reads return base64 plus response metadata and a checksum, without writing files. Ticket/customer content and tax IDs are marked sensitive in the schema; Terraform still persists those values in state.

Network discovery uses `GET /registry/ips/reverse_search/entity?handle={org}` on public RDAP and filters results by direct registrant role. Network handles key the Terraform map, so response order does not affect identity. No authorization header is sent to RDAP. Explicit truncation or pagination is an error; a 404 search is checked against the entity endpoint before an empty inventory is returned. A missing entity remains an error. CIDRs come from the optional cidr0 extension and are not inferred from a potentially non-CIDR address range.

The default origin is production. OT&E is selected explicitly with `base_url` or `ARIN_BASE_URL`. RDAP defaults follow the selected production/OT&E registration origin. Custom registration origins require `rdap_base_url` or `ARIN_RDAP_BASE_URL` for network discovery to prevent accidental production queries during testing. Custom HTTPS origins and HTTP loopback test servers are supported. API keys are optional at configuration time and required when a Reg-RWS operation is invoked. The client does not follow redirects, disable certificate verification, log request/response bodies, or put credentials in query parameters.

The HTTP timeout defaults to 30 seconds. The provider allows 1 to 300 seconds. Context cancellation reaches the HTTP request. Responses are limited to 4 MiB; operations requiring larger payloads will need a deliberate change. There are no automatic retries yet, including on 429 responses, to avoid prematurely defining behavior for future writes.

Non-success HTTP responses become `APIError` values with status code and sanitized XML or JSON error fields. Unstructured errors report status only. Missing organizations are data source errors. Managed AS sets remove missing objects from state on a 404, while other API errors preserve state.

## Verification

Unit tests cover origin and handle validation, request headers, XML decoding, credential redaction, response size, redirects, cancellation, timeout, status classification, configuration precedence, and unknown values.

Fake-server acceptance tests exercise Terraform protocol negotiation, provider configuration, every catalog entry, network discovery, state refresh, empty inventories, truncation, sensitive nested fields, binary attachments, and missing-record diagnostics. The opt-in live acceptance test performs registration reads with the user's shell credentials and public RDAP discovery for an explicit organization handle. It chains discovery into existing network, delegation, contact, ASN, IRR, and RPKI records. Families without existing test-account records remain mock-tested. The live suite stays separate from normal CI.

## Deferred decisions

- Identity and import formats for additional resource families.
- RPKI transaction batching and concurrent updates.
- Ticket polling and eventual consistency.
- Resource-specific retries and rate-limit backoff.
- Release signing, packaging, registry publication, and license selection.

No release workflow is enabled. Documentation generation uses a pinned Go tool dependency. GitHub Actions are pinned to commits and dependency updates are configured through Dependabot.

## Managed simple IRR AS sets

`arin_irr_as_set` uses a dedicated complete write model in `internal/arin/as_set.go`,
not the generic data-source model. XML uses the published `AsSetPayload.rnc`
spelling `membersByRef`, despite the prose guide's `mbrsByRef` label.
It includes empty membership containers to clear removed values and XML-escapes
text and attributes. Empty remarks are omitted: OT&E returns HTTP 500 for an
empty remarks container, while omission clears them. Timestamps and POC links
are server-owned and omitted from writes. OT&E rejects POC changes with
E_ENTITY_VALIDATION; the resource exposes POC links as computed values only.
Unsupported top-level fields fail reads rather than allowing a lossy update.

The AS set name is the state ID and import ID. Names and organization handles
require replacement. Creates check for an existing object first and require
import instead of adoption. POST and PUT responses must contain the completed
matching AS set; asynchronous responses are not treated as completion. Deletes
accept completed 200/204 responses or 404. Only read 404 removes a resource from
state. No automatic write retries or redirect following are enabled.

Stateful fake-server acceptance tests cover create, update, clearing collections,
import verification, drift correction, external deletion, replacement, and
final deletion. Client tests cover payload serialization, error redaction,
conflicts, redirects, asynchronous responses, malformed results, and no replay.
The opt-in OT&E lifecycle test has passed create, update, remarks clearing,
import into separate state, a clean plan, deletion, and absence verification.
All disposable test sets were cleaned up. Production write behavior is untested.
API diagnostics include redacted component messages and additional information
to explain field-level validation failures.

## Managed simple IRR routes

`arin_irr_route` uses a dedicated XML write model for both address families.
The stable ID is `canonical-prefix,AS<number>`, also used for import. Prefix,
origin, and organization are replacement attributes. Description and remarks are
writable; POC links, NET handle, and dates are computed. Empty remarks are omitted,
matching the behavior verified during AS-set validation.

GET, POST, PUT, and DELETE use `/rest/irr/route/IP/LENGTH/AS<number>`. Creation
requires confirmed absence. Updates and deletes first reread the record and
reject a ROA link. Managed reads also reject ROA links and `memberOf` associations,
which this resource cannot round-trip. Unknown XML fields are rejected. No
credentials or mutation payloads are logged, and there are no automatic retries.

OT&E verified both IPv4 /32 and IPv6 /128 route lifecycles, using randomly chosen
addresses inside the test organization's registrations. The parent registration
lookup checks ownership; `mostSpecificNet` is unsuitable for this preflight
because it requires an existing registration with the exact start/end range.
Production writes remain untested. Tests leave no disposable routes behind.

## Simple IRR route sets and aut-num

Route sets expose IPv4 members, multiprotocol members and MNT references as
unordered Terraform sets. XML member attributes preserve RPSL prefix ranges;
OT&E accepts `^+` despite the UI guide's contrary limitation. Collection clearing
uses empty member containers and omitted empty remarks. Server-generated POC
links and timestamps are computed.

Aut-num writes include its AS name, description, remarks, six ordered policy
collections, and repeated `<memberOf name="..."/>` entries. Empty optional policy
elements are omitted. The canonical ASN is the resource/import ID, separate from
its registration handle. The data source also exposes membership. OT&E tests
only create an IRR record after confirming it is absent for an ASN registered to
the test org. Cleanup deletes the new IRR object and its helper AS set, never the
ASN registration. All non-report read and managed coverage is tracked in
`implementation-status.md`; this is not a claim of full API coverage.
