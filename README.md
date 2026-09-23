# Terraform Provider for ARIN

A Terraform provider for ARIN, developed by AS19814 using the Terraform Plugin Framework and protocol version 6.

The provider includes 32 read-only data sources spanning registration records, network discovery, DNS delegations, contacts, customers, IRR, hosted RPKI, ASN registrations, and existing tickets. See the [complete catalog](docs/reference/data-sources.md). Managed resources [`arin_irr_as_set`](docs/resources/irr_as_set.md) and [`arin_irr_route`](docs/resources/irr_route.md) support simple IRR AS sets and IPv4/IPv6 routes, with creation, updates, deletion, and import. The repository is private and the provider has not been published to a registry.

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

## Next steps

Extend managed-resource support to additional IRR objects, network metadata, and delegations, with explicit lifecycle semantics and OT&E validation. Network discovery and authenticated detail reads are implemented; network metadata management can build on those models. Registration workflows and tickets need their own lifecycle decisions before being exposed as managed resources.

Start with the [API index](docs/reference/arin-api/README.md), [provider notes](docs/reference/arin-api/PROVIDER-NOTES.md), and [schema findings](docs/reference/arin-api/schemas/README.md).

## OT&E write validation

The AS-set and route lifecycles have a separate opt-in test command:

```sh
ARIN_TEST_ORG_HANDLE=FT-684 make testote
```

Set `ARIN_OTE_API_KEY` in the shell, or let the test fall back to `ARIN_API_KEY`.
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
These tests never mutate an existing account object. Normal tests and CI skip them.

OT&E account data and API keys are refreshed from production monthly. If the
preflight rejects a recently created production key, generate a key in
[OT&E ARIN Online](https://www.ote.arin.net/) and use `ARIN_OTE_API_KEY`.
See [ARIN's OT&E documentation](https://www.arin.net/reference/tools/testing/).
