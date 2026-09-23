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
synthetic BPKI identities. Remote outcome verification, scheduled-revocation
handling, durable reconciliation and Terraform certificate resources remain
unfinished. No native delegated enrollment is available for verification.

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
Durable outcome reconciliation, resource-path/CRL evidence and provider lifecycle
integration remain to be implemented and verified with native delegated setup.
