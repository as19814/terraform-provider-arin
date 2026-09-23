# Public Whois-RWS coverage

The implementation follows the [ARIN Whois-RWS API guide](https://www.arin.net/resources/registry/whois/rws/api/)
and its [local reading copy](arin-api/lookup/whois-rws-api.md). Whois-RWS is a
public lookup service, separate from authenticated Reg-RWS and from RDAP.

## Individual records

| Data source | Endpoint | Typed fields |
| --- | --- | --- |
| `arin_whois_org` | `/rest/org/HANDLE` | Handle, name, address, can-allocate flag, dates, comments, references |
| `arin_whois_customer` | `/rest/customer/HANDLE` | Handle, name, address, parent org, can-allocate flag, dates, comments, references |
| `arin_whois_poc` | `/rest/poc/HANDLE` | Handle, names, company, address, role-account flag, POC type/status, email, phone, dates, comments, references |
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
remain errors. Search operations are separate remaining work.

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

## Remaining coverage

- Documented handle/name and other field searches, including wildcard and
  multi-predicate behavior, reference/full-detail modes, and truncated results.
- IP address and CIDR lookup, including more/less-specific relations.
- Remaining query options and final field/endpoint audit against native behavior.
