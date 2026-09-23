# Delegated RPKI and publication audit

These protocol families remain in scope. Local setup-document inspection is
implemented, along with private signed publication and revocation clients tested
against fake servers. Terraform protocol integration remains incomplete. An API key alone is
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

## Signed HTTP exchange integration

The private HTTP exchange layer now holds the journal lease across signing,
POST dispatch, CMS verification, protocol validation and durable completion.
Peer history is bound to endpoint, protocol media type, caller-provided peer
scope and local/remote CA fingerprints. EE renewal does not reset that identity.
The caller must provide the existing private journal directory and trusted setup
configuration. HTTPS is required except for loopback test servers.

The layer posts CMS with the up-down or publication media type, without API-key
headers. Redirects, body replay and automatic application retries are disabled.
It checks status, content type and encoding, bounds responses to 4 MiB, verifies
CMS against the configured peer and stored timestamp, then invokes the required
protocol validator. Only successful validation and journal completion return XML.
Errors do not echo remote bodies, URLs or validator details. A post-dispatch
failure retains pending state and blocks another POST.

Signed fake-server tests cover both media types, journal-before-dispatch ordering,
held locks, response watermark persistence and rollback, lost/truncated/oversized
responses, redirects, HTTP/media/encoding failures, bad signatures, wrong peers,
correlation failures, cancellation and retry prevention. Test XML is synthetic;
these tests establish the shared transport, not either protocol's message schema.

Typed protocol clients must still build/validate messages, interpret authenticated
error replies, and implement recovery. An authenticated protocol error can complete
an exchange only after its identity and correlation are verified; arbitrary
validator errors retain pending state. No Terraform schema exposes this transport
and no native ARIN protocol exchange has occurred.

## Publication inventory client

The private RFC 8181 client now sends signed version-4 list queries through the
durable HTTP exchange layer. It validates reply namespaces, attributes, message
type, SHA-256 hashes and rsync object URIs, rejects duplicate object URIs and
mixed reply categories, and returns an empty collection for an empty inventory.
Hashes are normalized to lowercase; object URIs remain unchanged.

Authenticated protocol errors are validated before completing the exchange.
Only defined error codes are returned to callers; remote diagnostic text and
echoed PDUs are excluded from errors. If a failed PDU is supplied, it must match
the empty list request. Malformed replies retain pending state and block another
request. Signed fake-server tests exercise inventory, empty inventory, rejected
requests, malformed replies, subsequent calls and persisted journal state.

RFC 8181 list requests have no tag or nonce. This client relies on peer CMS
authentication, serialized exchanges and the persisted signing-time watermark;
it cannot distinguish replayed list replies with equal signing times. No claim
of unique request-response correlation is made. Native ARIN validation still
requires delegated enrollment and an existing BPKI identity. Explicit recovery and Terraform integration remain unimplemented.

## Publication mutation batches

The private publication client now builds one signed request containing a batch
of publish and withdraw operations. Creation omits an old hash; replacement and
withdrawal use the caller's SHA-256 precondition. It rejects invalid URIs/hashes,
empty publish bodies, duplicate target URIs and oversized batches before dispatch.
Random per-batch tags identify individual operations in error replies. The caller
must supply properly signed DER objects and the updated manifest; this layer
transports opaque object bytes and does not create or validate RPKI objects.

A single empty success PDU completes a batch. Authenticated error replies accept
only known codes and matching operation tags when supplied; optional failed PDUs
must match the requested operation, attributes and object bytes. Untagged generic
errors are accepted without an echoed mutation. Invalid or mixed replies retain
the pending journal. Success replies have no tag or nonce, so the same equal-time
replay limitation as inventory applies. No retries are performed automatically.

Signed fake-repository tests cover creation, replacement, withdrawal, stale-hash
rejection, atomic rollback of a staged multi-operation request, durable completion,
and a malformed reply after mutation that blocks a second POST. These prove client
behavior against the fake server, not native ARIN atomicity or real RPKI object
acceptance. Provider resources, explicit recovery and native testing remain.

## Delegated certificate revocation client

The private RFC 6492 client now sends signed revocation requests with the child
and parent handles, resource class and URL-safe SHA-1 key identifier. Peer journal
scope is derived from both handles. Replies must match sender, recipient, message
version/type, resource class and key. Both padded and unpadded canonical URL-safe
key encodings are accepted. XML token whitespace is normalized for labels.

Validated request-not-performed statuses complete an exchange and expose only the
numeric code, excluding server descriptions. Already-processing (1101), scheduled
(1104), unknown statuses, malformed replies and mismatched targets retain pending
state. This intentionally requires explicit reconciliation rather than retrying
an outcome that may still be in progress. Optional descriptions are checked for
XML language attributes, ordering and the required English description.

Signed fake-server tests verify request contents, successful replies, confirmed
rejection, wrong-key replies, scheduled outcomes, journal completion and retry
prevention. Parser tests cover identity, namespace, type, class, key and error
validation. No native certificate has been revoked. Up-down list/issue clients,
Terraform lifecycle integration and explicit uncertain-outcome recovery remain.

## Delegated resource-class inventory

The private up-down client now sends signed list requests and parses resource
classes, ordered certificate URL lists, ASN/IPv4/IPv6 resource sets, expiry times,
suggested publication directories, issuer certificates and issued certificates.
Absent requested-resource attributes remain distinct from explicitly empty sets.
No certificate URLs are fetched. Peer scope is shared with revocation for the
same child/parent pair, so both operations use the same durable history.

Validation checks identities, namespaces, attributes, class uniqueness, element
order, required fields, certificate URL lists containing rsync, UTC expiry times,
and bounded certificate encodings. Resource sets must contain valid, ordered,
non-overlapping ranges or network prefixes of the correct family. Certificates
must parse as CA certificates. This is structural inspection of authenticated
inventory, not RPKI path validation, revocation checking or proof that certificate
resource extensions agree with the listed entitlements. Those checks remain
necessary before implementing certificate issuance and lifecycle decisions.

Signed fake-server tests cover populated and empty inventories, authenticated
errors, invalid replies, durable completion and retry prevention. Unit tests
cover field preservation, optional attributes, resource ranges and malformed
classes. Up-down list messages have no nonce, so equal-time replay remains a
protocol limitation. No native delegated list request has been made. Issuance,
Terraform integration and uncertain-outcome recovery remain unfinished.

## Delegated certificate issuance client

The private up-down client now sends bounded, caller-supplied PKCS#10 requests
with optional requested-resource attributes. CSR parsing and proof-of-possession
signature checks happen before dispatch. Explicitly empty attributes remain
present; omitted attributes remain absent. The provider does not generate the
resource key or CSR. Full RFC 6487 request-profile validation remains outstanding.

Authenticated issuance replies must contain one matching resource class and one
certificate whose public key matches the CSR. Requested-resource attributes must
match both presence and value. The certificate must have a valid signature from
the supplied issuer, matching issuer identity/key identifier, and both certificates
must be currently valid. The existing strict class parser validates the surrounding
XML. Confirmed protocol errors complete the exchange; scheduled, malformed and
mismatched replies retain pending state. No automatic issuance retries occur.

These checks authenticate the parent's reply and correlate the issued key. They
do not establish a resource-certificate path to an RPKI trust anchor, check resource
containment, enforce the complete certificate profile, or validate the requested
publication extensions. Native enrollment, complete certificate validation,
Terraform integration and explicit asynchronous recovery remain required.

Tests use synthetic CSRs and certificates, not complete RPKI CA profiles. Signed
fake-server cases cover issuance, confirmed errors, wrong keys, scheduled outcomes,
journal persistence and retry prevention. Unit cases additionally cover CSR
signature failures, input limits, echoed resource attributes, certificate dates
and invalid certificate signatures. No native certificate has been requested.

## Requested publication locations

Issuance now requires a non-critical CA Subject Information Access extension in
the CSR. Its DER is bounded and parsed strictly; URI access descriptions must
include an rsync repository directory and rsync manifest object. Additional URI
access descriptions are preserved in their original order and are never fetched.
The issued certificate must contain the exact requested SIA DER, also non-critical.
Missing, changed or malformed publication locations leave an issuance exchange
pending rather than accepting a certificate for another publication location.

Tests cover absent/duplicate/critical SIA extensions, malformed DER, invalid URI
forms, missing mandatory access methods, preflight rejection and freshly signed
certificates with missing or changed SIA. Existing issuance fixtures now include
SIA in both CSR and certificate. Complete CSR attribute/extension and algorithm
profile checks, RPKI certificate/path/resource validation and native evidence
remain outstanding. This increment checks publication locations, not the entire
RFC 6487 profile.

## CA request profile preflight

The private issuance client now checks PKCS#10 version zero, RSA-2048 with
exponent 65537, SHA-256/PKCS#1 v1.5 signatures and the required RSA algorithm
parameters. It accepts exactly one extensionRequest attribute and rejects other
attributes, duplicate extensions and extensions outside the RFC 6487 request
allowlist. CA Basic Constraints must be critical and true, without a path-length
constraint. Optional Key Usage is restricted to certificate/CRL signing; optional
Extended Key Usage must be a nonempty, well-formed OID sequence. Required SIA
validation and preservation remain enforced.

Tests sign actual requests covering valid inputs, optional Key Usage, disallowed
attributes and extensions, weak/non-RSA keys, RSA-PSS/SHA-384 signatures, CA/EE
constraints and path-length rejection. Request DER is checked for unconsumed and
noncanonical fields. Existing issuance and SIA tests use the stricter preflight.

Subject-name reuse remains governed by the parent's issuance policy. These checks
do not assert that an enrolled ARIN parent accepts every optional request field.
Native CSR interoperability, complete issued-certificate profile/path/resource
validation, asynchronous recovery and Terraform integration remain unfinished.

## Resource-extension decoding

A private decoder now reads the original RFC 3779 IP and AS extension OIDs using
the RFC 6487 resource-certificate constraints. It preserves the distinction among
absent, inherited and explicitly allocated resources. Explicit sets become bounded
AS-number intervals and IPv4/IPv6 address intervals, including zero-length prefixes
covering an entire address family.

The decoder rejects duplicate or non-critical resource extensions, empty explicit
sets, unsupported families, SAFI/RDI fields, incorrect family ordering, oversized
AS numbers, inverted/overlapping/adjacent intervals and noncanonical range forms.
IP range bounds use RFC 3779 zero/one suffix expansion and require trimmed endpoint
encodings; ranges representable by a single prefix are rejected. Tests cover
valid AS/IP examples, inheritance, malformed sets and full-family boundaries.

This helper is not yet used to accept an issued certificate. It does not resolve
inheritance, check containment against an issuer or requested allocation, or
establish a trust path. It handles the original extension OIDs only; newer resource
validation profiles and unknown critical-extension handling need a separate audit
before claiming complete certificate validation. Native interoperability remains
unverified.

## Resource containment and inheritance

The private resource-path resolver now decodes a bounded leaf-to-anchor sequence
of extension sets and resolves inheritance from the anchor downward. The anchor
must contain explicit resources. Each child range must fit entirely within one
issuer range, so authorization cannot bridge a gap. An absent family stays absent,
and a later descendant cannot inherit a family that an intermediate dropped.
The returned effective sets own their storage rather than aliasing parent ranges.

Unit tests cover AS/IPv4/IPv6 inheritance, explicit subsets and equal sets, missing
families, overclaims, authorization gaps, invalid intermediate encodings, inherited
anchor resources and the 32-certificate path bound. These are resource-set tests,
not authenticated certificate-path tests. The caller must separately validate
signatures, profiles, revocation and the chosen trust anchor before using the
result. Issuance integration, comparison with requested/advised resource sets,
full path verification and native evidence remain unfinished.

## Authenticated resource-path primitive

A private verifier now combines an exact, leaf-first CA certificate path with an
explicitly configured resource trust anchor, caller-supplied CRLs and a validation
time. It reparses all DER, bounds inputs, validates PKIX signatures and dates,
checks issuer identities and key identifiers, checks current CRL status, and then
resolves resource inheritance and containment for that same path. It uses no
system roots and fetches no URLs. Alternative paths cannot silently replace the
supplied path.

Only successfully decoded original-profile resource extensions are removed from
the parsed copies' unhandled-critical list before PKIX verification. Unknown
critical extensions still fail. Caller-owned certificate objects are unchanged.
Signed three-level test chains cover successful inheritance, wrong anchors,
missing/reordered issuers, expired paths, revoked leaf/intermediate certificates,
missing CRLs, invalid signatures, forged parsed fields and signed overclaims.

This primitive is not yet connected to issuance acceptance. It is not complete
RFC 6487 validation: certificate/CRL profile rules, resource-anchor provisioning,
newer extension profiles, requested-allocation comparison and authenticated
manifest selection/rollback protection remain to be addressed. CRL profile
checks and RFC 9829 selection behavior are described below.
Terraform integration, recovery and native delegated evidence are still pending.

## Resource CRL profile checks

The resource-path verifier now applies additional CRL profile checks after DER
parsing. It requires version 2, SHA-256/RSA PKCS#1 v1.5, matching inner/outer
algorithm identifiers with NULL parameters, and exactly the non-critical AKI and
CRL-number extensions. The AKI must contain only a 20-byte key identifier. Unknown,
scoped and delta extensions are rejected, as are all entry extensions, duplicate
serials, invalid serial bounds and revocation dates after the CRL's issue time.
Signature, issuer and currentness checks remain in the revocation verification
step. Resource CRL selection follows the RFC 9829 correction below.

Tests create and sign CRLs with unknown non-critical extensions, delta/scoping
fields, entry extensions/reason codes, duplicate entries, future revocation dates
and alternate signature algorithms. Both the profile checker and the integrated
resource-path verifier reject these; ordinary empty CRLs and extension-free
revocation entries pass. The shared BPKI CMS validator keeps its separate policy.

This does not complete certificate profile validation, manifest rollback
protection, issuance integration, or native interoperability. Full DER time/name
profile auditing also remains before claiming every RPKI profile rule is covered.

## RFC 9829 CRL selection correction

The standards audit identified [RFC 9829](https://datatracker.ietf.org/doc/html/rfc9829),
which updates resource CRL selection. The resource-path verifier no longer reuses
the BPKI helper that ranks CRLs by number. Resource CRL numbers are checked for
valid syntax/range only. Multiple distinct signed CRLs for the same issuer now
fail as ambiguous, regardless of their relative numbers. Repeated identical DER
is harmless. The BPKI CMS revocation policy is unchanged.

The resource-path caller must provide the CRL selected from the issuer's
validated current manifest and matching the certificate's CRLDP. Manifest
validation and selection are still unimplemented, so this remains a private
verification primitive, not a complete RPKI relying-party validator. No CRL is
chosen merely because it has a larger number.

Tests cover zero and maximum permitted CRL numbers, conflicting candidates in
both orders, identical copies and revocation in a low-number candidate. Both
helper and path-level cases pass. Name/time profile checks remain outstanding;
this correction took priority when the updated standard was discovered.

## Resource names and DER time encodings

Resource-path checks now require issuer and subject names with one PrintableString
CommonName and at most one PrintableString serialNumber. Duplicate, extra,
empty, oversized or improperly encoded attributes and unsorted RDN sets fail.
Certificates must use the version-3 field layout without unique identifiers.
Certificate validity, CRL update times and revoked-entry dates must use UTC,
whole-second encodings with UTCTime through 2049 and GeneralizedTime from 2050.
Offsets, missing seconds, fractional seconds and invalid calendar values fail.

The checks inspect raw DER and are integrated into certificate-path and CRL
profile validation. Tests cover separate and combined name RDNs, string types,
attribute restrictions, year boundaries and malformed times. A freshly signed
certificate with an extra organization attribute is rejected at path level.
Existing signed path and CRL tests also pass.

Remaining work includes certificate extension/algorithm profile completion,
authenticated manifest selection, allocation matching, issuance and Terraform
integration, uncertain-outcome recovery and native delegated verification.

## CA certificate extension and algorithm checks

The resource-path verifier now checks RSA-2048/exponent 65537 and SHA-256/RSA
PKCS#1 v1.5 algorithm identifiers, serial bounds, CA Basic Constraints without a
path-length constraint, exact CA Key Usage, and the SHA-1-derived Subject Key
Identifier. Authority Key Identifiers must use only the key identifier form.
Unknown, duplicate and forbidden extensions fail, including CA Extended Key Usage.

Mandatory SIA, resource and policy extensions are checked. The policy supports
the optional single CPS URI qualifier allowed by RFC 7318. Non-self-signed
certificates require AIA and CRLDP references with rsync object locations; the
CRLDP must identify one complete issuer-wide distribution point. Self-signed
certificates omit AIA and CRLDP. No referenced URI is fetched.

Signed certificate tests cover wrong/missing identifiers, policies and locations,
forbidden extensions, extra key usages, path-length constraints and alternate
signature algorithms, through both the profile checker and path verifier. Tests
also cover permitted and rejected policy qualifiers and issuer-reference forms.
The path fixtures now contain the required certificate extensions.

Authenticated manifest/CRL selection, requested-resource matching, updated-profile
interoperability auditing, issuance integration and Terraform integration remain
unfinished. These CA-only checks do not implement manifest EE validation or prove
native ARIN interoperability.

## Manifest content and file-set checks

The private manifest decoder implements bounded DER eContent parsing from
[RFC 9286](https://www.rfc-editor.org/rfc/rfc9286.html), including the omitted
version-zero default, nonnegative manifest numbers up to 159 bits, whole-second
UTC GeneralizedTime, ordered update times, SHA-256 hashes, and unique filenames.
It accepts at most 4 MiB and 10,000 entries. Filename extensions follow the
[IANA RPKI Repository Name Schemes](https://www.iana.org/assignments/rpki)
registry checked on 2026-09-23, including temporary registrations. Recognition
of an extension does not implement validation of that object's payload.

A separate file-set check requires a current manifest, every listed file with
its exact digest, and exactly one CRL. Names retain their case. Unlisted files
are ignored and cannot be treated as manifest-listed objects. The parser allows
an empty list syntactically, but it cannot pass the file-set check without a CRL.
Tests exercise malformed encodings, integer/time/hash bounds, duplicate names,
path-like names, missing or changed files, and freshness boundaries.

These helpers do not authenticate CMS or EE certificates and are not yet called
by issuance or Terraform operations. Persistent rollback checks, authenticated
manifest/CRL selection and delegated native verification remain unfinished.

## Manifest CMS signature checks

The manifest decoder now verifies the RFC 6488 CMS envelope and RSA/SHA-256
signature before parsing its eContent. The shared CMS implementation keeps the
provisioning-message rules separate: manifests require one EE certificate,
omit embedded CRLs, identify manifest content in both CMS locations, and allow
signing-time attributes to be absent. Optional signing times do not determine
manifest freshness or impose the provisioning protocol's time-agreement rule.
Manifest signing keys require RSA-2048 with exponent 65537 under
[RFC 7935](https://www.rfc-editor.org/rfc/rfc7935.html).

Signed fixtures test accepted attributes and legacy signature identifiers,
profile separation, extra certificates/CRLs, content-type mismatches, digest
failures and signature tampering. OpenSSL independently verifies a fixture's
CMS signature and payload. Existing provisioning CMS tests still pass.

The decoder returns an explicitly untrusted manifest: a valid signature alone
does not authenticate the EE or its issuer. EE extension/path validation, SIA
binding, revocation, persistent rollback protection and provider integration
remain required before these objects can authorize issuance or repository use.

## Manifest EE certificate profile

Manifest decoding now requires an EE certificate that conforms to the original
resource-extension profile before returning its untrusted contents. Shared
certificate checks cover names, DER times, algorithms, identifiers, policies,
and issuer references. The EE profile forbids Basic Constraints and EKU,
requires digitalSignature as the only key usage, and requires every present
AS/IPv4/IPv6 resource family to use inheritance. EE SIA accepts only the
signedObject access method, including an rsync object URI; it preserves ordered
alternate locations for later binding to the retrieved manifest URI.

Signed tests cover valid inherited AS and IP resources, invalid EE extensions,
explicit resource sets, missing issuer references, wrong key identifiers and
SIA forms. Invalid signed EE certificates are rejected both directly and
through the CMS manifest decoder. CA profile tests and OpenSSL CMS verification
remain part of the regression coverage.

This implements the certificate profile, not an authenticated resource path.
Issuer trust, inheritance resolution against that issuer, revocation, exact
publication-location binding, persistent rollback checks and provider/native
integration remain unfinished. No native delegated requests were sent.

## Manifest checks against a supplied issuer

A private combined check now reparses the supplied CA DER, verifies the manifest
CMS and EE profile, and binds the EE signature, issuer name and AKI to that CA.
It checks current certificate validity, inherited-family presence, matching
CA repository/manifest SIA and EE signedObject locations, and every listed file.
It selects the single listed CRL by hash, matches its URI to the EE CRLDP, and
checks the CRL signature/profile/currentness and EE revocation status. Unlisted
CRLs do not participate in selection. Manifest self-reference is rejected.

Inputs are bounded to 4 MiB for CMS, 512,000 bytes for issuer DER, 4 MiB per
listed object and 64 MiB for the listed file set. Returned CRL DER owns its data.
Signed tests cover valid selection, tampered/missing files, wrong issuers and
locations, revoked or noncurrent EEs, stale/wrong-issuer CRLs and missing inherited
families. Different manifest and EE validity intervals are not rejected solely
for differing, provided each is current.

The result is checked against a supplied issuer, not an independently trusted
anchor. The caller still needs to authenticate that issuer's full resource path,
resolve its inherited resources and enforce persistent manifest rollback checks.
This helper is not yet connected to issuance or Terraform operations.

## Manifest-backed resource path checks

The private path verifier now accepts one publication snapshot for each
non-anchor CA certificate and walks from the configured anchor toward the leaf.
An issuer's exact path is verified before its manifest can authorize the next
child. The child must be present in the manifest at its supplied publication URI
with byte-identical certificate DER, and its CRLDP must match the selected CRL.
Existing PKIX, profile, revocation and resource-containment checks then verify
that extended path. Each step resolves resources before proceeding downward.

Signed three-level tests exercise valid inherited resources, wrong anchors,
missing/swapped manifests, wrong locations, altered or unlisted child files,
revoked children, resource overclaims and forged parsed certificate fields.
The aggregate publication input is bounded to 128 MiB; existing per-object,
manifest, path-length and file-count limits still apply.

This connects supplied publication snapshots to an explicit resource anchor.
It does not fetch repositories, persist manifest rollback history, perform
uncertain-outcome recovery or connect validation to issuance/Terraform yet.

## Persistent manifest version history

The private durable path-validation entry point now holds an exclusive
filesystem lock while reading history, verifying the full manifest-backed path,
and atomically recording all accepted versions. History is scoped to the exact
configured anchor DER; entries are keyed by issuer public key and manifest URI.
An identical manifest can be reused while current. A replacement must increase
both its manifest number and thisUpdate; equal-number changes and rollback fail.

History uses bounded, canonical JSON in a private directory with mode-0600
atomic replacement and file/directory fsync. Invalid paths or rejected versions
do not partially advance history. Tests cover reopen/reuse, increasing versions,
rollback, conflicting versions, incomplete paths, existing locks, corruption,
symlinks and permissions. A process crash leaves a lock requiring recovery.

The directory must persist across runs. Removing it discards local history;
changing the configured anchor creates a separate history scope. Anchor rotation,
crash recovery, repository retrieval and issuance/Terraform integration remain
unfinished. This storage does not add native delegated sandbox evidence.

## Issuance resource-path gate

The private IssueWithResourcePath entry point now preflights the configured
resource anchor and private history directory, then retrieves publication
snapshots through a caller-supplied resolver after a protocol-valid issuance
reply. The returned certificate and immediate issuer must match the exact DER
at the start of the supplied path, and the selected certificate publication URI
must be present in the signed reply. Manifest-backed path validation and durable
history recording must succeed before the exchange journal is completed.

Resolver inputs own their buffers. Retrieval errors, mismatched paths, missing
publications and validation failures leave the issue exchange pending, so a
second call cannot automatically submit the mutation again. Signed fake-server
tests cover successful acceptance, failure retention, retry prevention, invalid
configuration before dispatch and resolver-buffer isolation. Validation uses a
fresh clock reading after snapshot retrieval.

This gate does not yet compare resolved resources to the requested allocation.
The existing private Issue method remains protocol-only. Provider integration
must use the complete validation flow after allocation matching is implemented.
The repository resolver, pending-operation recovery and native delegated
verification are also still unfinished.

## Issued allocation matching

The resource-path issuance entry point now compares resolved certificate
resources with the signed resource class, bounded by each echoed request set.
Omitted requests select the whole family allocation; explicit empty requests
select none. Supplied request ranges are intersected with the allocated ranges.
Equivalent adjacent ranges and IP prefix/range representations are normalized
before exact comparison. This follows the request semantics in
[RFC 6492 section 3.4.1](https://datatracker.ietf.org/doc/html/rfc6492#section-3.4.1).

Comparison runs after path/resource resolution and before manifest history is
committed. Overclaims, missing expected resources and unresolved inheritance
fail. AS/IPv4/IPv6 tests cover empty/omitted requests, intersections, gaps,
adjacent ranges and address-space boundaries. Signed fake-server tests verify
that allocation/request mismatches retain the pending issue operation, prevent
automatic resubmission and leave manifest history unchanged.

Repository retrieval, uncertain-outcome recovery, anchor rotation, native
interoperability and Terraform integration remain unfinished. The protocol-only
private Issue helper does not perform these resource-path/allocation checks.

## RRDP snapshot decoding

The private RRDP snapshot parser verifies the expected SHA-256 digest, session
UUID and serial, then streams XML objects into a URI-keyed map. It checks the
RRDP namespace/version, snapshot-only operations, required attributes, ASCII
input, rsync object URIs and strict Base64. Duplicate URIs, nested payloads,
DTDs, multiple roots, extra text and malformed input fail.

Limits are 128 MiB of snapshot XML, 10,000 objects, 4 MiB per decoded object and
64 MiB of decoded object data. Serial values use a positive decimal form bounded
to 128 digits. Tests cover empty snapshots, US-ASCII declarations, whitespace,
identity/hash mismatches, malformed XML/Base64 and object/count boundaries.

These are transport checks from [RFC 8182](https://www.rfc-editor.org/rfc/rfc8182.html),
not RPKI authentication. Notification fetching/parsing, delta/cache processing,
persistent RRDP session tracking and resolver integration remain unfinished.
The existing manifest/resource-path checks are still required before accepting
any retrieved object. No native RRDP repository was fetched in this increment.

## RRDP notification decoding

The private notification parser now validates session/version/serial metadata,
exactly one snapshot reference and any advertised delta sequence. It accepts
unordered deltas, sorts them numerically, and requires a unique contiguous
sequence ending at the advertised current serial. Arithmetic supports the full
configured 128-digit bound without uint64 truncation. File references require
HTTPS URLs and exactly 32 decoded SHA-256 bytes; mixed-case hex is accepted.

Notifications are bounded to 4 MiB and 10,000 delta references. Both RRDP parsers
share ASCII XML decoding. Tests cover large serials, duplicate/gapped/future
sequences, malformed attributes, hashes, locations and documents, plus passing
a notification's identity/hash directly into snapshot decoding.

HTTP retrieval, conditional polling, delta application, persistent cache/session
tracking and issuance resolver wiring remain unfinished. Notification checks do
not authenticate the RPKI objects referenced by the repository.

## RRDP HTTPS retrieval

The private repository HTTP client now fetches and parses notifications and
referenced snapshots. Requests carry no ARIN API key or cookie state. It uses
HTTPS with normal transport certificate verification, bounded response reads,
a configurable timeout (30 seconds by default, at most five minutes), and redirect
chains limited to five requests whose destinations must remain HTTPS.

Notification retrieval supports Last-Modified/If-Modified-Since. A solicited
304 returns an explicit unchanged result without replacing cached metadata;
an unsolicited 304 fails. Snapshot downloads pass through the existing digest,
identity and bounded XML checks. HTTP/transport failures return generic errors
without response bodies or URLs.

TLS fake-server tests cover successful retrieval, hash mismatches, conditional
requests, unknown TLS trust, redirects, HTTPS downgrades, oversized declared and
streamed bodies, cancellation, timeouts and error privacy. The client currently
requires successful TLS verification; fallback behavior has not been added.

Persistent cache/session tracking, poll scheduling, delta processing, repository
resolver wiring and native retrieval evidence remain unfinished. Retrieved
objects still require manifest/resource-path authentication before issuance
acceptance.

## RRDP delta application

The bounded object-file decoder now supports delta publish/withdraw operations
alongside snapshots. Delta application verifies the referenced file hash,
notification URI, session and exact next serial before applying changes to a
new repository map. New publication requires absence; replacement and withdrawal
require an existing object with the supplied SHA-256 hash. Objects are scoped to
one repository, and base maps/buffers remain unchanged even if a later operation
fails. Successful results own their byte buffers.

HTTPS delta retrieval now feeds this application step. Tests exercise creation,
replacement, withdrawal, stale/missing preconditions, malformed payloads,
repository/session mismatches, skipped/repeated serials, partial-update rejection
and buffer isolation. Snapshot tests continue to enforce snapshot-only syntax
and duplicate-object rejection through the shared decoder.

Persistent repository storage, notification polling, delta-chain orchestration
with snapshot fallback and issuance resolver wiring remain unfinished. These
transport maps still require RPKI manifest/path validation before use.

## RRDP refresh coordination

The private refresh coordinator now chooses between cached data, a complete
ordered delta chain and a snapshot. New sessions require a snapshot; missing
chains or rejected deltas trigger snapshot fallback. A failed refresh retains
the previous repository, even after an earlier delta in the chain succeeded.
Serial rollback and conflicting snapshot references at an unchanged serial fail.

Conditional 304 responses and unchanged metadata avoid object downloads. A
one-minute polling interval and five-minute aggregate refresh timeout bound
requests. Retrieval attempts return an updated LastAttempt even on failure so
the eventual persistent cache can retain the polling limit. Returned object
maps own their buffers.

TLS tests cover initial retrieval, delta preference, missing-chain selection,
fallback, failed fallback, session changes, conditional responses, unchanged and
conflicting serials, rollback, polling and retention of prior data. Persistent
storage and concurrent-process coordination are still needed; callers must not
discard attempt metadata on retrieval failure. Issuance resolver wiring and
native repository verification also remain unfinished.


## Persistent RRDP cache

The private refresh client now stores repository objects, snapshot references,
conditional-request metadata and polling attempts in a private local directory.
Each notification URI has its own cache and exclusive process lock. Attempts
are saved before dispatch, so failed requests and restarts retain the one-minute
polling interval. Successful replacements and retained data on failure are saved
with atomic rename and file/directory synchronization.

Cache records use bounded, versioned JSON. Invalid records, mismatched repository
identity, symbolic links and permissive file permissions fail before HTTP.
Crashes leave the lock in place for explicit recovery; automated lock recovery
is not implemented. The cache does not authenticate repository objects, which
still require manifest and resource-path verification.

TLS tests cover restart, conditional retrieval, failed refresh retention,
first-attempt failure and concurrent refresh rejection. Corruption, schema,
identity, permissions and stale-lock tests exercise fail-closed behavior.
Issuance resolver wiring, recovery, Terraform integration and native delegated
verification remain unfinished.


## RRDP publication retrieval for supplied paths

The private RRDP client now assembles manifest publications for an explicitly
supplied leaf-first CA chain and per-issuer notification URLs. It reparses
certificate DER, selects the issuer's rsync manifest location from SIA, retrieves
repository caches, and copies the manifest-listed files. It identifies each
child certificate by exact DER equality and rejects missing or ambiguous child
locations. Distinct notifications retain separate object maps even when their
rsync URLs overlap. Repeated notifications are retrieved once per operation;
recent successful caches can be reused inside the polling interval.

Input chains are limited to 32 certificates. Retained repository object data and
assembled publication data each have a 128 MiB aggregate limit. The result is
explicitly untrusted: the configured anchor, manifest history, revocation,
resource containment and issuance-response binding remain validation duties.
Signed TLS fixtures exercise retrieval followed by durable path validation,
cache reuse, independent byte ownership, missing files, wrong children,
repository substitution, CRL tampering and wrong anchors.

Automatic issuer-chain discovery, issuance resolver configuration, recovery,
Terraform integration and native delegated verification remain unfinished.


## Issuance with RRDP verification

The private up-down client now has an issuance entry point that connects RRDP
retrieval to the existing manifest-backed validation gate. Configuration supplies
an explicit issuer-to-anchor chain, a separate trust anchor, per-issuer HTTPS
notification URLs, and private cache/history directories. Chain DER, issuer
signatures, anchor equality, URL schemes and directory configuration are checked
before dispatch. The response must identify the configured immediate issuer.

After the signed issue response, the client retrieves the issued certificate's
manifest publications, validates the anchored resource path and allocation,
and persists manifest history before completing the exchange journal. Repository
failure or verification failure leaves issuance pending and blocks resubmission.
An unchanged recent cache can be reused; absence from that cache still fails
verification and does not trigger another issuance request.

Signed fake up-down and TLS repository tests cover successful completion and
cache reuse, unavailable repositories, altered CRLs, issuer/anchor mismatches,
insecure URLs and allocation mismatches. They inspect pending exchange state and
manifest history, including a second call that must not retry failed issuance.

This entry point remains internal. Terraform configuration/resources, automatic
chain discovery, pending-operation recovery and native delegated enrollment and
interoperability remain unfinished.


## Terraform publication inventory

`arin_rpki_publication` now exposes RFC 8181 list requests through Terraform.
It accepts a trusted endpoint, assigned publisher handle, explicit local and peer
BPKI anchors, existing EE certificate, optional intermediate chains and local
CRLs. The RSA signing key is read from a private regular PEM file (PKCS#1 or
PKCS#8); symlinks, oversized files and group/other permissions are rejected.
Key bytes are not returned or stored in Terraform state. Certificate inputs and
the key path are stored in state.

The data source requires an existing private, persistent journal directory.
Authenticated lists complete the journal; interrupted or unverifiable exchanges
remain pending and block automatic retries. Server-reported objects are sorted
by URI. Their hashes are inventory metadata, not independent RPKI validation.
The API key is not used and remote repository contents are not modified.

Signed fake-server client tests exercise key loading, signature verification,
refresh and sorted inventories. Terraform acceptance tests exercise schema,
configuration mapping, state, empty inventories and error propagation through
an injected reader. They complement protocol tests rather than claiming native
ARIN interoperability. Native verification remains blocked by absent delegated
sandbox enrollment and BPKI credentials. Pending recovery and managed delegated
resources remain unfinished.


## Terraform provisioning inventory

`arin_rpki_provisioning` now exposes RFC 6492 list requests with the same existing
file-backed BPKI identity and persistent journals as publication inventory.
Child and parent handles scope the exchange. Classes are sorted by name, and
certificates by publication URLs then DER hash. Outputs include resource sets,
allocation expiry, issuer PEM, issued certificate PEM and hashes, publication
URLs and echoed requests. Omitted request attributes remain null; explicitly
empty requests remain empty strings.

The parent BPKI response and protocol structure are verified. This inventory
read does not validate the returned resource certificates against an RPKI trust
anchor and does not issue or revoke certificates. Signed client tests cover
request authentication, refresh and absent/empty output distinctions. Terraform
acceptance tests cover configuration mapping, nested state, empty inventories,
refresh and errors through an injected reader. Native delegated verification,
managed resources and pending-operation recovery remain unfinished.

## Recovering an interrupted inventory read

The `tools/rpki-journal` command inspects an existing journal and can explicitly
abandon one pending `publication-list` or `updown-list` request. Run from the
provider repository with the peer ID from `rpki-exchange-<peer>.json`:

```sh
go run ./tools/rpki-journal -directory /private/arin-rpki -peer PEER_ID
go run ./tools/rpki-journal -directory /private/arin-rpki -peer PEER_ID \
  -recover-read-sha256 REQUEST_SHA256
```

Use the exact `pending.request_sha256` from inspection. Recovery sends no network
request. It preserves both signing-time watermarks and stores the abandoned
request in `recovered_read`; this is the latest recovery record, not an unbounded
audit log. A later refresh must use a newer signing second, preventing identical
signed bytes from being confused with the abandoned request. The next Terraform
refresh can then perform a new authenticated list exchange.

Recovery rejects pending issuance, revocation, publication batches, unknown
operations, mismatched digests and repeated recovery of an already cleared read.
It takes the ordinary exclusive lease and cannot bypass a live or crashed lock.
It does not create missing journals or reset trust history. Crashed locks and
uncertain mutations still require separate reconciliation, which is unfinished.
Older provider builds reject journals containing the new recovery metadata;
keep using a build that understands this field after recovery.

Tests cover first-read and later-read recovery, reopen, unchanged timestamps,
metadata ownership, replay prevention, mutation rejection and locking. A signed
HTTP test covers failure, blocked retry, explicit recovery and successful reads.


## Managed publication bundles

`arin_rpki_publication_bundle` manages an explicit map of rsync URLs to canonical
base64 object contents. Callers supply signed objects and corresponding manifests;
the provider does not create or cryptographically validate these payloads. Changes
to the owned URL set are sent in one RFC 8181 atomic batch. Objects outside the
owned set are preserved. Overlapping ownership with another resource or publisher
is unsupported.

Creation uses absent-object preconditions. Refresh records current hashes for
owned URLs. Plans compare them with desired hashes, so missing or changed objects
produce updates. Updates use last-observed hashes for replacement/withdrawal and
absence for new URLs; a concurrent change after refresh is rejected by the server.
Destroy withdraws the owned objects still observed at refresh. A successful
protocol response confirms the batch before Terraform state is committed.

Desired data is bounded to 10,000 objects and 3 MiB decoded bytes. Actual mutation
XML must fit the existing 4 MiB transport limit. Key bytes remain in a private
local file, while object content and identity configuration remain in state.
Changes to endpoint, publisher or trust anchors require replacement.

Terraform tests using an injected repository cover create, update, additions,
removals, drift repair, missing-object recreation, clean plans, destroy and
preservation of unmanaged objects. Planner tests verify hash preconditions and
limits; the signed batch lifecycle also exercises planned replacement. These
complement existing signed protocol tests and do not establish native ARIN
interoperability. Import and uncertain-mutation recovery remain unfinished.
A pending mutation blocks further peer exchanges; the read-only recovery command
cannot abandon it. No native delegated publication was attempted.

## Publication bundle import

Import accepts the absolute path to a private regular JSON file, limited to
16 MiB. Use the resource's attribute names for these fields:

- Required: `endpoint`, `publisher_handle`, `journal_directory`,
  `signing_key_file`, `signing_certificate_pem`, `signing_ca_pem`,
  `signing_crls_pem`, `peer_ca_pem`, and a nonempty `objects` map.
- Optional: `signing_intermediates_pem`, `peer_intermediates_pem`, and `id`.

The `objects` values are the same canonical base64 contents used in configuration.
Certificate fields contain PEM text; `signing_key_file` contains only the local
key path. Never put private key bytes in this manifest. Create the file with
permissions 0600 and keep it out of Git. Symlinks, permissive permissions,
unknown fields, duplicate keys and invalid object contents are rejected.

```sh
terraform import arin_rpki_publication_bundle.example /absolute/path/publication-import.json
```

Import performs an authenticated inventory read and requires every listed object
to exist with exactly the supplied content hash. It adopts only those URLs and
performs no publication mutation. Align the resource configuration with the
manifest to obtain a clean subsequent plan.

If restoring an existing Terraform bundle after its membership has changed,
include its original 64-character lowercase hexadecimal `id` from prior state.
Otherwise the provider assigns an ID from the endpoint, publisher and imported
URL set, as it does on creation. The ID is a Terraform identity, not authorization.

Terraform tests cover initial import, import after membership updates, preserved
IDs, clean plans and mismatch rejection. Parser tests cover malformed manifests,
file permissions, symlinks and duplicate fields. Import cannot bypass a pending
exchange, and uncertain-mutation recovery remains unfinished.

## Concurrent changes to unchanged bundle members

When any owned member changes, the publication planner now includes unchanged
owned objects in the same batch, re-publishing their bytes with the last-observed
hash as a precondition. This prevents a concurrent edit to an otherwise unchanged
object from silently invalidating the newly published manifest. If any member's
hash no longer matches, the server rejects the entire batch. A completely
unchanged bundle still produces no mutation request.

A signed fake-repository test changes an unchanged member after planning, then
checks that both the planned replacement and unchanged-member guard are rejected
atomically. Planner tests also verify the guard and the no-op behavior. Encoded
request limits apply to all included objects, including these guarded members.

Local verification for this correction passed the race/unit suite, vet and build.
The broad race-enabled Terraform acceptance run reached its five-minute timeout
in TestAccWhoisErrors without an earlier assertion failure. That test and all
remaining Whois tests passed separately in 12.945 seconds. The preceding import
commit ca6645a also completed self-hosted CI successfully.

## Durable signed request evidence

Before dispatching a protocol request, the HTTP exchange now saves the exact CMS
bytes to `rpki-exchange-<peer>.json.request.der`, then records the pending request
hash in the journal. File and directory synchronization precede HTTP dispatch.
Each peer retains one private request file, limited to 4 MiB. Completed evidence
is replaced by the next request only when no operation is pending, so retention
does not grow with every exchange. CMS evidence contains signed protocol payloads
and public certificates/CRLs, never the private signing key.

The internal pending-request reader takes the existing peer lease, checks file
permissions, type, size and the journal's exact SHA-256 digest, and returns only
the matching bytes. Missing or altered evidence fails closed and never clears a
pending mutation. Old pending journals without an evidence file remain blocked;
the provider cannot reconstruct their signed requests from a digest.

Tests verify that dispatch follows durable evidence, reopen retains the exact
CMS request, pending evidence cannot be overwritten, completed requests permit a
bounded replacement, caller buffers cannot change persisted bytes, and malformed
or inaccessible files are rejected. A signed publication test verifies retained
request evidence after an uncertain response.

This supplies evidence for future reconciliation. It does not yet reconcile
uncertain mutations or permit their replay, and crash-lock recovery remains
unfinished. The evidence is local to the configured persistent journal directory.

## Inspecting pending publication intent

The private publication recovery inspector now authenticates the retained CMS
request against the configured local BPKI anchor at its recorded signing time.
It verifies the peer identity, pending operation, request digest and exact CMS
signing time before parsing the saved batch. It extracts the expected before and
after hashes for each affected URL, including unchanged-member guards. It never
reconstructs or resends the original request.

A separate bounded inventory comparator returns `matches_before`, `matches_after`,
`ambiguous` or `conflict`. Both-state matches, such as a batch containing only
no-op replacements, are ambiguous. Partial transitions, missing guards and third
party changes are conflicts. Unmanaged URLs are ignored. These labels describe
the observed state, not proof of which actor changed it or a remote transaction's
historical outcome.

Signed-request tests cover exact intent recovery, wrong peer/operation/time,
corrupted signatures, missing evidence and malformed payloads. Comparison tests
cover create/replace/withdraw transitions, no-op ambiguity, partial updates,
changed guards, malformed inventories and unmanaged objects. Inspection and
comparison leave the pending journal unchanged.

An authenticated recovery inventory exchange and durable reconciliation decision
are still required before uncertain mutations can be cleared. This comparison
helper alone does not authorize completion, abandonment or retry.

## Authenticated recovery inventory reads

The private recovery reader now holds the original publication peer's lock while
sending a signed list request through a separate journal scoped to the pending
mutation digest. Ordinary peer IDs remain unchanged. Different mutation digests
produce different recovery journals without changing the remote endpoint or
publisher identity.

The recovery request must use a newer signing second than the original request.
Response verification enforces both the original peer's receive-time watermark
and the recovery journal's own watermark. The list reply still requires the
configured peer BPKI chain, CRLs and protocol validation. A failed recovery read
remains pending in its separate journal and blocks automatic retry. The existing
explicit read-only recovery mechanism can abandon that list operation; it cannot
abandon the original mutation.

Successful reads return the inventory comparison and durable probe timestamps.
They leave the original journal and request evidence unchanged. Tests verify the
original lock remains held during HTTP, only a list request is sent, before/after
and conflict results are distinguished, stale replies and wrong signers fail,
failed probes are not retried, and recovery receive history cannot roll back.

This is still an internal observation operation. Committing a reconciliation
decision, exposing recovery to the CLI/provider, and native delegated verification
remain unfinished. A successful observation does not itself clear a mutation.

## Durable publication reconciliation

The private recovery flow can now commit an explicitly selected `matches_before`
or `matches_after` observation while still holding the original peer lock. It
requires the authenticated inventory comparison to match that selection.
Ambiguous, conflicting or unexpected results do not clear the pending mutation.
No mutation is resubmitted as part of reconciliation.

Commit atomically saves a `reconciled_publication` receipt containing the original
request identity, recovery peer ID, observed outcome and probe timestamps. It
clears pending state and advances the original send/receive watermarks to the
recovery exchange's timestamps. The journal retains the latest receipt rather
than an unbounded audit log. Later requests cannot roll signing time backward.
Older provider binaries reject the new receipt field; use a compatible build
after reconciliation.

Signed fake-server tests cover both commit outcomes, wrong expected outcomes,
conflicts, repeated commit rejection and unchanged state on observation failure.
State tests cover reopen, receipt ownership, invalid metadata, clock rollback,
corrupt receipts and atomic-save failure preserving the on-disk pending record.
The observation describes current hashes, not proof of transaction history.

The mechanism remains internal. CLI/provider exposure, recovery of issuance and
revocation, crash-lock handling and native delegated verification remain unfinished.

## Publication recovery CLI

`tools/rpki-journal` now exposes publication observation and explicit reconciliation.
The configuration is a private regular JSON file (0600, absolute path, at most
16 MiB) using these Terraform attribute names:

- Required: `endpoint`, `publisher_handle`, `journal_directory`,
  `signing_key_file`, `signing_certificate_pem`, `signing_ca_pem`,
  `signing_crls_pem`, and `peer_ca_pem`.
- Optional: `signing_intermediates_pem` and `peer_intermediates_pem`.

All values are strings. Certificate/CRL values contain PEM text. The signing key
field contains only a private local file path. Unlike the import manifest, this
file contains no `objects` or `id`. Unknown or duplicate fields, non-string
values, symlinks and permissive file permissions are rejected. Keep the file out
of Git.

First inspect the existing journal locally to obtain `pending.request_sha256`:

```sh
go run ./tools/rpki-journal -directory /private/arin-rpki -peer PEER_ID
```

Then obtain an authenticated inventory comparison without clearing the mutation:

```sh
go run ./tools/rpki-journal \
  -publication-config /absolute/path/publication-recovery.json \
  -request-sha256 REQUEST_SHA256
```

The JSON report includes before/after hash maps, the observed outcome, recovery
peer ID, signing timestamps and `committed: false`. Empty hashes represent absent
objects. No certificate or private-key contents are returned in this report.
To explicitly commit a selected matching outcome, perform a new authenticated
read with `-expect matches_after` or `-expect matches_before`:

```sh
go run ./tools/rpki-journal \
  -publication-config /absolute/path/publication-recovery.json \
  -request-sha256 REQUEST_SHA256 \
  -expect matches_after
```

Success returns `committed: true`. A changed, ambiguous or conflicting observation
fails without clearing the mutation. Neither mode sends publication mutations.
The command cannot bypass crash locks or reconcile issuance/revocation. A failed
recovery list remains pending in its separate recovery journal; failures report
the recovery journal peer ID. Inspect that journal and use exact read-only
abandonment before trying again.

After successful reconciliation, run Terraform refresh/plan before further
changes. An uncertain create whose objects exist may need exact-content import;
reconciliation does not invent Terraform state. Updates and deletes use the next
refreshed inventory as usual. Retain the original journal directory and use a
provider build that understands reconciliation receipts.

CLI mode tests cover flag exclusivity, observation versus commit and error exit
codes. File-loading tests cover configuration validation. A signed client test
loads the existing signing identity, observes a pending batch, commits a match
and verifies the report excludes identity material. Native ARIN verification
still requires delegated enrollment and credentials unavailable in this sandbox.

### Pending revocation intent

The private revocation recovery planner now verifies the retained signed request
against its original BPKI anchor and recorded signing time, checks its digest and
exact peer journal, and extracts the child, parent, resource class and normalized
key identifier. Publication recovery shares the same saved-request verification
helper. Neither inspection changes journal state nor sends network requests.

[RFC 6492 section 3.5](https://www.rfc-editor.org/rfc/rfc6492.html#section-3.5)
scopes revocation to a client's key within a resource class. Signed tests reject
wrong peers, handles, operations, timestamps, signatures and missing evidence;
parser tests reject malformed or mismatched revocation payloads. These tests use
synthetic BPKI identities. The recovery CLI below adds remote evidence and
durable reconciliation for completed revocations. Scheduled revocations remain
pending until completion can be proven. No native delegated enrollment is
available for verification.

### Revocation recovery inventory

The private recovery reader now holds the original mutation journal lease while
issuing a signed up-down list through a separate journal scoped to the original
request digest. The probe uses a newer request signing time and preserves the
original and recovery response-time floors. Failed probes remain pending and
cannot be retried implicitly. The original mutation journal stays unchanged.

Inventory classification distinguishes `key_present`, `key_absent` and
`class_absent`, comparing the RFC 5280 method-1 public-key hash within the target
resource class. An arbitrary certificate SKI extension cannot substitute for
that hash. Signed server tests cover each outcome, original locking, stale
responses, wrong signers, unavailable/scheduled replies and probe retry blocking.
Malformed inventories and duplicate classes fail classification.

This is an observation primitive, not completed revocation recovery. A missing
key does not prove CRL publication; a present key can still be scheduled for
revocation. No outcome clears the original journal or permits mutation replay.
The proof-backed recovery CLI below adds durable reconciliation when current
resource-path and CRL evidence are available. Native verification still requires
a delegated setup.

### Revocation observation CLI

Use a private JSON configuration file (mode `0600`) with the same transport and
BPKI fields as the publication recovery configuration above. Replace
`publisher_handle` with both `child_handle` and `parent_handle`; `endpoint` must
be the provisioning service URI. The signing key remains in a separate private
file referenced by `signing_key_file`. Publication-only fields, unknown fields,
duplicate keys and non-string values are rejected.

```sh
go run ./tools/rpki-journal \
  -provisioning-config /absolute/private/provisioning.json \
  -request-sha256 EXACT_PENDING_REVOCATION_SHA256
```

The command verifies the retained revocation request and performs a signed list
read. Its JSON report includes child/parent handles, resource class, normalized
key identifier, outcome, probe journal peer and durable sent/received timestamps.
`committed` is always false. The original mutation stays pending for every
outcome. `-expect`, publication configuration and local journal modes cannot be
combined with this mode. Errors include the recovery peer ID when available, so
an interrupted probe can be inspected with the existing local journal commands.
Only a pending inventory read may be explicitly abandoned by those commands.

This CLI does not issue certificates, resubmit revocations or establish CRL
publication. Use the proof-backed recovery mode below to reconcile a Terraform
certificate revocation. Signed local API tests verify report mapping and unchanged
pending state; CLI tests verify mode isolation, error handling and rejection of
commit flags. Native delegated verification still requires sandbox enrollment.

### Certificate management client API

`IssueRPKICertificate` now accepts the provider's file-backed BPKI configuration,
a PEM CA CSR, resource class, optional resource subsets, and separate resource
trust configuration. The latter requires a PEM anchor, an ordered issuer chain
(immediate issuer through that anchor), one HTTPS RRDP notification URL per
issuer, and existing private cache/history directories. It uses the existing
manifest-backed issuance gate before completing the exchange journal. Private
resource keys are never required or returned; the caller supplies a signed CSR.

The result contains the certificate and issuer PEM, publication URLs, class,
method-1 public-key identifier and the issued certificate's expiry. Omitted
resource subsets remain distinct from explicitly empty subsets. Revocation uses
`RevokeRPKICertificate` with a class and key identifier and the same transport
configuration. Uncertain or scheduled responses preserve pending state and block
subsequent mutations; no client-side automatic resubmission is added.

Signed issuance/RRDP tests now also exercise the file-backed API, including exact
returned certificate identity, cache/history reuse and blocked retry following
failed validation. Signed revocation tests cover confirmed responses, protocol
rejection, wrong keys and scheduled processing through that API. Cancellation
and malformed-input checks run before loading signing keys. These are synthetic
fixtures, not native delegated interoperability evidence. Terraform certificate
schema, lifecycle/import and complete uncertainty recovery remain unfinished.

### Managed resource certificate

`arin_rpki_certificate` now connects Terraform create/update to the validated
issuance API and destroy to key-scoped revocation. Refresh locates the CSR key
within the configured class in a signed inventory, rejects duplicate matching
certificates, checks echoed request subsets and publication locations, and
validates the current manifest-backed resource path before updating state.
Missing keys/classes remove the resource from state; malformed responses and
failed resource validation return diagnostics instead of implying absence.

Endpoint, handles, class and CSR public-key changes require replacement. A new
CSR for the same key updates in place, preserving the key. Planning validates
the CSR signature/profile locally and compares key identifiers before choosing
replacement. A changed CSR must be known when planning an existing resource; an
unknown CSR blocks the plan rather than guessing whether to revoke the key. Resource
subset updates request a certificate for the existing key. No automatic renewal
schedule is implemented: a Terraform refresh observes parent-issued changes but
does not create certificates. Destroy affects all certificates for that key in
that class, so ownership must not overlap. Signing key files and persistent local
journals/cache/history must remain available during refresh and destroy.

Terraform acceptance tests with injected clients cover create, clean plans,
resource-subset updates, key replacement and destroy. Signed client tests cover
validated refresh through the same RRDP cache/history used by issuance. Import,
complete uncertain-operation recovery, automatic chain discovery and native
sandbox verification remain unfinished. The absence of delegated enrollment
prevents native lifecycle testing at present.

### Certificate import

Import accepts the absolute path of an existing private regular JSON file
(mode `0600`, no symlink, at most 16 MiB). Its keys are the configurable Terraform
attributes for `arin_rpki_certificate`. Include every required attribute; optional
string attributes can be omitted or null. An empty requested resource subset
remains an explicit request for no resources. `rrdp_notifications` is an ordered
JSON array of strings, with one URL per issuer in `issuer_chain_pem`.

Required string keys:

- `endpoint`, `child_handle`, `parent_handle`, `journal_directory`
- `signing_key_file`, `signing_certificate_pem`, `signing_ca_pem`,
  `signing_crls_pem`, `peer_ca_pem`
- `class_name`, `csr_pem`, `resource_anchor_pem`, `issuer_chain_pem`
- `rrdp_cache_directory`, `manifest_history_directory`

Optional string keys are `signing_intermediates_pem`, `peer_intermediates_pem`,
`requested_asn`, `requested_ipv4` and `requested_ipv6`. Encode PEM newlines as JSON
escapes using a JSON serializer. Store only the signing key's file path, never
private key bytes. Computed fields such as `id`, `ski` and `certificate_pem` are
not accepted. Unknown keys, duplicates, case aliases and trailing JSON fail.

```sh
terraform import arin_rpki_certificate.example /absolute/private/certificate.json
```

Import reads signed parent inventory and validates the matching CSR key's current
resource path, publication locations and echoed resource subsets. Missing keys,
ambiguous matches and validation failures prevent adoption. Import does not
issue or revoke certificates. It derives the same identity and computed fields
as normal refresh. Keep the Terraform configuration aligned with the manifest
and inspect a subsequent plan. The manifest does not clear a pending exchange;
uncertain issuance still requires separate reconciliation before ordinary reads.

Terraform tests verify import before and after key replacement, complete state
agreement and clean plans. A missing-target import fails without additional
issuance or revocation. Private-file and JSON parser tests cover permissions,
symlinks, duplicates, missing fields, nulls, aliases and optional-value semantics.
Native delegated import verification still requires sandbox enrollment.

Certificate planning now distinguishes CSR content from key identity. Terraform
acceptance tests use signed CA CSRs and verify that a new CSR for the same key
updates without revocation, a different key replaces and revokes the old key,
and malformed CSR input fails planning without issuing or revoking. These tests
also retain import/state agreement and clean-plan coverage. The public-key
comparison does not replace the signed-response and resource-path checks during
issuance and refresh.

### Interrupted issuance observation

The private issuance recovery planner now authenticates the saved request
against its journal peer, BPKI anchor, digest and recorded signing time. It
extracts the exact CSR, class, child/parent handles and requested resource subsets;
absent and explicitly empty subsets remain distinct. It rejects malformed XML,
unsupported attributes, invalid resource ranges and invalid CSR signatures.

The recovery reader holds the original journal lease while issuing a signed
inventory request through a separate digest-scoped journal. It validates a match
against the saved CSR, echoed resources and current RRDP manifest-backed resource
path. It returns `matches_request` with the validated certificate, or `key_absent`
when the key/class is missing. Neither result clears or resends the mutation;
absence is not evidence that a scheduled request will never complete.

Signed tests cover saved-request identity/time/operation mismatches and missing
evidence. End-to-end signed inventory/TLS RRDP tests verify matching and absent
outcomes, cache reuse and byte-identical original pending journal state. Normal
certificate refresh shares the same validation path. Durable issuance recovery
commit, CLI integration and native verification remain unfinished.

### Durable issuance reconciliation

The private reconciliation entry point now accepts an exact pending request
digest and an explicitly selected certificate DER SHA-256. While retaining the
original exchange lease, it re-reads signed parent inventory and validates the
saved CSR/resource request through the current RRDP resource path. Only a
matching request and certificate hash permit commit. Absent keys, mismatched
certificates and validation failures leave the original mutation pending.

Successful commit atomically records the original request metadata, recovery
peer, certificate hash, resource class, key identifier and sent/received times;
it advances the original journal watermarks and clears pending state. This is
an observation of a valid current certificate, not proof that the interrupted
request caused its issuance. It does not resend issuance or create Terraform
state. A create failure without state still needs validated import afterward.

Signed/TLS tests cover successful persistence and reopen, rejection of absent
keys and different certificate hashes, receipt ownership and repeat commit
without dispatch. Journal tests cover invalid identities/operations/timestamps,
malformed receipts and failed persistence preserving the original pending state.
Older binaries that do not recognize the new `reconciled_issuance` field reject
such journals rather than ignoring it. Public API/CLI integration, revocation
completion and native delegated verification remain unfinished.

### Issuance recovery CLI

Use the private provisioning configuration described above with a second private
JSON file containing resource trust settings. The validation file accepts exactly
these four string fields plus an ordered notification URL array:

```json
{
  "resource_anchor_pem": "PEM resource trust anchor",
  "issuer_chain_pem": "PEM immediate issuer through anchor",
  "rrdp_cache_directory": "/absolute/private/rrdp",
  "manifest_history_directory": "/absolute/private/manifests",
  "rrdp_notifications": ["https://repository.example/notification.xml"]
}
```

Use actual PEM strings encoded with a JSON serializer. Both files must be private
regular files with absolute paths. The validation file accepts at most 31 HTTPS
notification URLs, rejects duplicate/unknown keys and requires a URL per issuer
when validating. No CSR or requested resource subsets are accepted here: those
come from the retained, authenticated pending issuance request.

Observe first:

```sh
go run ./tools/rpki-journal \
  -provisioning-config /absolute/private/provisioning.json \
  -issuance-validation /absolute/private/validation.json \
  -request-sha256 EXACT_PENDING_ISSUANCE_SHA256
```

A `matches_request` report includes `certificate_sha256`. To reconcile that exact
certificate, repeat with `-expect-certificate-sha256 OBSERVED_CERTIFICATE_SHA256`.
The CLI performs a new signed inventory read and resource-path validation before
committing. A missing key, changed certificate hash or failed validation leaves
the original operation pending. Observation reports `committed: false`; a
successful explicit commit reports `committed: true`. Neither operation issues
or revokes certificates. Reports include handles, class, key and journal metadata,
not signing key bytes or BPKI configuration.

After commit, run Terraform refresh/plan or validated certificate import if a
failed create left no resource state. The CLI does not edit Terraform state.
Errors identify the probe journal when possible; only pending inventory probes
can be explicitly abandoned through read recovery. Revocation completion has a
separate mode below; native delegated sandbox verification remains unavailable.

### Manifest-backed revocation evidence

A private proof checker now validates supplied revocation evidence against an
explicit resource anchor. It first verifies the issuer's upstream manifest path,
then binds the prior certificate's key, issuer signature, resource containment and
CRL distribution point. The issuer's current signed manifest must select a valid
CRL that contains the certificate's serial with an effective revocation time.
No certificate with the retired key may remain listed in that publication point,
including one with a different filename or serial.

The returned proof identifies the exact certificate, issuer, CRL and manifest by
SHA-256. This checker does not fetch repositories, record manifest history or
clear pending state. Before using it for recovery, callers must bind the issuer
to the signed resource class, verify current parent-inventory key absence and
persist rollback protection for both the issuer path and its own manifest.
Expired prior certificates currently fail this proof check; absence alone is
not substituted for explicit current revocation evidence.

Signed tests cover valid revocation, absent revocation entries, still-published
and reissued keys, incorrect keys/anchors/issuers/CRL locations, resource
overclaims, tampered CRLs, expired manifests and future revocation times.
Repository retrieval, durable history and reconciliation are described below.
Native verification remains unavailable. The relevant protocol scope is
[RFC 6492 section 3.5](https://www.rfc-editor.org/rfc/rfc6492.html#section-3.5).

### Durable revocation evidence history

Revocation proof validation now has a durable wrapper. Under the existing
per-anchor manifest-history lock, it verifies the complete revocation proof and
records the issuer's own manifest together with every upstream manifest in one
atomic save. A failed proof or rollback in any member leaves on-disk history
unchanged. No proof is returned if persistence or lock cleanup fails.

Issuance, refresh and revocation use the same history file and issuer-key/manifest
URI scopes. The schema is unchanged. A newer revocation manifest therefore
prevents a later issuance/refresh from accepting the earlier manifest and CRL
that showed the key as unrevoked. Recovery also cannot ignore a newer upstream
manifest already recorded by ordinary certificate validation.

Signed tests verify persistence/reopen, idempotent evidence, local and upstream
rollback rejection, unchanged history after a mixed-version failure, exclusive
locking and shared watermarks across issuance and revocation. The reconciliation flow below binds the proof to fresh signed parent
inventory; native delegated verification remains unavailable.


### RRDP retrieval for revocation evidence

The private RRDP client now assembles revocation evidence from the persistent
HTTPS repository cache. Callers supply the prior certificate followed by its
issuer chain and one notification URL per issuer. The retiring issuer's
manifest may omit the prior certificate; every upstream issuer certificate
must still be published. The same notification scoping, object bounds, polling
limits and cloned buffers used by issuance apply here.

Retrieval does not authenticate a revocation by itself. Its result must pass the
anchored revocation proof and shared durable history checks. Signed TLS tests
exercise these together, including a direct anchor issuer, a withdrawn child,
a still-published key, missing files and upstream certificates, swapped
repositories, tampered CRLs, wrong anchors and persistent cache reuse after
caller mutation. No exchange journal is cleared by retrieval. The reconciliation flow below adds fresh signed parent inventory binding and
a durable reconciliation receipt.


### Revocation recovery CLI

`RecoverRPKIRevocation` and `tools/rpki-journal` can now reconcile an uncertain
revocation without resending it. The original peer lease remains held while a
separate digest-scoped exchange reads signed parent inventory and retrieves
RRDP evidence. The resource class must still exist, the key must be absent, and
the class issuer must exactly match the configured immediate issuer. The prior
certificate must identify the requested key and pass the current anchored
revocation proof checks, including an effective CRL entry and withdrawal of all
published certificates for that key. Manifest history is durably recorded before
any exchange journal is cleared.

Create a mode-0600 JSON file at an absolute path, containing the same fields as
issuance validation plus `prior_certificate_pem`:

```json
{
  "prior_certificate_pem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n",
  "resource_anchor_pem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n",
  "issuer_chain_pem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n",
  "rrdp_notifications": ["https://repository.example/notification.xml"],
  "rrdp_cache_directory": "/private/arin/rrdp",
  "manifest_history_directory": "/private/arin/manifests"
}
```

Use the saved prior certificate from Terraform state or a retained certificate
file. The issuer chain runs from the immediate issuer through the configured
anchor, with a notification URL for each issuer. Both directories must already
exist with private permissions and persist between runs. The loader rejects
unknown, duplicate, missing, null and incorrectly typed fields, symlinks and
publicly readable configuration files.

First inspect the authenticated proof without clearing the pending mutation:

```sh
go run ./tools/rpki-journal \
  -provisioning-config /private/arin/provisioning.json \
  -revocation-validation /private/arin/revocation.json \
  -request-sha256 "$REQUEST_SHA256"
```

The JSON report includes certificate, issuer, CRL and manifest SHA-256 digests,
the selected CRL URI, class/key identity, recovery peer and signed timestamps.
It contains no private keys or certificate bodies. To commit, explicitly select
the reported prior certificate hash:

```sh
go run ./tools/rpki-journal \
  -provisioning-config /private/arin/provisioning.json \
  -revocation-validation /private/arin/revocation.json \
  -request-sha256 "$REQUEST_SHA256" \
  -expect-certificate-sha256 "$CERTIFICATE_SHA256"
```

Commit rereads inventory and revalidates repository evidence through the persistent
cache. An atomic `reconciled_revocation` receipt stores the original request,
class, proof fingerprints, key and recovery timestamps; it advances journal
watermarks and clears only that pending operation. A mismatched certificate,
invalid evidence or failure before the atomic journal replacement leaves the
durable pending journal in place. A filesystem error after replacement can leave
a receipt despite an error return; inspect the journal before proceeding.
Repeating a successful commit fails before another network request.
Older provider builds reject journals containing the new receipt field.

Run Terraform refresh/plan after reconciliation. Recovery does not edit Terraform
state. The inventory-only observation mode remains available by omitting
`-revocation-validation`; it never clears a mutation. The issuance and revocation
validation flags are mutually exclusive. Changed issuers and unavailable current
CRLs remain unresolved. Expired certificates and missing classes require the
explicit recovery modes described below. Equal-time signed inventory replay remains a protocol limitation because
list requests have no nonce. Native delegated ARIN verification still requires
sandbox enrollment and identities.

Signed HTTP/TLS tests cover private and API observation/commit, original-lease
retention during both inventory and repository requests, exact issuer/key/hash
binding, missing and invalid CRL evidence, durable reopen, receipt ownership,
watermark preservation and prevention of replay after completion. State tests
cover receipt corruption and atomic save failure; CLI tests cover mode isolation,
configuration failures and JSON output.

### Native public OT&E bootstrap and repository size (2026-09-23)

`make testotepublic` requires no enrollment, API key or mutation permission.
It retrieves the public sandbox trust anchor, matches its complete SPKI against
`arin-api/rpki/arin_ote.tal`, verifies the current self-signed certificate profile
and resource path, then parses the RRDP notification selected by its SIA.
The transport restricts every request, including redirects, to credential-free
HTTPS GETs on `rrdp.ote.arin.net`. The native test passed. It does not validate
published manifests, CRLs or subordinate certificates.

A separate bounded snapshot attempt failed at the existing retrieval limit.
The snapshot advertised by session `f3302a8b-c9ec-4e05-a285-4bbe281a5c03`, serial
`982`, returned HTTP 200 to HEAD with Content-Length `734113077` bytes (about
700 MiB). Its notification hash was
`63caeb45f94485eae691e6b12ac4bf0317a61e079c88c12f35aeece60d228e2c`.
This exceeds the client's 128 MiB response limit before XML/object validation.
The snapshot body was not retained or committed. Repository limits also include
64 MiB decoded snapshot content and 10,000 objects, so raising only the HTTP
limit would not establish compatibility.

This is an implementation gap independent of delegated account enrollment.
Full native path verification needs bounded streaming snapshot ingestion and
repository storage sized for this service, preserving whole-document digest,
XML, duplicate URI and transaction checks before accepting any objects. Merely
skipping the oversized repository must not count as a successful native path
validation. The bootstrap test intentionally stops at the notification.

ARIN documents the public sandbox TAL and enrollment requirements in its
[OT&E guide](https://www.arin.net/reference/tools/testing/), reviewed again on
2026-09-23. No signed provisioning/publication request was sent during this test.

### Streaming snapshot ingestion and native root path (2026-09-23)

`FetchSnapshotSpool` now streams an RRDP snapshot into a private temporary packed
object file, retaining only the URI/offset/size/digest index in memory. The shared
object-file parser preserves the existing snapshot/delta XML and base64 checks.
The spool becomes available only after EOF, complete-document SHA-256, session,
serial and duplicate-URI checks succeed. Failures close and remove staging data;
no partial spool is returned. Object reads verify their stored SHA-256 again.
Closing a spool removes the temporary file. It is not yet a durable cache.

Bounds are 2 GiB encoded input, 1 GiB decoded objects, one million objects,
128 MiB aggregate URI text, 4 MiB per object and 8 MiB per XML token (plus decoder
read-ahead). Token limits apply during reads, including comments and attributes,
so a single XML token cannot consume the full document allowance. HTTPS, normal
TLS verification, redirect restrictions and timeout policy are shared with the
buffered client. Existing in-memory cache paths keep their original limits.

`make testoterepository` explicitly opts into downloading the complete public
sandbox repository. On 2026-09-23 it verified 235,888 objects totaling 510,165,246
decoded bytes in 26.23 seconds, then validated the TAL-pinned root manifest, its
selected CRL and one published child CA path. Repeating path validation reopened
the durable manifest history successfully. The repository data was temporary and
removed afterwards. No delegated enrollment, API key or management request was
used. This proves public repository/root-path interoperability, not the signed
provisioning or publication lifecycle.

Synthetic coverage includes an input above 128 MiB containing 97 MiB of decoded
objects, more than 10,000 objects, digest mismatch after staged writes, duplicate
URIs, truncation, trailing data, read failures, cancellation, token/byte limits,
object tampering and private-file cleanup. TLS tests cover chunked success,
oversized Content-Length, truncation, downgrade redirects, unexpected 304 and
untrusted certificates. Existing snapshot/delta tests exercise the shared parser.

Remaining integration: persist packed repository generations and their indexes,
apply deltas transactionally, retain session/serial and polling safeguards across
restarts, migrate or explicitly reject incompatible caches, and use disk-backed
object lookup in issuance/refresh/revocation path assembly. Certificate resources
still use the previous bounded in-memory cache and cannot yet consume this large
native repository. The new spool must not be substituted for persistent refresh
without that work. Protocol reference: [RFC 8182](https://www.rfc-editor.org/rfc/rfc8182.html),
particularly complete snapshot verification and consistent repository replacement.

### Persistent disk repositories and certificate integration (2026-09-23)

Certificate issuance, validated refresh and revocation evidence now use
`RefreshDiskPersistent` through the shared path assembler. The repository cache
keeps a private packed object file and a canonical version 2 JSON index containing
URI, offset, size and SHA-256 metadata. Only the selected manifest files enter
path-validation memory. Reads check individual object hashes. Reopening validates
index scope, file privacy/identity, contiguous offsets, object/URI limits and
packed-file size before returning a handle.

Refresh holds the existing per-notification filesystem lease. It saves the
polling attempt before HTTP, retains conditional request metadata, rejects serial
rollback and changed snapshots at an unchanged serial, and uses complete ordered
delta chains where available. Deltas operate on a private copy, check old hashes,
and repack the completed result before publication. Any failed chain is discarded
and may fall back to a full snapshot; failed retrieval retains the previous
repository and the durable attempt timestamp. A new session requires a snapshot.

The new packed file and containing directory are synced before an atomic index
replacement. The prior generation is removed only after that replacement is
synced. Existing open readers retain their old file handle. A failure after index
rename may already have published the new generation, so its file is retained and
the call reports failure. Crashes may leave an exclusive lock and unreferenced
staging/generation files; these require explicit inspection and recovery, not an
automatic replay or deletion of current state.

Valid version 1 caches migrate under the same lease, preserving repository
identity, objects, snapshot reference, conditional timestamp and polling attempt.
Migration itself makes no network request and does not reset rollback protection.
Unknown versions, malformed records and missing/insecure/symlinked object files
stop before HTTP. Migration is one-way: older provider binaries reject version 2
cache files. Preserve the complete cache directory when moving installations.

Per-repository streaming limits remain 2 GiB XML, 1 GiB decoded objects, one million
objects and 128 MiB URI text. A delta candidate may temporarily contain up to
2 GiB of packed bytes before compaction. Across distinct repositories in one path,
limits are 2 GiB decoded data, one million indexed objects and 128 MiB URI text;
assembled manifest publications retain their 128 MiB memory limit. Index JSON is
bounded to 256 MiB. Snapshot/candidate/generation staging can temporarily require
multiple packed-file copies on disk.

The native public repository test now runs the persistent refresh and the actual
`RetrievePath` used by resources. It passed with 235,888 objects and 510,165,246
decoded bytes in 18.43 seconds, including repeated persisted-cache reads, root
manifest/CRL verification and reopening manifest history for the child CA path.
This closes the native repository-size integration gap. Signed delegated
provisioning/publication still lacks an enrolled sandbox identity.

Regression tests cover persisted polling, 304, rollback, new sessions, preferred
deltas, fallback, retained generations, old open readers, valid/empty v1 migration,
format/scope/file corruption, two-delta replacement/withdrawal/publication chains,
and rejection of late digest failures without exposing partial results.

### Explicit recovery after certificate expiry

An uncertain revoke can outlive its prior certificate. [RFC 6487 section 5](https://www.rfc-editor.org/rfc/rfc6487.html#section-5)
defines CRL contents in terms of non-expired revoked certificates, so absence of
an expired serial is not proof that the parent executed the revoke. Default
recovery remains CRL-based and rejects expired prior certificates.

`rpki-journal -accept-expired-certificate`, used with `-revocation-validation`,
explicitly enables a different result: the prior certificate has expired and its
key is absent from current signed class inventory and the issuer's current
manifest. The class must still exist and its signed issuer must exactly match
the configured issuer. Current issuer/anchor paths, resource containment,
manifest signatures/freshness, the manifest-selected CRL and durable rollback
history must all validate. The retained certificate must have a valid signature,
profile, issuer/key binding and ordered validity interval, with the current time
strictly after its `notAfter`. No certificate with that key may remain published,
even under another filename or serial. Expiry does not permit ordinary issuance
or refresh to accept an expired certificate as a valid resource path.

Observe without clearing the pending operation:

```sh
go run ./tools/rpki-journal \
  -provisioning-config /private/provisioning.json \
  -revocation-validation /private/revocation-validation.json \
  -accept-expired-certificate \
  -request-sha256 "$REQUEST_SHA256"
```

Review `evidence`, the certificate hash and timestamps. Add
`-expect-certificate-sha256 "$CERTIFICATE_SHA256"` to explicitly commit the
matching result. This performs another authenticated observation and validation;
it never resends the uncertain revoke. Without that hash the original pending
journal remains unchanged, although probe journals, repository cache and verified
manifest history are persisted as usual.

Reports use `evidence: "revoked"` for CRL-confirmed revocation and
`evidence: "expired_withdrawn"` for the expiry path. The latter includes `expired_at`
and `checked_at` in canonical UTC. The durable reconciliation receipt retains
both timestamps with the exact certificate, issuer, manifest and CRL digests.
The CRL digest in an expiry receipt identifies the current CRL used to validate
the publication, not a claim that it lists the expired serial. Inventory
`outcome` remains `key_absent` in both cases. This reconciles the verified retired
state; it does not prove that the original revoke caused that state, establish
server-side key revocation, or prevent future reissuance. Continue to use a fresh
key for replacement certificates.

The Go API enables this behavior through `RPKIRevocationValidation.AllowExpired`.
It is false by default. The JSON trust file has no implicit opt-in; the CLI flag
is required on each invocation and is rejected outside validated revocation
recovery. Existing CRL receipts retain their original serialized representation.
Older binaries cannot read new expiry receipts and must not be used to rewrite
those journals.

Signed inventory/TLS repository tests cover observation, explicit commit,
reopened expiry receipts, unchanged pending state on failures, disabled opt-in,
wrong selected hashes and missing classes. Proof tests reject exact expiry-boundary
ambiguity, unexpired/unrevoked certificates, invalid validity intervals, stale
manifests, wrong keys and published/renewed keys. Corrupt receipt timestamps and
incompatible CLI modes fail closed. Issuer rollover and expired issuer paths
still require additional evidence; signed native delegated
interoperability remains unverified without enrollment.

### Retained signed responses

Successful provisioning and publication exchanges now retain the exact accepted
CMS response in a private, digest-named sidecar beside the exchange journal.
The receipt records the associated request digest, operation and signing time,
plus the response digest and signing time. The sidecar is synced before the
journal clears the pending request. A later pending mutation preserves that last
accepted response, including across restarts. A successful replacement removes
the previous sidecar; failed commits retain evidence and fail closed. Crashes or
cleanup failures can leave unreferenced sidecars for explicit housekeeping.

Retention happens only after CMS authentication and protocol correlation. Reading
the retained bytes checks file safety and digest integrity; it does not replace
cryptographic reauthentication for future recovery. Legacy journals remain
readable but have no retained response. Older binaries reject journals containing
the new receipt and must not rewrite them.

This retained evidence supports the missing-class recovery mode below. Retention
alone does not reconcile or resend a mutation. Tests cover signed HTTP responses,
restart during a pending mutation, replacement cleanup, unsafe or corrupt files,
and sidecar or journal persistence failures.


### Missing-class revocation recovery

`-accept-missing-class`, used with `-revocation-validation`, permits recovery when
fresh authenticated inventory no longer contains the pending revoke's class.
The Go API exposes `RPKIRevocationValidation.AllowClassAbsent`. Both default to
false, and the JSON trust file cannot silently enable this mode.

Recovery requires the original peer journal's last completed response to be an
accepted `updown-list` exchange initiated strictly before the pending revoke.
It reauthenticates that exact CMS at its recorded signing time against the
configured peer BPKI trust, checks the child and parent handles, and requires the
historical class to contain the exact retained prior certificate DER. A renewed
certificate with the same key is insufficient. The historical issuer must match
the explicitly configured immediate issuer, whose current anchored path is still
validated. Current manifest/CRL checks, key withdrawal and durable manifest
history remain required. The expiry opt-in can be enabled separately.

Observation returns `outcome: class_absent` and `class_evidence_sha256`, the hash
of the authenticated historical CMS. Committing still requires the exact prior
certificate hash. The durable reconciliation proof retains the historical hash;
its presence distinguishes missing-class recovery from ordinary key absence.
Old receipts remain readable. Older binaries reject the new proof field.

Legacy journals without retained evidence, unrelated or rejected responses,
wrong peers/classes/issuers/certificates, tampered evidence and nonpreceding
requests fail closed. Recovery never repeats the revoke. Signed HTTP/TLS tests
cover successful commit, observation, restart and these rejection cases. Native
signed ARIN interoperability remains unverified without delegated enrollment.

### Public OT&E certificate profile census, 2026-09-23

The remaining profile audit concerns the alternative certificate policy and
resource-extension OIDs in [RFC 8360](https://www.rfc-editor.org/rfc/rfc8360.html).
The alternative policy is `1.3.6.1.5.5.7.14.3`; the alternative IP and AS resource
extensions are `1.3.6.1.5.5.7.1.28` and `1.3.6.1.5.5.7.1.29`. Supporting them
requires verified-resource intersection semantics, not merely accepting new OIDs.
At the time of this census, the certificate validator supported only the original
policy and extensions. The subsequent implementation is described below.

`TestOTEPublicRPKIRepository` now counts the policy/resource OID combinations of
every standalone `.cer` in the digest-checked public sandbox snapshot. A fresh,
credential-free run completed in 67.70 seconds with:

- 235,888 repository objects and 510,146,033 decoded bytes.
- 9,510 certificates advertising the original policy and resource extensions.
- Zero alternative-profile, mixed-profile or unclassified certificates.
- Successful TAL-pinned root, root manifest/CRL and one direct child CA path
  validation, including reopening durable history.

Reproduce with `make testoterepository`. The census parses certificate DER and
checks each packed object's digest; it does not validate every certificate chain,
inspect embedded EE certificates in signed objects, or prove that ARIN cannot
issue an alternative-profile certificate. It describes this observed snapshot.
The delegated service's native issuance profile remains unverified without
sandbox enrollment. No native alternative-profile example was observed in this census.


### Alternate certificate profile and verified resources

The shared certificate validator now supports [RFC 8360](https://www.rfc-editor.org/rfc/rfc8360.html#section-4.2.4.4)
policy/resource OIDs with AS, IPv4 and IPv6 intersection semantics. Each path step
uses its issuer's verified resource set. Alternate-profile overclaims can produce
a smaller or empty authorized set; original-profile overclaims still fail.
Inheritance cannot restore clipped resources, and omitted families remain absent.
Policy and resource-extension profiles must match within each certificate.

Issuance and refresh still compare the verified resources with the signed
allocation bounded by the request. Accepting the alternate profile does not
accept a partially authorized requested allocation. Revocation recovery uses the
same path rules. Manifest EE certificates still require inherited resources.
No Terraform schema or configuration change is required.

Tests cover signed mixed-profile paths, policy mismatches, noncritical and
duplicate extensions, AS authorization gaps, both IP families, empty intersections,
inheritance and manifest EE constraints. Native alternate-profile interoperability
remains unverified; the public census found only original-profile certificates.


Manifest-backed integration tests additionally cover all-alternate and mixed
certificate chains, an overclaiming intermediate, inherited manifest EE resources,
manifest-selected CRLs, durable history reopening and allocation matching against
the final verified set. A legacy descendant cannot reuse its alternate-profile
issuer's unverified overclaim. An allocation wider than the verified set remains
an issuance error even when certificate profile validation succeeds. These tests
use signed synthetic chains and publications, not native ARIN alternate-profile
certificates.
