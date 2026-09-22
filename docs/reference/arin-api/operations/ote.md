# Operational Test and Evaluation (OT&E) Environment

Source: https://www.arin.net/reference/tools/testing/

Retrieved: 2026-09-22T22:50:52.905916+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

The Operational Test & Evaluation (OT&E) testbed is an environment with the same feature set as the ARIN production services. The OT&E environment allows customers to experiment with ARIN services via the ARIN Online web interface and our Restful API.

**Note:** OT&E exists solely for experimental usage and research and is not linked to ARIN’s production system. User data (including API keys) is copied from the production database to OT&E monthly. Email interactions are not supported in OT&E.

## OT&E Services

| Service | What You Can Test |
| --- | --- |
| **ARIN Online (web interface)** | Create and modify Org, POC, and NET records; submit and track requests; explore reassignments and reallocations; manage resource records  -  all through the same web UI as production |
| **Registration RESTful Web Service (Reg-RWS)** | Automate the same actions as ARIN Online via RESTful API: Org, POC, NET, and Customer payloads; reallocations; reassignments; ticket management; IRR route objects |
| **Whois RESTful Web Service (Whois-RWS)** | Query and validate resource records, POC data, and Org information against a production snapshot |
| **Registration Data Access Protocol (RDAP)** | Test Registration Data Access Protocol queries and integrations before pointing at production |
| **Internet Routing Registry (IRR)** | Create, modify, and delete route objects; test IRR automation workflows against real-looking data |
| **Resource Public Key Infrastructure (RPKI)** | Create test ROAs, validate routing security configurations, and explore RPKI workflows |
| **DNSSEC/Reverse DNS** | Test delegation payloads, nameserver configurations, and DNS-related record management |

## Benefits of Using OT&E

* Testing can be done without affecting production data.
* With the API, users can create and validate the payloads of any RPKI transactions before making changes to their live configurations.

## Prerequisites

Before using OT&E, you need the following:

* **[ARIN Online user account](https://www.arin.net/resources/guide/account/):** This account must have been created before the first of the month in which you are using OT&E. User accounts, including API keys, are copied from the production environment each month.
* **Authority over resources in ARIN Online:** Your user account in ARIN Online must be linked to a Point of Contact associated with an Org with resources.
* **API key:** You must have an [API key for your user account](https://www.arin.net/reference/materials/security/api_keys/) to use RESTful services in OT&E. You may create a different API key for OT&E than the API key you use in the production system. However, you will have to change your API key after each monthly database refresh.

## OT&E Data Refresh

On or around the first Monday of every month, data in OT&E is replaced by a snapshot of the ARIN Online live environment. You will need to reconfigure any changes that you made prior to the refresh in OT&E, if necessary, to continue your testing.

## Working with ARIN Staff for OT&E Requests

**The OT&E environment is not actively monitored by ARIN Staff**. If you need to request a certificate, re-enroll resources into the test environment, or modify your account to use OT&E, you must submit an [Ask ARIN](https://account.arin.net/public/communication/message/beginQuestion.xhtml) in both the production environment as well as the OT&E environment.

Follow these steps to make your request:

1. Use Ask ARIN in the OT&E environment to create a ticket.
2. Log in to ARIN Online in the production environment ([www.arin.net](https://www.arin.net)).
3. Use Ask ARIN to create a ticket. Be sure to use the following:

   * Topic: Other
   * Subject: OT&E approval requested
   * Question: Provide the OT&E ticket number or function that ARIN staff needs to process for you.

**Note**: If you wish to test a different RPKI deployment type in OT&E, you will need to submit an Ask ARIN ticket to request that your RPKI configuration be deleted for your organization. Then, follow the steps to sign up for RPKI and select the RPKI deployment type you wish to test.

## OT&E Hosts

The following URLs should be used when interacting with ARIN’s OT&E services in place of their production counterparts:

### ARIN Online

* `www.ote.arin.net`

### API Endpoints

* `whois.ote.arin.net`: Whois-RWS
* `reg.ote.arin.net`: Reg-RWS and the IRR RESTful API
* `rpki.ote.arin.net`: RPKI
* `updown.ote.arin.net`: Up/Down RPKI
* `rdap.ote.arin.net`: RDAP

## Using ARIN Online in OT&E

The address `www.ote.arin.net` is used to reach the OT&E ARIN Online service. It provides a snapshot of production data and allows you to perform the same functions, but in a test environment.

## Using Whois-RWS in OT&E

`whois.ote.arin.net` is used to access Whois-RWS in OT&E; this functionality within OT&E is a monthly snapshot of production, refreshed the first Monday of each month. Remember to use `http://whois.ote.arin.net` with your RESTful calls to make changes in the OT&E. For more information on the Whois-RWS API, visit [Whois-RWS API Documentation)](https://www.arin.net/resources/registry/whois/rws/api/).

## Using Reg-RWS and the IRR RESTful API in OT&E

The `reg.ote.arin.net` URL provides Reg-RWS and IRR RESTful API functionality within OT&E. This system is a snapshot of production. Remember to use `https://reg.ote.arin.net` with your RESTful calls to make changes in the OT&E. For more information on Reg-RWS usage, visit [Automating Record Management with Reg-RWS](https://www.arin.net/resources/registry/regrws/). For more information on the IRR RESTful API, visit [IRR RESTful API](https://www.arin.net/resources/manage/irr/irr-restful/).

## Using RPKI in OT&E

### Signing up for RPKI in OT&E

If your resources are not covered under RPKI in production, log in to ARIN Online in the OT&E environment, choose your deployment option, and [follow the steps to configure RPKI](https://www.arin.net/resources/manage/rpki/options/).

**Note**:
Customers who are enrolled in Delegated (not Hosted) RPKI and would like to perform testing in OT&E will need to take additional steps after each monthly refresh. Please re-exchange the `identity.xml` in the OT&E public front-end and force your software to perform an issue request.

### OT&E Trust Anchor Locator (TAL)

The OT&E TAL is used with an RPKI validator to allow for the fetching and validation of ARIN OT&E repository objects. In order to validate your ROAs using ARIN’s OT&E data, you must use the [ARIN OT&E TAL](https://www.arin.net/reference/tools/testing/arin_ote.tal).

## Mailing List

ARIN encourages all OT&E users to subscribe and participate on the [ARIN Technical Discussions](https://www.arin.net/participate/community/mailing_lists/#technical-discussions) mailing list for information sharing and outage information.
