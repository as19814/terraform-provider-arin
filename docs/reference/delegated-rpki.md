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
