# Public RDAP coverage

The audit follows ARIN's current [RDAP guide](https://www.arin.net/resources/registry/whois/rdap/)
and the [local reference](arin-api/lookup/rdap.md). Public lookups use the configured
RDAP origin and never send the API key. Redirects and response-provided links are
not followed. Production access in the tests below is read-only.

## Implemented network lookup

`arin_rdap_network` accepts a canonical IPv4 or IPv6 address or network prefix in
`query`. Prefixes must have zero host bits. It requests `/registry/ip/QUERY` and
returns the registration handle, name, type, address range, IP version, optional
parent/country, CIDRs, direct registrant handles, status and events.

The result may be a containing registration rather than an exact prefix match.
The client verifies that both ends of the requested range are contained in the
returned registration. An address-family mismatch, invalid/reversed range,
truncation notice or pagination link is an error. When CIDR blocks are present,
they must cover the returned registration completely, without gaps, overlaps or
duplicates. Missing optional country/parent fields remain null. An omitted CIDR
extension produces an empty list; the provider does not invent missing blocks.
This is registration information, not proof of write authority or route reachability.

Client tests cover IPv4/IPv6, multi-block CIDRs, invalid queries, mismatched and
incomplete records, optional fields and referrals. Terraform fake-server tests
cover address/prefix lookup, typed fields, refresh, clean plans and missing records.
Native `TestLiveRDAPNetwork` discovers owned registrations and tests both address
and prefix queries for IPv4/IPv6 against OT&E and production. It compares Terraform
results with separate client reads and verifies clean plans. Both origins passed
with the API key explicitly unset.

## Implemented entity lookup

`arin_rdap_entity` requests `/registry/entity/HANDLE` for an organization or POC.
It exposes formatted names, entity kind, email and telephone values, address
labels, roles, status, events and directly linked entity handles/roles. Collections
are sorted and deduplicated where appropriate. Missing optional scalar fields
remain null and missing collections are empty. Handle identity is case-insensitive.

The complete `vcard_json` retains structured addresses, multiple language variants,
telephone parameters, repeated properties and extensions using the
[jCard representation](https://www.rfc-editor.org/rfc/rfc7095.html).
`rdap_json` retains the entire returned entity, including embedded entities,
public identifiers, remarks, notices, links and extensions. JSON numbers are
preserved without conversion to floating point. These JSON fields represent the
returned public record; omitted or private registration details are not inferred.
No linked records or referrals are followed. Truncation and pagination notices in
nested entities are errors, as are mismatched identities and malformed jCards.

Fake-server tests cover contact extraction, repeated formatted names, structured
addresses, extensions, identity failures, nested partial responses, refresh and
missing records. `TestLiveRDAPEntity` passed organization and discovered POC
Terraform reads and clean plans against OT&E and production on 2026-09-23,
with credentials unset. No account payloads are stored in fixtures.

```sh
ARIN_LIVE_TESTS=1 TF_ACC=1 ARIN_TEST_ORG_HANDLE=YOUR-ORG \
  go test ./internal/provider -run '^TestLiveRDAPEntity$' -count=1 -v
```

## Implemented entity searches

`arin_rdap_entities` uses `/registry/entities?handle=TERM` when `search_by` is
`handle`, or `/registry/entities?fn=TERM` when it is `name`. `query` accepts an exact
term or one trailing wildcard. Query values are URL-encoded, including spaces,
Unicode and punctuation in names. ARIN performs name-component matching, so a
result's formatted name need not start with the query. Handle results must match
the requested exact handle or prefix, case-insensitively.

Results expose the same fields and complete per-entity JSON as `arin_rdap_entity`,
ordered by handle. The client rejects duplicate handles, malformed records,
pagination and truncation notices at the envelope or nested-entity levels. It
never follows result links or sends the API key. Empty arrays and complete HTTP
404 RDAP errors with `errorCode=404` become empty lists. Unstructured HTTP 404s,
wrong error codes, authorization failures, throttling and server failures remain
errors. This avoids turning a proxy error into an empty inventory.

Native probes on 2026-09-23 confirmed exact/wildcard handle searches and structured
404 responses for no-match handle/name queries in OT&E. The live Terraform test
passed organization-name searches, discovered POC handle searches, no-match
queries and clean plans against both OT&E and production. Native Terraform
queries are serialized to avoid request bursts. A common POC name returned truncated results for both exact
and wildcard queries; the test verifies Terraform reports that failure rather
than accepting the partial collection. When a POC name yields a complete result,
the same test instead verifies its returned identity and clean plan.

Mock tests cover sorted multi-record results, name-component matches, encoded
query punctuation, contact fields, refresh, no matches, invalid inputs, malformed
responses, duplicate/mismatched identities and partial-result rejection.

```sh
ARIN_LIVE_TESTS=1 TF_ACC=1 ARIN_TEST_ORG_HANDLE=YOUR-ORG \
  go test ./internal/provider -run '^TestLiveRDAPEntities$' -count=1 -v
```

## Implemented reverse-domain lookup

`arin_rdap_domain` requests `/registry/domain/NAME`. It accepts ASCII reverse DNS
names under `in-addr.arpa` or `ip6.arpa`, normalizing case and the optional trailing
dot for lookup. The original configured spelling remains in Terraform state.
Forward domain referrals are outside this ARIN data source. A missing domain
remains an error, unlike an empty entity search.

Outputs include the domain handle, direct registrant handles, embedded network
handle, status, events, nameservers and IPv4/IPv6 glue. Nameservers are sorted and
normalized to lowercase without a trailing dot. Duplicate nameservers and invalid
or wrong-family glue addresses are errors. The complete `rdap_json` retains nested
network/entity objects, DNSSEC record metadata and extensions without following
links or losing JSON number precision.

Published DNSSEC fields follow the
[RDAP domain representation](https://www.rfc-editor.org/rfc/rfc9083.html#section-5.3):
`zone_signed`, `delegation_signed`, `max_sig_life`, `ds_records` and `key_records`.
Absent flags and other optional scalars stay null. Missing collections are empty.
The client validates numeric ranges and hexadecimal DS syntax but does not verify
DNS signatures, look up live DNS records, or infer unsigned status from omissions.
It rejects partial-response notices in nested entities, nameservers, networks and
DNSSEC records as well as the domain envelope.

Client and fake Terraform tests cover IPv4/IPv6 names, normalization, published
true/false flags versus missing flags, DS/DNSKEY metadata, sorted nameservers,
glue, refresh, missing records, malformed responses, mismatched identities and
referral/partial-response rejection. Native `TestLiveRDAPDomain` passed on OT&E and
production on 2026-09-23 with credentials unset. It discovers IPv4/IPv6 reverse
zones from the selected organization's public network prefixes, tests both name
spellings and clean plans, and reads the signed public zone
`3.112.149.in-addr.arpa.` from ARIN's guide. Native DS records are verified;
DNSKEY `keyData` currently has fixture evidence only; native coverage for that
optional representation is not yet established.

```sh
ARIN_LIVE_TESTS=1 TF_ACC=1 ARIN_TEST_ORG_HANDLE=YOUR-ORG \
  go test ./internal/provider -run '^TestLiveRDAPDomain$' -count=1 -v
```

## Implemented reverse-domain hierarchy searches

`arin_rdap_domains` requests `/registry/domains/rirSearch1/rdap-RELATION/NAME`.
The `relation` values are `top`, `up`, `down` and `bottom`. It returns a `domains`
list using the same fields as `arin_rdap_domain`. A single-object response becomes
a one-element list; multi-object results are sorted by normalized domain name.
`up` follows registered parents, which can skip DNS labels. `bottom` can contain
an enclosing domain alongside more-specific domains. The provider preserves these
results instead of assuming that the returned domains are disjoint.

ARIN's guide and [RFC 9910](https://www.rfc-editor.org/rfc/rfc9910.html) describe
these relations. Native OT&E probes on 2026-09-23 confirmed all four operations.
The optional `active_only=true` sends `status=active` for `top` and `up` only.
OT&E returned HTTP 501 for active filtering on `down`/`bottom` and for inactive
filtering on `top`/`up`, so those combinations are not exposed. The service applies
the filter; a domain's existence or its configured nameservers are not interpreted
as proof of active status. Filtered ancestor queries for the tested owned domains
returned structured no-match responses.

Complete RDAP 404 error responses become empty lists. For multi-object relations,
a complete HTTP 404 containing an empty `domainSearchResults` array is also
accepted. Plain HTTP errors, malformed or contradictory responses, duplicate
domains, unrelated results, referrals, pagination and truncation are rejected.
Response-provided links are never followed. Every returned domain receives the
same nested completeness checks as direct lookup.

Client and fake Terraform tests cover all relations, status filtering, both
no-match response forms, overlapping enclosing/child results, sorting, nested
DNSSEC fields, refresh, clean plans and invalid/partial results. Native tests
exercise IPv4/IPv6 ancestors and bottom results, empty child results, active
filters, a complete child collection from ARIN's documented reference hierarchy,
and rejection of a broad IPv6 child search when ARIN truncates it. These native
Terraform tests passed on OT&E and production on 2026-09-23, including clean
plans. Queries are serialized and use no credentials.

```sh
ARIN_LIVE_TESTS=1 TF_ACC=1 ARIN_TEST_ORG_HANDLE=YOUR-ORG \
  go test ./internal/provider -run '^TestLiveRDAPDomains$' -count=1 -v
```

## Implemented network and ASN searches

`arin_rdap_networks` and `arin_rdap_asns` expose these documented searches:

| `search_by` | Network URL | ASN URL |
| --- | --- | --- |
| `handle` | `/registry/ips?handle=QUERY` | `/registry/autnums?handle=QUERY` |
| `name` | `/registry/ips?name=QUERY` | `/registry/autnums?name=QUERY` |
| `entity_handle` | `/registry/ips/reverse_search/entity?handle=QUERY` | `/registry/autnums/reverse_search/entity?handle=QUERY` |
| `entity_name` | `/registry/ips/reverse_search/entity?fn=QUERY` | `/registry/autnums/reverse_search/entity?fn=QUERY` |
| `entity_email` | `/registry/ips/reverse_search/entity?email=QUERY` | `/registry/autnums/reverse_search/entity?email=QUERY` |

`query` accepts an exact term or one trailing wildcard. Query parameters are
URL-encoded. The optional `role` accepts `any` (the default, omit the parameter),
`abuse`, `noc` or `technical`. A specific role is valid only for entity searches
and is sent as an additional query parameter. ARIN applies name, email and role
matching; the provider does not infer ownership from a match.

The `networks`/`asns` output contains all returned associations, sorted by handle.
This differs from `arin_networks` and `arin_asns`, which select direct registrants.
Use those existing inventories when direct registration is the intended scope.
Neither a public association nor a registration record proves write authority.

Each result exposes the established typed network/ASN fields plus `rdap_json`,
which preserves the complete returned record and its extensions. Network address
ranges, family and CIDR coverage are validated using the same decoder as direct
IP lookup. ASN ranges and registrant references are checked too. Nested partial
entity records, truncated/paginated envelopes, duplicate handles, mismatched
handle searches, malformed collections and contradictory error responses fail.
A structured RDAP 404 no-match response becomes an empty list; other HTTP errors
remain errors. No API key is sent and no returned links are followed.

Client tests cover every field/role combination, parameter encoding, associated
records whose registrant differs from the queried contact, empty results, IPv6,
ASN ranges and failure handling. Fake Terraform tests cover both data sources,
all five search fields, role filters, deterministic order, typed/raw output,
refresh, no matches and clean plans.

`TestLiveRDAPResourceSearches` discovers owned IPv4/IPv6 networks, an ASN, the
organization's public name, and a POC's public name/email. It tests exact/wildcard
registration searches, organization/contact reverse searches, all role values,
no matches and clean plans against OT&E and production. Broad contact-name
queries that ARIN truncates are tested as expected Terraform errors. Requests
are serialized, credentials are unset, and account payloads are not committed.
Both origins passed on 2026-09-23. The complete live-test target now allows ten
minutes to accommodate the expanded read-only coverage.

```sh
ARIN_LIVE_TESTS=1 TF_ACC=1 ARIN_TEST_ORG_HANDLE=YOUR-ORG \
  go test ./internal/provider -run '^TestLiveRDAPResourceSearches$' -count=1 -v
```

## Remaining endpoint audit

| Family | Current coverage | Remaining work |
| --- | --- | --- |
| IP network lookup | `arin_rdap_network`, native IPv4/IPv6 evidence | Final field/endpoint audit |
| IP network searches | `arin_networks` direct registrant inventory; `arin_rdap_networks` handle/name and entity reverse searches with supported role filters | Hierarchy relations and final field/endpoint audit |
| ASN lookup | `arin_asn` | Final field audit |
| ASN searches | `arin_asns` direct registrant inventory; `arin_rdap_asns` handle/name and entity reverse searches with supported role filters | Capability audit for RFC 9910 ASN hierarchy searches and final field/endpoint audit |
| Entity lookup | `arin_rdap_entity` exposes contact fields and complete JSON; native organization/POC reads verified | Final endpoint audit |
| Entity searches | `arin_rdap_entities` covers handle/name searches, exact/trailing-wildcard queries, no matches and partial-result rejection | Final endpoint audit |
| Reverse domain lookup/search | `arin_rdap_domain` plus `arin_rdap_domains` for all four hierarchy relations and supported active filters | Final field/endpoint audit |
| Standalone nameserver lookup | Unsupported by ARIN RDAP | No data source for an unimplemented operation |

The standalone nameserver endpoint was checked in OT&E on 2026-09-23:
`GET /registry/nameserver/ns1.arin.net` returned HTTP 501, RDAP `errorCode=501`,
`title=NOT IMPLEMENTED`. ARIN's guide explicitly says standalone nameserver queries
are unsupported for its registration data. This does not affect nameservers
embedded in domain responses or authenticated delegation management.

Reproduce that read-only capability probe without credentials or redirects:

```sh
curl --max-time 30 -H 'Accept: application/rdap+json' \
  https://rdap.ote.arin.net/registry/nameserver/ns1.arin.net
```

Run the native lookup test with:

```sh
ARIN_LIVE_TESTS=1 TF_ACC=1 ARIN_TEST_ORG_HANDLE=YOUR-ORG \
  go test ./internal/provider -run '^TestLiveRDAPNetwork$' -count=1 -v
```
