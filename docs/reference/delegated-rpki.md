# Delegated RPKI and publication audit

These protocol families remain in scope. Local setup-document inspection is
implemented; signed provisioning and publication operations remain unimplemented. An API key alone is
not sufficient to exercise them. No enrollment, deployment switch, CA creation
or certificate/publication mutation was performed during this audit.

## Interfaces and prerequisites

ARIN's [delegated RPKI guide](https://www.arin.net/resources/manage/rpki/options/delegated/)
describes an RFC 8183 identity exchange through ARIN Online: submit a child
request, receive the parent response as a ticket attachment, and configure a CA.
The [publication guide](https://www.arin.net/resources/manage/rpki/options/rps/)
describes a separate publisher request and repository response exchange. These
guides were checked on 2026-09-23; no API-key enrollment endpoint is documented
there. This does not establish that no other enrollment interface exists.

| Protocol | Operations to implement or integrate | Evidence needed for native tests |
| --- | --- | --- |
| RFC 8183 setup | Child/parent and publisher/repository identity exchange | Enrolled sandbox identity and returned XML documents |
| [RFC 6492 Up/Down](https://www.rfc-editor.org/rfc/rfc6492.html) | List resource classes/certificates, issue certificates, revoke certificates | Child signing identity, parent trust material, handles and service URI |
| [RFC 8181 publication](https://www.rfc-editor.org/rfc/rfc8181.html) | List published objects, publish/replace and withdraw objects | Publisher signing identity, repository trust material and assigned publication URI |

The protocol messages use signed CMS envelopes with XML content. They need
separate certificate/signing configuration and response verification; forwarding
the Reg-RWS API key is not an authentication implementation. Terraform ownership
must also account for CA software that continually renews certificates and
publishes manifests and revocation lists. A provider must not compete with an
active CA for the same objects.

## Sandbox implications and remaining work

ARIN explicitly supports delegated RPKI in
[OT&E](https://www.arin.net/reference/tools/testing/), with identity re-exchange
after monthly refreshes. Its instructions require staff assistance to change an
organization's RPKI deployment type. The existing hosted RPKI tests do not prove
delegated protocol coverage, and a deployment switch must not be used as an
implicit prerequisite for unrelated hosted-resource tests.

The session has supplied an OT&E API key, but no delegated signing identity,
parent response or repository response. Native protocol validation therefore
needs additional setup. Preserve this as missing evidence, not an unsupported
API finding. Remaining work includes a concrete Terraform ownership design,
protocol client and CMS verification, fake-server coverage, enrollment evidence
and disposable native lifecycle tests. Existing ticket attachment data sources
may retrieve setup documents after enrollment; that alone does not implement
the protocols.

## Implemented setup-document inspection

`arin_rpki_setup` parses local XML for all four exchange messages from
[RFC 8183](https://www.rfc-editor.org/rfc/rfc8183.html). It exposes handles, URIs,
certificate PEM/fingerprint/dates, publication offers and ordered referrals.
It performs no HTTP requests or enrollment. Input XML and referral tokens are
sensitive in Terraform, but remain in state. Request tags retain the distinction
between absent and explicitly empty values.

The parser bounds XML to 4 MiB and binary fields to 512,000 decoded bytes.
It rejects unexpected namespaces, attributes, ordering and duplicates. BPKI
certificates must have CA properties, matching issuer/subject and a valid
self-signature, without resource extensions. Dates are exposed without a current
validity check, allowing historical documents to be inspected. Self-signature
verification does not authenticate the sender. Referral CMS is opaque and
unverified. No URLs are followed.

Unit tests cover all four types, optional fields, referrals, invalid XML,
certificate failures and expired documents. Terraform acceptance covers all four
types, refresh, clean plans and invalid input. The synthetic CA fixture has no
committed private key. Native ARIN setup-document evidence remains unavailable.

## Ownership plan for remaining protocol work

The setup data source supplies configuration for future signed clients. The next
layer must verify CMS chains, message identity, content type and replay behavior
before returning typed inventory data. Separate provisioning and publication
identities will be explicit configuration, with no API-key fallback.

Managed certificate and publication resources must own only their explicitly
configured keys and object URIs. Existing objects require import. Publication
replacement must compare the current object hash, and uncertain transactions must
reconcile against inventory before another write. No resource may take ownership
of an entire active CA repository by implication. External CA automation must not
manage the same certificates, manifests, CRLs or object URIs concurrently.

Remaining work includes CMS verification/signing, protocol
clients, resource ownership/recovery implementation, fake-server tests and native
enrollment plus disposable lifecycles. The local data source is a prerequisite,
not evidence that signed provisioning or publication is implemented.

## Setup request generation

`arin_rpki_setup_request` generates child and publisher requests from an existing
public BPKI CA certificate. It accepts one PEM certificate, rejects extra PEM
blocks or surrounding content, and reuses the reader's CA/self-signature checks.
It neither generates nor accepts a private key. The CA software must retain the
key corresponding to the certificate for subsequent protocol operations.

Output is deterministic XML with a SHA-256 ID. Tags retain absent/empty semantics
and normalize XML token whitespace. Publisher requests preserve ordered referral
handles and opaque authorization tokens; child requests reject referrals. Tokens
are base64-validated, not CMS-verified. Generated XML is sensitive in Terraform
and retained in state. Generation makes no network requests or enrollment changes.
As with document inspection, current certificate validity is not checked.

Unit tests independently decode generated XML and check identities, certificate
bytes, referral ordering, tag escaping and repeatability. Invalid PEM, tags,
handles, request types and referrals are rejected. Terraform acceptance connects
both request types to `arin_rpki_setup`, verifies decoded output, refreshes a changed
handle, and checks clean plans before and after the change. No native enrollment
request was submitted; ARIN acceptance remains unverified without sandbox setup.

## CMS profile decoder and signature checks

The private `decodeRPKICMS` layer checks the envelope described by
[RFC 6492 section 3.1](https://www.rfc-editor.org/rfc/rfc6492.html#section-3.1).
It requires version-3 SignedData/SignerInfo, one signer identified by the EE
certificate's subject-key identifier, XML content type, certificates and CRLs,
mandatory signed attributes, matching content digest and a valid RSA signature.
It handles signing-time, binary-signing-time, or both with agreement. DER set
ordering, duplicate/unknown fields and unsigned attributes are checked explicitly.
Input is limited to 4 MiB, with at most 32 certificates and 32 CRLs. The initial
algorithm implementation supports SHA-256 and RSA keys from 2048 to 8192 bits;
this is not a claim of native interoperability or complete algorithm coverage.

The return type is explicitly untrusted and private. It does not validate the
certificate trust path, CRL signatures/freshness/revocation, signing-time replay,
or XML sender/recipient semantics. No protocol client consumes its payload yet.
Those checks are required before exposing authenticated content to Terraform.

Tests include independently verified OpenSSL signature/content interoperability,
signed malformed profiles, tampered content/signatures, both time encodings, and
re-signed noncanonical attribute ordering. A fuzz target exercises bounded input.
No live protocol request, identity enrollment or signing-key persistence occurred.

The inspected [digitorus PKCS7 signing implementation](https://github.com/digitorus/pkcs7/blob/master/sign.go)
uses issuer-and-serial signer identifiers. The provider's profile decoder uses
Go standard-library ASN.1, X.509 and RSA primitives instead of introducing that
library and rewriting its signer representation. It remains internal; the trust-validation layer below is also not yet connected
to a protocol client.

## BPKI trust and revocation validation

The private `verifyRPKICMS` wrapper now authenticates a decoded envelope against
one explicit BPKI trust anchor. It uses a fresh certificate pool, with no system
roots or automatically trusted embedded certificates. Embedded and explicitly
configured intermediates may complete the path. Go X.509 checks current path
validity; the wrapper checks signing key usage and rejects resource extensions
anywhere in the BPKI chain.

Every non-root certificate requires a signed, issuer-matching CRL with matching
key identity. The wrapper selects the highest supplied CRL number, rejects number/
time conflicts and conflicting contents at the same number, checks freshness,
and rejects listed serials. Scoped, indirect, delta and unknown critical CRL
semantics are unsupported and cause rejection. Expired newer CRLs cannot cause
fallback to older current lists. No CRL URL is fetched.

Signing time must be at least the caller's last accepted time. A provider policy
also rejects times more than five minutes ahead of the supplied current clock.
Equal times are allowed; this is a monotonic-time check, not complete replay
prevention. Current certificate validity is checked independently of signing time.
The caller must authenticate the setup exchange, validate XML message identities
and response semantics, then persist the accepted timestamp per peer before later
exchanges. Durable timestamp storage is implemented below; protocol integration and
CRL-number history remain outstanding.

Tests cover direct and intermediate chains, configured intermediates, missing
intermediate CRLs, revoked EEs/intermediates, wrong roots, expired anchors and EEs,
key usage, resource extensions, bad CRL signatures, freshness, conflicting CRLs,
unsupported scope/entry semantics, signing-time ordering and future skew. All are
synthetic tests; no native delegated RPKI exchange has occurred. HTTP protocol clients and their stateful recovery remain outstanding; local
signed-message creation is implemented below.

## Local CMS request signing

The private `signRPKICMS` helper now signs bounded XML with an existing EE
certificate and `crypto.Signer`. It checks that the key matches the certificate
and validates the local trust path, CRLs and caller-supplied signing time before
invoking the signer. The EE and optional intermediate certificates plus CRLs are
included in DER-sorted sets. Output uses SHA-256/RSA, a subject-key identifier,
XML content type, content digest and whole-second UTC signing time.

The helper verifies the returned signature and then checks the complete generated
envelope with the receiving verifier. Signer failures yield generic errors without
returning partial output or exposing signer error details. Tests cover distinct
CA and EE keys, intermediate chains, invalid configurations rejected before the
signer is called, clock rollback, malformed XML, and faulty signers. OpenSSL
independently verifies generated content, signature, chain and embedded CRL using
the synthetic anchor and a fixed validation time.

No private key is generated or persisted by the implementation. Tests generate
synthetic keys in memory and write only public certificates and CMS envelopes to
temporary files. The helper is not yet connected to a provider configuration or
HTTP client. Outgoing timestamp persistence, serialized exchanges, message
correlation, recovery and native ARIN protocol evidence remain outstanding.

## Durable exchange journal

The private exchange lease now persists outgoing and accepted incoming signing
times, plus the hash, operation name and time of an unfinished request. It requires
an existing absolute private directory and an opaque 64-character peer hash.
Peer-key derivation and provider configuration remain part of client integration.

One atomically created lock directory serializes cooperating processes using the
same peer and storage directory. State uses mode-0600 files, file fsync, atomic
rename and directory fsync. Begin must succeed before HTTP dispatch. Complete
requires the matching request hash and a nondecreasing received timestamp; the
caller must first authenticate and correlate the response. Completed timestamps
remain after closing and reopening. Keys, certificates and XML are not stored.

Corrupt, noncanonical, oversized, mismatched or overly permissive files are
rejected. A failed save disables the current lease. A crash retains its lock for
explicit reconciliation; removing only that lock does not clear a pending record.
The implementation never automatically deletes stale locks or retries pending
operations. Tests cover separate processes, abrupt exit, reopen behavior, failed
atomic replacement, timestamp rollback, file permissions and state corruption.

This is local coordination, not a distributed lock or tamper-proof audit log.
Deleting journals, losing the directory or choosing a different directory loses
history. It requires filesystem support for directory fsync. Protocol clients
still need to hold the lease across signing, dispatch, verification and durable
completion, and implement explicit recovery for uncertain outcomes. Equal-time
messages, CRL rollback history, response correlation and native ARIN exchanges
remain outside this journal's guarantees. No protocol request has been sent.
