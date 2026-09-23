# Delegated RPKI and publication audit

These protocol families remain in scope and unimplemented. An API key alone is
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
