# Terraform Provider for ARIN

A Terraform provider for ARIN, developed by AS19814 using the Terraform Plugin Framework and protocol version 6.

The foundation includes an authenticated Reg-RWS client and the read-only `arin_org` data source. Managed resources are not implemented yet. The repository is private and the provider has not been published to a registry.

## Configuration

Set `ARIN_API_KEY` in your shell. The provider sends it as an authorization header. Your ARIN account must have authority over the requested records.

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
```

`base_url` defaults to `ARIN_BASE_URL`, then production. Set it to `https://reg.ote.arin.net` for OT&E. Explicit provider attributes take precedence over environment variables. See the [provider schema](docs/index.md) and [organization data source](docs/data-sources/org.md).

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
| `make testacc` | Real Terraform against an in-process fake API, including refresh and missing records | None |
| `make testlive` | Real Terraform reading an existing organization | Read-only |
| `make check` | Vet, unit tests, fake acceptance tests, binary build | None |

The fake server remains the default for deterministic tests and CI. Live tests need a separate opt-in, an existing organization handle, and your shell's API key:

```sh
ARIN_TEST_ORG_HANDLE=FT-684 ARIN_BASE_URL=https://reg.arin.net make testlive
```

`make testlive` sets `ARIN_LIVE_TESTS=1` and `TF_ACC=1` and disables test caching. It reads only; it does not create, update, or delete ARIN records. CI receives no ARIN credentials and does not run live tests. No private fixtures or account responses are committed.

## Layout

- `internal/arin/`: HTTP client, XML response models, typed errors, and fake-server unit tests. Independent of Terraform.
- `internal/provider/`: provider configuration, data sources, and Terraform acceptance tests.
- `examples/`: Terraform configuration examples used in generated documentation.
- `docs/`: generated provider and data source documentation.
- `docs/reference/arin-api/`: official API snapshots, source provenance, and implementation research.
- `scripts/`: documentation snapshot tooling.

API requests have deadlines and bounded responses. Redirects are rejected, raw response bodies are excluded from diagnostics, and the API key is redacted from parsed server errors. Retries are intentionally not automatic: individual mutation APIs will need explicit completion and retry semantics.

## Next steps

Build resource support one API family at a time, with import, refresh, update, deletion, and error behavior tested against both a fake server and OT&E. The likely first candidates are IRR objects and RPKI ROAs. Registration workflows and tickets need their own lifecycle decisions before being exposed as managed resources.

Start with the [API index](docs/reference/arin-api/README.md), [provider notes](docs/reference/arin-api/PROVIDER-NOTES.md), and [schema findings](docs/reference/arin-api/schemas/README.md).
