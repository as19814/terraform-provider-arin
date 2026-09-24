# OT&E completion prerequisites

Investigated September 24, 2026. This records incomplete native verification,
not missing Terraform registrations. The provider exposes 25 resources and 75
data sources. Account-specific request drafts and observations remain in the
private user cache, outside Git. Bulk Whois, invalid-POC downloads and WhoWas
requests are excluded from scope and no longer completion prerequisites.

| Remaining verification | Observed cause or evidence | Completion check |
| --- | --- | --- |
| Disposable organization CRUD and Admin POC replacement | Creation ticket remains Pending Review with action assigned to ARIN. The organization appears in the authenticated UI without Modify, but Reg-RWS returns 404. The same key reads the baseline organization successfully. | ARIN confirms activation, GET returns the intended disposable record, then Terraform import/update/Admin replacement/delete and absence verification pass. |
| Ticket-message submission | The original POST was a provider defect, corrected to documented PUT. The approved PUT returned HTTP 500 E_UNSPECIFIED. Full-ticket reads and UI inspection show no matching subject. The server-side cause and commit outcome are unknown. | ARIN reconciles the attempt. An existing confirmed message can be imported; any further submission needs separate approval. Verify attachment bytes, refresh, clean plan and receipt-only destroy. |
| Both ticket-closing methods | Disposable reports close automatically, so native tests have exercised reads and no-ops rather than RESOLVED-to-CLOSED writes. | Obtain two disposable RESOLVED tickets, test one per method, and confirm CLOSED through fresh reads. |
| Delegated RPKI provisioning and publication | The eligible organization is enrolled in Hosted RPKI, with no delegated parent/repository responses available. | Establish delegated enrollment and exchange identities, then verify authenticated signed lifecycles and restoration against OT&E. |

The pending organization and missing Modify action are consistent with incomplete
approval. They do not establish why ARIN's UI and Reg-RWS expose different views;
that requires ARIN confirmation. Do not import the visible handle as a completed
registration while its API lookup fails.

## Staff processing is explicit in OT&E

ARIN's [OT&E instructions](https://www.arin.net/reference/tools/testing/) say
staff do not actively monitor the sandbox. Create an Ask ARIN ticket in OT&E,
then a production Ask ARIN ticket with topic Other and subject
`OT&E approval requested`, referencing the sandbox request. Waiting or polling
alone does not arrange staff processing. Support drafts are prepared privately;
they have not been sent as part of this investigation.

The [organization guide](https://www.arin.net/resources/guide/account/records/org/)
describes staff review and approval requirements. A fictitious sandbox fixture
must be identified as such to staff, not represented as a validated legal entity.

ARIN documents that switching OT&E RPKI deployment types requires staff deletion
of the current configuration followed by enrollment. Ask for isolated test
resources or a migration/restoration procedure before requesting that deletion.
The prepared inquiry does not authorize resetting Hosted RPKI, accepting terms,
signing forms, or changing production registry records.

## Ticket payload investigation

The saved [method guide](arin-api/reg-rws/methods.md) specifies PUT for appending
a message. The client now uses that verb and prevents automatic replay. The
[message schema](arin-api/schemas/extracted/MessagePayload.rnc) allows the emitted
subject, text, category and attachment fields in any order. Its
[multiline schema](arin-api/schemas/extracted/utils/multiline.rnc) accepts integer
line numbers without imposing a minimum of one. Starting at zero is therefore
not a demonstrated schema violation. Attachment data and filename match the
published structure. These checks do not prove that the server accepts every
schema-valid payload or explain its internal error.

The HTTP 500 regression asserts one PUT, an unconfirmed receipt with the ticket
identity retained, and no invented message ID. Existing native intent receipts
must remain intact. Absence of matching text is insufficient evidence to replay
an uncertain submission.

The mutation verb/path review found no additional mismatch in the mapped Reg-RWS
operations. The phone-add header example omits `/phone`; the query example and
previous native test establish the implemented `/phone` route. See the
[operation reconciliation](reg-rws-coverage.md) and [POC evidence](pocs.md).
