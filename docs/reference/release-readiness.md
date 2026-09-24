# Release readiness

This is an alpha provider. It exposes 24 resources and 75 data
sources after excluding ticket-message submission, WhoWas requests, Bulk Whois
and invalid-POC downloads. No registry release or production mutation is implied
by running CI or packaging a candidate.

## What OT&E proves

The completed lifecycle tests cover networks, recipient customers, POCs and
individual contacts, DNS/DNSSEC, simple and advanced IRR, hosted ROAs/ASPAs and
atomic hosted bundles. They include import, refresh, clean plans and cleanup or
restoration. Public lookup tests also run against production read-only.

The following narrower gaps remain. They are not failures of the already-tested
operations and must not be described as complete native lifecycle coverage:

- Organization creation returned a pending staff-review ticket. Mutable fields,
  Admin replacement and deletion on that disposable organization never completed.
- Ticket tests used automatically closed reports. Neither closing method has
  completed a RESOLVED-to-CLOSED transition against ARIN.
- Delegated RPKI signed provisioning/publication is tested with signed fixtures
  and synthetic Terraform lifecycles, but there is no delegated sandbox enrollment.
  Public repository and certificate-path validation passed independently.
- Asynchronous NET responses, native anchor rotation and lost/corrupt manifest
  history remain subject to the limits in the family-specific recovery audits.

Use the [coverage inventory](implementation-status.md) to distinguish these paths.
OT&E success does not establish production permissions, load behavior or every
possible server response.

## Independent API contract checks

`TestRegRWSUpstreamContracts` reads method/path expectations from the saved ARIN
method guide, then intercepts existing client lifecycle tests before their fake
servers. It checks organization, POC, contact, delegation, network, report and
ticket-closing traffic. Negative cases reject incorrect write verbs. This prevents
those fake handlers and the client from silently agreeing on an undocumented verb.
The guide's optional phone selectors and escaped path values are accounted for.

This gate checks methods and paths, not every XML field, query parameter or the
signed delegated protocols. Payload tests, upstream schema review and native
lifecycle tests remain necessary. It does not make all mocks authoritative.

## Schema and state compatibility

`make checkschema` compares the actual Terraform protocol schema with
`testdata/provider-schema.json`, excluding descriptions. Attribute types,
required/optional/computed flags, sensitivity and schema versions are included.
Any structural change fails CI until explicitly reviewed and accepted using
`python3 scripts/check-schema.py --update` after rebuilding.

This is a change-detection baseline, not proof that arbitrary historical state
can be upgraded. Before a release that changes an existing resource's stored
shape, add fixture-based state upgrade tests and an explicit migration path.
The first candidate has no published predecessor. The removed pre-release
features are breaking changes for development configurations; see the README.
Never discard an uncertain-write receipt merely to make a new version apply.

## Security and destructive behavior

CI runs a pinned `govulncheck` against the current vulnerability database. The
September 24 audit found reachable advisories in gRPC, x/net and x/text; dependency
versions were updated and Go builds aligned to 1.27.0. A passing scan only covers
known vulnerabilities in the analyzed call graph.

Existing tests cover credential redaction, redirect rejection, mutation replay
prevention, uncertain-write recovery and preservation of unrelated objects.
Sensitive Terraform attributes still reside in state. Store state and private
RPKI keys/journals in access-controlled locations; do not publish test fixtures
containing account data. Resource destroy behavior varies: some deletes affect
ARIN, while report receipts and ticket-status management only leave local state.
Review the resource documentation and saved plan before applying a candidate.

## Candidate packaging

Run `make check checkschema vuln`, then generate docs and verify a clean diff.
`make package VERSION=0.1.0-rc.1` creates Linux/macOS amd64/arm64 and Windows amd64
ZIPs, a protocol-6 manifest and SHA-256 checksums under `dist/`. The packager rejects
invalid versions and existing output directories, uses trimmed build paths and
fixed ZIP timestamps, and marks incomplete builds. It does not sign or publish.
Cross-compilation verifies builds, not runtime behavior on every operating system.

The manually dispatched **Release candidate** workflow runs on
standard GitHub-hosted `ubuntu-24.04` runners, checks the candidate and uploads workflow artifacts (visibility follows the repository).
It has no repository write permission and does not create tags or releases.
For a private pilot, verify archive checksums and install through Terraform's
filesystem mirror or an explicit development override. Public registry delivery,
cross-platform runtime tests remain separate release work. Signed delivery is
implemented by the [release workflow](publishing.md).


## Alpha compatibility and public readiness

Original project code is MPL-2.0, with upstream materials excluded as described
in NOTICE. Before 1.0, schema and behavior can change between releases. Pin the
provider version, retain state backups, and review migration notes before an
upgrade. Uncertain-operation state and recovery journals must be preserved.

The September 24 production pilot exercised import of existing resources,
creation of 428 networks, network public-comment updates, expansion of one
hosted ROA to 479 exact prefixes with automatic IRR links, and AUT-NUM creation.
Post-apply plans were clean. RIPE's public validator confirmed all 479 prefix
and origin authorizations. This evidence does not close the other live-test
limitations listed above.

History scanning uses Gitleaks with default rules plus an ARIN-key rule. Narrow
exceptions cover only publicly documented sample keys in specific upstream
files and XML property paths. No real credentials were found in the full-history
Gitleaks/TruffleHog audit or exact comparisons with local keys. No history rewrite
was needed for secrets.

## Repository publication checklist

The repository is public. Main requires
an up-to-date passing `test` check, a pull request, resolved conversations and linear
history, including for administrators. Force pushes and deletion are blocked. Release
tags matching `v*` cannot be rewritten or deleted. Independent review is not required
while there is only one maintainer; enable it when a second maintainer joins.

Secret scanning and push protection are enabled. Actions use read-only tokens and
pinned action revisions. Standard GitHub-hosted Ubuntu runners keep contributor code
off the organization's shared runners. No live ARIN credentials are needed by CI.

External contributor approval and private vulnerability reporting were enabled
after publication. To reapply and verify these settings, run:

```sh
python3 scripts/enable-public-security.py
```

This enables approval for all external contributors' workflow runs and private
vulnerability reporting, then verifies both settings. It refuses to run on a private
repository and never changes repository visibility. Source publication does not
publish binaries or a Terraform Registry release. Follow the
[publishing procedure](publishing.md) for signed releases.
