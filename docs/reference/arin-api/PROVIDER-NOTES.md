# Provider implementation notes

Research notes, not an implemented or tested provider contract. References below point to the collected ARIN documentation.

## API boundaries

| Interface | Purpose | Implementation implication |
| --- | --- | --- |
| Reg-RWS | Registration records, delegations, relationships, tickets and reports | Model each operation's response and completion behavior; do not assume every write returns the managed object. |
| IRR REST | Routing-policy objects in XML or RPSL | Use a consistent representation and normalize API-generated fields for stable plans. |
| RPKI REST | Organization-scoped ROA/ASPA transactions and listing | Implement transaction semantics and distinguish object handles from desired configuration. |
| RDAP / Whois-RWS | Public registration lookups | Potential data sources; public visibility is not proof of mutation authority. |
| Account download endpoints | Generated reports and bulk data | Treat these separately from synchronous resource CRUD. |

Sources: [Reg-RWS](reg-rws/methods.md), [IRR](irr/api.md), [RPKI](rpki/api.md), [RDAP](lookup/rdap.md), [bulk downloads](reports/bulk-whois.md).

## Authentication and environments

The management endpoint is `https://reg.arin.net`; OT&E uses `https://reg.ote.arin.net`. The July 2026 documentation adds the preferred `Authorization: ApiKey <key>` header while retaining query-string authentication. The API key acts with the associated account's record authority. Check each API family separately, especially older report download examples that still show query parameters.

OT&E uses periodically refreshed production-like data. Read the current refresh schedule and service limitations before relying on it for acceptance testing. A local fake API will still be useful for deterministic tests.

Sources: [quick start](reg-rws/quickstart.md), [API keys](operations/api-keys.md), [OT&E](operations/ote.md).

## Resource lifecycle questions to resolve

- Registration methods document both direct payload responses and ticket-based workflows. Decide separately for each resource whether create/update/delete can wait for a terminal result, should be import-only, or belongs outside Terraform. Do not infer that all registration operations are unsupported just because another provider excludes them.
- Fetch existing payloads before edits as ARIN recommends. Identify editable fields, server-owned values, optional fields, and omitted-versus-empty behavior from the payload reference and OT&E.
- RPKI uses a unified transaction with add/delete lists, including combined ROA and ASPA changes. Investigate how to reconcile Terraform's individual resource operations with those transactions and concurrent changes.
- ROA auto-linking can create and maintain IRR route objects. Establish ownership rules before managing the same route independently.
- IRR currently documents one action per REST call, without batch processing. Implement object-specific permissions and canonical identity rules.
- Delegation operations manage nameservers and DNSSEC data. Distinguish removal of Terraform management from deletion of published delegation contents.
- Define import IDs and missing-object behavior from actual endpoint identities. Validate retries, partial failures, ticket completion, and propagation behavior before exposing destructive operations.

Sources: [methods and errors](reg-rws/methods.md), [XML payloads](reg-rws/payloads.md), [IRR guide](irr/api.md), [RPKI guide](rpki/api.md), [reverse DNS](dns/reverse.md).

## Documentation limitations

No API calls have been tested against an account. The documentation snapshot is evidence of the published interface, not evidence that a particular account is authorized or that every example succeeds unchanged. The Relax NG archive linked from Whois-RWS actually contains registration and RPKI payload definitions; some appear older than the current guides. See [schema findings](schemas/README.md). There is no generated provider schema yet.
