# Security policy

This provider is alpha software. Use the latest reviewed build and consult the
[coverage and limitations](docs/reference/release-readiness.md) before using
write operations. Security fixes target the latest development version; older
alpha builds do not have a separate maintenance commitment.

## Reporting a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/as19814/terraform-provider-arin/security/advisories/new).
Do not report vulnerabilities with exploit details in public issues or PRs.
Never include API keys, private keys, Terraform state, saved plans, recovery
journals or unredacted account responses. Supply a minimal synthetic reproducer.

If a credential is exposed, revoke or rotate it with the issuing service.
Deleting a file or commit does not revoke a credential.

## Development and CI

CI has no ARIN credentials and runs only fake-server tests. Live production
reads and OT&E writes require explicit local opt-in. Never enable live writes
in pull-request CI.

CI uses standard GitHub-hosted Ubuntu runners with read-only repository tokens.
Checkout does not persist credentials. Do not use pull_request_target to execute
contributor code, pass repository secrets to untrusted jobs, or switch fork PR
jobs to internal self-hosted runners. External contributor workflows require
maintainer approval once the repository is public.
