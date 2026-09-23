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

## Remaining endpoint audit

| Family | Current coverage | Remaining work |
| --- | --- | --- |
| IP network lookup | `arin_rdap_network`, native IPv4/IPv6 evidence | Final field/endpoint audit |
| IP network searches | `arin_networks` handles direct registrant inventory | Handle/name search, hierarchy relations, other entity reverse-search filters |
| ASN lookup | `arin_asn` | Final field audit |
| ASN searches | `arin_asns` handles direct registrant inventory | Handle/name search and other entity reverse-search filters |
| Entity lookup | `arin_rdap_entity` exposes contact fields and complete JSON; native organization/POC reads verified | Final endpoint audit |
| Entity searches | `arin_rdap_entities` covers handle/name searches, exact/trailing-wildcard queries, no matches and partial-result rejection | Final endpoint audit |
| Reverse domain lookup/search | Authenticated DNS data sources exist | Public RDAP domain lookup and hierarchy searches |
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
