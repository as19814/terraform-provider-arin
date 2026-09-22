# Reg-RWS Quick Start Guide

Source: https://www.arin.net/resources/registry/regrws/quickstart/

Retrieved: 2026-09-22T22:50:50.987637+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## Overview

ARIN Online users can also use Reg-RWS in addition to the graphical user interface to retrieve and modify records within ARIN’s database by sending URLs and XML or Routing Policy Specification Language (RPSL) payloads in an automatable, computer-friendly manner using modern application interfaces that provide strong authentication. You can use Reg-RWS to perform these and other actions:

* Retrieve and modify information about delegations and networks
* Reassign and reallocate network space
* Retrieve and modify information about Orgs, Points of Contact (POCs), and customers
* Get information about tickets (requests for ARIN to perform an action) and add messages to tickets
* Submit Route Origin Authorizations (ROAs)
* Retrieve and edit Internet Routing Registry (IRR) objects
* Request reports, such as WhoWas (historical registration information) or reassignments for a given network

For more information about Reg-RWS and frequently asked questions, visit [Automating Record Management with Reg-RWS](https://www.arin.net/resources/manage/regrws/).

## Before You Begin

You’ll need the following before using Reg-RWS:

* Understanding of [ARIN’s database structure](https://www.arin.net/resources/guide/account/database/)
* [ARIN Online account](https://www.arin.net/resources/guide/account/)
* [API key](https://www.arin.net/reference/materials/security/api_keys/)
* POC account in ARIN Online

**Note:** Your POC must have the authority to manage records for an Org. (For example, you must be linked to an Admin or Tech POC for the Org whose records you are managing.) ARIN’s Reg-RWS won’t process any modifications to a database record unless you have an ARIN Online account linked to a POC with proper authority over that record.

## Reg-RWS Components

### URLs

In its simplest form, a Reg-RWS action can be a URL that is entered in a browser. The URL that is used when interacting with ARIN through Reg-RWS will specify these key things:

* The address of the database being interacted with (for example, reg.arin.net).
* The database object being interacted with (for example, a POC or NET). In addition, when performing some actions such as retrieving or deleting an object, the object **name** or **handle** must be included in the URL.

For example, to retrieve information about a specific POC, you could enter the following URL into a browser address window:

`https://reg.arin.net/rest/poc/POCHANDLE`

where the following information is included:

* Address: reg.arin.net
* Database object: POC
* Object name: the handle of the POC for which you want to view information

### Methods and Payloads

Most uses of Reg-RWS involve coding software or scripts to perform multiple transactions at one time with the ARIN database. When using Reg-RWS this way, there are multiple components to a Reg-RWS action:

* method
* URL (see previous description)
* header
* payload (only required when using some methods)

A method is a type of interaction, or “call,” with ARIN’s database records, within Reg-RWS. When sending URLs through Reg-RWS, users must also specify the method of interaction their specific call uses. Methods include the following:

* GET: retrieves information about a record
* POST: creates a new record
* PUT: modifies an existing record
* DELETE: deletes a record

As part of the 28 July 2026 release, ARIN’s RESTful API now supports the preferred method of sending your API key as a header, instead of within the URL. This endpoint is designed to keep your API key more private and secure, by transmitting it within the call and not allowing it to be captured at any point in the process. ARIN still accepts RESTful calls with the API key in the URL.

When using methods that alter database records without deleting them (for example, PUT and POST), Reg-RWS will require more information than the URL provides. For example, if you wish to create or change contact information for a POC or ORG, Reg-RWS will require you to provide that information in a computer-friendly manner. To accomplish this, you must send formatted XML layouts known as **payloads**. These payloads contain details about the record being created or modified, such as organization information, POC information, IP address ranges, and customer data. To ensure that your payload is current, perform a GET before performing a PUT or POST with a given record.

## Reg-RWS Examples

A few examples of common Reg-RWS transactions are listed. ARIN provides full information about methods and payloads for Reg-RWS users. The [Methods Page](https://www.arin.net/resources/manage/regrws/methods/) describes each database interaction, the appropriate method and URL structure, and which payloads will be sent and returned. The [Payloads Page](https://www.arin.net/resources/manage/regrws/payloads/) details each payload structure, provides examples of completed payloads, and explains common error codes returned by Reg-RWS when it receives information it cannot understand or process correctly.

### Getting Information About a POC

You want to get information about a POC. The following table shows the method, URL, and other information. Because you are only retrieving information about a POC, there is no content (payload). The Returns field specifies the payload that you will see returned from Reg-RWS upon sending a correct URL using the correct method.

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

Returns the details of a POC record. If no POC can be found with the handle specified, an exception containing an [Error Payload](https://www.arin.net/resources/manage/regrws/payloads/#error-payload) will be thrown.

Command Example

```
GET http://reg.arin.net/rest/poc/DOEJA5-ARIN?apikey=API-E85E-A05C-4A23-BB9C
```


Return Payload Example

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<poc
  xmlns="http://www.arin.net/regrws/core/v1">
  <city>Edmond</city>
  <companyName>Test Company</companyName>
  <contactType>PERSON</contactType>
  <iso3166-1>
    <code2>US</code2>
    <code3>USA</code3>
    <e164>1</e164>
    <name>United States</name>
  </iso3166-1>
  <emails>
    <email>jdoe1@test.com</email>
  </emails>
  <firstName>Jane</firstName>
  <lastName>Doe</lastName>
  <streetAddress>
    <line number="0">111 Test Dr.</line>
  </streetAddress>
  <phones>
    <phone>
      <number>+1-555-555-1212</number>
      <type>
        <code>O</code>
        <description>OFFICE</description>
      </type>
    </phone>
  </phones>
  <handle>DOEJA5-ARIN</handle>
  <postalCode>23344</postalCode>
  <registrationDate>2019-06-10T11:36:55.136-04:00</registrationDate>
  <iso3166-2>OK</iso3166-2>
</poc>
```

### Performing a Reassignment to a New Customer

#### Step One: Creating a Recipient Customer

You want to create a customer who will be receiving a simple reassignment from you. This call will create an Org using the details in the Customer Payload you provide and the handle in your URL specifying the parent NET from which this Org will be receiving a simple reassignment. This call will then return a payload containing information about this newly created Customer. Customers automatically inherit the POCs of their parent Org.

**Note:** Be sure your API Key is linked to a POC that is either the Admin or Tech POC for the Org holding the parent NET, or the Tech POC for the NET itself. Otherwise you will receive an error.

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

Command Example

```
POST http://reg.arin.net/rest/net/NET6-2605-49C0-1/customer?apikey=API-E85E-A05C-4A23-BB9C
```


Content Payload Example

```
<customer
  xmlns="http://www.arin.net/regrws/core/v1" >
  <customerName>NewCustomer</customerName>
  <iso3166-1>
    <name>UNITED STATES</name>
    <code2>US</code2>
    <code3>USA</code3>
    <e164>1</e164>
  </iso3166-1>
  <handle></handle>
  <streetAddress>
    <line number = "1">Line 1</line>
  </streetAddress>
  <city>Chantilly</city>
  <iso3166-2>VA</iso3166-2>
  <postalCode>20151</postalCode>
  <comment>
    <line number = "1">Line 1</line>
  </comment>
  <parentOrgHandle>TI-615</parentOrgHandle>
  <privateCustomer>false</privateCustomer>
</customer>
```


Return Payload Example

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<customer
  xmlns="http://www.arin.net/regrws/core/v1">
  <city>Chantilly</city>
  <iso3166-1>
    <code2>US</code2>
    <code3>USA</code3>
    <e164>1</e164>
    <name>United States</name>
  </iso3166-1>
  <handle>C07361391</handle>
  <comment>
    <line number="0">Line 1</line>
  </comment>
  <streetAddress>
    <line number="0">Line 1</line>
  </streetAddress>
  <customerName>NewCustomer</customerName>
  <parentOrgHandle>TI-615</parentOrgHandle>
  <postalCode>20151</postalCode>
  <privateCustomer>false</privateCustomer>
  <registrationDate>2019-06-12T09:22:58-04:00</registrationDate>
  <iso3166-2>VA</iso3166-2>
</customer>
```

#### Step Two: Reassign Space to the Newly Created Customer

You want to perform a simple reassignment to the customer you created in Step One. This call performs a reassignment from the NET specified in your URL using the recipient information contained in the [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload) you provide. Reassignments are given to an Org or Customer for their own use and may not be reallocated or reassigned further. To perform a simple reassignment, specify a Customer in the [NET Payload](https://www.arin.net/resources/manage/regrws/payloads/#net-payload) you send.

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

Command Example

```
PUT https://reg.arin.net/rest/net/NET6-2605-49C0-1/reassign?apikey=API-E85E-A05C-4A23-BB9C
```


Content Payload Example

```
<net
  xmlns="http://www.arin.net/regrws/core/v1" >
  <version>6</version>
  <comment>
    <line number = "1">Line 1</line>
  </comment>
  <registrationDate></registrationDate>
  <orgHandle></orgHandle>
  <handle></handle>
  <netBlocks>
    <netBlock>
      <type>S</type>
      <description>DESCRIPTION</description>
      <startAddress>2001:db8::</startAddress>
      <endAddress></endAddress>
      <cidrLength>36</cidrLength>
    </netBlock>
  </netBlocks>
  <customerHandle>C07361391</customerHandle>
  <parentNetHandle>NET6-2605-49C0-1</parentNetHandle>
  <netName>netname</netName>
</net>
```


Return Payload Example

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<ticketedRequest
  xmlns="http://www.arin.net/regrws/core/v1"
  xmlns:ns2="http://www.arin.net/regrws/messages/v1"
  xmlns:ns3="http://www.arin.net/regrws/shared-ticket/v1">
  <net>
    <pocLinks/>
    <customerHandle>C07361391</customerHandle>
    <comment>
      <line number="0">Line 1</line>
    </comment>
    <netBlocks>
      <netBlock>
        <cidrLength>36</cidrLength>
        <description>Reassigned</description>
        <endAddress>2001:db8:0FFF:FFFF:FFFF:FFFF:FFFF:FFFF</endAddress>
        <startAddress>2001:db8:0000:0000:0000:0000:0000:0000</startAddress>
        <type>S</type>
      </netBlock>
    </netBlocks>
    <handle>NET6-2605-49C0-2</handle>
    <netName>NETNAME</netName>
    <originASes/>
    <parentNetHandle>NET6-2605-49C0-1</parentNetHandle>
    <registrationDate>2019-06-12T09:35:14-04:00</registrationDate>
    <version>6</version>
  </net>
</ticketedRequest>
```

## Common Reg-RWS Tasks

Some of the common Reg-RWS tasks you might want to perform are listed below. Click the links to see syntax information for each task.

* [Reporting Reassignments](https://www.arin.net/resources/manage/regrws/methods/#reassign-net)
* [Reporting Reallocations](https://www.arin.net/resources/manage/regrws/methods/#reallocate-net)
* [Requesting Reassignment Reports](https://www.arin.net/resources/manage/regrws/methods/#request-reassignment-report)
* [Requesting Associations Reports](https://www.arin.net/resources/manage/regrws/methods/#request-associations-report)
* Requesting WhoWas Reports (Note: These only contain the data that would have been publicly viewable in Whois.)
  + [WhoWas NET Reports](https://www.arin.net/resources/manage/regrws/methods/#request-whowas-net-report)
  + [WhoWas ASN Reports](https://www.arin.net/resources/manage/regrws/methods/#request-whowas-asn-report)
