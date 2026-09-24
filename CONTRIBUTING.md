# Contributing

Contributions to original project files are made under MPL-2.0. Third-party
reference material retains its own terms; preserve provenance and notices.

Use Go 1.27.0 or newer and Terraform 1.15.5. Run:

```sh
make check checkschema vuln
go generate ./...
git diff --check
```

Describe the problem, resulting behavior and validation in your pull request.
Document schema changes and state migration requirements. Never silently replay
an uncertain ARIN mutation. Use synthetic fixtures rather than account exports.

Do not commit credentials, .env files, state, saved plans, local CLI configuration,
private keys or recovery journals. To scan all fetched history with Gitleaks 8.30.1:

```sh
gitleaks git . --log-opts="--all --full-history" --redact --ignore-gitleaks-allow
```

CI runs on standard GitHub-hosted Ubuntu runners for main and pull requests.
External contributors may need a maintainer to approve their workflow run.
No ARIN credentials or live writes are available in CI.

Main requires a pull request, current passing checks and resolved conversations.
The project currently has one maintainer, so an independent approval is not
required. Add a reviewer requirement when a second maintainer is available.
