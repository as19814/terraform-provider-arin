# Provider foundation

## Boundaries

The provider layer owns Terraform schemas, diagnostics, and state. The ARIN client owns authentication, HTTP behavior, and XML/JSON decoding. The client does not import Terraform packages, so it can be exercised independently.

A provider instance creates one immutable client shared by its resources and data sources. Provider aliases can use different credentials or origins. Organization handles belong to individual data sources rather than being a provider-global setting; future organization-scoped resources should make that association explicit.

## Current API behavior

The initial authenticated operation is `GET /rest/org/{handle}`. Its response model contains only fields exposed by the organization data source. Do not serialize this partial model for updates: omitted organization fields can carry meaning in Reg-RWS.

Network discovery uses `GET /registry/ips/reverse_search/entity?handle={org}` on public RDAP and filters results by direct registrant role. Network handles key the Terraform map, so response order does not affect identity. No authorization header is sent to RDAP. Explicit truncation or pagination is an error; a 404 search is checked against the entity endpoint before an empty inventory is returned. A missing entity remains an error. CIDRs come from the optional cidr0 extension and are not inferred from a potentially non-CIDR address range.

The default origin is production. OT&E is selected explicitly with `base_url` or `ARIN_BASE_URL`. RDAP defaults follow the selected production/OT&E registration origin. Custom registration origins require `rdap_base_url` or `ARIN_RDAP_BASE_URL` for network discovery to prevent accidental production queries during testing. Custom HTTPS origins and HTTP loopback test servers are supported. API keys are optional at configuration time and required when a Reg-RWS operation is invoked. The client does not follow redirects, disable certificate verification, log request/response bodies, or put credentials in query parameters.

The HTTP timeout defaults to 30 seconds. The provider allows 1 to 300 seconds. Context cancellation reaches the HTTP request. Responses are limited to 4 MiB; operations requiring larger payloads will need a deliberate change. There are no automatic retries yet, including on 429 responses, to avoid prematurely defining behavior for future writes.

Non-success HTTP responses become `APIError` values with status code and sanitized XML or JSON error fields. Unstructured errors report status only. Missing organizations are data source errors. A future managed resource must decide separately whether a missing object should remove it from Terraform state.

## Verification

Unit tests cover origin and handle validation, request headers, XML decoding, credential redaction, response size, redirects, cancellation, timeout, status classification, configuration precedence, and unknown values.

Fake-server acceptance tests exercise Terraform protocol negotiation, provider configuration, organization reads, network discovery, state refresh, empty inventories, truncation, and missing-record diagnostics. The opt-in live acceptance test performs registration reads with the user's shell credentials and public RDAP discovery for an explicit organization handle. It must stay separate from normal CI.

## Deferred decisions

- Managed-resource identity and import formats.
- RPKI transaction batching and concurrent updates.
- Ticket polling and eventual consistency.
- Resource-specific retries and rate-limit backoff.
- Release signing, packaging, registry publication, and license selection.

No release workflow is enabled. Documentation generation uses a pinned Go tool dependency. GitHub Actions are pinned to commits and dependency updates are configured through Dependabot.
