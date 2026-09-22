# Read Me: Report of Resources with No Valid POCs

Source: https://www.arin.net/resources/guide/account/records/poc/validation/readme/

Retrieved: 2026-09-22T22:50:54.075682+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

**Note**: The schema used in the report of number resources without valid POCs changed with the [16 September 2017](https://www.arin.net/vault/announcements/20170916/) update of ARIN Online. This schema is included in the .zip file of the report that is downloaded. Please be sure that your automated scripts parse this new XML format.

The list of number resources without valid POCs is provided subject to an approved request under [ARIN’s Bulk Whois Acceptable Use Policy](https://www.arin.net/about/corporate/agreements/bulkwhois.pdf).

ARIN’s Whois data is for Internet operational or technical research purposes pertaining to Internet operations only. It may not be used for advertising, direct marketing, marketing research, or similar purposes. Use of ARIN’s Whois data for these activities is explicitly forbidden. ARIN requests to be notified of any such activities or suspicions thereof.

Redistributing this data is explicitly forbidden. Distribution of derivative data is only permitted with the express written permission of ARIN and under the same terms as this AUP. It is permissible to publish the data on an individual query or small number of queries at a time basis, as long as reasonable precautions are taken to prevent automated querying by database harvesters.

By retrieving this data, you agree to the acceptable use policy for ARIN’s Whois data and confirm the accuracy of the information provided in your request.

## Report Summary

This report contains a list of Networks and ASNs that meet the following criteria:

* There are no POCs associated with the resource either directly (a resource POC) or indirectly (a POC associated with the resource’s Org)  
  OR
* ALL POCs associated with the resource are invalid. A POC handle is marked invalid by ARIN staff when the POC has not been modified in more than one year and the POC fails to respond to ARIN’s requests to validate the POC information.

### Contents of ZIP File

no-valid-poc-resource-report.xml
:   the report in XML format

no-valid-poc-resource-report.txt
:   the report in text format

no-valid-poc-resource-report.xsl
:   the XSLT template used to generate the text format from the XML format

schemas
:   directory of RelaxNG schema files that define the xml document above

README.txt
:   this file

### Description of Elements

This section provides a semantic description of many of the XML elements in the report.

#### General XML Elements

handle
:   either an ASN handle, Net Handle, or Org handle depending on the type of the immediate parent XML element

orgHandle & orgName
:   element containing info about an organization. Always
    nested within ASN and net XML elements. Identifies the
    organization to which the net/ASN has been allocated.

name
:   when nested within an org XML element, this represents the
    organization name.

#### ASN XML Elements

asn
:   element containing info about a single Autonomous System
    Number resource

startAsNumber
:   the first ASN in the registration

endAsNumber
:   the last ASN in the registration range. If the
    resource registration only represents a single ASN,
    then the endAsNumber is the same as the startAsNumber.

#### Invalid POC XML Elements

These elements are nested within an invalidPoc element.

handle
:   The handle of the invalid POC

name
:   The name of the invalid POC

updateDate
:   The date that the POC was last updated

#### NET XML Elements

net
:   element containing info about a single Network resource

startAddress
:   the lowest IP (v4 or v6) address of a range of consecutive
    IP address blocks assigned to a single net handle

endAddress
:   the highest IP (v4 or v6) address of a range of consecutive
    IP address blocks assigned to a single net handle

## Schema Validation

This section contains additional information for those who wish to review the schemas or use them to validate the xml.

The provided schemas are written in Relax NG Compact Syntax. Detailed info about Relax NG syntax can be found here: <http://www.relaxng.org/compact-tutorial-20030326.html>

Each major element of the schema is broken out into its own .rnc file. The top level file is nvpr.rnc. All other schema fragments will be referenced from nvpr.rnc.

Validation and parsing with RelaxNG Compact Schema can be done in Java using Jing.

* Main: <https://github.com/relaxng/jing-trang>
* Download: <http://code.google.com/p/jing-trang/downloads/list>

Jing provides a jar file and API to invoke Relax NG validation for a given schema and xml document. You can also run it from the command line:

`java -jar jing.jar -c schema/nvpr.rnc no-valid-poc-resource-report.xml`

## Downloading Using an API Key

The report can be downloaded directly without logging into ARIN Online using a RESTful HTTP request containing your API key. The URL must look like:

`https://accountws.arin.net/public/rest/downloads/nvpr?apikey=YOUR-API-KEY`

There are a variety of ways to automate the retrieval of this report. For example, on a Linux system, where your API key is API-1111-2222-3333-4444, you can use the following ‘curl’ command to download the report file:

`curl https://accountws.arin.net/public/rest/downloads/nvpr?apikey=API-1111-2222-3333-4444 \> arin\_nvpr.zip`

You can manage your API keys from the [Settings](https://account.arin.net/public/secure/profile) page when logged into your ARIN Online account.
