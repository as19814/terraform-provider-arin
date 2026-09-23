# Customers and network assignments together

Use `arin_customer.recipient.id` as `arin_net.assignment.customer_handle` to
manage a recipient and its simple reassignment in one Terraform state. The
reference creates the dependency: Terraform creates the customer first and
removes the network before deleting the customer. No explicit `depends_on` is
needed. See the [complete configuration](../../examples/customer-network/main.tf).

Customer address/privacy/comment updates are independent of the NET's name and
comments. Changes to a customer that require its replacement also replace the
NET because its `customer_handle` is immutable. The graph handles the old and new
recipient lifecycles, including when ARIN reuses the old NET handle after deleting
and recreating the assignment. Do not infer that no replacement occurred merely
because the NET handle stayed the same.

Import both objects into their configured addresses:

```sh
terraform import arin_customer.recipient 'PARENT-NET-HANDLE/CUSTOMER-HANDLE'
terraform import arin_net.assignment 'NET-HANDLE'
terraform plan
```

The customer's import ID includes its creation context because ARIN does not
return the original parent NET in customer reads. The configuration must specify
the actual parent and assignment prefix.

## Evidence

`TestOTECustomerNetGraphLifecycle` passed on 2026-09-23 for a disposable IPv4 /32
and IPv6 /64 under FT-684 allocations. Both lifecycles covered:

- Terraform creation of the customer and referenced NET.
- Customer street-address updates and NET comment clearing, keeping their IDs.
- Independent import verification for both resources and a clean plan.
- Forced customer replacement and dependent NET replacement in the same graph.
- A clean plan after replacement and destruction of both resources.
- Authenticated GET checks confirming every created object was absent afterward.

ARIN reused each NET handle during replacement while issuing a different customer
handle. Fake-server acceptance tests cover both reused and newly issued NET handles;
the fake rejects a NET creation without its customer and customer deletion while
its NET still exists. This verifies dependency ordering in CI.

The native test pins all mutations to `reg.ote.arin.net`, an available test range,
and handles it created. It records mutation intent before sending the request,
then records returned handles in a private local receipt. A lost or unconfirmed
response blocks further writes. An existing receipt blocks a fresh run until
reconciled, preventing repeated uncertain creations. Receipts are removed only
after all recorded customer and NET handles are confirmed absent.

Receipts live under the user's cache directory as
`terraform-provider-arin/ote-customer-net-<org-hash>-v4.json` or `-v6.json`.
They contain operation metadata and object handles, never credentials. Do not
remove a pending receipt to retry a test; reconcile its recorded operation first.

```sh
ARIN_OTE_WRITE_TESTS=1 TF_ACC=1 ARIN_TEST_ORG_HANDLE=FT-684 \
  go test ./internal/provider -run '^TestOTECustomerNetGraphLifecycle$' \
  -v -count=1 -timeout 5m
```

The command also requires `ARIN_OTE_API_KEY` in the environment. `make testote`
includes this test in its serialized sandbox suite. Production resources are not
used by this test.

## Customer endpoint audit

The current [Reg-RWS methods](https://www.arin.net/resources/registry/regrws/methods/#customers)
and [customer payload](https://www.arin.net/resources/registry/regrws/payloads/#customer-payload)
were checked on 2026-09-23 against the client and provider:

| Operation | Implementation | Evidence |
| --- | --- | --- |
| GET `/rest/customer/HANDLE` | `arin_customer` data source and resource refresh | Standalone and graph tests |
| POST `/rest/net/PARENT/customer` | Resource create | Both OT&E families and replacement |
| PUT `/rest/customer/HANDLE` | Resource update, preserving server identity/date | Standalone name/privacy/comments and graph address tests |
| DELETE `/rest/customer/HANDLE` | Resource destroy | Standalone and graph cleanup, confirmed by GET |

Name, address lines, city, subdivision, postal code, country code, comments and
privacy are configurable. Handle, registration date, parent organization and
country name are read from ARIN. The client refreshes server identity before PUT.
Country `code3` and `e164` response metadata are not yet exposed as typed customer
attributes; retain that item in the final field audit. This endpoint audit does
not claim completion of the remaining provider-wide API inventory.
