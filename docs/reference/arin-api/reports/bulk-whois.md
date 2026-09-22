# Bulk Whois Data

Source: https://www.arin.net/reference/research/bulkwhois/

Retrieved: 2026-09-22T22:50:53.723980+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

ARIN provides access to a bulk copy of all objects in the public ARIN Whois directory service to organizations that will use the data for Internet operational or technical research purposes. ARIN does not provide bulk copies of Whois data to operators who wish to incorporate this data into products, services, or internal systems with no clear benefit to the broader community.

**Note:** ARIN also publishes a daily listing of all active IPv4, IPv6, and ASN registrations, complete with country code data. This free report can be accessed on our [website here](https://ftp.arin.net/pub/stats/).

Bulk Whois data contains the Point of Contact (POC) records, organization records, and AS, IPv4, and IPv6 registrations that are displayed in ARIN’s public Whois. You can download a .zip file of the entire bulk Whois data, or selectively download only parts of the Bulk Whois data in either .zip, .txt, or .xml format.

## Getting Started

To access the daily-generated bulk copy of the ARIN Whois directory service, you need to have an [ARIN Online account](https://www.arin.net/resources/guide/account/).

1. Download the [Bulk Whois ToU and Data Request Form](https://www.arin.net/about/corporate/agreements/bulkwhois.pdf) and sign it. Scan this file and save it, as you’ll need to submit it to ARIN as an attachment.
2. Log in to ARIN Online and choose **Ask ARIN** from the navigation menu.
3. Create an **Ask ARIN** ticket. As Topic field, select **Resource and Reporting Inquiries** and enter **Bulk Whois Request** in the **Subject** field.
4. In the **Question** field, briefly describe your intended use of the bulk Whois data.
5. Attach a scanned copy of the signed Bulk Whois Terms of Use and submit your request. ARIN Online creates a ticket for you. Notifications about your tickets can be accessed by choosing the icon next to your name, or choose **Tickets** from the navigation menu to see a list of your tickets.

### Understanding the Review and Approval Process

Upon receipt of your signed ToU, ARIN staff will begin a thorough review of your request by contacting you via your Ask ARIN request. ARIN will ask you a set of questions aimed at understanding why you require access to a bulk copy of the data. Please reply to our questions via your Ask ARIN request. We may have further questions depending on your answers.

ARIN will notify the submitter via your Ask ARIN request to indicate whether your request is approved or denied.

For questions about your request, communicate via your Ask ARIN request, or call ARIN’s Registration Services Help Desk at +1.703.227.0660.

## Accessing Bulk Whois Data

If your request is approved, you will be able to access the Bulk Whois data by performing these steps:

1. Log in to ARIN Online.
2. Select **Downloads & Services** from the navigation menu.
3. In the Bulk Whois section, choose **Bulk Whois Data**. The next window provides a list of files and formats in which the data is available. You have the option to download a file of all Whois object types (POCs, Orgs, NETs, and ASNs) or to selectively download a particular Whois object type in compressed archive (.zip), text (.txt), or extended markup language (.xml) format.

If you want to download a file that contains a combination of Whois object types, you can enter the download URL directly into your browser window, using the following format:

```
https://accountws.arin.net/public/rest/downloads/bulkwhois/XXX+YYY.zip
```

where `XXX` and `YYY` can be any combination of:

* asns
* pocs
* orgs
* nets

Specifying an extension of .txt or .xml yields the same result, but only rendering the text or markup format, respectively.

The name of the file download returned is always either **arin\_db.txt** or **arin\_db.xml**, regardless of which objects are specified.

### Downloading Using an API Key

The report can be downloaded directly without logging into ARIN Online using a RESTful HTTP request containing your [API key](https://www.arin.net/reference/materials/security/api_keys/). For the file containing all Whois object types, the URL must look like:

`https://accountws.arin.net/public/rest/downloads/bulkwhois?apikey=YOUR-API-KEY`

You can also selectively download only parts of the Bulk Whois through a RESTful HTTP request. The URL must look like:

`https://accountws.arin.net/public/rest/downloads/bulkwhois/XXX+YYY.zip?apikey=YOUR-API-KEY`

where `XXX` and `YYY` can be any combination of “asns,” “pocs,” “orgs,” or “nets,” which will yield only that information in the file. The combination can be any of these or all of these (e.g., nets+asns+orgs.zip). The file type can be .zip, .txt, or .xml.

There are a variety of ways to automate the retrieval of this report. For example, on a Linux system, where your API key is API-1111-2222-3333-4444-5555-6666-7777-8888, you can use the following `curl` command to download the report file:

`curl https://accountws.arin.net/public/rest/downloads/bulkwhois?apikey=API-1111-2222-3333-4444-5555-6666-7777-8888 > arin_bulkwhois.zip`

To manage your API keys:

1. Choose **Settings** from the menu under your name.
2. In the **Security Info** section, from the **Actions** menu, choose **Manage API Keys**.

## Using the Bulk Whois Data

Output from a Whois query is comprised of multiple objects drawn from a relational database. This data is available in two formats:

1. The **arin\_db.txt** file is a flat text dump of objects in this relational database. In total, the file may contain millions of database objects (POCs, Org IDs, NETs, and/or Autonomous System Numbers [ASNs]). You will need to spend a considerable amount of time constructing a framework into which you can import the raw, unsorted data. The individual objects contain referential information to the other objects with which they are associated. This referential information enables you to correlate relationships between objects.
2. This data is also provided in xml format (**arin\_db.xml**) to provide an easy way of parsing and manipulating the raw, unsorted data. By integrating a freely available XML parser or libraries with your custom software, you can simplify your process for accessing this data.

For information on the content and format of the Bulk Whois report, visit [Report Content](https://www.arin.net/reference/research/bulkwhois/#report-content). The [Report of Resources with No Valid POCs](https://www.arin.net/resources/guide/account/records/poc/validation/readme/) page describes content and format of Bulk Whois reports on invalid POCs.

ARIN does not publish or support external databases or frameworks. Use the [Ask ARIN](https://account.arin.net/public/communication/message/beginQuestion.xhtml) feature of your ARIN Online account to submit any questions about Bulk Whois or the data output.

## Report Content

### Description of Elements

This section provides a semantic description of many of the XML and legacy elements in the report, depending on the data object types specified.

#### General XML Elements

* `ref`: The URL for this record in the RESTful Whois service
* `handle`: Unique id for this record
* `registrationDate`: Date the resource/poc/org was created in ARIN’s database
* `updateDate`: Date the record was last changed

#### ASN XML Elements

* `asn`: Element containing info about a single Autonomous System Number registration
* `name`: The Autonomous System name
* `startAsNumber`: The first ASN in the registration
* `endAsNumber`: The last ASN in the registration range. If the resource registration only represents a single ASN, then the `endAsNumber` is the same as the `startAsNumber.`
* `orgHandle`: The Org handle of the Org to which this resource has been issued. This field will match the `handle` field of an `org` entity.
* `pocLinks`: A list of POCs that are directly associated with this ASN resource. Each POC link lists the POC’s handle, function code, and a short description of the function.

#### NET XML Elements

* `net`: Element containing info about a single network registration
* `name`: The network name
* `startAddress`: When the parent XML element is a `netBlock,` this represents the starting IP address of that block. When the parent XML element is a `net,` this represents the lowest `startAddress` of all listed `netBlocks.`
* `endAddress`: When the parent XML element is a `netBlock,` this represents the ending IP address of that block. When the parent XML element is a `net,` this represents the highest `endAddress` of all listed `netBlocks.`
* `orgHandle`: The Org handle of the Org to which this resource has been issued. This field will match the `handle` field of an `org` entity.
* `pocLinks`: A list of POCs that are directly associated with this network resource. Each POC link lists the POC’s handle, function code, and a short description of the function.
* `netBlocks`: A list of contiguous net blocks assigned to this net
* `cidrLength`: The CIDR prefix for a net block
* `type`: the type of net block. One of the following:   
  + A (Reallocation)
  + AF (Allocated to AFRINIC)
  + AP (Allocated to APNIC)
  + AR (Allocated to ARIN)
  + AV (Early registrations maintained by ARIN)
  + DA (Direct Allocation)
  + DS (Direct Assignment)
  + FX (Transferred to AFRINIC)
  + IR (IANA Reserved)
  + IU (IANA Special Use)
  + LN (Allocated to LACNIC)
  + LX (Transferred to LACNIC)
  + PV (Early registrations maintained by APNIC)
  + PX (Early registrations transferred to APNIC)
  + RN (Allocated to RIPE NCC)
  + RV (Early registrations maintained by RIPE NCC)
  + RX (Early registrations transferred to RIPE NCC)
  + S (Reassignment)
* `parentNetHandle`: the net handle for the parent net
* `version`: IP version, either 4 or 6

#### POC XML Elements

* `poc`: element containing info about a single Point Of Contact
* `iso3166-1`: the ISO standard codes defining the country where this POC is registered
* `iso3166-2`: the ISO standard codes defining the state/province where this POC is registered
* `companyName`: the company/organization this POC is representing
* `isRoleAccount`: if “N,” this POC represents an individual, if “Y,” this POC represents a role, or group

#### ORG XML Elements

* `org`: Element containing info about a single organization
* `iso3166-1`: The ISO standard codes defining the country where this organization is registered
* `iso3166-2`: The ISO standard codes defining the state/province where this organization is registered
* `customer`: “Y” if the organization record does not have its own contact information or reverse DNS delegation (simple reassignment), otherwise, “N”
* `pocLinks`: A list of POCs that are directly associated with this organization. Each POC link lists the POC’s handle, function code and a short description of the function.

## Schema Validation

This section contains additional information for those who wish to review the schemas or use them to validate the xml.

The provided schemas are written in Relax NG Compact Syntax. Detailed info about Relax NG syntax can be found here: <http://www.relaxng.org/compact-tutorial-20030326.html>

Each major element of the schema is broken out into its own .rnc file. The top level file is `bwi.rnc.` All other schema fragments will be referenced from `bwi.rnc.`

Validation and parsing with RelaxNG Compact Schema can be done in Java using Jing:

* Main: <http://www.thaiopensource.com/relaxng/jing.html>
* Download: <http://code.google.com/p/jing-trang/downloads/list>

Jing provides a jar file and API to invoke Relax NG validation for a given schema and xml document. You can also run it from the command line; for example:

`java -jar jing.jar -c schema/bwi.rnc arin_db.xml`
