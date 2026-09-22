# ARIN IRR RESTful API User Guide

Source: https://www.arin.net/resources/manage/irr/irr-restful/

Retrieved: 2026-09-22T22:50:51.343989+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

ARIN provides a way to automate creation and maintenance of Internet Routing Registry (IRR) records using an application programming interface (API) to ARIN’s database. (For an overview of ARIN’s IRR and additional information on how to submit objects and perform queries, visit the [Internet Routing Registry](https://www.arin.net/resources/manage/irr/) page.)

You can use Representational State Transfer (REST) commands to add, update, or delete IRR objects. These commands (also known as REST calls) are entered in a script or configured on the back end of a custom software program and are then submitted to the ARIN database to create, view, update, or delete records.

RESTful commands are structured to include the following:

* the command method (such as `GET`, `POST`, and `PUT`)
* a URL, or address of the RESTful server
* other information, including your API key
* (Optional) payloads

The command structure and payload formats are described in more detail in the following sections.

## IRR REST Commands: Format and Example

IRR REST commands (also referred to as “calls”) generally have the format of `method /URL/<object>/<objectID>/<objectName>` where:

* `method` is one of the following:
  + `GET` retrieves information about a record
  + `POST` creates a new record
  + `PUT` modifies an existing record
  + `DELETE` deletes a record
* `URL` is the address of ARIN’s IRR RESTful server, `https://reg.arin.net/rest/irr`
* `object` is the type of IRR object:
  + route
  + route6
  + aut-num
  + as-set
  + route-set
* `<objectID>` is the identifying information for the object. This can contain multiple parts divided by the `/` character (for example, the IP address block, in CIDR format, of a route would have a `/` character in between the IPv4 address and prefix length, or block size).
* `<objectName>` is the name you have given the object

Additionally, the `<api-key>` will be included in the header of the call. This API key is configured using the **Settings** > **Security Info** > **Manage API Keys** option in ARIN Online. Visit [API Keys](https://www.arin.net/reference/materials/security/api_keys/) for more information.

**Note**: The commands to create `as-set` and `route-set` objects are slightly different in that they append `&orgHandle=ORGHANDLE` to the end of the command; this is explained in the description of these commands.

To give an example of a typical command, the following command creates a `route6` object:

`POST https://reg.arin.net/rest/irr/route/2001:db8::/32/AS65536`

REST commands also require headers and sometimes, when you’re creating an object, require a payload or a message body that contains the information of the object you’re creating in RPSL or XML format. Commands that require a payload are noted in this document.

## Headers

IRR REST commands require these headers be used in your scripts or REST client software.

The API Key will be included in the form:

**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)

For all calls, the header must indicate either:

* `Accept: application/rpsl` or
* `Accept: application/xml`

If the call requires a payload, the header must include the `Content-Type` line (as shown in the following examples).

### RPSL Header Example

```
Accept: application/rpsl
Content-Type: application/rpsl
```

### XML Header Example

```
Authorization: ApiKey API-xxxx-xxxx-xxxx 
Accept: application/xml
Content-Type: application/xml
```

## Payloads

Some REST calls require you to format and send information to ARIN with your command in a formatted “payload” file, or message body. IRR REST commands accept payloads in the following formats:

* Routing Policy Specification Language (RPSL): Visit [RFC 2622](https://datatracker.ietf.org/doc/rfc2622) for specifics of this format. RPSL is accepted only for *IRR* REST commands.
* Extensible Markup Language (XML): This language consists of tags that identify data in the payloads, and is the language accepted for *all* of ARIN’s REST commands (including the suite of REST commands known as [Reg-RWS](https://www.arin.net/resources/manage/regrws/) that are used for tasks such as updating registration information for IP addresses and ASNs or reporting transferred resources).

### Allowable Characters in Payloads

RPSL payloads allow the following encoding:

* ASCII
* UTF-8 in the `descr:` fields (also supported by the Internet Routing Registry Daemon (IRRd) database server software)

XML payloads support ASCII encoding only; however, comment fields which support free-form text (e.g., [aut-num payloads](https://www.arin.net/resources/manage/irr/irr-restful/#creating-an-aut-num-object)) allow some UTF-8 encoding (characters for accented letters only).

If you are converting from email templates previously used with ARIN’s IRR system, you may want to format payloads in RPSL format (which is what email templates used). If you are developing new scripts to connect to ARIN’s API, you may want to use XML format for payloads.

### Payload Examples

Examples of the payload required for the `route-set` object:

RPSL Payload Example

```
route-set: RS-ARIZONA
members: 2001:db8::/32
descr: Example Corp LLC Arizona 1
descr: Phoenix AZ 33333
descr: USA
tech-c: EXAMPLETECH-ARIN
admin-c: EXAMPLEADMIN-ARIN
remarks: This is an example.
mnt-by: MNT-EXAMPLECORP
source: ARIN
```


XML Payload Example

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<routeSet xmlns="http://www.arin.net/regrws/core/v1" >
  <creationDate>2021-03-11T13:47:20Z</creationDate>
  <description>
    <line number="0">Example Corp LLC Arizona 1</line>
    <line number="1">Phoenix AZ 33333</line>
    <line number="2">United States</line>
  </description>
  <lastModifiedDate>2021-03-11T13:47:20Z</lastModifiedDate>
  <orgHandle>EXAMPLECORP</orgHandle>
  <pocLinks>
    <pocLinkRef description="Tech" function="T" handle="EXAMPLETECH-ARIN"/>
    <pocLinkRef description="Admin" function="AD" handle="EXAMPLEADMIN-ARIN"/>
  </pocLinks>
  <remarks>
    <line number="0">This is an example.</line>
  </remarks>
  <source>ARIN</source>
  <name>RS-ARIZONA</name>
  <mpMembers>
    <mpMember name="2001:db8::/32"/>
    </mpMembers>
</routeSet>
```

  

Note the following when constructing payloads:

RPSL payloads:

* The `mnt-by` field is the prefix `MNT` and the Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the `route` object. It is in the format `MNT-OrgID`; for example, `MNT-EXAMPLECORP`. The `mnt-by` field for new IRR records created using IRR-online or IRR REST commands no longer has any relationship to legacy Maintainer Objects that were migrated from ARIN’s previous IRR system in June 2020.

All payloads:

* The `source` field value must be `ARIN`. No other values are valid in this field.

## Automation and Batch Processing

Multiple REST calls can be grouped together to automate creating, updating, and deleting IRR objects with a single script, or scripts can be set up to automatically send individual REST calls to ARIN. For example, you might configure scripts on the back end of your custom network software that send REST commands to ARIN whenever route objects are created, when the name of a route is changed, or when a route is deleted. Currently, only one action is supported in a single REST call. Batch processing, or performing multiple actions with a single REST call, is not yet implemented.

## Objects

The following sections list the types of objects and how to create, view, edit, and delete the object.

### Object Permissions

Viewing, editing, and deleting of some objects is limited depending on the method used to originally create the object; refer to [this table](https://www.arin.net/resources/manage/irr/#object-permissions).

## route and route6 Objects

`route` objects are created for IPv4 routes and `route6` objects are created for IPv6 routes. The only difference between these objects (aside from the type of IP address) is the payload content.

### Creating a route Object

This call creates a `route` object. These objects specify the IPv4 address block and the ASN of the Autonomous System from which the route originates.

**API Key in Header (Recommended)**

**Method:** `POST`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** `200 OK` response and RPSL or XML of the created object

**API Key in URL (Supported)**

**Method:** `POST`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS?apikey=APIKEY  
**Content:** NONE  
**Returns:** `200 OK` response and RPSL or XML of the created object

**Additional Header**

RPSL

```
Accept: application/rpsl
Content-Type: application/rpsl
```


XML

```
Accept: application/xml
Content-Type: application/xml
```

**Content**

Payload in RPSL or XML format with the fields shown in the following tables.

RPSL Payload Fields

| Field | Definition |
| --- | --- |
| route | The IPv4 address and prefix length, in CIDR format, of the route to be originated. Together with the `origin` attribute, constitutes the primary key of the `route` object. |
| origin | The ASN from which the route originates. |
| descr | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. |
| admin-c | Admin POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. |
| tech-c | Tech POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. |
| mnt-by | The prefix `MNT` and the Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the `route` object. It is in the format `MNT-OrgID`; for example, `MNT-EXAMPLECORP`. |
| source: ARIN | Do not use any other value; all objects must have this line. |

XML Payload Fields

| Field | Definition |
| --- | --- |
| route | Use `xmlns="http://www.arin.net/regrws/core/v1".` |
| creationDate | Ignored by system. |
| description | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. Use line numbers for each line of information, as shown in the payload example. |
| lastModifiedDate | Ignored by system. |
| route | Use `xmlns="http://www.arin.net/regrws/core/v1".` |
| creationDate | Ignored by system. |
| description | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. Use line numbers for each line of information, as shown in the payload example. |
| lastModifiedDate | Ignored by system. |
| orgHandle | The Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the `route` object. |
| pocLinks | Admin (AD), Tech (T), or Routing (R) POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. Use the `pocLinkRef` tag for each individual POC, as shown in the payload example. |
| source | Must be `ARIN`. |
| netHandle | Name of the network. |
| originAS | The ASN from which the route originates. |
| prefix | The IPv4 address and prefix length, in CIDR format, of the route to be originated. Together with the `originAS` attribute, constitutes the primary key of the `route` object. |
| autoLinkedRoaHandle | This field cannot be set on create. Route/Route6 objects can only be autoLinked on ROA creation, not Route/Route6 creation. This field is only included in the list response, and only to allow a customer to keep track of what is auto linked / auto managed. |
| version | Ignored by system. |

**Payload Examples**

RPSL Payload Example

```
route: 192.0.2.0/24
origin: AS65536
descr: Example Corp LLC Arizona 1
descr: Phoenix AZ 33333
descr: USA
admin-c: EXAMPLEADMIN-ARIN
tech-c: EXAMPLETECH-ARIN
mnt-by: MNT-EXAMPLECORP
source: ARIN
```


XML Payload Example

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
  <route xmlns="http://www.arin.net/regrws/core/v1" >
  <creationDate>2021-05-26T17:39:16Z</creationDate>
  <description>
    <line number="0">Example Corp LLC Arizona 1</line>
    <line number="1">Phoenix AZ 33333</line>
    <line number="2">USA</line>
  </description>
  <lastModifiedDate>2021-05-26T17:51:30Z</lastModifiedDate>
  <orgHandle>EXAMPLECORP</orgHandle>
  <pocLinks>
    <pocLinkRef description="Tech" function="T" handle="EXAMPLETECH-ARIN"/>
    <pocLinkRef description="Admin" function="AD" handle="EXAMPLEADMIN-ARIN"/>
  </pocLinks>
  <source>ARIN</source>
  <netHandle>NET-192-0-2-0</netHandle>
  <originAS>AS65536</originAS>
  <prefix>192.0.2.0/24</prefix>
  <autoLinkedRoaHandle>2caa8088924f205f01924f289120001a</autoLinkedRoaHandle>
</route>
```



---

### Creating a route6 Object

This call creates a `route6` object. These objects specify the IPv6 address block and the ASN of the Autonomous System from which the route originates.

**API Key in Header (Recommended)**

**Method:** `POST`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** `200 OK` response and RPSL or XML of the created object.

**API Key in URL (Supported)**

**Method:** `POST`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS?apikey=APIKEY  
**Content:** NONE  
**Returns:** `200 OK` response and RPSL or XML of the created object

**Additional Header**

RPSL

```
Accept: application/rpsl
Content-Type: application/rpsl
```


XML

```
Accept: application/xml
Content-Type: application/xml
```

**Content**

Payload in RPSL or XML format with the fields shown in the following tables.

RPSL Payload Fields

| Field | Definition |
| --- | --- |
| route6 | The IPv6 address and prefix length, in CIDR format, of the route to be originated. Together with the `origin` attribute, constitutes the primary key of the `route6` object. |
| origin | The ASN from which the route originates. |
| descr | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. |
| admin-c | Admin POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. |
| tech-c | Tech POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. |
| mnt-by | The prefix `MNT` and the Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the `route6` object. It is in the format `MNT-OrgID`; for example, `MNT-EXAMPLECORP`. |
| source: ARIN | Do not use any other value; all objects must have this line. |

XML Payload Fields

| Field | Definition |
| --- | --- |
| route | Use `xmlns="http://www.arin.net/regrws/core/v1".` |
| creationDate | Ignored by system. |
| description | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. Use line numbers for each line of information, as shown in the payload example. |
| lastModifiedDate | Ignored by system. |
| route | Use `xmlns="http://www.arin.net/regrws/core/v1".` |
| creationDate | Ignored by system. |
| description | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. Use line numbers for each line of information, as shown in the payload example. |
| lastModifiedDate | Ignored by system. |
| orgHandle | The Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the `route6` object. |
| pocLinks | Admin (AD), Tech (T), or Routing (R) POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. Use the `pocLinkRef` tag for each individual POC, as shown in the payload example. |
| source | Must be `ARIN`. |
| netHandle | Name of the network. |
| originAS | The ASN from which the route originates. |
| prefix | The IPv6 address and prefix length, in CIDR format, of the route to be originated. Together with the `originAS` attribute, constitutes the primary key of the `route6` object. |
| autoLinkedRoaHandle | This field cannot be set on create. Route/Route6 objects can only be autoLinked on ROA creation, not Route/Route6 creation. This field is only included in the list response, and only to allow a customer to keep track of what is auto linked / auto managed. |
| version | Ignored by system. |

**Payload Examples**

RPSL Payload Example

```
route6: 2001:db8::/32
origin: AS65536
descr: Example Corp LLC Arizona 1
descr: Phoenix AZ 33333
descr: USA
admin-c: EXAMPLEADMIN-ARIN
tech-c: EXAMPLETECH-ARIN
mnt-by: MNT-EXAMPLECORP
source: ARIN
```


XML Payload Example

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<route xmlns="http://www.arin.net/regrws/core/v1" >
  <creationDate>2021-05-26T19:17:45Z</creationDate>
  <description>
    <line number="0">Example Corp LLC Arizona 1</line>
    <line number="1">Phoenix AZ 33333</line>
    <line number="2">USA</line>
  </description>
  <lastModifiedDate>2021-05-26T19:17:45Z</lastModifiedDate>
  <orgHandle>EXAMPLECORP</orgHandle>
  <pocLinks>
    <pocLinkRef description="Tech" function="T" handle="EXAMPLETECH-ARIN"/>
    <pocLinkRef description="Routing" function="R" handle="EXAMPLEROUT-ARIN"/>
    <pocLinkRef description="Admin" function="AD" handle="EXAMPLEADMIN-ARIN"/>
  </pocLinks>
  <source>ARIN</source>
  <netHandle>NET6-2001-DB8-1</netHandle>
  <originAS>AS65536</originAS>
  <prefix>2001:db8::/32</prefix>
  <autoLinkedRoaHandle>2caa8088924f205f01924f289120001a</autoLinkedRoaHandle>
</route>
```



---

### Viewing a route or route6 Object

This call gets information about a `route` or `route6` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** 200 OK` response and RPSL or XML of the object.

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS?apikey=APIKEY  
**Content:** NONE  
**Returns:** 200 OK` response and RPSL or XML of the object.

**Additional Header s**

RPSL

```
Accept: application/rpsl
```


XML

```
Accept: application/xml
```

**Content**

None

---

### Viewing a List of route Objects for a NET

This call retrieves a list of all routes for a NET. The payload returns a list of all routes identified by the key-value pair prefix and origin for a given NET handle for the direct NET. It does not provide subnets. To be included, a route must be both visible and authorized in the IRR system.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/net/NETHANDLE/routes  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** `200 OK` response and XML payload containing a list of routes.

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/net/NETHANDLE/routes&apikey=APIKEY  
**Content:** NONE  
**Returns:** `200 OK` response and XML payload containing a list of routes.

**Additional Header**

XML

```
Accept: application/xml
Content-Type: application/xml
```

RPSL: Not available (XML only)

**Content**

None

**Returns**

The response includes the following fields for each route:

Route List Payload Fields

| Field | Definition |
| --- | --- |
| entry=“type” | The type of object (simple or advanced), which determines how the object can be viewed, updated or deleted; visit [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions) for more information. |
| orgHandle | Org ID under which the route is configured. |
| originAS | ASN authorized to announce the route. |
| prefix | IP address from which the route originates. |

**Example Return Payload**

XML

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<collection
  xmlns="http://www.arin.net/regrws/core/v1"
  xmlns:ns2="http://www.arin.net/regrws/messages/v1"
  xmlns:ns3="http://www.arin.net/regrws/shared-ticket/v1"
  xmlns:ns4="http://www.arin.net/regrws/ttl/v1"
  xmlns:ns5="http://www.arin.net/regrws/rpki/v1">
  <routeRef entry="ADVANCED">
    <orgHandle>EXAMPLECORP</orgHandle>
    <originAS>AS65535</originAS>
    <prefix>192.0.2.0/24</prefix>
  </routeRef>
  <routeRef entry="ADVANCED">
    <orgHandle>EXAMPLECORP</orgHandle>
    <originAS>AS65536</originAS>
    <prefix>192.0.2.0/28</prefix>
  </routeRef>
  <routeRef entry="ADVANCED">
    <orgHandle>EXAMPLECORP</orgHandle>
    <originAS>AS65537</originAS>
    <prefix>192.0.2.0/32</prefix>
  </routeRef>
</collection>
```

### Viewing a List of route Objects for Reassignments of a NET

This call retrieves a list of all routes for a NET and its downstream (reassignment) NETs. The payload returns a list of all routes identified by the key-value pair prefix and origin for a given NET handle for the NET and reassignments of the NET. To be included, a route must be both visible and authorized in the IRR system.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/net/NETHANDLE/routes?reassignments=true  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** `200 OK` response and XML payload containing a list of routes.

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/net/NETHANDLE/routes?reassignments=true&apikey=APIKEY   
**Content:** NONE  
**Returns:** `200 OK` response and XML payload containing a list of routes.

**Additional Header**

XML

```
Accept: application/xml
Content-Type: application/xml
```

RPSL: Not available (XML only)

**Content**

None

**Returns**

The response includes the following fields for each route:

Route List Payload Fields

| Field | Definition |
| --- | --- |
| entry=“type” | The type of object (simple or advanced), which determines how the object can be viewed, updated or deleted; visit [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions) for more information. |
| orgHandle | Org ID under which the route is configured. |
| originAS | ASN authorized to announce the route. |
| prefix | IP address from which the route originates. |

**Example Return Payload**

XML

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<collection
  xmlns="http://www.arin.net/regrws/core/v1"
  xmlns:ns2="http://www.arin.net/regrws/messages/v1"
  xmlns:ns3="http://www.arin.net/regrws/shared-ticket/v1"
  xmlns:ns4="http://www.arin.net/regrws/ttl/v1"
  xmlns:ns5="http://www.arin.net/regrws/rpki/v1">
  <routeRef entry="SIMPLE">
    <orgHandle>EXAMPLECORP</orgHandle>
    <originAS>AS65536</originAS>
    <prefix>192.0.2.0/24</prefix>
  </routeRef>
  <routeRef entry="SIMPLE">
    <orgHandle>EXAMPLECORP</orgHandle>
    <originAS>AS65536</originAS>
    <prefix>192.0.2.0/25</prefix>
  </routeRef>
  <routeRef entry="SIMPLE">
    <orgHandle>EXAMPLECORP</orgHandle>
    <originAS>AS65540</originAS>
    <prefix>192.0.2.0/26</prefix>
  </routeRef>
  <routeRef entry="ADVANCED">
    <orgHandle>DWNSTRMEXAMPLECORP-1</orgHandle>
    <originAS>AS65542</originAS>
    <prefix>192.0.2.0/28</prefix>
  </routeRef>
  <routeRef entry="ADVANCED">
    <orgHandle>DWNSTRMEXAMPLECORP-2</orgHandle>
    <originAS>AS65543</originAS>
    <prefix>192.0.2.0/30</prefix>
  </routeRef>
  <routeRef entry="ADVANCED">
    <orgHandle>DWNSTRMEXAMPLECORP-3</orgHandle>
    <originAS>AS65545</originAS>
    <prefix>192.0.2.0/32</prefix>
  </routeRef>
</collection>
```

### Viewing a List of route Objects for an Org ID

This call retrieves a list of all routes for an Org ID. The payload returns a list of all routes identified by the key-value pair prefix and origin for a given Org ID. To be included, a route must be both visible and authorized in the IRR system.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/org/ORGHANDLE/routes  
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** `200 OK` response and XML payload containing a list of routes.

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/org/ORGHANDLE/routes?apikey=APIKEY   
**Content:** NONE  
**Returns:** `200 OK` response and XML payload containing a list of routes.

**Additional Header**

XML

```
Accept: application/xml
Content-Type: application/xml
```

RPSL: Not available (XML only)

**Content**

None

**Returns**

The response includes the following fields for each route:

Route List Payload Fields

| Field | Definition |
| --- | --- |
| entry=“type” | The type of object (simple or advanced), which determines how the object can be viewed, updated or deleted; visit [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions) for more information. |
| orgHandle | Org ID under which the route is configured. |
| originAS | ASN authorized to announce the route. |
| prefix | IP address from which the route originates. |

**Example Return Payload**

XML

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<collection
  xmlns="http://www.arin.net/regrws/core/v1"
  xmlns:ns2="http://www.arin.net/regrws/messages/v1"
  xmlns:ns3="http://www.arin.net/regrws/shared-ticket/v1"
  xmlns:ns4="http://www.arin.net/regrws/ttl/v1"
  xmlns:ns5="http://www.arin.net/regrws/rpki/v1">
  <routeRef entry="ADVANCED">
    <orgHandle>EXAMPLECORP</orgHandle>
    <originAS>AS65535</originAS>
    <prefix>192.0.2.0/24</prefix>
  </routeRef>
  <routeRef entry="ADVANCED">
    <orgHandle>EXAMPLECORP</orgHandle>
    <originAS>AS65536</originAS>
    <prefix>198.51.100.0/24</prefix>
  </routeRef>
  <routeRef entry="SIMPLE">
    <orgHandle>EXAMPLECORP</orgHandle>
    <originAS>AS65537</originAS>
    <prefix>233.252.0.0/24</prefix>
  </routeRef>
</collection>
```

### Modifying a route Object

This call modifies an existing `route` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** Payload as described in [Creating a route Object](https://www.arin.net/resources/manage/irr/irr-restful/#creating-a-route-object)  
**Returns:** `200 OK` response and RPSL or XML of the object.

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS?apikey=APIKEY   
**Content:** Payload as described in [Creating a route Object](https://www.arin.net/resources/manage/irr/irr-restful/#creating-a-route-object)  
**Returns:** `200 OK` response and RPSL or XML of the object.

**Additional Header**

RPSL

```
Accept: application/rpsl
Content-Type: application/rpsl
```


XML

```
Accept: application/xml
Content-Type: application/xml
```



---

### Modifying a route6 Object

This call modifies an existing `route6` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `PUT`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** Payload as described in [Creating a route6 Object](https://www.arin.net/resources/manage/irr/irr-restful/#creating-a-route6-object)  
**Returns:** `200 OK` response and RPSL or XML of the object

**API Key in URL (Supported)**

**Method:** `PUT`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS?apikey=APIKEY   
**Content:** Payload as described in [Creating a route6 Object](https://www.arin.net/resources/manage/irr/irr-restful/#creating-a-route6-object)  
**Returns:** `200 OK` response and RPSL or XML of the object

**Additional Header**

RPSL

```
Accept: application/rpsl
Content-Type: application/rpsl
```


XML

```
Accept: application/xml
Content-Type: application/xml
```

---

### Deleting a route or route6 object

This call deletes a `route` or `route6` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `DELETE`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** `200 OK` response and RPSL or XML of the deleted object

**API Key in URL (Supported)**

**Method:** `DELETE`  
**URL:** /rest/irr/route/IPADDRESS/PREFIXLENGTH/ORIGINAS?apikey=APIKEY   
**Content:** NONE   
**Returns:** `200 OK` response and RPSL or XML of the deleted object

**Additional Header**

RPSL

```
Accept: application/rpsl
```


XML

```
Accept: application/xml
```

---

## aut-num Objects

### Creating an aut-num Object

This call creates an `aut-num` object. These objects specify an ASN and define its routing policies using the fields in the record.

**API Key in Header (Recommended)**

**Method:** `POST`   
**URL:** /rest/irr/aut-num/ASN   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** Payload detailed below  
**Returns:** `200 OK` response and RPSL or XML of the created object

**API Key in URL (Supported)**

**Method:** `POST`  
**URL:** /rest/irr/aut-num/ASN?apikey=APIKEY   
**Content:** Payload decribed below  
**Returns:** `200 OK` response and RPSL or XML of the created object

**Additional Header**

RPSL

```
Accept: application/rpsl
Content-Type: application/rpsl
```


XML

```
Accept: application/xml
Content-Type: application/xml
```

**Content**

Payload in RPSL or XML format with the fields shown in the following tables.

RPSL Payload Fields

| Field | Definition |
| --- | --- |
| aut-num: | The ASN for the routing policy. |
| as-name: | An alphanumeric identifier for the aut-num object. |
| descr: | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. |
| admin-c: | Admin POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. |
| tech-c: | Tech POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. |
| mnt-by: | The prefix `MNT` and the Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the `aut-num` object. It is in the format `MNT-OrgID`; for example, `MNT-EXAMPLECORP`. |
| source: ARIN | Do not use any other value; all objects must have this line. |
| member-of: | The `as-set` (one or more) to which the object belongs. The `as-set` objects that are listed also need to include the maintainer associated with this `aut-num` in their object record for the cross reference to work. |
| import: | Indicates the ASNs of the peers from which your AS will receive routing information and describes the routing information you’ll accept. For example, you can configure your routing to import a set of routes from a specific ASN that matches a filter, such as an IPv4 address. You can also specify actions to set or modify route attributes (such as assigning a preference to a route). More information is available in Section 6.1 of RFC 2622. (The `mp-import` field extends this function for IPv6.) |
| export: | Describes export policies (for example, exporting or announcing a set of routes or routes that match a filter) to specified ASes. More information is available in Section 6.2 of RFC 2622. (The `mp-export` field extends this function for IPv6.) |
| default: | Specifies the default routing policy (for example, if the AS should default to another AS for routing and under which conditions, such as if a specific filter is matched). More information is available in Section 6.5 of RFC 2622. (The `mp-default` field extends this function for IPv6.) |
| mp-import: | Describes import policies as in the “import” field, but extends RPSL to allow “multi-protocol” routing policy. More information is available in Section 2.5 of RFC 4012. |
| mp-export: | Describes export policies as in the “export” field, but extends RPSL to allow “multi-protocol” routing policy. More information is available in Section 2.5 of RFC 4012. |
| mp-default: | Specifies the default routing policy as in the “default” field, but extends RPSL to allow “multi-protocol” routing policy. More information is available in Section 2.5 of RFC 4012. |

XML Payload Fields

| Field | Definition |
| --- | --- |
| autnum | Use `xmlns="http://www.arin.net/regrws/core/v1".` |
| creationDate | Ignored by system. |
| description | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. Use line numbers for each line of information, as shown in the payload example. |
| lastModifiedDate | Ignored by system. |
| autnum | Use `xmlns="http://www.arin.net/regrws/core/v1".` |
| creationDate | Ignored by system. |
| description | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. Use line numbers for each line of information, as shown in the payload example. |
| lastModifiedDate | Ignored by system. |
| orgHandle | The Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the `aut-num` object. |
| pocLinks | Admin (AD), Tech (T), or Routing (R) POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. Use the `pocLinkRef` tag for each individual POC, as shown in the payload example. |
| source | Must be `ARIN`. |
| netHandle | Name of the network. |
| asName | An alphanumeric identifier for the `aut-num` object. |
| asNumber | The ASN for the routing policy. |
| default | Specifies the default routing policy (for example, if the AS should default to another AS for routing and under which conditions, such as if a specific filter is matched). More information is available in Section 6.5 of RFC 2622. (The `mpDefault` field extends this function for IPv6.) |
| memberOf | The `as-set` (one or more) to which the object belongs. The `as-set` objects that are listed also need to include the maintainer associated with this `aut-num` in their object record for the cross reference to work. |
| export | Describes export policies (for example, exporting or announcing a set of routes or routes that match a filter) to specified ASes. More information is available in Section 6.2 of RFC 2622. (The `mpExport` field extends this function for IPv6.) |
| import | Indicates the ASNs of the peers from which your AS will receive routing information and describes the routing information you’ll accept. For example, you can configure your routing to import a set of routes from a specific ASN that matches a filter, such as an IPv4 address. You can also specify actions to set or modify route attributes (such as assigning a preference to a route). More information is available in Section 6.1 of RFC 2622. (The `mp-import` field extends this function for IPv6.) |
| mpDefault | Specifies the default routing policy as in the “default” field, but extends RPSL to allow “multi-protocol” routing policy. More information is available in Section 2.5 of RFC 4012. |
| mpExport | Describes export policies as in the “export” field, but extends RPSL to allow “multi-protocol” routing policy. More information is available in Section 2.5 of RFC 4012. |
| mpImport | Describes import policies as in the “import” field, but extends RPSL to allow “multi-protocol” routing policy. More information is available in Section 2.5 of RFC 4012. |

**Payload Examples**

RPSL Payload Example

```
aut-num: AS65536
as-name: AS-ARIZ-NODE1
descr: Example Corp LLC Arizona 1
descr: Phoenix AZ 33333
descr: United States
import: from AS65540 accept ANY
export: to AS65550 announce ANY
admin-c: EXAMPLEADMIN-ARIN
tech-c: EXAMPLETECH-ARIN
mnt-by: MNT-EXAMPLECORP
source: ARIN
```


XML Payload Example

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<autnum xmlns="http://www.arin.net/regrws/core/v1" >
  <creationDate>2021-03-11T13:47:20Z</creationDate>
  <description>
    <line number="0">Example Corp LLC Arizona 1</line>
    <line number="1">Phoenix AZ 33333</line>
    <line number="2">United States</line>
  </description>
  <lastModifiedDate>2021-03-11T13:47:20Z</lastModifiedDate>
  <orgHandle>EXAMPLECORP</orgHandle>
  <pocLinks>
    <pocLinkRef description="Tech" function="T" handle="EXAMPLETECH-ARIN"/>
    <pocLinkRef description="Admin" function="AD" handle="EXAMPLEADMIN-ARIN"/>
  </pocLinks>
  <remarks>
    <line number="0">This is a comment.</line>
  </remarks>
  <source>ARIN</source>
  <asName>AS-ARIZ-NODE1</asName>
  <asNumber>AS65536</asNumber>
  <default>
    <line number="0">to AS2 7.7.7.2 at 7.7.7.1</line>
  </default>
  <export>
    <line number="0">to AS65550 announce ANY</line>
  </export>
  <import>
    <line number="0">from AS65540 accept ANY</line>
  </import>
  <mpDefault>
    <line number="0">to AS2 7.7.7.2 at 7.7.7.1</line>
  </mpDefault>
  <mpExport>
    <line number="0">to AS65539 announce AS64496</line>
  </mpExport>
  <mpImport>
    <line number="0">from AS64496 accept ANY</line>
  </mpImport>
</autnum>
```

### Viewing an aut-num Object

This call gets information about an `aut-num` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `GET`   
**URL:** /rest/irr/aut-num/ASN   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** `200 OK` response and RPSL or XML of the object

**API Key in URL (Supported)**

**Method:** `GET`   
**URL:** /rest/irr/aut-num/ASN?apikey=APIKEY   
**Content:** NONE  
**Returns:** `200 OK` response and RPSL or XML of the object

**Additional Header**

RPSL

```
Accept: application/rpsl
```


XML

```
Accept: application/xml
```



---

### Viewing a List of aut-num Objects for an Org ID

This call returns all `aut-num` objects for a given Org ID.

**API Key in Header (Recommended)**

**Method:** `GET`   
**URL:** /rest/org/ORGHANDLE/aut-nums   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** `200 OK` response and XML payload containing a list of `aut-num` objects.

**API Key in URL (Supported)**

**Method:** `GET`   
**URL:** /rest/org/ORGHANDLE/aut-nums?apikey=APIKEY   
**Content:** NONE  
**Returns:** `200 OK` response and XML payload containing a list of `aut-num` objects.

**Additional Header**

XML

```
Accept: application/xml
Content-Type: application/xml
```

RPSL: Not available (XML only)

**Returns**

The response includes the following fields for each `aut-num`:

aut-num List Payload Fields

| Field | Definition |
| --- | --- |
| asNumber | ASN for the `aut-num` object |
| entry=“type” | The type of object (simple or advanced), which determines how the object can be viewed, updated or deleted; visit [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions) for more information. |

**Example Return Payload**

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<collection
  xmlns="http://www.arin.net/regrws/core/v1"
  xmlns:ns2="http://www.arin.net/regrws/messages/v1"
  xmlns:ns3="http://www.arin.net/regrws/shared-ticket/v1"
  xmlns:ns4="http://www.arin.net/regrws/ttl/v1"
  xmlns:ns5="http://www.arin.net/regrws/rpki/v1">
  <autNumRef entry="ADVANCED">
    <asNumber>AS65535</asNumber>
  </autNumRef>
  <autNumRef entry="ADVANCED">
    <asNumber>AS65536</asNumber>
  </autNumRef>
  <autNumRef entry="SIMPLE">
    <asNumber>AS65537</asNumber>
  </autNumRef>
  <autNumRef entry="SIMPLE">
    <asNumber>AS65538</asNumber>
  </autNumRef>
</collection>
```

### Modifying an aut-num Object

This call modifies an existing `aut-num` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**Method:** `PUT`   
**URL:** /rest/irr/aut-num/ASN   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** Payload as described in [Creating an aut-num Object](https://www.arin.net/resources/manage/irr/irr-restful/#creating-an-aut-num-object)   
**Returns:** `200 OK` response and RPSL or XML of the modified object

**API Key in URL (Supported)**

**Method:** `PUT`   
**URL:** /rest/irr/aut-num/ASN?apikey=APIKEY   
**Content:** Payload as described in [Creating an aut-num Object](https://www.arin.net/resources/manage/irr/irr-restful/#creating-an-aut-num-object)   
**Returns:** `200 OK` response and RPSL or XML of the modified object

**Additional Header**

RPSL

```
Accept: application/rpsl
Content-Type: application/rpsl
```


XML

```
Accept: application/xml
Content-Type: application/xml
```



---

### Deleting an aut-num Object

This call deletes an `aut-num` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `DELETE`   
**URL:** /rest/irr/aut-num/ASN   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE   
**Returns:** `200 OK` response and RPSL or XML of the deleted object

**API Key in URL (Supported)**

**Method:** `DELETE`   
**URL:** /rest/irr/aut-num/ASN?apikey=APIKEY   
**Content:** NONE   
**Returns:** `200 OK` response and RPSL or XML of the deleted object

**Additional Header**

RPSL

```
Accept: application/rpsl
```


XML

```
Accept: application/xml
```

---

## as-set Objects

### Creating an as-set Object

This call creates an `as-set` object. These objects specify a set of ASNs through which traffic can be routed. The members of the set can also include other `as-set` names.

**API Key in Header (Recommended)**

**Method:** `POST`   
**URL:** /rest/irr/as-set?orgHandle=ORGHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** Payload as described below   
**Returns:** `200 OK` response and RPSL or XML of the created object

**API Key in URL (Supported)**

**Method:** `POST`   
**URL:** /rest/irr/as-set?apikey=APIKEY&orgHandle=ORGHANDLE   
**Content:** Payload as described below   
**Returns:** `200 OK` response and RPSL or XML of the created object

**Additional Header**

RPSL

```
Accept: application/rpsl
Content-Type: application/rpsl
```


XML

```
Accept: application/xml
Content-Type: application/xml
```

**Content**

Payload in RPSL or XML format with the fields shown in the following tables:

RPSL Payload Fields

| Field | Definition |
| --- | --- |
| as-set: | A name you give to a group of ASNs. Begins with `AS-`. |
| descr: | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. |
| admin-c: | Admin POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. |
| tech-c: | Tech POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. |
| mnt-by: | The prefix `MNT` and the Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the route object. It is in the format `MNT-OrgID`; for example, `MNT-EXAMPLECORP`. |
| source: ARIN | Do not use any other value; all objects must have this line. |
| remarks: | Any additional information the creator of the objects wants to provide (this information will also be published in the IRR database). |
| members: | Members of this set of ASNs; can be a list of ASNs or multiple `as-set` objects. |
| mbrs-by-ref: | The Org IDs, preceded by `MNT-`, whose `aut-num` objects can be included in this set. Those `aut-num` objects must themselves contain this `as-set` in their `member-of` field. If this field contains “ANY,” any `aut-num` referring to the set (in its `member-of` field) will be included as a member of the set. If this field is left blank, the set is defined explicitly by the `members` field. |

XML Payload Fields

| Field | Definition |
| --- | --- |
| asSet | Use `xmlns="http://www.arin.net/regrws/core/v1".` |
| creationDate | Ignored by system. |
| description | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. Use line numbers for each line of information, as shown in the payload example. |
| lastModifiedDate | Ignored by system. |
| asSet | Use `xmlns="http://www.arin.net/regrws/core/v1".` |
| creationDate | Ignored by system. |
| description | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. Use line numbers for each line of information, as shown in the payload example. |
| lastModifiedDate | Ignored by system. |
| orgHandle | The Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the `as-set` object. |
| pocLinks | Admin (AD), Tech (T), or Routing (R) POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. Use the `pocLinkRef` tag for each individual POC, as shown in the payload example. |
| remarks | Any additional information the creator of the objects wants to provide (this information will also be published in the IRR database). |
| source | Must be `ARIN`. |
| members | Members of this set of ASNs; can be a list of ASNs or multiple `as-set` objects. Add each member using the `member` tag, as shown in the XML payload example. |
| mbrsByRef | The Org IDs, preceded by `MNT-`, whose `aut-num` objects can be included in this set. Those `aut-num` objects must themselves contain this `as-set` in their `memberOf` field. If this field contains “ANY,” any `aut-num` referring to the set (in its `memberOf` field) will be included as a member of the set. If this field is left blank, the set is defined explicitly by the `members` field. Add each memberByRef using the `memberByRef` tag as shown in the XML payload example. |
| name | A name you give to a group of ASNs. Begins with `AS-`. |

**Payload Example**

RPSL Payload Example

```
as-set: AS-EXAMPLE-ARIZ
mnt-by: MNT-EXAMPLECORP
descr: Peers in AZ network
tech-c: EXAMPLETECH-ARIN
admin-c: EXAMPLEADMIN-ARIN
remarks: This is an example.
members: AS-PHOENIX
members: AS-MESA
members: AS-SEDONA
source: ARIN
```


XML Payload Example

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<asSet xmlns="http://www.arin.net/regrws/core/v1" >
  <creationDate>2021-05-27T18:24:28Z</creationDate>
  <description>
    <line number="0">peers in AZ network</line>
  </description>
  <lastModifiedDate>2021-05-27T18:24:28Z</lastModifiedDate>
  <orgHandle>EXAMPLECORP</orgHandle>
  <pocLinks>
    <pocLinkRef description="Tech" function="T" handle="EXAMPLETECH"/>
    <pocLinkRef description="Routing" function="R" handle="EXAMPLEROUT"/>
    <pocLinkRef description="Admin" function="AD" handle="EXAMPLEADMIN"/>
  </pocLinks>
  <source>ARIN</source>
  <members>
    <member name="AS-PHOENIX"/>
    <member name="AS-MESA"/>
    <member name="AS-SEDONA"/>
  </members>
  <name>AS-EXAMPLE-ARIZ</name>
</asSet>
```

  


---

### Viewing an as-set Object

This call gets information about an `as-set` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `GET`   
**URL:** /rest/irr/as-set/AS-SETNAME   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE   
**Returns:** `200 OK` response and RPSL or XML of the object

**API Key in URL (Supported)**

**Method:** `GET`   
**URL:** /rest/irr/as-set/AS-SETNAME?apikey=APIKEY   
**Content:** NONE   
**Returns:** `200 OK` response and RPSL or XML of the object

**Additional Header**

RPSL

```
Accept: application/rpsl
```


XML

```
Accept: application/xml
```

---

### Viewing a List of as-set Objects for an Org ID

This call returns all `as-set` objects for a given Org ID. To be included, an `as-set` must be both visible and authorized in the IRR system.

**API Key in Header (Recommended)**

**Method:** `GET`   
**URL:** /rest/org/ORGHANDLE/as-sets   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE   
**Returns:** `200 OK` response and XML payload containing a list of `as-set` objects

**API Key in URL (Supported)**

**Method:** `GET`   
**URL:** /rest/org/ORGHANDLE/as-sets?apikey=APIKEY   
**Content:** NONE   
**Returns:** `200 OK` response and XML payload containing a list of `as-set` objects

**Additional Header**

XML

```
Accept: application/xml
Content-Type: application/xml
```

RPSL: Not available (XML only)

**Returns**

The response includes the following fields for each object:

as-set List Payload Fields

| Field | Definition |
| --- | --- |
| name | Name of the `as-set` |
| entry=“type”: | The type of object (simple or advanced), which determines how the object can be viewed, updated or deleted; visit [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions) for more information. |

**Example Return Payload**

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<collection
  xmlns="http://www.arin.net/regrws/core/v1"
  xmlns:ns2="http://www.arin.net/regrws/messages/v1"
  xmlns:ns3="http://www.arin.net/regrws/shared-ticket/v1"
  xmlns:ns4="http://www.arin.net/regrws/ttl/v1"
  xmlns:ns5="http://www.arin.net/regrws/rpki/v1">
  <asSetRef name="AS-PHOENIX" entry="SIMPLE"/>
  <asSetRef name="AS-MESA" entry="SIMPLE"/>
</collection>
```

### Modifying an as-set Object

This call modifies an `as-set` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `PUT`   
**URL:** /rest/irr/as-set/AS-SETNAME   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** Payload as described in [Creating an as-set Object](https://www.arin.net/resources/manage/irr/irr-restful/#creating-an-as-set-object)   
**Returns:** `200 OK` response and RPSL or XML of the modified object

**API Key in URL (Supported)**

**Method:** `PUT`   
**URL:** /rest/irr/as-set/AS-SETNAME?apikey=APIKEY   
**Content:** Payload as described in [Creating an as-set Object](https://www.arin.net/resources/manage/irr/irr-restful/#creating-an-as-set-object)  
**Returns:** `200 OK` response and RPSL or XML of the modified object

**Additional Header**

RPSL

```
Accept: application/rpsl
Content-Type: application/rpsl
```


XML

```
Accept: application/xml
Content-Type: application/xml
```

---

### Deleting an as-set Object

This call deletes an `as-set` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `DELETE`   
**URL:** /rest/irr/as-set/AS-SETNAME   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE   
**Returns:** `200 OK` response and RPSL or XML of the deleted object

**API Key in URL (Supported)**

**Method:** `DELETE`   
**URL:** /rest/irr/as-set/AS-SETNAME?apikey=APIKEY   
**Content:** NONE  
**Returns:** `200 OK` response and RPSL or XML of the deleted object

**Additional Header**

RPSL

```
Accept: application/rpsl
```


XML

```
Accept: application/xml
```

---

## route-set Objects

### Creating a route-set Object

This call creates a `route-set` object. These objects group a list of individual IPv4 address prefixes (called `members`), IPv6 address prefixes (`mp-members`), and other objects referred to in the `mbrs-by-ref` field. The `route-set` can then be referred to in other IRR objects.

**API Key in Header (Recommended)**

**Method:** `POST`   
**URL:** /rest/irr/route-set?orgHandle=ORGHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** Payload as described below   
**Returns:** `200 OK` response and RPSL or XML of the created object

**API Key in URL (Supported)**

**Method:** `POST`   
**URL:** /rest/irr/route-set?apikey=APIKEY&orgHandle=ORGHANDLE   
**Content:** Payload as described below   
**Returns:** `200 OK` response and RPSL or XML of the creatted object

**Additional Header**

RPSL

```
Accept: application/rpsl
Content-Type: application/rpsl
```


XML

```
Accept: application/xml
Content-Type: application/xml
```

**Content**

Payload in RPSL or XML format with the fields shown in the following tables:

RPSL Payload Fields

| Field | Definition |
| --- | --- |
| route-set: | A name you give to the route-set. Begins with `RS-` or with an AS that is managed by your organization followed by a colon and `RS-` (for example, AS65536:RS-ARIZ-SE-5). |
| descr: | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. |
| admin-c: | Admin POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. |
| tech-c: | Tech POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. |
| mnt-by: | The prefix `MNT` and the Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the `route-set` object. It is in the format `MNT-OrgID`; for example, `MNT-EXAMPLECORP`. |
| source: ARIN | Do not use any other value; all objects must have this line. |
| remarks: | Any additional information the creator of the objects wants to provide (this information will also be published in the IRR database). |
| members: | Members of the set; ARIN accepts IPv4 address prefixes or other `route-set` names. |
| mp-members: | Members of the set; IPv4 and IPv6 address prefixes or other `route-set` names. |
| mbrs-by-ref: | The Org IDs, preceded by `MNT-`, whose `route` or `route6` objects should be included in this set. Those route/route6 objects must then contain this `rs-set` in their `member-of` field. |

XML Payload Fields

| Field | Definition |
| --- | --- |
| routeSet | Use `xmlns="http://www.arin.net/regrws/core/v1".` |
| creationDate | Ignored by system. |
| description | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. Use line numbers for each line of information, as shown in the payload example. |
| lastModifiedDate | Ignored by system. |
| routeSet | Use `xmlns="http://www.arin.net/regrws/core/v1".` |
| creationDate | Ignored by system. |
| description | In the description, enter any information you think needs to be here. This could be your name and address, your ISP’s name, or a network name or location. Use line numbers for each line of information, as shown in the payload example. |
| lastModifiedDate | Ignored by system. |
| orgHandle | The Org ID of the organization that configures (maintains) the IRR object and manages the resource that is specified in the `route-set` object. |
| pocLinks | Admin (AD), Tech (T), or Routing (R) POC for the organization that is maintaining the object. This field is the exact POC Handle as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the Org ID. Use the `pocLinkRef` tag for each individual POC, as shown in the payload example. |
| remarks | Any additional information the creator of the objects wants to provide (this information will also be published in the IRR database). |
| source | Must be `ARIN`. |
| members | Members of the set; ARIN accepts IPv4 address prefixes or other `route-set` names. Add each member using the `member` tag, as shown in the XML payload example. |
| mpMembers | Members of the set; IPv4 and IPv6 address prefixes or other `route-set` names. Add each member using the `mpMember` tag, as shown in the XML payload example. |
| membersByRef | The Org IDs, preceded by `MNT-`, whose `route` or `route6` objects should be included in this set. Those route/route6 objects must then contain this `rs-set` in their `memberOf` field. Add each member using the `memberByRef` tag, as shown in the XML payload example. |
| name | A name you give to the route-set. Begins with `RS-` or with an AS that is managed by your organization followed by a colon and `RS-` (for example, AS65536:RS-ARIZ-SE-5). |

**Payload Examples**

RPSL Payload Example

```
route-set: RS-ARIZONA-SOUTH
mnt-by: MNT-EXAMPLECORP
descr: set of Southern AZ routes
tech-c: EXAMPLETECH-ARIN
admin-c: EXAMPLEADMIN-ARIN
remarks: This is an example.
members: 192.0.2.0/24
mp-members: 2001:db8::/32
source: ARIN
```


XML Payload Example

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<routeSet xmlns="http://www.arin.net/regrws/core/v1" >
  <creationDate>2021-05-27T18:42:51Z</creationDate>
  <description>
    <line number="0">set of Southern AZ routes</line>
  </description>
  <lastModifiedDate>2021-05-27T18:43:10Z</lastModifiedDate>
  <orgHandle>EXAMPLECORP</orgHandle>
  <pocLinks>
    <pocLinkRef description="Tech" function="T" handle="EXAMPLETECH-ARIN"/>
    <pocLinkRef description="Routing" function="R" handle="EXAMPLEROUT-ARIN"/>
    <pocLinkRef description="Admin" function="AD" handle="EXAMPLEADMIN-ARIN"/>
  </pocLinks>
  <source>ARIN</source>
  <members>
    <member name="192.0.2.0/24"/>
  </members>
  <membersByRef>
    <memberByRef name="MNT-OTHEREXAMPLECORP"/>
  </membersByRef>
  <name>RS-ARIZONA-SOUTH</name>
  <mpMembers>
    <mpMember name="2001:db8::/32"/>
  </mpMembers>
</routeSet>
```

---

### Viewing a route-set Object

This call gets information about a `route-set` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `GET`   
**URL:** /rest/irr/route-set/RS-SETNAME   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE   
**Returns:** `200 OK` response and RPSL or XML of the object

**API Key in URL (Supported)**

**Method:** `GET`   
**URL:** /rest/irr/route-set/RS-SETNAME?apikey=APIKEY   
**Content:** NONE   
**Returns:** `200 OK` response and RPSL or XML of the object

**Additional Header**

RPSL

```
Accept: application/rpsl
```


XML

```
Accept: application/xml
```

---

### Viewing a List of route-set Objects for an Org ID

This call returns all `route-set` objects for a given Org ID. To be included, a `route-set` must be both visible and authorized in the IRR system.

**API Key in Header (Recommended)**

**Method:** `GET`   
**URL:** /rest/org/ORGHANDLE/route-sets   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE   
**Returns:** `200 OK` response and XML payload containing a list of route-set objects

**API Key in URL (Supported)**

**Method:** `GET`   
**URL:** /rest/org/ORGHANDLE/route-sets?apikey=APIKEY   
**Content:** NONE   
**Returns:** `200 OK` response and XML payload containing a list of route-set objects

**Additional Header**

XML

```
Accept: application/xml
Content-Type: application/xml
```

RPSL: Not available (XML only)

**Returns**

The response includes the following fields for each object:

route-set List Payload Fields

| Field | Definition |
| --- | --- |
| name | Name of the `route-set` |
| entry=“type” | The type of object (simple or advanced), which determines how the object can be viewed, updated or deleted; visit [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions) for more information. |

**Example Return Payload**

```
<collection
  xmlns="http://www.arin.net/regrws/core/v1"
  xmlns:ns2="http://www.arin.net/regrws/messages/v1"
  xmlns:ns3="http://www.arin.net/regrws/shared-ticket/v1"
  xmlns:ns4="http://www.arin.net/regrws/ttl/v1"
  xmlns:ns5="http://www.arin.net/regrws/rpki/v1">
  <routeSetRef name="RS-ARIZONA-NORTH" entry="SIMPLE"/>
  <routeSetRef name="RS-ARIZONA-SOUTH" entry="SIMPLE"/>
</collection>
```

### Modifying a route-set Object

This call modifies a `route-set` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `PUT`   
**URL:** /rest/irr/route-set/RS-SETNAME   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** Payload as described in [Creating a route-set Object](https://www.arin.net/resources/manage/irr/irr-restful/#creating-a-route-set-object)   
**Returns:** `200 OK` response and RPSL or XML of the modified object

**API Key in URL (Supported)**

**Method:** `PUT`   
**URL:** /rest/irr/route-set/RS-SETNAME?apikey=APIKEY   
**Content:** Payload as described in [Creating a route-set Object](https://www.arin.net/resources/manage/irr/irr-restful/#creating-a-route-set-object)   
**Returns:** `200 OK` response and RPSL or XML of the modified object

**Additioal Header**

RPSL

```
Accept: application/rpsl
Content-Type: application/rpsl
```


XML

```
Accept: application/xml
Content-Type: application/xml
```

---

### Deleting a route-set Object

This call deletes a `route-set` object. (Note the [Object Permissions](https://www.arin.net/resources/manage/irr/#object-permissions).)

**API Key in Header (Recommended)**

**Method:** `DELETE`   
**URL:** /rest/irr/route-set/RS-SETNAME   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE   
**Returns:** `200 OK` response and RPSL or XML of the deleted object

**API Key in URL (Supported)**

**Method:** `DELETE`   
**URL:** /rest/irr/route-set/RS-SETNAME?apikey=APIKEY   
**Content:** NONE   
**Returns:** `200 OK` response and RPSL or XML of the deleted object

**Additional Header**

RPSL

```
Accept: application/rpsl
```


XML

```
Accept: application/xml
```

---

## Error Codes

This list includes some of the error codes that may be returned if you send an incorrect command or payload. A message is returned with additional information on the error; the following table provides some additional information on errors that may be returned. (**Tip**: Use your browser’s search feature to find a specific message.)

### 400 Bad Request

Some of the causes of a `400 Bad Request` response are provided in the following sections:

* E\_AUTHENTICATION: The API key specified was missing or invalid.
* E\_BAD\_REQUEST: An `aut-num` object with the specified ASN already exists.
* E\_ENTITY\_VALIDATION:
  + The payload entity failed to validate; refer to the component messages for details.
  + Changing the value of the `mnt-by` field is not supported. The `mnt-by` value in the call does not match the `mnt-by` value of the existing record or the Org Handle (Org ID) of the organization that maintains the resource.
  + Prefix: Unable to create a route with this prefix. Make sure that your organization, as specified in the `mnt-by` field (`MNT-OrgID`) in your payload, manages the IP address block specified in your request.
  + Each specified `tech-c` must match a Tech or Routing Point of Contact (POC) associated with the Org ID (as shown in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/)) that is specified as a maintainer in the `mnt-by` field. One or more `tech-c` fields are not valid Tech POCs for the specified organization.
  + A route object with the specified prefix and origin already exists.
  + Changing the origin of a `route` object is not supported. You cannot modify a `route` or `route6` object to change the ASN origination; you need to delete the object and submit a new `route` or `route6` object with the desired new ASN.
  + Duplicate entries are not allowed. Duplicate members in the `member-of` attribute are not supported.
  + The ASN in the request URL path must match the ASN in the RPSL payload.
  + The specified `admin-c` must be the Admin POC of the Org ID specified as the maintainer in the `mnt-by` field. The specified `admin-c` does not match the Admin POC listed in [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) for the specified organization.
  + The specified prefix is off-bit. The route contains an address that is not valid with the mask (prefix length), or block size, specified (`/xx`). For example, 2001:db8::/20 would be off-bit, but 2001:db8::/32 would be valid.
* E\_SCHEMA\_VALIDATION:
  + Something is wrong with your payload; for example, information is missing from your payload, a value is incorrect in your payload, or your payload has an extra line space. A message may be returned that provides more specific information.
  + The RPSL payload must specify `source: ARIN`.
  + ARIN does not support use of the `notify` field in RESTful payloads.

### Other Error Responses

* 405 Method Not Allowed
  + E\_BAD\_REQUEST: Method not allowed. Ensure that the proper HTTP method has been used for this request. The wrong method (e.g., `PUT`, `POST`) is specified, or the URL contains incorrect information.
* 406 Not Acceptable
  + E\_BAD\_REQUEST: No match for accept header. The `Accept` header must be set to the following media type: `application/rpsl` or `application/xml`. Make sure this header is included correctly in your call.
* 415 Unsupported Media Type
  + E\_BAD\_REQUEST: Cannot consume content type. The `Content-Type` header for this request must be set to the following media type: `application/rpsl` or `application/xml`. Make sure this header is included correctly in your call if you are sending a payload.
* 500 Internal Server Error
  + E\_UNSPECIFIED: An error occurred while attempting to process your request.
* 503 Service Unavailable
  + E\_UNSUPPORTED\_OPERATION: Operation not supported. Viewing, editing, and deleting of some objects is limited depending on the method used to originally create the object; refer to the table in the [Object Permissions](https://www.arin.net/resources/manage/irr/irr-restful/#object-permissions) section.
* exception.rpslParse: The RPSL payload you provided cannot be parsed. The system does not understand the format of the `aut-num` payload that you sent. Refer to [aut-num Objects](https://www.arin.net/resources/manage/irr/irr-restful/#aut-num-objects) for the correct payload format.
* java.lang.IllegalStateException: Cannot validate payload with unsupported media type. You do not have the required `accept` or `content-type` [headers of application/rpsl](https://www.arin.net/resources/manage/irr/irr-restful/#headers) or [application/xml](https://www.arin.net/resources/manage/irr/irr-restful/#headers) in your request.
* exception.multipleMntBy: Specify only one maintainer in the `mnt-by` field.
* exception.asZeroException: The specified origin `AS0` is not supported.
