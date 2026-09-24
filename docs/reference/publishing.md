# Publishing the provider

The Registry address is `as19814/arin`. Initial version: `0.1.0-alpha.1`.

## Signing credentials

The GitHub `release` environment holds `GPG_PRIVATE_KEY` and `PASSPHRASE` as
secrets and `GPG_FINGERPRINT` as a variable. Its deployment policy permits only
`v*` tags. PR CI does not use this environment. Never commit private signing
material or copy it into release artifacts.

Register the ASCII-armored public key with the Terraform Registry under the
`as19814` namespace. Keep an independent secure backup of the private key,
passphrase and revocation certificate. The release key uses RSA-4096 with a
two-year expiry; renew or replace it before expiry and update the Registry and
GitHub environment together.

## Release procedure

1. Update RELEASE_NOTES.md, generate documentation, and merge through passing CI.
2. Wait for the main-branch CI run to pass.
3. Tag that main commit, for example `git tag v0.1.0-alpha.1`, then push the tag.
4. Watch the Release workflow. It verifies main ancestry, reruns checks, builds
   five platform archives, validates checksums, signs them in a temporary GPG
   keyring, verifies the signature, and uploads a draft with all assets before
   publishing it. Tags containing a prerelease suffix are marked prerelease.
5. For the first version, sign in to Terraform Registry, add the public signing
   key, and publish the provider from `as19814/terraform-provider-arin`. Review
   and accept the Registry terms. Later GitHub release events trigger ingestion.
6. Verify the Registry documentation and install the exact version with
   `terraform init` in a clean directory with no development overrides. Exercise
   a read-only data source before updating production configurations.

Do not replace assets of a published version. Fixes need a new version and tag.
A rerun fails if a release already exists rather than overwriting it. If uploading
or finalizing a draft fails, inspect its assets and complete the draft manually;
do not delete or rewrite an already-published release.

The Release candidate workflow remains available for unsigned packaging rehearsals.
The release signer refuses incomplete directories, extra files, checksum mismatches,
and keys that do not match the configured fingerprint.

Reference: [HashiCorp publishing requirements](https://developer.hashicorp.com/terraform/registry/providers/publishing).
