# Terraform Provider for ARIN

A Terraform provider for ARIN, developed by AS19814 using the Terraform Plugin Framework and protocol version 6.

The provider includes 32 read-only data sources spanning registration records, network discovery, DNS delegations, contacts, customers, IRR, hosted RPKI, ASN registrations, and existing tickets. See the [complete catalog](docs/reference/data-sources.md). Thirteen managed resources cover reverse DNS delegations and individual nameservers, existing network metadata, downstream network registrations, customer and POC records, individual POC emails and phones, organization POC associations, simple IRR AS sets, route sets, aut-num routing policies, and IPv4/IPv6 routes, with creation, updates, deletion, and import. Track the full sandbox implementation in the [coverage inventory](docs/reference/implementation-status.md). The repository is private and the provider has not been published to a registry.

## Configuration

For authenticated registration, IRR, RPKI, and ticket reads, set `ARIN_API_KEY` in your shell. The provider sends it as an authorization header, and your account must have authority over the requested records. Public network/ASN discovery and contact references use RDAP without sending an API key and work without credentials.

```hcl
terraform {
  required_providers {
    arin = {
      source = "as19814/arin"
    }
  }
}

provider "arin" {
  base_url        = "https://reg.arin.net"
  timeout_seconds = 30
}

data "arin_org" "ours" {
  handle = "FT-684"
}

data "arin_networks" "ours" {
  org_handle = "FT-684"
}

output "networks" {
  value = data.arin_networks.ours.networks
}
```

`base_url` defaults to `ARIN_BASE_URL`, then production. Set it to `https://reg.ote.arin.net` for OT&E. Explicit provider attributes take precedence over environment variables. The RDAP origin automatically follows production or OT&E; override it with `rdap_base_url` or `ARIN_RDAP_BASE_URL`. Custom registration origins require an explicit RDAP origin for network discovery. See the [provider schema](docs/index.md), [organization data source](docs/data-sources/org.md), and [network listing](docs/data-sources/networks.md).

`arin_networks` returns a map keyed by network handle, with names, address families, registration types, address ranges, and CIDRs. It includes allocations and assignments directly registered to the organization, including overlapping parent and more-specific registrations. Contact-only associations and networks reassigned to other organizations are excluded. A public registration does not establish API-key authority. Searches that report truncation or pagination fail rather than returning a partial inventory.

## Local development

Requires Go 1.25.8 or newer and Terraform CLI. CI uses Terraform 1.15.5.

```sh
make build
make check
make generate
```

To use the unpublished binary, create `.terraformrc.local` in this checkout with an absolute path to its `bin` directory:

```hcl
provider_installation {
  dev_overrides {
    "registry.terraform.io/as19814/arin" = "/absolute/path/to/terraform-provider-arin/bin"
  }
  direct {}
}
```

Set `TF_CLI_CONFIG_FILE` to the absolute path of this file, then run `terraform plan` in a directory containing your configuration. With a development override, skip `terraform init` for a configuration that only uses this provider; it is not available from the public registry. Configurations needing other providers or modules still need those dependencies initialized separately.

Rebuild with `make build` after source changes. Use `bin/terraform-provider-arin -debug` when attaching a Go debugger.

## Tests

| Command | Coverage | ARIN access |
| --- | --- | --- |
| `make test` | Client and provider unit tests, race detector | None |
| `make testacc` | Real Terraform against an in-process fake API, including refresh, empty inventories, truncation, and missing records | None |
| `make testlive` | Real Terraform following existing organization, network, contact, ASN, IRR, and RPKI records | Read-only |
| `make check` | Vet, unit tests, fake acceptance tests, binary build | None |

The fake server remains the default for deterministic tests and CI. Live tests need a separate opt-in, an existing organization handle, and your shell's API key:

```sh
ARIN_TEST_ORG_HANDLE=FT-684 ARIN_BASE_URL=https://reg.arin.net make testlive
```

`make testlive` sets `ARIN_LIVE_TESTS=1` and `TF_ACC=1` and disables test caching. It reads only; it does not create, update, or delete ARIN records. Public discovery uses RDAP and does not transmit the key. The selected live organization must have a network to exercise the chained registration test. Optional families are read only when existing objects are discovered. CI receives no ARIN credentials and does not run live tests. No private fixtures or account responses are committed.

## Layout

- `internal/arin/`: HTTP client, XML/JSON response models, an explicit catalog of read endpoints, typed errors, and fake-server unit tests. Independent of Terraform.
- `internal/provider/`: provider configuration, data sources, and Terraform acceptance tests.
- `examples/`: Terraform configuration examples used in generated documentation.
- `docs/`: generated provider and data source documentation.
- `docs/reference/arin-api/`: official API snapshots, source provenance, and implementation research.
- `scripts/`: documentation snapshot tooling.
- `tools/catalog/`: deterministic example and catalog generation for every data source.

API requests have deadlines and bounded responses. Redirects are rejected, raw response bodies are excluded from diagnostics, and the API key is redacted from parsed server errors. Retries are intentionally not automatic: individual mutation APIs will need explicit completion and retry semantics.

## Read-only scope

Every non-report read endpoint in the collected Reg-RWS, IRR, and hosted RPKI guides is covered. Report-request endpoints are excluded because they create tickets, even when called with GET. Existing ticket summaries, messages, and attachments can be read. Sensitive customer/ticket content and organization tax IDs remain in Terraform state even when Terraform masks them. Full API coverage and live-test limitations are recorded in the [catalog](docs/reference/data-sources.md).

## Managed AS sets

`arin_irr_as_set` manages the full XML representation of a simple IRR AS set.
Names and organization handles are uppercase and changes to either require
replacement. Membership is an unordered set; description and remarks
are ordered lists of lines. Optional collections default to empty, so omitting
existing values after import plans to remove them. Source is always `ARIN`.
POC links are computed by ARIN from the maintaining organization and cannot be configured on the AS set. Advanced RPSL objects are not supported.

Import an existing set by name before applying configuration:

```sh
terraform import arin_irr_as_set.peers AS-EXAMPLE-PEERS
terraform plan
```

Match configuration to the imported record and review the plan before applying.
For an existing operational set, consider `lifecycle { prevent_destroy = true }`.
This guards planned deletion/replacement while the resource block remains in
configuration; removing the block also removes that protection.
Deletion removes the AS set from ARIN, not just from Terraform state.

Resource lifecycle tests use a stateful fake API and the real OT&E service.
Create, update, clearing remarks, import, clean plans, and deletion have passed
in OT&E. Production writes have not been performed. Requests are never
automatically retried. If a create times out or returns an unreadable result,
check whether the object exists and import it if necessary before retrying.
Updates and deletes retain state on errors. Only a 404 means the object is absent;
authentication and server errors never cause state removal.

## Managed IRR routes

`arin_irr_route` manages simple IPv4 and IPv6 IRR route objects. Configure a
canonical `prefix`, `origin_as` (such as `AS64496`), `org_handle`, and ordered
`description` lines. Optional `remarks` default to empty. POC links, network
handle, and timestamps are computed by ARIN. Prefix, origin ASN, and organization
changes require replacement. Removing the resource deletes the IRR object.

The import ID is the canonical prefix followed by a comma and origin ASN:

```sh
terraform import arin_irr_route.example '192.0.2.0/24,AS64496'
terraform import arin_irr_route.example_v6 '2001:db8::/48,AS64496'
```

Import existing objects before managing them. ROA-linked routes are rejected,
including a fresh check before update or delete, because their lifecycle belongs
to RPKI. Advanced RPSL objects and routes with `memberOf` associations are not
supported. Unsupported XML fields fail reads rather than permitting a partial
replacement payload. HTTP 404 removes missing objects from state; other errors
preserve state. Mutations are not automatically retried.

Both address families pass mock and OT&E lifecycle tests: create, update, clearing
remarks, import, clean plan, deletion, and confirmation of absence. Mock tests also
exercise drift repair, external deletion, origin replacement, and error handling.
Production writes have not been performed.

## Managed route sets and aut-num policies

[`arin_irr_route_set`](docs/resources/irr_route_set.md) manages IPv4 `members`,
IPv4/IPv6 `mp_members`, and `members_by_ref`, plus description and remarks. The
import ID is its uppercase set name. Prefix range expressions are supported by
the XML API; `^+` was verified in OT&E. Names and organization changes require
replacement. POC links are computed. Omitting optional collections clears them.

[`arin_irr_aut_num`](docs/resources/irr_aut_num.md) manages the IRR policy object
for an ASN, not the ASN registration itself. It supports `as_name`, description,
remarks, AS-set `member_of`, and all six policy collections: `import_policy`,
`export_policy`, `default_policy`, and their `mp_` counterparts. Each policy is
an ordered list of RPSL lines. Import by canonical `as_number`, such as `AS64496`.
The ASN and organization are replacement fields; other configured fields update
in place. Deletion removes the IRR object and leaves the ASN registration intact.

Both resources pass mock and OT&E create/update/clear/import/delete lifecycles.
Advanced RPSL objects remain in the implementation backlog.

## Customer records

[`arin_customer`](docs/resources/customer.md) manages the recipient record used
for simple network reassignments. It supports name, address, privacy and comments,
and preserves ARIN-generated identity and registration date during updates.
Import uses `PARENT-NET-HANDLE/CUSTOMER-HANDLE` because the API does not return the
creation parent. Creating a customer does not itself reassign address space.
Mock and OT&E create/update/clear/import/delete lifecycles pass.

## Downstream network registrations

[`arin_net`](docs/resources/net.md) manages simple reassignments to customers,
detailed reassignments to organizations, and reallocations. Name and comments
update in place; changes to parent, recipient, prefixes or creation mode replace
the registration. Reference an `arin_customer` ID to establish deletion order.

Import existing downstream registrations by NET handle. [`arin_net_metadata`](docs/resources/net_metadata.md) manages name, comments
and explicit POC links on existing NETs, including direct allocations. Destroy
removes metadata management and leaves the NET and its last values unchanged.
When combining both resources, configure only POC links in the metadata resource
so their managed fields do not overlap. ARIN retired NET
Origin AS in July 2025; use `arin_irr_route` for routing announcements.

Pending and uncertain writes retain recovery state. See the
[network lifecycle and recovery guide](docs/reference/net-registration.md)
before retrying a failed apply.

[`arin_delegation`](docs/resources/delegation.md) manages all nameservers and DNSSEC DS records on an existing reverse zone. Import zones with existing records first. Destroy clears both collections. Omitted TTLs preserve existing values; newly added records inherit TTL. Do not manage the same records through multiple resources.

[`arin_delegation_nameserver`](docs/resources/delegation_nameserver.md) manages one nameserver without changing sibling nameservers or DS records. Its optional TTL resets to inheritance when omitted. Use either full-delegation management or individual nameserver resources for a zone, never both.

[`arin_poc`](docs/resources/poc.md) manages role and person contacts, including address, email and phone collections. New contacts are linked to the API account. Changes to contact type or first/middle/last names require replacement; remove dependent associations before destroy.

[`arin_poc_email`](docs/resources/poc_email.md) and [`arin_poc_phone`](docs/resources/poc_phone.md) manage individual records on existing contacts. They preserve sibling records, support import, and require replacement for changes. Do not overlap their ownership with the full `arin_poc` collections.

[`arin_org_poc`](docs/resources/org_poc.md) manages one organization association for a Tech, NOC, Abuse, Routing or DNS POC. Import existing links first. Destroy removes only the association. Admin changes require a full organization update. Association writes to the same organization are serialized within the provider process because concurrent OT&E writes can lose changes. See the [association evidence and recovery notes](docs/reference/org-pocs.md).

## Next steps

Extend managed-resource support to additional IRR objects, organizations, contacts, and hosted RPKI, with explicit lifecycle semantics and OT&E validation. Network and reverse DNS management are implemented. Registration workflows and tickets need their own lifecycle decisions before being exposed as managed resources.

Start with the [API index](docs/reference/arin-api/README.md), [provider notes](docs/reference/arin-api/PROVIDER-NOTES.md), and [schema findings](docs/reference/arin-api/schemas/README.md).

## OT&E write validation

The managed resources and API clients have a separate opt-in sandbox test command:

```sh
ARIN_TEST_ORG_HANDLE=FT-684 make testote
```

Set `ARIN_OTE_API_KEY` in the shell. Older IRR tests also support an
`ARIN_API_KEY` fallback; network tests require the explicit sandbox variable.
The test pins both API origins to OT&E regardless of `ARIN_BASE_URL` and
`ARIN_RDAP_BASE_URL`. It validates organization access before writing.
It creates a randomly named `AS-TF-OTE-*` set, updates membership and descriptions,
clears remarks, imports into separate state, verifies a clean plan, deletes the
set, and confirms it is absent. Cleanup also checks for an object left behind by
a failed apply. The generated name is printed for recovery if cleanup fails.
The test uses documentation ASNs AS64496 and AS64497 as disposable members.
The route test selects a random IPv4 /32 and IPv6 /128 within OT&E registrations
held by the organization, confirms parent registration ownership, and confirms
each prefix/origin pair is absent. It uses the documentation ASN AS64496. It then
creates, updates, imports, verifies a clean plan, deletes, and checks absence for
both families. Prefixes are printed for recovery if cleanup fails.
The route-set test exercises IPv4/IPv6 membership, MNT references, prefix ranges,
and collection clearing. The aut-num test discovers an ASN registered to the
organization with no existing IRR aut-num, creates a disposable helper AS set,
and exercises all policy fields and membership before deleting both IRR objects.
The ASN registration remains unchanged. A lack of a suitable unused IRR identity
fails preflight rather than changing an existing object.
The NET tests select a free IPv4 /32 and IPv6 /64, then test customer
reassignment, organization reassignment, and reallocation. They verify updates,
import, clean plans and deletion, and remove their disposable customer records.
Metadata tests edit only disposable child NETs. The direct-allocation test
sends an unchanged metadata payload to an owned parent and verifies identical
before-and-after values. DNS client tests use an existing delegation on an owned
direct allocation in each address family. They write a mode-0600 recovery copy
outside the repository, exercise nameservers and DS records, restore the original
configuration and verify equality. Failed restoration retains the recovery copy
and prevents overwriting it on a later run. See the
[DNS lifecycle notes](docs/reference/delegations.md).
Organization association tests create a disposable POC, snapshot the original links outside the repository, exercise all five supported roles, and verify the original links after cleanup. The client and Terraform test packages run sequentially to avoid overlapping snapshots. Normal tests and CI skip these sandbox tests.

OT&E account data and API keys are refreshed from production monthly. If the
preflight rejects a recently created production key, generate a key in
[OT&E ARIN Online](https://www.ote.arin.net/) and use `ARIN_OTE_API_KEY`.
See [ARIN's OT&E documentation](https://www.arin.net/reference/tools/testing/).

CI runs on the YYJ self-hosted runners (`runs-on: [self-hosted, yyj]`) inside an Ubuntu 24.04 container. The workflow installs build dependencies and uses the pinned Go and Terraform setup steps. CI uses fake-server tests and does not enable live ARIN writes.
