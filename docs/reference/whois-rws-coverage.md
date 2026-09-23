# Whois-RWS operation reconciliation

Audit date: 2026-09-23. Sources: the [current ARIN API guide](https://www.arin.net/resources/registry/whois/rws/api/),
[local guide](arin-api/lookup/whois-rws-api.md), provider catalogs and the native
Terraform suites below. All requests are public reads with credentials unset.

## Endpoint mapping

The current catalog exposes 27 data sources: six individual lookups, thirteen
relationships, five collection searches and three IP/CIDR queries.

| Guide operation | Terraform data source suffix after `arin_whois_` | Implementation |
| --- | --- | --- |
| `/org/HANDLE`, `/customer/HANDLE`, `/poc/HANDLE`, `/asn/HANDLE`, `/net/HANDLE` | `org`, `customer`, `poc`, `asn`, `net` | `internal/arin/whois.go` |
| `/rdns/NAME` | `delegation` | `internal/arin/whois.go` |
| `/poc/HANDLE/orgs`, `/asns`, `/nets` | `poc_orgs`, `poc_asns`, `poc_nets` | `internal/arin/whois_related.go` |
| `/org/HANDLE/pocs`, `/asns`, `/nets` | `org_pocs`, `org_asns`, `org_nets` | `internal/arin/whois_related.go` |
| `/asn/HANDLE/pocs` | `asn_pocs` | `internal/arin/whois_related.go` |
| `/net/HANDLE/pocs`, `/parent`, `/children`, `/rdns` | `net_pocs`, `net_parent`, `net_children`, `net_delegations` | `internal/arin/whois_related.go` |
| `/rdns/NAME/nets` | `delegation_nets` | `internal/arin/whois_related.go` |
| `/customer/HANDLE/nets` (native relationship) | `customer_nets` | `internal/arin/whois_related.go` |
| `/orgs`, `/customers`, `/pocs`, `/asns`, `/nets` with matrix predicates | `orgs`, `customers`, `pocs`, `asns`, `nets` | `internal/arin/whois_search.go` |
| `/ip/ADDRESS`, `/cidr/ADDRESS/LENGTH` | `ip`, `cidr` | `internal/arin/whois_network.go` |
| `/cidr/ADDRESS/LENGTH/less` or `/more` | `cidr_networks` | `internal/arin/whois_network.go` |
| Ambiguous `/rdns` collection-search entry | No separate search data source: contract still unproven | [Bounded probe and evidence](whois-rws.md#delegation-search-ambiguity) |

The five search catalogs include every matrix key named in the guide: org
handle/name/dba; customer handle/name; POC handle/domain/first/middle/last/company/city;
ASN handle/name; and network handle/name. General `q` is also implemented and
native-tested. XML is the chosen representation; JSON, text, HTML and versioned
media aliases do not require duplicate data sources. `showDetails`, org
`showPocs`, and IP/CIDR `showARIN` are exposed where supported.

## Field evidence and limits

Fresh OT&E inspection covered FT-684, AS19814, NET-23-189-120-0-1, KOSTE-ARIN,
C00000055 and the signed delegation 3.112.149.in-addr.arpa. Only field names were
recorded in the audit output; no account response bodies or contacts were saved.
The returned scalar and nested value fields map to the individual lookup catalogs:

- Common identity, dates, comments and reference URLs.
- Org/customer addresses, country metadata and allocation flag; customer parent org.
- POC names, company, role flag, type/status descriptions, emails and typed phones.
- ASN range and org handle; network range, family, block metadata and parent/org handles.
- Delegation nameservers, DS numeric values, digest and algorithm/digest display names.

Reference labels, reference URLs beyond the common fields, root attributes and
inline expansions remain in complete `whois_xml`. Relationship data sources
expose related identities separately. Nested truncation flags are checked before
returning any response. An absent optional field is not evidence that ARIN never
publishes it; middle-name and other optional-field mocks remain necessary.

The guide-linked schema archive was re-fetched and matched the saved file exactly:
SHA-256 `9b69235e12d218b535bed9724a7f73a8e0f01551fd4c05e108c1e36d7ce24edf`.
Its `OrgPayload.rnc` declares the Reg-RWS namespace and fields such as `orgName`
and `dbaName`, not the public Whois `name` representation. This confirms the
existing [schema warning](arin-api/schemas/README.md); the archive cannot establish
Whois field completeness or justify adding authenticated registration fields to
public lookup schemas.

## Native verification

`TestLiveWhoisLookups`, `TestLiveWhoisRelationships`, `TestLiveWhoisSearches` and
`TestLiveWhoisNetworkQueries` exercise both OT&E and production. They create
actual Terraform state, check typed values and retained XML, and run subsequent
clean plans. Searches include empty results and truncation rejection; network
queries cover both families and hierarchy directions. Their fixtures do not
prove every possible filter/optional-field combination.

The combined suite passed on 2026-09-23 in 115.504 seconds.

Reproduce with:

```sh
ARIN_LIVE_TESTS=1 TF_ACC=1 go test ./internal/provider \
  -run '^TestLiveWhois' -count=1 -timeout 5m
```

No additional concrete endpoint gap was found in this reconciliation. Delegation
collection search remains unresolved, not declared unsupported. The complete
provider objective remains open for that ambiguity and the other API families'
remaining evidence requirements.
