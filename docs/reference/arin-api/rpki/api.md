# ARIN RPKI RESTful API User Guide

Source: https://www.arin.net/resources/manage/rpki/rpki-restful/

Retrieved: 2026-09-22T22:50:51.586865+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

As part of the 28 July 2026 release, ARIN’s RESTful API now supports the preferred method of sending your API key as a header, instead of within the URL. This endpoint is designed to keep your API key more private and secure, by transmitting it within the call and not allowing it to be captured at any point in the process. ARIN still accepts RESTful calls with the API key in the URL.

## Introduction

In the [Resource Public Key Infrastructure (RPKI)](https://www.arin.net/rpki), a Route Origin Authorization (ROA) is an attestation that the holder of a set of prefixes has authorized an Autonomous System to originate routes for those prefixes. An Autonomous System Provider Authorization (ASPA) is a cryptographically signed object made by the authorized resource holder that allows holders of Autonomous System (AS) identifiers in their capacity as customers to authorize other ASes as their providers.

You can use ARIN’s Representational State Transfer (REST) commands to create, delete, and list your ROAs and ASPAs.

Full details on ARIN’s RESTful endpoints [can be found here](https://www.arin.net/resources/manage/regrws/quickstart/).

## Create, Modify and Delete ROAs

This is a unified call for creating, modifying, and deleting ROAs. This operation can be performed in a single transaction combined with the “Create, Modify, and Delete ASPAs” operation, meaning that they will all succeed or fail together.

ARIN will automatically renew all ROAs created with this service.

If Reg-RWS returns an error code and/or Error Payload to you when performing this call, refer to this list of [error codes](https://www.arin.net/resources/manage/regrws/methods/#error-codes).

**API Key in Header (Recommended)**

**Method:** `POST`  
**URL:** /rest/rpki/ORGHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** RpkiTransactionPayload  
**Returns:** RpkiTransactionPayload

**API Key in URL (Supported)**

**Method:** `POST`  
**URL:** /rest/rpki/ORGHANDLE?apikey=APIKEY  
**Content:** RpkiTransactionPayload  
**Returns:** RpkiTransactionPayload

The **RPKI Transaction Payload** describes a list of ROAs to be deleted and a list of ROAs to be created.

The `autoRenewed` and `roaSpecAdd.roaSpec.roaHandle` fields are only set by the server in the response.

**Note**: As part of the 4 November 2024 release, and the introduction of an [IRR Auto-Manager](https://www.arin.net/resources/manage/rpki/irrautomanager), the RPKI Transaction Payload has been updated.

At creation, autoLink behavior is controlled entirely by the “roaSpec:autoLink” tag. If “roaSpec:autoLink” is set to true on the request, every “roaResources” for that ROA will be autolinked to an appropriate route/route6 object (creating them if necessary). The “roaSpecResource:autolinked” tag is informational, and only set in the response.

You can look up the autoLinked route/route6 object using the [REST API](https://www.arin.net/resources/manage/irr/irr-restful/#viewing-a-route-or-route6-object).

**Payload**

```
<rpkiTransaction
  xmlns="http://www.arin.net/regrws/rpki/v1" >
  <roaSpecDelete>
    <roaHandle autoLink = ""></roaHandle>
    <roaHandle autoLink = ""></roaHandle>
  </roaSpecDelete>
  <roaSpecAdd>
    <roaSpec>
      <autoLink></autoLink>
      <asNumber></asNumber>
      <name></name>
      <resources>
        <roaSpecResource>
          <autoLinked></autoLinked>
          <startAddress></startAddress>
          <cidrLength></cidrLength>
        </roaSpecResource>
      </resources>
    </roaSpec>
    <roaSpec>
      <autoLink></autoLink>
      <asNumber></asNumber>
      <name></name>
      <resources>
        <roaSpecResource>
          <autoLinked></autoLinked>
          <startAddress></startAddress>
          <cidrLength></cidrLength>
          <maxLength></maxLength>
        </roaSpecResource>
        <roaSpecResource>
          <autoLinked></autoLinked>
          <startAddress></startAddress>
          <cidrLength></cidrLength>
          <maxLength></maxLength>
        </roaSpecResource>
      </resources>
    </roaSpec>
  </roaSpecAdd>
</rpkiTransaction>
```

**Example**

```
<rpkiTransaction
  xmlns="http://www.arin.net/regrws/rpki/v1" >
  <roaSpecDelete>
    <roaHandle autolink = "true">24ab90ed2342355e12343aca12345150</roaHandle>
    <roaHandle autolink = "true">1980ed234270809a079700723b987088</roaHandle>
  </roaSpecDelete>
  <roaSpecAdd>
    <roaSpec>
      <autoLink>true</autoLink>
      <asNumber>64496</asNumber>
      <name>headquarters</name>
      <resources>
        <roaSpecResource>
          <startAddress>192.0.2.0</startAddress>
          <cidrLength>24</cidrLength>
        </roaSpecResource>
      </resources>
    </roaSpec>
    <roaSpec>
      <autoLink>true</autoLink>
      <asNumber>64497</asNumber>
      <name>cloud_offerings</name>
      <resources>
        <roaSpecResource>
          <startAddress>198.51.100.0</startAddress>
          <cidrLength>24</cidrLength>
          <maxLength>25</maxLength>
        </roaSpecResource>
        <roaSpecResource>
          <startAddress>203.0.113.0</startAddress>
          <cidrLength>24</cidrLength>
          <maxLength>25</maxLength>
        </roaSpecResource>
      </resources>
    </roaSpec>
  </roaSpecAdd>
</rpkiTransaction>
```

## Get a List of ROAs for an Org

This call returns a list of ROAs for the Org specified in your URL, with a ROA Spec Payload included for each ROA. The ROA Spec Payload contains information about an individual ROA such as ASNs, IP addresses, and valid dates.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/roa/ORGHANDLE?   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** ROA Spec Payload

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/roa/ORGHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** ROA Spec Payload

The **ROA Spec Payload** provides information about a ROA, including:

* `asNumber`: The Autonomous System Number (ASN) from which the route will originate.
* `name`: A descriptive name associated with the ASN.
* `notValidBefore/notValidAfter`: Dates that the ROA is valid.
* `cidrLength`: Prefix that indicates the number of bits in the IP address block that is routed.
* `startAddress/endAddress`: Beginning and end IP addresses of the IP address block that is routed.
* `ipVersion`: IP version of the address block.
* `maxLength`: Specifies the maximum length of IP address prefix that the AS is authorized to advertise.
* `autoRenewed`: Boolean specifying whether ARIN will automatically renew the ROA.
* `autoLinked`: Identifies true or false, the existance of a linked IRR object.

**Payload**

```
<roaSpec xmlns="http://www.arin.net/regrws/rpki/v1" >
  <asNumber></asNumber>
  <name></name>
  <notValidAfter></notValidAfter>
  <notValidBefore></notValidBefore>
  <resources>
    <cidrLength></cidrLength>
    <endAddress></endAddress>
    <ipVersion></ipVersion>
    <maxLength></maxLength>
    <startAddress></startAddress>
    <autoLinked></autoLinked>
  </resources>
  <roaHandle></roaHandle>
</roaSpec>
```

**Example**

```
<roaSpec>
  <ns5:asNumber>64496</ns5:asNumber>
  <ns5:name>IANA-RSVD</ns5:name>
  <ns5:notValidAfter>2020-12-13T00:00:00-05:00</ns5:notValidAfter>
  <ns5:notValidBefore>2019-12-14T00:00:00-05:00</ns5:notValidBefore>
  <ns5:resources>
    <ns5:cidrLength>32</ns5:cidrLength>
    <ns5:endAddress>2001:db8:ffff:ffff:ffff:ffff:ffff:ffff
    </ns5:endAddress>
    <ns5:ipVersion>6</ns5:ipVersion>
    <ns5:maxLength>48</ns5:maxLength>
    <ns5:startAddress>2001:db8:0:0:0:0:0:0</ns5:startAddress>
    <ns5:autoLinked>true</ns5:autoLinked>
  </ns5:resources>
  <ns5:roaHandle>58bc1674f7784054ba743b9f5c23885b</ns5:roaHandle>
</roaSpec>
```

## Create, Modify, and Delete ASPAs

This is a unified call for creating, modifying, and deleting ASPAs. This operation can be performed in a single transaction combined with the “Create, Modify and Delete ROAs” operation, meaning that they will all succeed or fail together.

ARIN will automatically renew all ASPAs created with this service.

If Reg-RWS returns an error code and/or Error Payload to you when performing this call, refer to this list of [error codes](https://www.arin.net/resources/manage/regrws/methods/#error-codes).

**API Key in Header (Recommended)**

**Method:** `POST`  
**URL:** /rest/rpki/ORGHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** RpkiTransactionPayloadForASPAS  
**Returns:** RpkiTransactionPayloadForASPAS

**API Key in URL (Supported)**

**Method:** `POST`  
**URL:** /rest/rpki/ORGHANDLE?apikey=APIKEY  
**Content:** RpkiTransactionPayloadForASPAS  
**Returns:** RpkiTransactionPayloadForASPAS

The **RPKI Transaction Payload** describes ASPAs to be deleted and ASPAs to be created.

**Payload**

```
<rpkiTransaction>
  <aspaDelete>
    <customerAsId></customerAsId>
    <customerAsId></customerAsId>
  </aspaDelete>
  <aspaAdd>
    <aspa>
      <customerAsId></customerAsId>
      <providerAsIds>
        <providerAsId></providerAsId>
        <providerAsId></providerAsId>
      </providerAsIds>
    </aspa>
  </aspaAdd>
</rpkiTransaction>
```

**Example**

```
<rpkiTransaction>
  <aspaDelete>
    <customerAsId>64496</customerAsId>
    <customerAsId>644967</customerAsId>
  </aspaDelete>
  <aspaAdd>
    <aspa>
      <customerAsId>64496</customerAsId>
      <providerAsIds>
        <providerAsId>64497</providerAsId>
        <providerAsId>64498</providerAsId>
      </providerAsIds>
    </aspa>
  </aspaAdd>
</rpkiTransaction>
```

## Get a List of ASPAs for an Org

This call returns a list of ASPAs for the Org specified in your URL, with an ASPA Spec Payload included for each ASPA. The ASPA Spec Payload contains information about an individual ASPA such as customer and provider ASNs.

**API Key in Header (Recommended)**

**Method:** `GET`  
**URL:** /rest/aspa/ORGHANDLE   
**Authorization Header Name:** Authorization  
**Authorization Header Value:** ApiKey ‘apikey’ (e.g. ApiKey API-xxxx-xxxx-xxxx)  
**Content:** NONE  
**Returns:** ASPA Spec Payload

**API Key in URL (Supported)**

**Method:** `GET`  
**URL:** /rest/aspa/ORGHANDLE?apikey=APIKEY  
**Content:** NONE  
**Returns:** ASPA Spec Payload

The **ASPA Spec Payload** provides information about ASPAs, including:

* `customerAsId`: An AS registered to the customer.
* `providerAsIds`: A list of provider ASes.

**Payload**

```
<aspa>
  <customerAsId></customerAsId>
  <providerAsIds>
    <providerAsId></providerAsId>
    <providerAsId></providerAsId>
  </providerAsIds>
</aspa>
```

**Example**

```
<aspa>
  <customerAsId>64496</customerAsId>
  <providerAsIds>
    <providerAsId>64497</providerAsId>
    <providerAsId>64498</providerAsId>
  </providerAsIds>
</aspa>
```
