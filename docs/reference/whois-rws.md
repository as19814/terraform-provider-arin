# Public Whois-RWS coverage

The implementation follows the [ARIN Whois-RWS API guide](https://www.arin.net/resources/registry/whois/rws/api/)
and its [local reading copy](arin-api/lookup/whois-rws-api.md). Whois-RWS is a
public lookup service, separate from authenticated Reg-RWS and from RDAP.

## Individual records

| Data source | Endpoint | Typed fields |
| --- | --- | --- |
| `arin_whois_org` | `/rest/org/HANDLE` | Handle, name, address, can-allocate flag, dates, comments, references |
| `arin_whois_customer` | `/rest/customer/HANDLE` | Handle, name, address, parent org, can-allocate flag, dates, comments, references |
| `arin_whois_poc` | `/rest/poc/HANDLE` | Handle, names, company, address, role-account flag, POC type/status and descriptions, email, phone type/description, dates, comments, references |
| `arin_whois_asn` | `/rest/asn/HANDLE` | Handle, name, ASN range, org handle, dates, comments, references |
| `arin_whois_net` | `/rest/net/HANDLE` | Handle, name, address range, family, blocks/CIDR lengths, org/customer/parent handles, dates, comments, references |
| `arin_whois_delegation` | `/rest/rdns/NAME` | Delegation name, nameservers, DS records, update date, references |

Every record includes complete `whois_xml`, retaining all returned metadata,
references, extensions and expanded related objects. Typed fields omit unpublished
values rather than synthesizing private registration information. No reference,
RDAP URL, stylesheet or redirect is followed. HTTP failures, including 404, remain
errors for these individual lookups.

Optional `show_details = true` sends `showDetails=true`. Additional inline records
remain in the complete XML. Any nested `limitExceeded=true` rejects the entire
response, even when the primary record appears complete. Invalid limit flags also
remain errors. Search data sources are described below.

POC outputs include `poc_type_description`, `status_description`, and each
phone's `description` from its type metadata. These fields are also available in
full-detail POC search and relationship results; reference-only results do not
invent them. Missing or foreign-namespace descriptions remain null, while the
complete XML retains the original response. Mock lookup/search/relationship
acceptance and read-only Terraform lookups on both origins passed on 2026-09-23.

## POC name field audit

The guide's Data Transformation example explicitly reads `firstName`,
`middleName` and `lastName`; its search table also documents a `middle` filter.
`middle_name` is now exposed alongside the first and last names in individual
POC lookups, full-detail POC searches and the org/ASN/network POC relationships.
Reference-only results retain null name fields when names are not published.

Decoder tests cover populated, absent and foreign-namespace middle names while
preserving complete XML. Fake Terraform tests cover populated values in each of
the five affected data sources. The read-only six-record lookup suite passed on
OT&E and production with clean subsequent plans. The reference POC omitted
`middleName` on both origins, so that native run establishes null handling only;
a populated native middle name has not been observed in this audit.

The public records inspected during the audit also include root terms/copyright
attributes, related-resource containers and reference names/URLs. These remain
available in complete `whois_xml`; typed relationship data sources expose their
identities separately. No response URL is followed and no API key is sent.

## Delegation DNSSEC display names

Native field inspection on 2026-09-23 found `name` attributes on delegation
`algorithm` and `digestType` elements. Both `arin_whois_delegation` and detailed
`arin_whois_net_delegations` now expose these as `ds_records.algorithm_name` and
`ds_records.digest_type_name`, alongside their numeric codes. The text comes from
ARIN's response, rather than a local algorithm registry. Absent names remain null;
foreign-namespace attributes cannot supply typed values. Complete XML is retained.

Unit tests cover present, absent and foreign attributes. Mock Terraform lookup
and relationship tests verify populated values and clean plans. Native read-only
Terraform lookup tests passed on OT&E and production (11.50 seconds), checking
every returned DS record's display names for `3.112.149.in-addr.arpa.` and clean
subsequent plans. Native populated relationship display-name coverage remains
unverified; the relationship uses the shared delegation decoder.

A bounded native inspection also compared element and attribute paths for all
six record types. The sampled core scalar fields are represented in the typed
schemas; notices, reference URLs/names and related-resource containers remain
available in complete XML. This sample does not establish that every possible
optional field or undocumented endpoint has been exhausted. Delegation-search
ambiguity and the final coverage reconciliation remain open.

## Relationships

All twelve documented relationship operations have data sources:

| Data source | Path after `/rest/` | Output list |
| --- | --- | --- |
| `arin_whois_poc_orgs` | `poc/HANDLE/orgs` | `orgs` |
| `arin_whois_poc_asns` | `poc/HANDLE/asns` | `asns` |
| `arin_whois_poc_nets` | `poc/HANDLE/nets` | `networks` |
| `arin_whois_org_pocs` | `org/HANDLE/pocs` | `pocs` |
| `arin_whois_org_asns` | `org/HANDLE/asns` | `asns` |
| `arin_whois_org_nets` | `org/HANDLE/nets` | `networks` |
| `arin_whois_asn_pocs` | `asn/HANDLE/pocs` | `pocs` |
| `arin_whois_net_pocs` | `net/HANDLE/pocs` | `pocs` |
| `arin_whois_net_parent` | `net/HANDLE/parent` | `networks` |
| `arin_whois_net_children` | `net/HANDLE/children` | `networks` |
| `arin_whois_net_delegations` | `net/HANDLE/rdns` | `delegations` |
| `arin_whois_delegation_nets` | `rdns/NAME/nets` | `networks` |

References are the default. They expose identity, available name and address
range attributes, and POC function codes. Unpublished detail fields are null or
empty; ASN ranges are not inferred from handles. `show_details = true` requests
full records using the same validators as individual lookups. Parent responses
always contain a full network, returned in a one-element list.

The complete response is stored once in top-level `whois_xml`. Lists sort by
identity and field values, retaining repeated handles with distinct POC role
links. `poc_functions` preserves reference role codes and collects expanded POC
roles from inline links to the requested owner. These are Whois associations,
which may differ from RDAP direct-registrant inventories.

Native empty relationships can return HTML HTTP 404. Only the known Whois page
and exact no-results messages trigger an independent owner lookup. A confirmed
existing owner permits an empty list and null `whois_xml`; unknown owners,
unrecognized 404 pages, partial responses and referrals remain errors. An org's
POCs are not automatically inherited by its network/ASN relationship endpoints.

## Searches

| Data source | Endpoint | Filter keys |
| --- | --- | --- |
| `arin_whois_orgs` | `/rest/orgs` | `handle`, `name`, `dba` |
| `arin_whois_customers` | `/rest/customers` | `handle`, `name` |
| `arin_whois_pocs` | `/rest/pocs` | `handle`, `domain`, `first`, `middle`, `last`, `company`, `city` |
| `arin_whois_asns` | `/rest/asns` | `handle`, `name` |
| `arin_whois_nets` | `/rest/nets` | `handle`, `name` |

A required `filters` map supplies one or more predicates, combined with AND.
Values match case-insensitively and allow one trailing `*` for a prefix search.
Filter keys are checked locally: native OT&E silently ignores unknown keys.
Empty/null values, control characters and unsupported wildcard positions are
rejected. Keys are sorted and values are escaped independently as URL matrix
parameters, preserving spaces, Unicode and punctuation without injecting paths,
additional predicates or query options.

Searches return references by default and full records with `show_details = true`.
Typed fields use the same record validators as lookups and relationships. Full
XML is retained once at the top level. A recognized native Whois no-results
HTTP 404 produces an empty list and null XML; other HTTP errors remain errors.
Any nested truncation rejects the response. No pagination or response links are
followed, and the provider never labels a capped result a complete inventory.

On 2026-09-23, native OT&E accepted all documented secondary filters: adding an
impossible value to a known handle produced no matches instead of ignoring the
filter. Public handle searches and combined filters also passed Terraform on both
origins in reference and detail modes. Prefix searches, empty results, clean plans
and rejection of an over-limit `A*` org search passed. `TestLiveWhoisSearches`
retains these checks; fixtures and state contain no committed live payloads.

The guide lists `/rdns` with a "delegation name" parameter without spelling out
a matrix key. A bounded probe, rerun on 2026-09-23, produced these results on
both OT&E and production for a known delegation (`NAME` below):

| Request | Observed result |
| --- | --- |
| `/rest/rdns/NAME` and `/rest/rdns/NAME.` | HTTP 200, expected individual delegation |
| `/rest/rdns/NAME*` | HTTP 404 |
| `/rest/rdns;name=NAME` | HTTP 400 |
| `/rest/rdns;dname=NAME` | HTTP 400 |
| `/rest/rdns;delegationName=NAME` | HTTP 400 |
| `/rest/rdns;delegation%20name=NAME` | HTTP 400 |
| `/rest/rdns?name=NAME` | HTTP 400 |
| `/rest/rdns/NAME;name=does-not-exist.invalid` | HTTP 200, original delegation; matrix predicate ignored |

Reproduce with `python3 tools/probe-whois-delegations/probe.py`. It uses public
GETs, fixed origins, no credentials, no redirects, two workers and bounded
response reads. It prints status and root/identity checks rather than payloads.
Transport errors are recorded separately and are not evidence of an HTTP result;
failed search requests were retried individually for the table above. The network
relationship control succeeded on OT&E; the production control encountered a
transport error in this probe, with previous native relationship evidence retained.

A successful lookup with an ignored matrix parameter does not establish search
support. A separate delegation search remains unproven, rather than declared
unsupported. Existing exact delegation lookup and network-to-delegation queries
remain available. The current guide and its linked schema archive were rechecked:
the archive still contains Reg-RWS payload schemas, so it cannot resolve this
public Whois-RWS ambiguity. Keep it in the final endpoint audit.

## Origins and XML handling

`whois_base_url` overrides `ARIN_WHOIS_BASE_URL`. With neither set, `base_url`
selects `https://whois.arin.net` or `https://whois.ote.arin.net` for production or
OT&E. A custom registration origin requires an explicit Whois origin. Non-loopback
origins require HTTPS. The API key is never sent to Whois-RWS.

ARIN serves core records in its `whoisrws/core/v1` namespace and delegations in
`whoisrws/rdns/v1`. The observed namespace URI scheme is HTTPS; corresponding HTTP
namespace identifiers are accepted as well. Only recognized namespaces contribute
typed fields. Foreign namespaces cannot impersonate registration fields, but their
original XML is preserved. Malformed XML, multiple roots and DTD declarations are
rejected; the shared 4 MiB response and 64-level nesting limits apply.

Native canAllocate/isRoleAccount flags use Y/N. They become Terraform booleans.
Network family values 4/6 become v4/v6, and addresses are canonicalized. Numeric
street/comment line indices determine text order. ASN ranges and network block
boundaries are validated, including agreement between each block and its CIDR
length. DS integers must fit their protocol widths and digest text must be hex;
this is not cryptographic validation. Input delegation case/trailing-dot spelling
is preserved in Terraform while the request uses normalized reverse DNS syntax.

## Verification

`TestLiveWhoisLookups` passed on both OT&E and production on 2026-09-23 with
credentials unset. The native cases cover all six object types, org inline details,
IPv4 and IPv6 networks/delegations, and the published DS data for
`3.112.149.in-addr.arpa.`. The test compares typed fields with independent client
reads, checks complete XML in state, and verifies a clean subsequent plan.
`C00000055` is an existing public customer reference discovered through Whois;
no customer or other record was created for these tests.

Unit and fake Terraform tests cover identity and namespace mismatches, foreign
field injection, malformed XML, nested truncation, redirects/status failures,
flag/address/line normalization, origin defaults and overrides, complete XML
preservation, refreshes and clean plans. Fixtures contain synthetic documentation
records. Live payloads, contacts, keys and state are not committed.

```sh
ARIN_LIVE_TESTS=1 TF_ACC=1 go test ./internal/provider \
  -run '^TestLiveWhoisLookups$' -v -count=1
```

`TestLiveWhoisRelationships` also passed on both origins on 2026-09-23 with
credentials unset. All twelve operations were exercised in reference and detail
modes, with populated results, POC function codes, IPv4/IPv6 delegation links,
confirmed empty relationships, complete XML and clean subsequent plans. Existing
public `ZG39-ARIN`, `AS15169` and `NET-216-239-32-0-1` records supply positive
direct POC links; FT-684 records supply organization and network cases.

```sh
ARIN_LIVE_TESTS=1 TF_ACC=1 go test ./internal/provider \
  -run '^TestLiveWhois' -v -count=1 -timeout 5m
```

## IP and CIDR queries

| Data source | Endpoint | Result |
| --- | --- | --- |
| `arin_whois_ip` | `/rest/ip/ADDRESS` | Full containing network |
| `arin_whois_cidr` | `/rest/cidr/ADDRESS/LENGTH` | Full registration for the allocation segment |
| `arin_whois_cidr_networks` | `/rest/cidr/ADDRESS/LENGTH/less` or `/more` | List of enclosing or more-specific network references/full records |

All three accept IPv4 and IPv6, `show_details` and `show_arin` (default true).
IP/CIDR individual lookups retain missing-record errors. Hierarchies return empty
lists for successful empty XML or recognized no-results HTTP 404 pages. Other
failures, partial responses and referrals remain errors. Complete XML is retained.

These operations have distinct semantics. OT&E `23.189.120.0/24` returns its
registration, while an exact `/25` query returns 404. An address in
`2602:f805::/36` can resolve to a child reassignment, while the allocation CIDR
resolves to `NET6-2602-F805-1`. Both hierarchy directions can include the queried
registration. More-specific results can also contain a larger multi-block
registration matching an allocation segment. IP-address `/less` and `/more`
paths returned 404; only CIDR hierarchy operations are exposed.

The provider validates address families and range overlap/containment. Full
records are checked against their actual blocks, including gaps between disjoint
blocks. Input IPv6 spellings are preserved in state and normalized in request
paths. Noncanonical CIDRs, mapped IPv6 and scoped addresses are rejected.

Native `show_arin=false` hides ARIN's allocation for `260f:ffff::1`, changing the
address lookup from `NET6-2600-1` to 404. A `/48/less` query for that space returns
an empty network list with the flag false. This flag does not hide ordinary
registrant records and does not redefine exact CIDR lookup as containment search.

## Organization contact expansion

`arin_whois_org.show_pocs = true` sends `showPocs=true` and retains POC references
in complete XML without the network/ASN lists. If `show_details = true` is also
set, those other relationships are included too. This option is deliberately
limited to individual org lookups, where ARIN recognizes it. Typed related-contact
queries remain available through `arin_whois_org_pocs`.

`TestLiveWhoisNetworkQueries` passed on OT&E and production on 2026-09-23,
including IPv4/IPv6 single lookups, both CIDR hierarchy directions in reference
and full-detail modes, clean subsequent plans, ARIN-allocation filtering, exact
CIDR misses, child-versus-parent identity, and both org expansion modes. No keys
or writes were used. Fake Terraform tests and unit tests cover invalid inputs,
wrong families/ranges, disjoint blocks, empty/error results and URL options.

## Remaining coverage

- Resolve the guide's ambiguous delegation-search entry during the final audit.
- Final field/endpoint audit against native behavior.
