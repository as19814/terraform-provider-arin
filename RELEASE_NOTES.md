# v0.1.0-alpha.1

First public alpha of the independent ARIN Terraform provider from Foundability Technologies, Inc.

- 75 read-only data sources and 24 managed resources covering ARIN registration, DNS, IRR and supported RPKI operations.
- Import support for existing manageable resources.
- Fake-server acceptance tests, opt-in live and OT&E tests, and recovery handling for uncertain mutations.
- Signed packages for Linux and macOS on AMD64/ARM64, and Windows on AMD64.

This is alpha software. Schemas and Terraform state compatibility may change before 1.0. Pin the exact version, retain state backups and review plans before applying registry changes.

See the [release readiness review](https://github.com/as19814/terraform-provider-arin/blob/main/docs/reference/release-readiness.md) for tested scope and outstanding interoperability limitations. This release does not add support for excluded account-approval features or ticket-message submission.

Original project code is MPL-2.0. This project is not an official ARIN or HashiCorp provider.
