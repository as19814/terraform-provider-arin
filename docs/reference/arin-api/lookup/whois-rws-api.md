# Whois-RWS API Documentation

Source: https://www.arin.net/resources/registry/whois/rws/api/

Retrieved: 2026-09-22T22:50:52.639970+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## Background

Traditionally, Whois services were offered using the NICNAME/WHOIS protocol as described by RFC 954 and RFC 3912. This protocol is a simple, text-based TCP protocol registered with in the Internet Assigned Numbers Authority (IANA) for well-known port 43. The specification for the protocol, RFC 3912, did not define either data types or data formats. As a consequence, Whois data varied from service provider to service provider and was far from ideal for programmatic consumption. Consequently, ARIN first provided expanded Whois capabilities in its own ARIN-specific Whois implementation of Whois-RWS.

However, with the development of Registration Access Data Protocol (RDAP) in 2015 under the guidance of the Internet Engineering Task Force (IETF), Regional Internet Registries (RIRs) like ARIN and other registrars were able to provide additional benefits over both Whois port 43 and Whois-RWS. ARIN first supported RDAP queries and responses through the command-line interface and API. As of 2019, ARIN strongly recommends using [Whois/RDAP](https://www.arin.net/resources/registry/whois/rdap/) because of its standardization, support for security, and access to registration information from other RIRs, registrars, and organizations who support RDAP.

This document describes the application programming interface (API) for the ARIN’s RESTful web service and a NICNAME/WHOIS port 43 service.

## Terms of Use

Usage of all ARIN Whois data services are subject to [ARIN’s Whois Terms of Use](https://www.arin.net/resources/registry/whois/tou/).

## ARIN’s Data Model

ARIN’s Whois data model is composed of six first order objects: networks, autonomous system numbers (ASNs), delegations, organizations (Orgs), points of contact (POCs), and customers. Each of these types, except delegations, is directly addressable in a Whois service via a handle (i.e., identifier). Within the Whois RESTful web service, these handles are part of URLs that may be used to identify objects in ARIN’s Whois registration database by external, non-ARIN systems.

### Networks and ASNs

Networks and ASNs are collectively referred to as “resources” in ARIN parlance (this should not be confused with the term “resources” in the context of RESTful web services and Resource Oriented Architectures). They are the pieces of information assigned or allocated to organizations for the coordinated administration and operation of the Internet.

Networks signify the allocation of IP address space and the contiguous IP CIDR blocks that make up that IP address space. Handles for IPv4 networks start with `NET-`, and handles for IPv6 networks start with `NET6-`.

ASNs are used for the proper routing of Internet packets. ARIN assigns ASNs in ranges, therefore a single ASN is part of an ASN range allocation. The handles for these registrations start with `AS` and are usually appended with the first number of the ASN range.

### Delegations

Delegations contain the information necessary for Reverse DNS, including the associated nameservers, and DNS DS record information. Unlike the other first order objects, delegations do not have a handle. Rather, they are directly addressable in a Whois service via a delegation name (i.e., `0.192.in-addr.arpa`).

### Organizations and Customers

Organizations are the registrants of resources and have a direct relationship with ARIN. Organizations may be associated with many resources. Customers do not have a direct relationship with ARIN and, at present, may only be associated with one network registration.

Customer handles start with “C,” while organization handles have no prefix.

### POCs

A POC is the registration of names, mailboxes, and/or phone numbers which fulfill technical or administrative functions on behalf of either an organization or a resource. POC handles are usually appended with `-ARIN`, though there are a few exceptions.

## General Query Behavior

For the first order objects addressable via handle, ARIN’s Whois services also allow for searching against names and other appropriate fields contained within those objects. Unless otherwise specified, these searches are case insensitive. Because ARIN’s networks are composed of multiple CIDR-based network blocks, searches by IP address or CIDR notation may sometimes yield what may, at first blush, appear incorrect. For instance, a search for a particular CIDR block may yield a network that covers the given CIDR block and additional CIDR blocks composing the network, while a search by an IP address will always yield the network or networks in which the IP address falls.

## RESTful Web Services

RESTful web services have the following characteristics:

* The “verbs” of the service are strictly those defined by the HTTP methods `GET`, `PUT`, `POST`, and `DELETE`. They are similar to the basic `CRUD` operations in data storage terminology.
* The “verbs” are used to act upon resources.
* Resources are addressable using URLs.

This differs significantly from other types of web services, such as SOAP, where the concept of resources are not defined and verbs are defined by the service authors.

While it is common to use XML with RESTful web services, XML is not strictly a requirement for RESTful web services. In fact, many RESTful web services forgo XML altogether in favor of JSON. This is also a key difference between RESTful web services and other types of web services such as SOAP.

An excellent tutorial on this subject is available from O’Reilly publishing: *[RESTful Web Services](https://www.oreilly.com/library/view/restful-web-services/9780596529260/)* by Leonard Richardson and Sam Ruby.

## ARIN’s Whois RESTful Web Service

ARIN’s Whois RESTful web service is the programmatic interface to accessing ARIN’s Whois data. Unlike other solutions to the problem of Whois data access, RESTful web services are well within the mainstream of information exchange protocols, and the re-use of HTTP and web technologies in the simple REST design patterns does not require ARIN to write client software.

### Command Line Clients and Web Browsers

As stated above, one of the benefits to a RESTful web service is that clients already exist. RESTful web service client software is anything capable of basic HTTP. In the case of Whois-RWS, the only real HTTP method ever used is `GET`. One of the best web browsers for this is Firefox, as it will take the returned raw XML and display it in pretty format.

ARIN offers a “web-based” interface to Whois data at <http://whois.arin.net>.

However, command line tools may be used as well:

```
curl -s http://whois.arin.net/rest/poc/KOSTE-ARIN <?xml version="1.0"encoding="UTF-8" standalone="yes"><poc>...</poc>
```

In the example above, `curl` retrieves the XML. The example doesn’t show the XML as it would be one big jumble of stuff. Other command line tools, such as `xmllint` can read HTTP resources by themselves and reformat XML to our liking. This next example shows this:

```
    $ xmllint --format http://whois.arin.net/rest/poc/KOSTE-ARIN
    <?xml version="1.0" encoding="UTF-8" standalone="yes"?>
    <poc>
      <ref>http://whois.arin.net/rest/poc/KOSTE-ARIN</ref>
      <city>Centreville</city>
      <companyName>ARIN</companyName>
      <iso3166-1>
        <code2>US</code2>
        <code3>USA</code3>
        <name>UNITED STATES</name>
        <e164>1</e164>
      </iso3166-1>
      <firstName>Mark</firstName>
      <handle>KOSTE-ARIN</handle>
      <emails>
        <email>markk@bjmk.com</email>
      </emails>
      <lastName>Kosters</lastName>
      <phones>
        <phone>
          <number>+1-703-227-9870</number>
          <type>
            <description>Office</description>
            <code>O</code>
          </type>
        </phone>
      </phones>
      <postalCode>20120</postalCode>
      <comment>
        <line number="0">I'm really MAK-21-ARIN</line>
      </comment>
      <registrationDate>2009-10-02T11:54:45-04:00</registrationDate>
      <iso3166-2>VA</iso3166-2>
      <streetAddress>
        <line number="0">PO Box 232290</line>
      </streetAddress>
      <updateDate>2009-10-02T11:54:45-04:00</updateDate>
    </poc>
```

### Data Formats

As already stated, RESTful web services are not strictly tied to XML. ARIN’s Whois RESTful web service offers data in XML, JSON, plain text, and HTML (actually, XHTML). However, ARIN’s first order data format is XML, as there are many format and validation tools readily available for it, and the other formats are offered on a best-effort basis.

If no desired format is specified, XML is the default format.

#### Specifying a Data Format

Specifying a desired data format may be accomplished in one of two ways: either using the HTTP `Accept` header or using file extensions.

The following is an example where JSON is specified using the `application/json` content type with the HTTP `Accept` header.

```
curl -H "Accept: application/json" http://whois.arin.net/rest/poc/KOSTE-ARIN{"poc":{"ref":{"$":"http:\\/\\/whois.arin.net\/rest\/poc\/KOSTE-ARIN"},"city":{"$":"Centreville"},"companyName":{"$":"ARIN"},"iso3166-1":{"code2":{"$":"US"},"code3":{"$":"USA"},"name":{"$":"UNITED STATES"},"e164":{"$":"1"}},"firstName":{"$":"Mark"},"handle":{"$":"KOSTE-ARIN"},"emails":{"email":{"$":"markk@bjmk.com"}},"lastName":{"$":"Kosters"},"phones":{"phone":{"number":{"$":"+1-703-227-9870"},"type":{"description":{"$":"Office"},"code":{"$":"O"}}}},"postalCode":{"$":"20120"},"comment":{"line":{"@number":"0","$":"I'm really MAK-21-ARIN"}},"registrationDate":{"$":"2009-10-02T11:54:45-04:00"},"iso3166-2":{"$":"VA"},"streetAddress":{"line":{"@number":"0","$":"PO Box 232290"}},"updateDate":{"$":"2009-10-02T11:54:45-04:00"}}}
```

A file extension appends a DOS-like file extension to the path. Therefore, in the following example, `http://whois.arin.net/rest/poc/KOSTE-ARIN.txt` would specify the data format for KOSTE-ARIN to be plain text.

```
$ curl http://whois.arin.net/rest/poc/KOSTE-ARIN.txt # ARIN WHOIS data and services are subject to the Terms of Use # available at: https://www.arin.net/whois_tou.html Name: Kosters, Mark Handle: KOSTE-ARIN Company: ARIN Address: PO Box 232290 City: Centreville StateProv: VA PostalCode: 20120 Country: US RegDate: 2009-10-02 Updated: 2009-10-02 Comment: I'm really MAK-21-ARIN Phone: +1-703-227-9870 (Office) Email: markk@bjmk.com Ref: http://whois.arin.net/rest/poc/KOSTE-ARIN
```

#### Specifying a Version of a Format

Though there only exists one version of this RESTful interface, it is possible that the data model of the structured data such as XML and JSON that is output by this service will need revision. Should it be possible for ARIN to provide a backward-compatible version, the HTTP `Accept` header is the mechanism for specifying the desired version.

The MIME type used in the `Accept` header will follow the format of `application/arin.whoisrws-{version}+xml`.

As an example, the version detailed in this document (v1) would be accessed the following way:

`GET /rest/poc/DUDE1-ARIN HTTP Accept: application/arin.whoisrws-v1+xml`

To use the latest version of this service you would use the default MIME types of `application/xml` or `application/json`.

#### MIME Types and File Extensions

The following table lists the data types and their associated MIME types and file extensions:

| Data type | Current Version MIME Type | Version 1 MIME Type | File Extension |
| --- | --- | --- | --- |
| XML | application/xml | application/arin.whoisrws-v1+xml | xml |
| JSON | application/json | application/arin.whoisrws-v1+json | json |
| plain text | text/plain |  | txt |
| HTML | text/html |  | html |

#### Data Format Extensibility

By design, the XML data returned by this service is intended to allow the introduction of new XML elements in other namespaces without the need to re-version or upgrade the current XML format. This is accomplished using namespaces in XML. The current XML namespace used by ARIN is identified with the `http://www.arin.net/whoisrws/core/v1` URL. Future elements may be added to the XML using other namespaces. Programs and processes parsing this XML should gracefully ignore XML elements they do not recognize.

Likewise, the JSON data returned by this service is mapped from XML using the “BadgerFish convention.” Namespaces are specified as object properties. The following is an example:

```
{"asn":{"@xmlns":{"$":"http:\/\/www.arin.net\/whoisrws\/core\/v1"},"@termsOfUse":"https:\/\/www.arin.net\/whois_tou.html","ref":{"$":"http:\/\/localhost:8080\/whoisrws\/seam\/resource\/rest\/asn\/AS8043"},"endAsNumber":{"$":"8043"},"handle":{"$":"AS8043"},"name":{"$":"LVLT-8043"},"orgRef":{"@name":"Level 3 Communications, Inc.","@handle":"LVLT","$":"http:\/\/localhost:8080\/whoisrws\/seam\/resource\/rest\/org\/LVLT"},"registrationDate":{"$":"1998-05-15T00:00:00-04:00"},"startAsNumber":{"$":"8043"},"updateDate":{"$":"2010-04-09T14:52:52.053-04:00"}}}
```

The XML that is returned for each RESTful web service call is distinct. ARIN provides [Relax NG schemas](https://www.arin.net/resources/registry/whois/rws/whoisrws-relaxng-compact-schemas.zip) so that client writers may understand the format, content and type of the XML that is returned by each RESTful web service call. The [RELAX NG](http://relaxng.org/) page provides further details about the RelaxNG schema language.

#### Data Transformation

XML has transformation capabilities in the way of XSLT. XSLT is built into modern browsers and allows the browser to convert the XML to a textual format or HTML or any other type on the client side. This gives the end-user great flexibility.

From the command line, there is a Unix tool called `xsltproc`. Here is an example of `xsltproc` transforming the XML from Whois-RWS into output that looks similar to traditional Whois output:

```
$ xsltproc poc.xsl http://whois.arin.net/rest/poc/KOSTE-ARIN    

Ref:       http://whois.arin.net/rest/poc/KOSTE-ARIN  
Name:      Kosters, Mark  
Handle:    KOSTE-ARIN  
Company:   ARIN  
Address:   PO Box 232290  
City:      Centreville  
StateProv: VA  
Country:   US  
RegDate:   2009-10-02T11:54:45-04:00
```

The stylesheet used for this transformation is as follows:

```
<?xml version="1.0"?>  
  <xsl:stylesheet version="1.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">    

  <xsl:output method="text" />    

  <xsl:template match="poc">  
  Ref:       <xsl:value-of select="ref" />  
  Name:      <xsl:value-of select="lastName"/>, <xsl:value-of select="firstName"/> <xsl:value-of select="middleName"/>  
  Handle:    <xsl:value-of select="handle"/>  
  Company:   <xsl:value-of select="companyName" />  
  Address:   <xsl:value-of select="streetAddress" />  
  City:      <xsl:value-of select="city" />  
  StateProv: <xsl:value-of select="iso3166-2" />  
  Country:   <xsl:value-of select="iso3166-1/code2" />  
  RegDate:   <xsl:value-of select="registrationDate" />  
  <xsl:text>sampletext</xsl:text>  
  </xsl:template>    

  </xsl:stylesheet>
```

ARIN uses XSL transformations with XML to provide the NICNAME/Whois port 43 proxy for this service and to provide HTML for use with web browsers. The XSL templates used for these services are available for public use as well.

```
    $ xsltproc http://whois.arin.net/xsl/legacy/detailed.xsl http://whois.arin.net/rest/poc/KOSTE-ARIN    

    Name:           Kosters, Mark  
    Handle:         KOSTE-ARIN  
    Company:        ARIN  
    Address:        PO Box 232290
    City:           Centreville  
    StateProv:      VA  
    PostalCode:     20120  
    Country:        US  
    RegDate:        2009-10-02  
    Updated:        2009-10-02  
    Comment:        I'm really MAK-21-ARIN  
    Email:          markk@bjmk.com  
    Phone:          +1-703-227-9870 (Office)  
    Ref:            http://whois.arin.net/rest/poc/KOSTE-ARIN
```

ARIN also inserts the `xml-stylesheet` XML processing instruction in XML results so that web browsers will automatically transform XML into the Whois style text format. The default stylesheet renders the document to look like ARIN’s website.

### ARIN’s RESTful Resources

In the examples from the previous sections, the resources being queried had the URL `http://whois.arin.net/rest/poc/KOSTE-ARIN`. Conceptually, this can be broken into a base URL and the resource identifier:

* base URL = `http://whois.arin.net/rest`
* resource = `/poc/KOSTE-ARIN`

The base URL is just where ARIN is hosting the service. The interesting parts are the bits that identify the resources:

* `/poc`: Indicates the resource is a Point of Contact
* `/KOSTE-ARIN`: The unique identifier for the POC, or the POC Handle

ARIN’s data model has six types of addressable resources. These are reflected in Whois-RWS in the following way:

* `/poc`: point of contact
* `/org`: organization
* `/net`: network
* `/asn`: autonomous system number(s)
* `/customer`: customer of an organization
* `/rdns`: delegation

For example, to get an organization record that has the handle “ARIN,” one would do the following:

```
    $ xmllint --format http://whois.arin.net/rest/org/ARIN  
    <?xml version="1.0" encoding="UTF-8" standalone="yes"?>  
    <org>
        <ref>http://whois.arin.net/rest/org/ARIN</ref>
        <city>Centreville</city>
        <iso3166-1>
          <code2>US</code2>
          <code3>USA</code3>
          <name>UNITED STATES</name>
          <e164>1</e164>
        </iso3166-1>
        <handle>ARIN</handle>
        <name>American Registry for Internet Numbers</name>
        <postalCode>20120</postalCode>
        <comment>
          <line number="0">For abuse issues please see URL:</line>
          <line number="1">http://www.arin.net/abuse.html</line>
          <line number="2">The Registration Services Help Desk is open</line>
          <line number="3">from 7 a.m. to 7 p.m., U.S. Eastern time to
          assist you.</line>
          <line number="4">Phone Number: (703) 227-0660; Fax Number:
          (703) 227-0676.</line>
        </comment>    <registrationDate>1997-12-22T00:00:00-
        05:00</registrationDate>
        <iso3166-2>VA</iso3166-2>
        <streetAddress>
          <line number="0">PO Box 232290</line>
        </streetAddress>
        <updateDate>2007-02-27T14:56:43-05:00</updateDate>
      </org>
```

#### Resources Related To Resources

In the ARIN Whois data model, resources have relationships to other resources. Getting references to these resources can be accomplished by addressing the resource in question and applying a `resource type` qualifier.

For instance, to fetch references to the POCs of the Org “ARIN,” one would append `/pocs` to the end of the resource `/org/ARIN` to come up with `/org/ARIN/pocs`. Here is an example:

```
    $ xmllint --format http://whois.arin.net/rest/org/ARIN/pocs
    <?xml version="1.0" encoding="UTF-8" standalone="yes"?>
      <pocs>
        <limitExceeded limit="10">false</limitExceeded>
        <pocLinkRef handle="ARIN-HOSTMASTER" function="T" description="Tech">http://whois.arin.net/rest/poc/ARIN-HOSTMASTER</pocLinkRef>
        <pocLinkRef handle="ARINN-ARIN" function="N" description="NOC">http://whois.arin.net/rest/poc/ARINN-ARIN</pocLinkRef>
      </pocs>
```

In this example, note that the results are references to POCs addressed using URLs.

Here is the list of relationships (where “XXXX” signifies a handle):

* `/poc/XXXX`
  + `/orgs` ==> `/poc/XXXX/orgs`: Lists the organizations associated with a given POC
  + `/asns` ==> `/poc/XXXX/asns`: Lists the ASNs associated with a given POC
  + `/nets` ==> `/poc/XXXX/nets`: Lists the networks associated with a given POC
* `/org/XXXX`
  + `/pocs` ==> `/org/XXXX/pocs`: Lists the POCs associate with a given organization
  + `/asns` ==> `/org/XXXX/asns`: Lists the ASNs associated with a given organization
  + `/nets` ==> `/org/XXXX/nets`: Lists the networks associated with a given organization
* `/asn/XXXX`
  + `/pocs` ==> `/asn/XXXX/pocs`: Lists the POCs associated with a given ASN
* `/net/XXXX`
  + `/pocs` ==> `/net/XXXX/pocs`: Lists the POCs associated with a given network
  + `/parent` ==> `/net/XXXX/parent`: Lists the parent network of a given network
  + `/children` ==> `/net/XXXX/children`: Lists the child networks of a given network
  + `/rdns` ==> `/net/XXXX/rdns`: Lists the delegations of a given network
* `/rdns/DELEGATION_NAME`
  + `/nets` ==> `/rdns/DELEGATION_NAME/nets`: Lists networks related to a given delegation

#### Getting Unrelated Lists of Resources

Unrelated lists of resources may be addressed using URL [matrix parameters](http://www.w3.org/DesignIssues/MatrixURIs.html). As an example, to find organizations with the name “ARIN,” the `/orgs` list is appended with the matrix parameter `name` to form `/orgs;name=ARIN` (this is without the base URL). The full example would be:

```
$ xmllint --format "http://whois.arin.net/rest/orgs;name=ARIN"
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
  <orgs>
    <limitExceeded limit="10">false</limitExceeded>
    <orgRef name="American Registry for Internet Numbers" handle="ARIN">http://whois.arin.net/rest/org/ARIN</orgRef>
  </orgs>
```

Substring searches can be applied to the trailing end of parameters. To find all the organizations that start with “ARIN,” the example would be:

```
$ xmllint --format "http://whois.arin.net/rest/orgs;name=ARIN*"
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
  <orgs>
    <limitExceeded limit="10">true</limitExceeded>
    <orgRef name="American Registry for Internet Numbers" handle="ARIN">http://whois.arin.net/rest/org/ARIN</orgRef>
    <orgRef name="ARINBE INCORPORATED" handle="ARINBE">http://whois.arin.net/rest/org/ARINBE</orgRef>
    <orgRef name="ARINC" handle="ARINC-10">http://whois.arin.net/rest/org/ARINC-10</orgRef>
    <orgRef name="ARINC" handle="ARINC-2">http://whois.arin.net/rest/org/ARINC-2</orgRef>
    <orgRef name="ARINC" handle="ARINC-4">http://whois.arin.net/rest/org/ARINC-4</orgRef>
    <orgRef name="ARINC, Inc." handle="ARINCI">http://whois.arin.net/rest/org/ARINCI</orgRef>
    <orgRef name="ARINC, Inc." handle="ARINCI-1">http://whois.arin.net/rest/org/ARINCI-1</orgRef>
    <orgRef name="ARINC Wireless Broadband" handle="ARINC-9">http://whois.arin.net/rest/org/ARINC-9</orgRef>
    <orgRef name="ARING EQUIPMENT COMPANY" handle="ARING">http://whois.arin.net/rest/org/ARING</orgRef>
    <orgRef name="ARINSO Canada Inc" handle="ARINSO-1">http://whois.arin.net/rest/org/ARINSO-1</orgRef>
  </orgs>
```

Note the quoting of the URL. This is because these examples are using the Unix command line, where characters such as `;` and `\*` are meaningful unless escaped.

Matrix parameters may appear in any order. In the following example, both URLs produce the same POC list:

```
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
  <pocs>
    <limitExceeded limit="10">false</limitExceeded>
  </pocs>

<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
  <pocs>
    <limitExceeded limit="10">false</limitExceeded>
  </pocs>
```

Here is the list of resources and their supported parameters:

* `/orgs`
  + handle: the handle of the organization
  + name: the name of organization
  + dba: the name the organization does business as
* `/customers`
  + handle: the handle of the customer
  + name: the name of the customer
* `/pocs`
  + handle: the handle of the POC
  + domain: the domain of the email address for the POC
  + first: the first name of the POC
  + middle: the middle name of the POC
  + last: the last name of the POC
  + company: the company name registered by the POC
  + city: the city registered by the POC
* `/asns`
  + handle: the handle of the ASN
  + name: the name of the ASN
* `/nets`
  + handle: the handle of the network
  + name: the name of the network
* `/rdns`
  + delegation name: the name of the delegation (i.e., `0.192.in-addr.arpa`).

#### IP Addresses and Networks

In the ARIN data model, an IP “network” is a set of associated IP address blocks and the information related to these IP address blocks and the set as a whole. A network can be composed of one IP address block or of multiple IP address blocks. Networks are also hierarchical, meaning that a network can have a parent and can have children. An IP address or range of IP addresses can therefore fall within the IP address block of multiple networks.

To get the networks that a particular IP address may fall within, one can use the `/ip/XX.XX.XX.XX` resource, where `XX.XX.XX.XX` signifies the IP address. Here is an example:

```
$ xmllint --format http://whois.arin.net/rest/ip/192.149.252.75
<?xml version="1.0"?>
  <?xml-stylesheet type='text/xsl' href='http://whois.arin.net/xsl/website.xsl' ?>
  <net xmlns="http://www.arin.net/whoisrws/core/v1" termsOfUse="https://www.arin.net/whois_tou.html">
    <registrationDate>1997-11-05T00:00:00-05:00</registrationDate>
    <ref>http://whois.arin.net/rest/net/NET-192-149-252-0-1</ref>
    <endAddress>192.149.252.255</endAddress>
    <handle>NET-192-149-252-0-1</handle>
    <name>ARIN-NET</name><netBlocks>
      <netBlock>
        <cidrLength>24</cidrLength>
        <endAddress>192.149.252.255</endAddress>
        <description>Direct Allocation</description>
        <type>DA</type>
        <startAddress>192.149.252.0</startAddress>
      </netBlock>
    </netBlocks>
    <originASes>
      <originAS>AS10745</originAS>
    </originASes>
    <parentNetRef name="NET192" handle="NET-192-0-0-0-0">http://whois.arin.net/rest/net/NET-192-0-0-0-0</parentNetRef>
    <startAddress>192.149.252.0</startAddress>
    <updateDate>2007-03-28T00:00:00-04:00</updateDate>
    <version>4</version>
  </net>
```

Networks may also be looked up by supplying the full CIDR notation of a range. To use the full CIDR notation, the resource looks like `/cidr/XX.XX.XX.XX/YY` where `XX.XX.XX.XX` is the IP address prefix and `YY` is the CIDR length.

Here are some examples:

```
# IP address block by full CIDR notation
http://whois.arin.net/rest/cidr/192.149.252.0/24
```

Resources relative to the networks may be fetched using the `/less` and `/more` path prefixes. `/less` will find the networks that are less specific in scope (or wider), and `/more` will find the networks that are more specific in scope (or narrower).

Also, in some cases a CIDR block query will return a less-specific network registration when the CIDR block in the query is a proper-CIDR-boundary allocation segment of the overall network registration, and one of the following occurs:

* the full network registration is displayed on a single match
* the `/more` flag is used, and a list of network registrations are displayed (the `more-specifics` will display, and so will the registration containing the CIDR block in the query)

Here are the examples from above with the `/less` and `/more` prefixes:

```
# the less specific IP address block by full CIDR notation
http://whois.arin.net/rest/cidr/192.149.252.0/24/less

# the more specific IP address block by full CIDR notation
http://whois.arin.net/rest/cidr/192.149.252.0/24/more
```

IPv6 equivalents for each of these are also available by simply using a v6 address instead.

### HTTP Query Parameters

Query parameters can be used to provide additional context to some URLs. They are primarily used to instruct the server to return additional data or scope the type of data returned. These can be useful in getting desired data in fewer RESTful calls.

#### Showing More Detail

By default, lists of resources only show references to the resources. However, lists can be expanded to show full detail by tacking on a `showDetails=true` URL query parameter (e.g.,`http://whois.arin.net/rest/org/ARIN/pocs?showDetails=true`). This query parameter also forces a resource to list its related resources in line. The following example shows a detailed list of the POCs related to the organization “ARIN.” Notice how each POC also lists references to its related organization.

```
$ xmllint --format http://whois.arin.net/rest/org/ARIN/pocs
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
  <pocs>
    <limitExceeded limit="10">false</limitExceeded>
    <pocLinkRef handle="ARIN-HOSTMASTER" function="T" description="Tech">http://whois.arin.net/rest/poc/ARIN-HOSTMASTER</pocLinkRef>
    <pocLinkRef handle="ARINN-ARIN" function="N" description="NOC">http://whois.arin.net/rest/poc/ARINN-ARIN</pocLinkRef>
  </pocs>
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
  <pocs>
    <limitExceeded limit="10">false</limitExceeded>
    <poc>
      <ref>http://whois.arin.net/rest/poc/ARIN-HOSTMASTER</ref>
      <asns>
        <limitExceeded limit="10">false</limitExceeded>
        <asnPocLinkRef relPocName="Registration Services Department" relPocHandle="ARIN-HOSTMASTER" name="ARIN" handle="AS10745" relPocFunction="T" relPocDescription="Tech">http://whois.arin.net/rest/asn/AS10745</asnPocLinkRef>
        <asnPocLinkRef relPocName="Registration Services Department" relPocHandle="ARIN-HOSTMASTER" name="PUBLICVA" handle="AS393225" relPocFunction="AB" relPocDescription="Abuse">http://whois.arin.net/rest/asn/AS393225</asnPocLinkRef>
        <asnPocLinkRef relPocName="Registration Services Department" relPocHandle="ARIN-HOSTMASTER" name="PUBLICSJC" handle="AS393220" relPocFunction="AB" relPocDescription="Abuse">http://whois.arin.net/rest/asn/AS393220</asnPocLinkRef>  
        <asnPocLinkRef relPocName="Registration Services Department" relPocHandle="ARIN-HOSTMASTER" name="PUBLICSJC" handle="AS393220" relPocFunction="T" relPocDescription="Tech">http://whois.arin.net/rest/asn/AS393220</asnPocLinkRef>   
        <asnPocLinkRef relPocName="Registration Services Department" relPocHandle="ARIN-HOSTMASTER" name="PUBLICVA" handle="AS393225" relPocFunction="T" relPocDescription="Tech">http://whois.arin.net/rest/asn/AS393225</asnPocLinkRef>
     </asns>
      <city>Centreville</city>
      <companyName>American Registry for Internet Numbers</companyName>
      <iso3166-1>
        <code2>US</code2>
        <code3>USA</code3>
        <name>UNITED STATES</name>
        <e164>1</e164>
      </iso3166-1>
      <handle>ARIN-HOSTMASTER</handle>
      <emails>
        <email>hostmaster@arin.net</email>
      </emails>
      <lastName>Registration Services Department</lastName>
      <nets>
        <limitExceeded limit="10">false</limitExceeded>
        <netPocLinkRef relPocName="Registration Services Department" relPocHandle="ARIN-HOSTMASTER" name="ARIN-BLK-2" handle="NET-192-136-136-0-1" relPocFunction="T" relPocDescription="Tech">http://whois.arin.net/rest/net/NET-192-136-136-0-1</netPocLinkRef>
        <netPocLinkRef relPocName="Registration Services Department" relPocHandle="ARIN-HOSTMASTER" name="ARIN-NET" handle="NET-192-149-252-0-1" relPocFunction="T" relPocDescription="Tech">http://whois.arin.net/rest/net/NET-192-149-252-0-1</netPocLinkRef>
      </nets>
      <orgs>
        <limitExceeded limit="10">false</limitExceeded>
        <orgPocLinkRef relPocName="Registration Services Department" relPocHandle="ARIN-HOSTMASTER" name="American Registry for Internet Numbers" handle="ARIN" relPocFunction="T" relPocDescription="Tech">http://whois.arin.net/rest/org/ARIN</orgPocLinkRef>
        <orgPocLinkRef relPocName="Registration Services Department" relPocHandle="ARIN-HOSTMASTER" name="American Registry for Internet Numbers" handle="ARIN" relPocFunction="AD" relPocDescription="Admin">http://whois.arin.net/rest/org/ARIN</orgPocLinkRef>
      </orgs>
      <phones>
        <phone>
          <number>+1-703-227-0660</number>
          <type>
            <description>Office</description>
            <code>O</code>
          </type>
        </phone>
      </phones>
      <postalCode>20120</postalCode>
      <registrationDate>2003-04-30T12:32:56-04:00</registrationDate>
      <iso3166-2>VA</iso3166-2>
      <streetAddress>
        <line number="0">PO Box 232290</line>
      </streetAddress>
      <updateDate>2007-02-27T14:57:56-05:00</updateDate>
    </poc>
    <poc>
      <ref>http://whois.arin.net/rest/poc/ARINN-ARIN</ref>
      <asns>
        <limitExceeded limit="10">false</limitExceeded>
        <asnPocLinkRef relPocName="ARIN NOC" relPocHandle="ARINN-ARIN" name="PUBLICVA" handle="AS393225" relPocFunction="N" relPocDescription="NOC">http://whois.arin.net/rest/asn/AS393225</asnPocLinkRef>
        <asnPocLinkRef relPocName="ARIN NOC" relPocHandle="ARINN-ARIN" name="PUBLICSJC" handle="AS393220" relPocFunction="N" relPocDescription="NOC">http://whois.arin.net/rest/asn/AS393220</asnPocLinkRef>
      </asns>
      <city>Centreville</city>
      <companyName>American Registry for Internet Numbers</companyName>
      <iso3166-1>
        <code2>US</code2>
        <code3>USA</code3>
        <name>UNITED STATES</name>
        <e164>1</e164>
      </iso3166-1>
      <handle>ARINN-ARIN</handle>
      <emails>
        <email>noc@arin.net</email>
      </emails>
      <lastName>ARIN NOC</lastName>
      <nets>
        <limitExceeded limit="10">false</limitExceeded>
      </nets>
      <orgs>
        <limitExceeded limit="10">false</limitExceeded>
        <orgPocLinkRef relPocName="ARIN NOC" relPocHandle="ARINN-ARIN" name="American Registry for Internet Numbers" handle="ARIN" relPocFunction="N" relPocDescription="NOC">http://whois.arin.net/rest/org/ARIN</orgPocLinkRef>
      </orgs>
      <phones>
        <phone>
          <number>+1-703-227-9840</number>
          <type>
            <description>Office</description>
            <code>O</code>
          </type>
        </phone>
      </phones>
      <postalCode>20120</postalCode>
      <registrationDate>2002-11-05T12:03:56-05:00</registrationDate>
      <iso3166-2>VA</iso3166-2>
      <streetAddress>
        <line number="0">PO Box 232290</line>
      </streetAddress>
      <updateDate>2004-02-24T12:27:52-05:00</updateDate>
    </poc>
  </pocs>
```

#### Showing Only POCs

Some organizations have many associated resources. Having all of those resources returned when gathering information about an organization may be very undesirable. To retrieve all of the POCs of an organization without taking the penalty of retrieving all the resources of the organization, the `showPocs=true` parameter may be used. This parameter is only recognized when an organization’s information is referenced by handle.

#### Showing ARIN Resources

In the past, ARIN’s NICNAME/WHOIS service yielded a “No Match” result for searches of IP address space unallocated by ARIN but within ARIN’s IP address pool. This was somewhat confusing and could possibly mislead a person into thinking the address space is simply unallocated to any available address pool (in other words, still held by the IANA). We hope to some day make this change in ARIN’s NICNAME/WHOIS service so it will return the appropriate ARIN network allocation for IP addresses unallocated by ARIN.

By default, the RESTful service will return the appropriate ARIN network allocation for IP addresses unallocated by ARIN to ARIN constituents. If this behavior is not desired, you can use the `showARIN=false` query parameter on both IP and CIDR queries.

### HTTP Caching

One of the advantages of a RESTful web service is that HTTP infrastructure may be employed to make it more reliable and/or robust. HTTP caching is one such tool to help with these goals. ARIN employs caching at multiple levels and it is customized to address the query pattern seen against ARIN’s services.

If HTTP caching is be employed on the client side, there are a number of issues to consider to craft a custom caching solution.

First, caching should be restricted to URLs that have the highest likelihood of remaining in the cache using the cache’s retention strategy. In almost any scenario, caching RESTful resources under `/rest/ip` will lead to a large dataset and a low cache hit ratio. However, it is likely acceptable to cache resources under `/rest/org`, `/rest/poc`, and `/rest/net`. It is recommended that cache statistics be available for proper cache tuning.

Second, many HTTP caches base the cache objects on URLs. However, the `Accept` header dictates the content of the object and may not be part of the cache key of cached objects. Therefore, it is possible to get a cached object of one type when another type was requested. For instance, `/rest/poc/KOSTE-ARIN` might return XML when JSON was requested. To overcome this, it is advisable to use file extensions to specify data format, as file extensions are part of the URL.

## Update Frequency

ARIN’s current Whois infrastructure receives updates from ARIN’s registration database only once a day. For Whois-RWS, a new data distribution system has been engineered to push data from ARIN’s registration database out to the Whois-RWS servers many times throughout the day. At present, the Whois-RWS servers are updated every 10 minutes.

## NICNAME/Whois Port 43 Proxy

ARIN will continue to provide services for the NICNAME/Whois protocol operational on TCP/43 and has created a proxy service that translates traditional ARIN Whois queries into Whois-RWS queries. This proxy service will replace the current NICNAME/Whois protocol service. It uses XSL stylesheets to return the information into a form very similar to current Whois output, and it has been enhanced to support new query types, provide better feedback for ambiguous queries, and provide options for NICNAME/Whois clients that re-interpret traditional parameters used by ARIN’s service.

The following is an example of using the demonstration Whois proxy service:

```
    $ whois -h whois.arin.net ARIN  
    [Querying whois.arin.net]  
    [whois.arin.net]    

    #  
    # The following results may also be obtained via:  
    # http://whois.arin.net/rest/asns/;q=ARIN?showDetails=true  
    #    

    American Registry for Internet Numbers (AS10745) ARIN 10745    

    #  
    # The following results may also be obtained via:  
    # http://whois.arin.net/rest/orgs;q=ARIN?showDetails=true  
    #    

    American Registry for Internet Numbers (ARIN)    

    #  
    # The following results may also be obtained via:  
    # http://whois.arin.net/rest/pocs/;q=ARIN?showDetails=true  
    #    

    ARIN,  (ARIN1-ARIN) arin@us.loreal.com  
    ARIN,  (ARIN3-ARIN) ARIN@arrival.com  
    ARIN,  (ARIN4-ARIN) arin@livewireservicesinc.com  
    Arin,  (ARIN6-ARIN) arin@logisticsplus.net  
    ARIN,  (ARIN7-ARIN) arin@evolveip.net  
    ARIN,  (ARIN8-ARIN) arin@sotech.com  
    ARIN,  (BM916-ARIN) arin@purchasepro.com  
    Arin, Amsouth (AA578-ARIN) amsouth.arin@amsouth.com  
    Arin, MORIC (MAR177-ARIN) moricarin@moric.org  
    ARIN, NETWORK (NAR13-ARIN) NETWORK.ARIN@rocklandtrust.com
```

This is just an example of how to use a typical Whois NICNAME client. You will need to consult the documentation for your particular client to determine how to specify a specific host to query (in this case, whois.arin.net). The proxy service attempts to format the data in the same style as the current Whois service, but the formatting is not 100% compatible.

### Using NICNAME/Whois Clients

Though the NICNAME/Whois protocol specified in RFC 3912 is very simple, NICNAME/Whois clients vary in their capabilities and features, and these variances can create quite a bit of confusion for users. It is for these reasons ARIN is championing Whois-RWS as it takes advantage of more rigorously specified, well-understood web technologies. If you wish to use the NICNAME/Whois protocol service offered by ARIN, you must carefully read the documentation that accompanies it.

To use a NICNAME/Whois client with ARIN’s NICNAME/Whois service, you must instruct the client to connect against ARIN’s servers. The following is a list of various methods you may need to employ to connect to ARIN’s NICNAME/Whois servers:

* Some clients use the `-h` option to directly specify a server’s hostname. For example, `-h whois.arin.net` instructs the clients to connect to ARIN’s Whois server at host `whois.arin.net`.
* Instead of the `-h` option, some clients use `@`, followed by the hostname of the server.
* Some clients use the `-a` option to explicitly specify ARIN’s Whois servers, instead of servers from other information sources.
* Some clients consult a master NICNAME/Whois redirection server.
* Some clients determine the servers to query based on the type of query being issued.

In addition to directing NICNAME/Whois clients at the proper server, you must be aware of how some clients handle the query itself. The following is a list of issues for which you should be aware when using NICNAME/Whois clients:

* As mentioned above, some clients will determine which server or set of servers to use based on the query issued and the results of the query.
* It is important to understand any quoting or escaping of the query that maybe adversely interpreted by the operating system being used. For example, the semi-colon (";") would need to be quoted or escaped on most Unix or Unix-like systems.
* In order to accommodate Internationalized Domain Names, some clients interpret the period (".") as signifying an Internationalized Domain Name. ARIN has traditionally used the period character to narrow searches to names of entities. With some clients, these two uses will conflict.
* Unless otherwise quoted, some clients will interpret whitespace present in a query to signify individual queries. However, whitespace is significant in separating query parameters for query targets, especially with ARIN’s service.

### NICNAME/Whois Queries

Queries to the NICNAME/Whois service can be scoped by a combination of record type, record attribute, record hierarchy, and display type flags. Many of these flags can be combined to help constrain queries. Flags must be separated from each other and from the search term by space characters.

If no record type flag is given, an attempt is made to match the query against an assumed record type. If it is not possible to assume a record type, the “e” flag is assumed if the query looks like a search that would normally be found in ARIN’s Whois data set. The following record types are supported:

* “n” for network address space
* “r” for network address space by CIDR
* “d” for delegations
* “a” for ASNs
* “p” for POCs
* “o” for Orgs
* “c” for customers
* “e” for POCs, Orgs, and customers
* “z” for all of the above (a legacy behavior when no record type was given)

Without an explicit record attribute flag, queries will only search against handles (or identifiers) and names. The record attributes supported are:

* “@” for matching the domain portion of an email address
* “!” for matching against a record handle or identifier
* “/” for matching against a name
* “.” for matching against a name (same as above, but some NICNAME/Whois clients treat the period character as part of an IDN, thus making it difficult to use for this case)

Some records in the ARIN Whois dataset have a hierarchical relationship with other records. To those related records, the following record hierarchy flags may be used:

* “<” to obtain records “up” the hierarchy
* “>” to obtain records “down” the hierarchy
* “=” to obtain records that are an exact match

Without an explicit display flag, record details are only displayed if one match is found and summary information is used when more than one record is found. The display flags supported are:

* “+” to obtain a full listing of the records
* “-” to obtain only a summary listing of the records

### Assumed and Ambiguous Queries

Queries missing record type and possible attribute flags can be ambiguous. In cases where these flags are not given but the service can deduce the intent of the query, the service will state the assumed query and return the results.

To avoid assumed queries, use record type and appropriate attribute flags. Using “?” will yield help information for properly formulating queries.

### IP and CIDR Queries

Network address space can be queried by either a single IP address or with CIDR network notation. CIDR notation must give the full network address in the prefix portion. The following is an example of a CIDR lookup.

```
$ whois -h whois.arin.net "192.0.0.0/8"  
#  
# Query terms are ambiguous.  The query is assumed to be:  
#     "r 192.0.0.0/8"  
#  
# Use "?" to get help.  
#    

#  
# The following results may also be obtained via:  
# http://whois.arin.net/rest/cidr/192.0.0.0/8?showDetails=true  
#    
NetRange:       192.0.0.0 - 192.255.255.255  
CIDR:           192.0.0.0/8  
OriginAS:  
NetName:        NET192  
NetHandle:      NET-192-0-0-0-0  
Parent:  
NetType:        Early Registrations, Maintained by ARIN  
NameServer:     DILL.ARIN.NET  
RegDate:        1993-05-01  
Updated:        2010-01-20  
Ref:            http://whois.arin.net/rest/net/NET-192-0-0-0-0

OrgName:        Various Registries (Maintained by ARIN)  
OrgId:          VR-ARIN  
Address:        PO Box 232290
City:           Centreville  
StateProv:      VA  
PostalCode:     20120  
Country:        US  
RegDate:        1993-05-01  
Updated:        2004-02-24  
Comment:        Address space was assigned by the InterNIC regardless of  
Comment:        geographic region.  The registrations are now being maintained  
Comment:        by various registries, and the in-addr.arpa delegations are  
Comment:        being maintained by ARIN.  
Ref:            http://whois.arin.net/rest/org/VR-ARIN    

#  
# ARIN WHOIS data and services are subject to the Terms of Use  
# available at: https://www.arin.net/whois_tou.html  
#
```

The hierarchy flags may be used with CIDR queries, with the " <" flag signifying less specific matching, “>” signifying more specific matching, and “=” signifying exact matching. If no flag is given, the default is to use less specific matching.

### Delegation Queries

In March 2011, ARIN made improvements to DNS, allowing finer control of DNS delegations. Results for network, IP, and CIDR queries no longer return nameserver information. The following delegation queries have been added to the NICNAME/WHOIS port 43 service:

* “d ! NET\_HANDLE” – given a network handle, what are the related delegations?
* “d / DELEGATION\_NAME” – lookup of a specific delegation by name

If the query does not specify any flags, but the query string ends in:

* .in-addr.arpa
* .in-addr.arpa.
* .ip6.arpa
* .ip6.arpa.

then the query may be interpreted as if it was a lookup of a specific delegation by name (i.e. `d / DELEGATION_NAME`).

Delegation information consists of delegation name, the associated nameservers and DNS DS record information.

When retrieving the delegation information via `NET_HANDLE`, the results will be listed in summary format if more than one delegation is returned. Otherwise the delegation information will be displayed in detail format (show all nameservers and DS information of the delegation). This behavior will be overridden in the presence of the “+” detail summary flag and the “-” list summary flag.

The hierarchy flags will have no meaning for these queries and will be ignored.
