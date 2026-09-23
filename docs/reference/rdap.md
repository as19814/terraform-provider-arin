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

## Remaining endpoint audit

| Family | Current coverage | Remaining work |
| --- | --- | --- |
| IP network lookup | `arin_rdap_network`, native IPv4/IPv6 evidence | Final field/endpoint audit |
| IP network searches | `arin_networks` handles direct registrant inventory | Handle/name search, hierarchy relations, other entity reverse-search filters |
| ASN lookup | `arin_asn` | Final field audit |
| ASN searches | `arin_asns` handles direct registrant inventory | Handle/name search and other entity reverse-search filters |
| Entity lookup | `arin_org_pocs` reads linked public contacts | Full public organization/POC entity representation |
| Entity searches | Not implemented | Handle/name searches and complete-result handling |
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
