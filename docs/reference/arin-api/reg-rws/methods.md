# RESTful Methods

Source: https://www.arin.net/resources/registry/regrws/methods/

Retrieved: 2026-09-22T22:50:51.079954+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

As part of the 28 July 2026 release, ARIN’s RESTful API now supports the preferred method of sending your API key as a header, instead of within the URL. This endpoint is designed to keep your API key more private and secure, by transmitting it within the call and not allowing it to be captured at any point in the process. ARIN still accepts RESTful calls with the API key in the URL.

## Introduction

A method is a type of interaction, or “call,” with ARIN’s database records using Reg-RWS. When sending URLs through Reg-RWS, users must also specify the method of interaction their specific call uses.

* `GET` retrieves information about a record
* `POST` creates a new record
* `PUT` modifies an existing record
* `DELETE` deletes a record

Each of the following entries describes a database interaction, the appropriate method to use, the proper URL structure, and which payloads will be sent and/or returned to the user.

The calls are listed in alphabetical order under the type of record.

## as-set (IRR)

Visit the [ARIN IRR RESTful API User Guide](https://www.arin.net/resources/manage/irr/irr-restful/#as-set-objects).

## aut-num (IRR)

Visit the [ARIN IRR RESTful API User Guide](https://www.arin.net/resources/manage/irr/irr-restful/#aut-num-objects).

## Customers

Customers are organizations associated with ARIN through a provider organization. While searchable in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/), customers may only have authority over a single network registration: a simple reassignment. Customers may also be made private to keep contact information hidden. RESTful calls relating to customers usually resemble Org calls, apart from the different payload being returned. You need to reference the network that will be reassigned when creating a customer; visit [NETS](https://www.arin.net/resources/registry/regrws/methods/#nets) for more information.

### Get Customer Information

This call will return a payload containing details about the customer specified in your URL.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/customer/CUSTOMERHANDLE  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/customer/CUSTOMERHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)

### Create Recipient Customer

This call will create an Org using the details in the [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload) you provide, and the handle in your URL specifying the parent NET from which this Org will be receiving a simple reassignment. This call will then return a payload containing information about this newly-created customer.

**Note:** Be sure your API key is linked to a POC that is either the Admin or Tech POC for the Org holding the parent NET, or the Tech POC for the NET itself. Otherwise, you will receive an error.

**API Key in Header (Recommended)**

**Method:** `POST`  
**URL:** /rest/net/PARENTNETHANDLE/customer  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)  
**Returns:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)

**API Key in URL (Supported)**

**Method:** `POST`  
**URL:** /rest/net/PARENTNETHANDLE/customer?apikey=APIKEY  
**Content:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)  
**Returns:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)

### Delete Customer

This call will delete the customer specified in your URL, and return a payload containing that customer’s information as it existed just before deletion.

**API Key in Header (Recommended)**

**Method:** `DELETE`  
**URL:** /rest/customer/CUSTOMERHANDLE  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)

**API Key in URL (Supported)**

**Method:** `DELETE`  
**URL:** /rest/customer/CUSTOMERHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)

### Modify Customer

This call will modify the details of the customer specified in your URL. When using this call, attach a [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload) containing the details of the customer you intend to modify. To ensure accuracy, use [Get Customer](https://www.arin.net/resources/registry/regrws/methods/#customers-get) to get the most current information before making changes. This call returns a payload containing details about the customer as it exists after modification.

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/customer/CUSTOMERHANDLE  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)  
**Returns:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/customer/CUSTOMERHANDLE?apikey=APIKEY  
**Content:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)  
**Returns:** [Customer Payload](https://www.arin.net/resources/manage/regrws/payloads/#customer-payload)

## Delegations

Delegations are entries that relate IP addresses to domain names using the Domain Name System (DNS) of the Internet. Delegations contain the information necessary for Reverse DNS, including the associated nameservers, and DNS Delegation Signer (DS Record) information. Delegations may not be created or deleted independently. They appear and disappear automatically alongside their associated networks. Unlike the other objects, delegations are not given a handle. They are searched for within [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) using a delegation name; for example, 0.192.in-addr.arpa.

**Note:** Any changes made to delegations using these calls may take up to 24 hours to appear in the DNS.

### Get Delegation Information

This call will return a payload containing details about the delegation specified in your URL.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/delegation/DELEGATIONNAME   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/delegation/DELEGATIONNAME?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)

### Modify Delegation

This call will modify the details of the delegation specified in your URL. When making this call, attach a [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload) containing the details of the delegation you intend to modify. To ensure accuracy, use [Get Delegation](https://www.arin.net/resources/registry/regrws/methods/#delegations-get) to get the most current information before making changes. This call returns a payload containing that delegation’s information as it exists after modification.

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/delegation/DELEGATIONNAME   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)  
**Returns:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/delegation/DELEGATIONNAME?apikey=APIKEY  
**Content:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)  
**Returns:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)

### Add or Update Nameserver to Delegation

This call will add or update the single nameserver specified in your URL to the delegation specified in your URL, and return a payload containing that delegation’s information after the nameserver has been added or updated.

**API Key in Header (Recommended)**

**Method:** `POST`  
**URL:** /rest/delegation/DELEGATIONNAME/nameserver/NAMESERVER?ttl=TTL   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)

**API Key in URL (Supported)**

**Method:** `POST`  
**URL:** /rest/delegation/DELEGATIONNAME/nameserver/NAMESERVER?apikey=APIKEY&ttl=TTL  
**Content:** NONE  
**Returns:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)

### Delete Nameserver from Delegation

This call will delete the single nameserver specified in your URL from the delegation specified in your URL, and return a payload containing that delegation’s information after the nameserver has been deleted.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**API Key in Header (Recommended)**

**Method:** `DELETE`  
**URL:** /rest/delegation/DELEGATIONNAME/nameserver/NAMESERVER   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)

**API Key in URL (Supported)**

**Method:** `DELETE`  
**URL:** /rest/delegation/DELEGATIONNAME/nameserver/NAMESERVER?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)

### Delete Multiple Nameservers from Delegation

This call will delete all nameservers from the delegation specified in your URL, and return a payload containing that delegation’s information after the nameservers have been deleted.

**API Key in Header (Recommended)**

**Method:** `DELETE`  
**URL:** /rest/delegation/DELEGATIONNAME/nameservers   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)

**API Key in URL (Supported)**

**Method:** `DELETE`  
**URL:** /rest/delegation/DELEGATIONNAME/nameservers?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload)

## NETS

Network records (NETs) define a range of IPv4 or IPv6 addresses and show the organizations and POCs with authority over them. NETs may contain multiple netblocks in a case where a single range covers multiple Classless Inter-Domain Routing (CIDR) prefixes.

There are three main types of resource records within the database:

**Direct Allocation**: IP address space allocated directly from ARIN to an organization. The organization may reallocate or reassign that space to downstream customers.

**Reallocation**: IP address space allocated from an organization (the upstream) to a downstream customer. The downstream customer may reallocate or reassign that space further.

**Reassignment**: IP address space assigned from an organization (the upstream) to a downstream customer for its own exclusive use. Reassignments are one of these types:

* Detailed Reassignment: Address space assigned to a customer who may need to:

  + subdelegate the addresses to their own customers
  + maintain their own in-addr.arpa or ipv6.arpa delegations
  + display their own point of contact (POC) information
* Simple Reassignment: Address space assigned to a customer that does not require the above capabilities.

**Note:** The following NET calls work with IPv4 and IPv6 addresses.

### Get NET Information

This call will return a payload containing details about the NET specified in your URL.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/net/NETHANDLE  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/net/NETHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)

### Delete NET

This call will generate an automatically-processed ticketed request to delete the NET specified in your URL. If that NET has no NETs reallocated or reassigned from them, this call will return a [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload) with an embedded [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload) containing details of the deleted network.

If the NET being deleted has NETs under it that are reallocations or reassignments made from within that network range, this method will generate a ticketed request to delete the NET specified in your URL, and will return a [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload) with an embedded [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) containing the details of the request. Wait until that ticket is successfully processed before reissuing that space to other customers.

This call will only delete reassignments or reallocations. If attempted on a direct allocation NET, you will receive an [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload). To return direct allocations, use Ask ARIN in ARIN Online to create a ticket to request assistance.

**Note:** Deleting a NET does not delete its associated Org ID. This must be done separately. When deleting a customer NET obtained via a simple reassignment, you must also delete the customer separately.

**API Key in Header (Recommended)**

**Method:** `DELETE`  
**URL:** /rest/net/NETHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload)

**API Key in URL (Supported)**

**Method:** `DELETE`  
**URL:** /rest/net/NETHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload)

### Remove NET

This call will remove the network from the ARIN database. It is only applicable for reallocations or reassignments. It differs from the [Delete NET](https://www.arin.net/resources/manage/regrws/methods/#delete-net) call in that attachments can be sent using the NET payload.

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/net/NETHANDLE/remove   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)  
**Returns:** [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/net/NETHANDLE/remove?apikey=APIKEY  
**Content:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)  
**Returns:** [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload)

### Modify NET

This call will modify the details of the NET specified in your URL. When making this call, attach a [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload) containing the details of the NET you intend to modify. To ensure accuracy, use [Get NET](https://www.arin.net/resources/registry/regrws/methods/#net-get) to get the most current information before making changes. This call returns a payload containing that NET’s information as it exists after modification.

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/net/NETHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)  
**Returns:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/net/NETHANDLE?apikey=APIKEY  
**Content:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)  
**Returns:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)

### Get Delegations of a NET

This call will return a list of [Delegation Payloads](https://www.arin.net/resources/manage/regrws/payloads/#delegation-payload) containing information about delegations made from the NET specified in your URL.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/net/NETHANDLE/delegations  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Payload List of Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/net/NETHANDLE/delegations?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Payload List of Delegation Payload](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload)

### Get Parent of a NET

This call finds the parent of the network represented by the start and end IP range and returns a NET payload containing the details of the parent NET.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/net/parentNet/STARTIP/ENDIP   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/net/parentNet/STARTIP/ENDIP?apikey=APIKEY  
**Content:** NONE  
**Returns:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)

### Get Routes for a NET

Visit [Viewing a List of route Objects for a NET](https://www.arin.net/resources/manage/irr/irr-restful/#viewing-a-list-of-route-objects-for-a-net).

### Get Routes for Reassignments of a NET

Visit [Viewing a List of route Objects for Reassignments of a NET](https://www.arin.net/resources/manage/irr/irr-restful/#viewing-a-list-of-route-objects-for-reassignments-of-a-net).

### Get Network Details of a Start and End IP Range

This call finds the network details related to the start and end IP range.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/net/netsByIpRange/STARTIP/ENDIP   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Payload List of Net Payload](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/net/netsByIpRange/STARTIP/ENDIP?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Payload List of Net Payload](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload)

### Get Network Details of a Start and End IP Range (Most Specific)

This call finds the network details related to the start and end IP range. If multiple networks exist for the same IP range, then the most specific network is returned.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/net/mostSpecificNet/STARTIP/ENDIP   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/net/mostSpecificNet/STARTIP/ENDIP?apikey=APIKEY  
**Content:** NONE  
**Returns:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)

### Reassign NET

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/net/PARENTNETHANDLE/reassign   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)  
**Returns:** [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/net/PARENTNETHANDLE/reassign?apikey=APIKEY  
**Content:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)  
**Returns:** [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload)

This call performs a reassignment from the NET specified in your URL using the recipient information contained in the [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload) you provide. Reassignments are given to an Org or customer for its own use and may not be reallocated or reassigned further.

There are two types of reassignments that can take place: simple and detailed.

* **Simple:** Registering resources to a customer (similar to an Org, but without customer contact information). To create a customer, use [Create Recipient Customer](https://www.arin.net/resources/registry/regrws/methods/#create-recipient-customer) or add a Customer Payload to NET Payload. Customers automatically inherit the POCs of their parent Org. To perform a simple reassignment, specify a customer, as either customer handle or customer payload, in the [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload) you send.

**Note:** Due to database constraints, a single customer may not be tied to multiple simple reassignments. To perform multiple simple reassignments to the same organization, you’ll need to create separate customers for each reassignment. Alternatively, you can have the customer create their own Org ID and then you can perform a detailed reassignment to it, as Orgs may have an unlimited amount of NETs or ASNs associated with them.

* **Detailed:** Registering resources to an Org. The recipient Org must have already created an Org ID, and that Org ID must have a validated POC associated with it. To perform a detailed reassignment, specify an Org in the [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload) you send.

If there are no errors, this call will return a [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload). This payload will contain a [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload) with the details of the reassignment. If a reassignment cannot be processed, the call will return an [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload).

A simple reassignment is rejected if:

1. The Customer specified in your payload does not exist, or cannot be created.
2. You are attempting to perform a simple reassignment using a NET that has POCs that are still associated with it.
3. The NET name specified in your payload contains characters that are not letters, numbers, hyphens, or spaces.
4. The parent NET specified in your payload is:
   * Nonexistent
   * Not active
   * Not a direct allocation nor a reallocation
5. IP addresses within the range specified in your payload extend beyond that of the parent NET.
6. IP addresses within the range specified in your payload overlap with existing reservations or registrations that have the same parent NET.
7. The API key used is not associated with an ARIN Online user who is linked to a POC with authority over the parent NET or its Org.
8. The API key specified in your URL is not active.
9. The reassignment request is smaller than a /64 of IPv6 space.

A detailed reassignment is rejected if:

1. The Org specified in your payload does not exist.
2. The Org specified in your payload does not have all of the following:
   * One Admin POC
   * At least one Tech POC
   * At least one Abuse POC
3. POCs are currently associated with the NET you are performing a detailed reassignment with (POC relationships are not allowed to be added during network creation).
4. The NET name specified in your payload contains characters that are not letters, numbers, hyphens, or spaces.
5. The parent NET specified in your payload is:
   * Nonexistent
   * Not active
   * Not a direct allocation nor a reallocation
6. IP addresses within the range specified in your payload extend beyond the range of the parent NET.
7. IP addresses within the range specified in your payload overlap existing NETs with the same parent NET.
8. The API key specified in your URL is not associated with an ARIN Online user who is linked to an Admin or Tech POC for the parent NET or its Org.
9. The API key specified in your URL is not active.
10. The Org specified in your payload is not accepting reassignments.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

### Reallocate NET

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/net/PARENTNETHANDLE/reallocate   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)  
**Returns:** [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/net/PARENTNETHANDLE/reallocate?apikey=APIKEY  
**Content:** [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload)  
**Returns:** [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload)

This call performs a reallocation from the NET specified in your URL using the recipient information contained in the [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload) that you provide. Reallocations are given to an Org for further reallocation or reassignment.

This call will return a [Ticketed Request Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticketed-request-payload) and will include a [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload) with details of the reallocation. If a reallocation is rejected, the call will return an [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload). Reallocations will be rejected if:

1. The Org specified in your payload does not exist
2. The Org specified in your payload does not have all of the following:
   * One Admin POC
   * At least one Tech POC
   * At least one Abuse POC
3. POCs are currently associated with the NET you are performing a detailed reassignment with (POC relationships are not allowed to be added during network creation).
4. The NET name specified in your payload contains characters that are not letters, numbers, hyphens, or spaces.
5. The parent NET specified in your payload is:
   * Nonexistent
   * Not active
   * Not a direct allocation nor a reallocation
6. IP addresses within the range specified in your payload extend beyond the range of the parent NET.
7. IP addresses within the range specified in your payload overlap existing NETs with the same parent NET.
8. The API key specified in your URL is not associated with an ARIN Online user who is linked to an Admin or Tech POC for the parent NET or its Org.
9. The API key specified in your URL is not active.
10. The Org specified in your payload is not accepting reassignments.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

## Orgs

An Org represents an organization that is registered in the ARIN database, and shows the entity’s name, its physical address, and any POCs with authority over it. All Internet number resources directly allocated from ARIN, as well as any downstream resources, must be registered to the appropriate Org. Therefore, you must create an Org before requesting resources. Any entity may maintain multiple Orgs for different accounts or consolidate all of its resources under a single Org. For each Org, there must be at least one Admin, Tech, and Abuse POC with authority over it (NOC POCs are optional).

### Create Org for Direct Allocation

This call will generate a ticket using the details in the Org Payload you provide. This ticket will be reviewed by ARIN staff. The call will return a payload containing information about this ticket, which you can use to retrieve that ticket’s status. You can only create Orgs for your own organization, when you intend to request a direct allocation from ARIN or from your upstream provider for that Org.

If Reg-RWS returns an error code and/or Error Payload to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**Note:** If the API key specified in your URL is not linked to an Admin or Tech POC for your Org, the Org will still be created, but you will not have authorization to make changes to it.

**API Key in Header (Recommended)**

**Method:** `POST`  
**URL:** /rest/org   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

**API Key in URL (Supported)**

**Method:** `POST`  
**URL:** /rest/org?apikey=APIKEY  
**Content:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

### Get Org Information

This call will return a payload containing details about the Org specified in your URL.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/org/ORGHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/org/ORGHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)

### Get a List of route Objects for an Org ID

Visit [Viewing a List of route Objects for an Org ID](https://www.arin.net/resources/manage/irr/irr-restful/#viewing-a-list-of-route-objects-for-an-org-id).

### Get a List of route-set Objects for an Org ID

Visit [Viewing a List of route-set Objects for an Org ID](https://www.arin.net/resources/manage/irr/irr-restful/#viewing-a-list-of-route-set-objects-for-an-org-id).

### Get a List of as-set Objects for an Org ID

Visit [Viewing a List of as-set Objects for an Org ID](https://www.arin.net/resources/manage/irr/irr-restful/#viewing-a-list-of-as-set-objects-for-an-org-id).

### Get a List of aut-num Objects for an Org ID

Visit [Viewing a List of aut-num Objects for an Org ID](https://www.arin.net/resources/manage/irr/irr-restful/#viewing-a-list-of-aut-num-objects-for-an-org-id).

### Delete Org

This call will delete the Org specified in your URL, and return a payload containing that Org’s information as it existed just before deletion.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**API Key in Header (Recommended)**

**Method:** `DELETE`  
**URL:** /rest/org/ORGHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)

**API Key in URL (Supported)**

**Method:** `DELETE`  
**URL:** /rest/org/ORGHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)

### Modify Org

This call will modify the details of the Org specified in your URL. When making this call, attach an [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload) containing the details of the Org you intend to modify. To ensure accuracy, use [Get Org](https://www.arin.net/resources/registry/regrws/methods/#org-get) to get the most current information before making changes. This call returns a payload containing that Org’s information as it exists after modification. If you wish only to add or remove one or multiple POCs from an Org, you can use [Add POC](https://www.arin.net/resources/registry/regrws/methods/#add-poc) or [Remove POC](https://www.arin.net/resources/registry/regrws/methods/#remove-poc), as they do not require formatting/sending a payload.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**Note:** If any POC information is provided in the Org payload of the PUT request, all current POCs associated with the Org will be removed, and replaced with those provided in the request.

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/org/ORGHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)  
**Returns:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/org/ORGHANDLE?apikey=APIKEY  
**Content:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)  
**Returns:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)

### Remove POC from Org

This call will remove one or multiple POCs from the Org specified in your URL. At least one of the two POC identifiers in your URL (POCHANDLE and POCFUNCTION) must be included. These fields are found on the Org Payload for the Org you intend to alter. POCHANDLE defines a particular POC’s identity in the database. POCFUNCTION defines its role (for example, Abuse POC, Admin POC, or Tech POC).

* If you do not include POCHANDLE, every POC matching the POCFUNCTION you specify will be removed.
* If you do not include POCFUNCTION, every POC matching the POCHANDLE you specify will be removed.

Once the POCs have been removed, the call will return an [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload) with information about the Org as it appears in the database after the POC has been deleted.

**Note:** Every Org must have at least one Tech POC. If you attempt to remove all Tech POCs from an Org, you will receive an error.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**API Key in Header (Recommended)**

**Method:** `DELETE`  
**URL:** /rest/org/ORGHANDLE/poc/POCHANDLE;pocFunction=POCFUNCTION   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)

**API Key in URL (Supported)**

**Method:** `DELETE`  
**URL:** /rest/org/ORGHANDLE/poc/POCHANDLE;pocFunction=POCFUNCTION?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)

### Add POC to Org

This call will add a POC to an Org under a POC type (for example, Tech POC or Admin POC). Your URL must include the POC, Org, and POC type. Once complete, this call will return an Org Payload containing information about the Org to which you have added POCs. This payload will contain a list of all POCs assigned to that Org, in order of function.

**Note:** All Org IDs must have exactly one Admin POC. If you attempt to add or change an Admin POC, you will receive an error. To change an Admin POC, perform a [Modify Org](https://www.arin.net/resources/registry/regrws/methods/#org-modify) call instead.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/org/ORGHANDLE/poc/POCHANDLE;pocFunction=POCFUNCTION   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/org/ORGHANDLE/poc/POCHANDLE;pocFunction=POCFUNCTION?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Org Payload](https://www.arin.net/resources/manage/regrws/payloads/#org-payload)

## POCs

POCs are standalone objects that define a person, include the person’s mailing address and contact information, and contain a listing of any organizations or resources that POC has authority over. POCs can also define role accounts like noc@, abuse@, etc. An individual’s area of responsibility is defined by how his or her POC is connected to an organization or the resources of an organization. For more information on POCs, visit [Introduction to ARIN’s Database](https://www.arin.net/resources/guide/account/database/).

**Note:** POCs tied to an organization are automatically inherited by any resource that organization has authority over. You do not need to list the same POC on both an Org and its resources.

### Get POC Information

This call will return a payload containing details about the POC specified in your URL.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/poc/POCHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/poc/POCHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

### Delete POC

This call will delete the POC specified in your URL, and return a payload containing that POC’s information as it existed just before deletion.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**Note:** You may only remove POCs that are not associated with an Org or a NET. Any POC associated with an Org or NET must first be disassociated with the Org using [Modify Org](https://www.arin.net/resources/registry/regrws/methods/#org-modify), unlinked from the NET using [Modify NET](https://www.arin.net/resources/registry/regrws/methods/#modify-net), or removed from an Org using [Remove Org POC](https://www.arin.net/resources/registry/regrws/methods/#remove-poc).

**API Key in Header (Recommended)**

**Method:** `DELETE`  
**URL:** /rest/poc/POCHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

**API Key in URL (Supported)**

**Method:** `DELETE`  
**URL:** /rest/poc/POCHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

### Create POC

This call will create a POC using the details in the [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload) you provide, and will return a payload containing information about this newly-created POC. If MAKELINK is specified as *true*, the call will also link the POC to the ARIN Online user who owns the API key that is specified in the URL. If MAKELINK is omitted, or specified as *false*, then no link will be created, which means that user will not be able to make any changes to that POC.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**Note:** Only one ARIN Online user may be linked to a non-role account POC.

**API Key in Header (Recommended)**

**Method:** `POST`  
**URL:** /rest/poc;makeLink=MAKELINK   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

**API Key in URL (Supported)**

**Method:** `POST`  
**URL:** /rest/poc;makeLink=MAKELINK?apikey=APIKEY  
**Content:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

### Modify POC

This call will modify the details of the POC specified in your URL. When making this call, attach a [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload) containing the details of the POC you intend to modify. To ensure accuracy, use [Get POC](https://www.arin.net/resources/registry/regrws/methods/#poc-get) to get the most current information before making changes. This call returns a payload containing that POC’s information as it exists after modification.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**Note:** If you specify a phone number and/or email address within the POC Payload you send, all current phone numbers and/or email addresses currently listed in that POC will be replaced by those specified. To add or remove phone numbers or email addresses from a POC, use [Add Phone](https://www.arin.net/resources/registry/regrws/methods/#add-phone-to-poc), [Delete Phone](https://www.arin.net/resources/registry/regrws/methods/#delete-phone-from-poc), [Add Email](https://www.arin.net/resources/registry/regrws/methods/#add-email-to-poc) or [Delete Email](https://www.arin.net/resources/registry/regrws/methods/#delete-email-from-poc).

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/poc/POCHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/poc/POCHANDLE?apikey=APIKEY  
**Content:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

### Add Phone to POC

This call associates a phone number specified in your [Phone Payload](https://www.arin.net/resources/manage/regrws/payloads/#phone-payload) with the POC specified in your URL, and returns a payload containing information about the newly-added phone number. This call does not erase any phone numbers from the specified POC.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/poc/POCHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [Phone Payload](https://www.arin.net/resources/manage/regrws/payloads/#phone-payload)  
**Returns:** [Phone Payload](https://www.arin.net/resources/manage/regrws/payloads/#phone-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/poc/POCHANDLE/phone?apikey=APIKEY  
**Content:** [Phone Payload](https://www.arin.net/resources/manage/regrws/payloads/#phone-payload)  
**Returns:** [Phone Payload](https://www.arin.net/resources/manage/regrws/payloads/#phone-payload)

### Delete Phone from POC

This call removes one or more phone number specified in your URL from the POC specified in your URL, and returns a payload containing information about the newly-deleted phone number(s). This call does not erase any phone numbers from the specified POC. At least one of the two phone number identifiers in your URL (NUMBER and TYPE) must be included. Both fields may be found in the POC Payload for the POC you intend to alter. NUMBER refers simply to the phone number itself, while TYPE refers to the category that number fits into (O for Office, F for Fax, M for Mobile).

* If you do not include NUMBER, every phone number matching the TYPE you specify will be removed.
* If you do not include TYPE, every phone number matching the NUMBER you specify will be removed.

**Note:** NUMBER must match the existing phone number exactly, including all “+” and “-” characters.

**API Key in Header (Recommended)**

**Method:** `DELETE`  
**URL:** /rest/poc/POCHANDLE/phone/NUMBER;type=TYPE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Payload List](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload)

**API Key in URL (Supported)**

**Method:** `DELETE`  
**URL:** /rest/poc/POCHANDLE/phone/NUMBER;type=TYPE?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Payload List](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload)

### Add Email to POC

This call associates the email address specified in your URL with the POC specified in your URL, and returns a payload containing information about the POC as it exists after adding the email address.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**API Key in Header (Recommended)**

**Method:** `POST`  
**URL:** /rest/poc/POCHANDLE/email/EMAIL   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

**API Key in URL (Supported)**

**Method:** `POST`  
**URL:** /rest/poc/POCHANDLE/email/EMAIL?apikey=APIKEY  
**Content:** NONE  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

### Delete Email from POC

This call removes an email address specified in your URL from the POC specified in your URL, and returns a payload containing information about the POC as it exists after deleting the email address.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**API Key in Header (Recommended)**

**Method:** `DELETE`  
**URL:** /rest/poc/POCHANDLE/email/EMAIL   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

**API Key in URL (Supported)**

**Method:** `DELETE`  
**URL:** /rest/poc/POCHANDLE/email/EMAIL?apikey=APIKEY  
**Content:** NONE  
**Returns:** [POC Payload](https://www.arin.net/resources/manage/regrws/payloads/#poc-payload)

## Reports

The following calls provide the ability to request reports. Reassignments and Associations reports are available to all registered ARIN Online users. Note that before you can request a WhoWas report, you must [request WhoWas access](https://www.arin.net/reference/research/whowas/) and accept the [WhoWas Terms of Use](https://www.arin.net/reference/research/whowas/tou/).

### Request WhoWas ASN Report

ARIN’s WhoWas service provides authorized users access to historical registration information for a given IP address or ASN. This call will allow you to request a WhoWas Report for the ASNUMBER you specify. If successful, a [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) will be returned. You can then use [ticket calls](https://www.arin.net/resources/registry/regrws/methods/#tickets-all) to check the status of the ticket and get the attached report. Depending on system usage, the time needed to produce the report may vary.

#### Automatic Deletion

These tickets are subject to automatic deletion 90 days from the closed date. To prevent a ticket and its associated report from being automatically deleted, you must log in to ARIN Online and, in the **Retention Setting** field on the ticket, choose **Retain This Report**.

#### API Key

The APIKEY used in the request must belong to an ARIN Online user who is authorized to access WhoWas reports. The authorization process is described at: [https://www.arin.net/resources/whowas/](https://www.arin.net/reference/research/whowas/).

An attempt to request a WhoWas Report by an unauthorized user will result in an HTTP 401 (unauthorized) response containing an E\_AUTHENTICATION [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload). Invalid or inactivated APIKEYs will result in an HTTP 400 (bad request) response.

**Note:** The API key in the APIKEY field does not need to be linked to a POC associated with the ASN; you can request a WhoWas Report for any ASN.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/report/whoWas/asn/ASNUMBER   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/report/whoWas/asn/ASNUMBER?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

### Request WhoWas NET Report

ARIN’s WhoWas service provides authorized users access to historical registration information for a given IP address or ASN. This call will allow you to request a WhoWas Report for the address in the IPADDRESS field. The address must be specified as an IPv4 or IPv6 address (for example, 192.0.2.0 or 2001:DB8::), not as a handle (for example, NET-192-0-2-0-1) or as a CIDR prefix (for example, 192.0.2.0/24 or 2001:DB8::/32).

If successful, a [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) will be returned. You can then use [ticket calls](https://www.arin.net/resources/registry/regrws/methods/#tickets-all) to check the status of the ticket and get the attached report. Depending on system usage, the time needed to produce the report may vary.

#### Automatic Deletion

These tickets are subject to automatic deletion 90 days from the closed date. To prevent a ticket and its associated report from being automatically deleted, you must log in to ARIN Online and, in the **Retention Setting** field on the ticket, choose **Retain This Report**.

#### API Key

The APIKEY used in the request must belong to an ARIN Online user who is authorized to access WhoWas reports. Learn more about the [authorization process](https://www.arin.net/reference/research/whowas/).

An attempt to request a WhoWas Report by an unauthorized user will result in an HTTP 401 (unauthorized) response containing an E\_AUTHENTICATION [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload). Invalid or inactivated APIKEYs will result in an HTTP 400 (bad request) response.

**Note:** The API key in the APIKEY field does not need to be linked to a POC associated with the IP Address; you can request a WhoWas Report for any IP Address.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/report/whoWas/net/IPADDRESS   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/report/whoWas/net/IPADDRESS?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

### Request Associations Report

An Associations Report provides:

* A list of all POCs your ARIN Online account is linked to
* A list of all organizations those POCs are associated with
* A list of all number resources those POCs are associated with directly or through the organization
* The roles served by the POCs on each organization and resource (e.g., Org Admin, Resource Tech)

This call will allow you to request an Associations Report. If successful, a [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) will be returned. You can then use [ticket calls](https://www.arin.net/resources/registry/regrws/methods/#tickets-all) to check the status of the ticket and get the attached report. Depending on system usage, the time needed to produce the report may vary.

#### Automatic Deletion

These tickets are subject to automatic deletion 90 days from the closed date. To prevent a ticket and its associated report from being automatically deleted, you must log in to ARIN Online and, in the **Retention Setting** field on the ticket, choose **Retain This Report**.

#### API Key

The API key in the APIKEY field in the request must belong to your ARIN Online account. Invalid or inactivated API keys will result in an HTTP 400 (bad request) response.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/report/associations   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/report/associations?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

### Request Reassignment Report

A Reassignment Report lists all subdelegations for the requested NET handle that are registered in ARIN’s [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) directory service. ARIN provides Reassignment Reports only for network types of Direct Allocation or Reallocation. An attempt to request a report for any other type of network will result in an HTTP 400 (bad request) response. This call will allow you to request a Reassignments Report for the specified NETHANDLE.

If successful, a [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) will be returned. You can then use [ticket calls](https://www.arin.net/resources/registry/regrws/methods/#tickets-all) to check the status of the ticket and get the attached report. Depending on system usage, the time needed to produce the report may vary.

#### Automatic Deletion

These tickets are subject to automatic deletion 90 days from the closed date. To prevent a ticket and its associated report from being automatically deleted, you must log in to ARIN Online and, in the **Retention Setting** field on the ticket, choose **Retain This Report**.

#### API Key

The API key in the APIKEY field in the request must belong to an ARIN Online user that is linked to a POC that is authoritative for the requested network. An attempt to request a Reassignments Report using a non-authoritative API key will result in an HTTP 401 (unauthorized) response containing an E\_AUTHENTICATION [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload). Invalid or inactivated API keys will result in an HTTP 400 (bad request) response.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/report/reassignment/NETHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/report/reassignment/NETHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

## route and route6 (IRR)

Visit the [ARIN IRR RESTful API User Guide](https://www.arin.net/resources/manage/irr/irr-restful/#route-and-route6-objects).

## route-set (IRR)

Visit the [ARIN IRR RESTful API User Guide](https://www.arin.net/resources/manage/irr/irr-restful/#route-set-objects).

## Resource Public Key Infrastructure Objects

In the Resource Public Key Infrastructure (RPKI), a ROA is an attestation that the holder of a set of prefixes has authorized an Autonomous System to originate routes for those prefixes. An ASPA is a cryptographically signed object made by the authorized resource holder, that allows holders of Autonomous System (AS) identifiers in their capacity as Customers to authorize other ASes as their Providers.

You can use our RESTful endpoints to create ROAs, delete, and list your ROAs and ASPAs.

### Create and Delete ROAs

Visit the [ARIN RPKI RESTful API User Guide](https://www.arin.net/resources/manage/rpki/rpki-restful/).

### Get a List of ROAs for an Org

Visit the [ARIN RPKI RESTful API User Guide](https://www.arin.net/resources/manage/rpki/rpki-restful/).

### Create & Delete ASPAs

Visit the [ARIN RPKI RESTful API User Guide](https://www.arin.net/resources/manage/rpki/rpki-restful/).

### Get a List of ASPAs for an Org

Visit the [ARIN RPKI RESTful API User Guide](https://www.arin.net/resources/manage/rpki/rpki-restful/).



## Tickets

Tickets are requests for ARIN to perform a task. Some tickets are automatically processed, while others require more interaction with and/or manual approval by ARIN staff. Reg-RWS can access any ticket related to your ARIN Online account (not only tickets created using Reg-RWS). When specifying a ticket number in the URL or as a parameter for any calls, use the format YYYYMMDD-NUM (for example, 20090526-X1).

### Add Message to Ticket

This call will add the message and/or attachment from your [Message Payload](https://www.arin.net/resources/manage/regrws/payloads/#message-payload) to the Ticket specified in your URL, and return a payload containing details of that message.

**Note:** Messages cannot be attached to closed tickets.

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/ticket/TICKETNUMBER/message   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [Message Payload](https://www.arin.net/resources/manage/regrws/payloads/#message-payload)  
**Returns:** [Message Payload](https://www.arin.net/resources/manage/regrws/payloads/#message-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/ticket/TICKETNUMBER/message?apikey=APIKEY  
**Content:** [Message Payload](https://www.arin.net/resources/manage/regrws/payloads/#message-payload)  
**Returns:** [Message Payload](https://www.arin.net/resources/manage/regrws/payloads/#message-payload)

### Modify Ticket

This call will modify the status of the ticket specified in your URL. When making this call, attach a [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) containing the details of the ticket you intend to modify. To ensure accuracy, use one of the get ticket methods described in the following sections to get the most current ticket information before making changes. This call returns a payload containing that ticket’s information as it exists after modification.

**Note:** The `msgRefs` field is an optional Boolean (true/false) parameter. When true, message reference element(s) are returned instead of message element(s).The msgRefs field defaults to false if not set. To avoid passing large payloads, set `MSGREFS` to true.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/ticket/TICKETNUMBER?msgRefs=MSGREFS   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/ticket/TICKETNUMBER?apikey=APIKEY&msgRefs=MSGREFS  
**Content:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

### Modify Ticket Status

This call will modify a ticket status. The only field which is modifiable is the status and it can only be set to CLOSED if the ticket is already RESOLVED. This call does not take a payload.

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/ticket/TICKETNUMBER/ticketStatus/TICKETSTATUS?msgRefs=MSGREFS   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/ticket/TICKETNUMBER/ticketStatus/TICKETSTATUS?apikey=APIKEY&msgRefs=MSGREFS  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

### Get Ticket Details

This call will return a payload containing details about the ticket specified in your URL, including messages.

**Note:** `msgRefs` is an optional parameter in your URL, and may only be specified as “true” or “false” within your URL. If *true*, the returned [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) will contain message reference field(s) instead of message element(s). If the ticket specified in your URL has attachments totaling a size beyond the allowed threshold, you will receive a 413 (Payload Too Large) error and an [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) stating the attachment size limit has been exceeded. For these tickets, you may want to specify the `msgRefs` parameter as *true*. You can then perform a [Get Message](https://www.arin.net/resources/registry/regrws/methods/#get-ticket-message) call for the message information.

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/ticket/TICKETNUMBER?msgRefs=MSGREFS   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/ticket/TICKETNUMBER?apikey=APIKEY&msgRefs=MSGREFS  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

### Get Ticket Summary

This call will return a payload containing details about the ticket specified in your URL, without messages.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/ticket/TICKETNUMBER/summary   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/ticket/TICKETNUMBER/summary?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

### Get Ticket Payload List

This call will return a [Payload List Payload](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload) containing a [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) for each ticket associated with the API key specified in your URL.

At least one of the two ticket identifier fields in your URL (TICKETTYPE and TICKETSTATUS) must be included. These fields are found on the [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) for the ticket you are referring to (webTicketType and webTicketStatus). TICKETTYPE refers to the category in which the ticket fits (CREATE\_ROA, ASN\_REQUEST, etc.). TICKETSTATUS refers to the current state that the ticket is in (ACCEPTED, DENIED, ABANDONED, etc.).

If Reg-RWS returns an error code and/or [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) to you when performing this call, refer to the [Error Codes](https://www.arin.net/resources/registry/regrws/methods/#error-codes) section.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/ticket/TICKETNUMBER/summary   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/ticket;ticketType=TICKETTYPE;ticketStatus=TICKETSTATUS?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Payload List of Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload)

### Get Ticket Summaries

This call will return a [Payload List Payload](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload) containing a [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) without message information for each ticket associated with the API key specified in your URL.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/ticket/summary;ticketType=TICKETTYPE;ticketStatus=TICKETSTATUS   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Payload List of Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/ticket/summary;ticketType=TICKETTYPE;ticketStatus=TICKETSTATUS?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Payload List of Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#payload-list-payload)

### Get Ticket Message

This call will return a [Message Payload](https://www.arin.net/resources/manage/regrws/payloads/#message-payload) containing details about the message specified in your URL. The payload will contain [Attachment Reference Payloads](https://www.arin.net/resources/manage/regrws/payloads/#attachment-reference-payload) rather than directly containing the attachments. The [Get Ticket Attachment](https://www.arin.net/resources/manage/regrws/payloads/#get-ticket-attachment) call can be used to download a specific attachment using the AttachmentID specified in this payload. The value for the MESSAGEID may be found in the [Ticket Payload](https://www.arin.net/resources/manage/regrws/payloads/#ticket-payload) for the ticket you are retrieving a message from.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/ticket/TICKETNUMBER/message/MESSAGEID   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** [Message Payload](https://www.arin.net/resources/manage/regrws/payloads/#message-payload)

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/ticket/TICKETNUMBER/message/MESSAGEID?apikey=APIKEY  
**Content:** NONE  
**Returns:** [Message Payload](https://www.arin.net/resources/manage/regrws/payloads/#message-payload)

### Get Ticket Attachment

This call returns the contents of the attachment specified in your URL.

**Note:** HTTP Header Content-Disposition will include the filename and Content-Type will be ‘application/octet-stream’.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/ticket/TICKETNUMBER/message/MESSAGEID/attachment/ATTACHMENTID   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** Response

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/ticket/TICKETNUMBER/message/MESSAGEID/attachment/ATTACHMENTID?apikey=APIKEY  
**Content:** NONE  
**Returns:** Response

## Error Codes

The following error codes may occur while using Reg-RWS. If there is a generic status code (for example, 400 Bad Request), Reg-RWS will also return an [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) containing further details about the error and how to resolve it.

Error Codes

| Error | Meaning/Causes |
| --- | --- |
| 400 | Bad Request. This can be caused by a missing or incorrect API key, sending information that Reg-RWS cannot validate, or syntax errors within the URL/payload you sent. This error also occurs when you attempt to remove all Tech POCs from an Org, as all Orgs must have at least one Tech POC. |
| 401 | Unauthorized. The API key used in the request does not belong to an ARIN Online user that is linked to a POC with the authority to modify the specified record. Ensure that the record you specified is one that your POC has authority over, and that your ARIN Online account is linked to that POC. |
| 403 | Forbidden. This error indicates a forbidden request. For example, if you attempt to reallocate resources from a parent NET not associated with the API key specified in your URL, you will receive this error. This error also occurs if a downstream Org is not accepting reassignments. |
| 404 | Not Found. The record you identified couldn’t be found in the database. For instance, if you get a 404 while trying to modify a POC, it probably means Reg-RWS couldn’t find a matching POC in the database. If you are trying to retrieve a ticket or ticket attachment for a report (such as a WhoWas Report) that was deleted 90 days after it expired, you would also receive this error. |
| 405 | Method Not Allowed. Either the wrong method is specified, or the URL contains incorrect information. |
| 406 | Not Acceptable. The server cannot return content in the format the client is expecting. |
| 409 | Conflict. You have attempted to delete something that was still linked to something else in ARIN’s database. For example, you tried to delete POC that is still associated with an Org or NET, or you tried to delete a customer that is still linked to a NET. This error may also occur if you attempt to add a nameserver to a delegation and that nameserver already exists for that delegation, or if you attempt to remove the only nameserver from a delegation with DS records. Additionally, you will see this error if you attempt to reallocate resources that are not within the parent NET you specify. |
| 413 | Payload Too Large. This can occur if there are too many attachments on a ticket’s messages. The [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) returned to you will show the actual size limit you’ve reached. Try again with a summary to receive a smaller response. |
| 415 | Unsupported Media Type. Ensure that “application/xml” or “application/rpsl” is specified as the MIME type. |
| 429 | Too Many Requests. You have exceeded the allowed number of requests for the measured time period. |
