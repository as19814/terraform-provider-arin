# OT&E completion prerequisites

Investigated September 24, 2026. This records incomplete native verification,
not missing Terraform registrations. The provider exposes 24 resources and 75
data sources. Account-specific request drafts and observations remain in the
private user cache, outside Git. Bulk Whois, invalid-POC downloads and WhoWas
requests are excluded from scope and no longer completion prerequisites.

| Remaining verification | Observed cause or evidence | Completion check |
| --- | --- | --- |
| Disposable organization CRUD and Admin POC replacement | Creation ticket remains Pending Review with action assigned to ARIN. The organization appears in the authenticated UI without Modify, but Reg-RWS returns 404. The same key reads the baseline organization successfully. | ARIN confirms activation, GET returns the intended disposable record, then Terraform import/update/Admin replacement/delete and absence verification pass. |
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

## Removed ticket submission

Ticket-message submission is now excluded. Its historical HTTP 500 is retained
as investigation evidence, not a current completion prerequisite. Keep the private
intent receipts intact. Message and attachment data sources remain supported.
