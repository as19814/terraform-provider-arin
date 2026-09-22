# WhoWas ReadMe

Source: https://www.arin.net/reference/research/whowas/readme/

Retrieved: 2026-09-22T22:50:53.980298+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

ARIN’s WhoWas report contains historical registration information for a given IP address or Autonomous System Number (ASN). The publication of WhoWas data is provided subject to an approved request under ARIN’s WhoWas [Terms of Use (ToU)](https://www.arin.net/reference/research/whowas/tou/).

## File Format

Because the WhoWas report contains the entire public history for a given IP address or ASN, it is much larger and more complex than a Whois query response. Each IP address or ASN could have been a part of multiple networks/AS ranges over time and have been issued to multiple organizations, each with its own associated Points of Contact (POCs).

A WhoWas report will be returned in a .zip file which contains a ReadMe file and a series of `.tsv` files. The .tsv format allows for programmable parsing and is recognizable by spreadsheets. The .tsv files can be opened in spreadsheet software, in any plain text reader, or with any other software you have which supports .tsv files.

## Report Summary

The first file you should open is the `summary.tsv`, which lists each object that matched your WhoWas request (the ASN or the IP address), and each of the Org IDs and POCs that were historically associated with that number resource. Each of the listed objects in the summary will have its own .tsv file.

The list of objects in the `summary.tsv` are ordered by:

* `NET` or `ASN` (depending on your request)
* `Org ID`
* `Customer` (applicable for IP Address requests only)
* `POC`

[Find out more](https://www.arin.net/resources/guide/account/database/) about these object types and how they relate to each other.

## Content of Individual Reports

Each .tsv file will contain a complete history of publicly-viewable information for the given Handle.

For each object, the report will contain the data fields normally included in Whois query results, plus two additional fields:

* Action: This is either `CREATED`, `MODIFIED`, or `REGISTRATION REMOVED`.
  + Created represents the object’s first time appearing in Whois.
  + Modified represents any changes to the individual data fields of the object.
  + Registration Removed is the date the object was removed from ARIN’s Whois.
* Action Date: The month, day, and year for the listed action, in `MM/DD/YY` format (with no zero-padding)

Below is a description of the data fields included for each object type.

* [Networks (NET) Report](https://www.arin.net/reference/research/whowas/readme/#net)
* [Autonomous System Number (ASN) Report](https://www.arin.net/reference/research/whowas/readme/#asn)
* [Organization (Org ID) Report](https://www.arin.net/reference/research/whowas/readme/#org)
* [Customer Report](https://www.arin.net/reference/research/whowas/readme/#customer)
* [Point of Contact (POC) Report](https://www.arin.net/reference/research/whowas/readme/#poc)

Networks (NET) Report

| Field Name | Description |
| --- | --- |
| Action | The action taken on the network record. As previously explained, the value will be one of the following: `Created`,`Modified`,`Registration Removed` |
| Action Date | The month, day, and year for the listed action, in `MM/DD/YY format` (with no zero-padding) |
| Net Range | Specifies a range of IPv4 or IPv6 addresses that comprise the network, in the format of `<start IP address> - <end IP address>`. Spaces are present between the beginning address, the dash ("-"), and the end address. IPv4 and IPv6 addresses are expressed in shorthand notation. |
| Net Name | The ARIN-registered Net Name for the IP network |
| Net Handle | The Handle, or identifier, for the network |
| Parent | The Handle of the network’s parent |
| Net Type | The network type. There are four main types of network records: `Direct Allocation`,`Direct Assignment`,`Reallocation`,`Reassignment` |
| Origin AS | A comma-separated list of AS Numbers. Each number is preceded by `AS`. |
| Organization | The Org or Customer Handle to which the network was issued. A separate .tsv file is provided for each Org/Customer Handle, showing the history of the organization record. |
| Registration Date | The month, day, and year the network record was registered, in `MM/DD/YY` format (with no zero-padding) |
| Comments | Public comments |
| Resource Tech POCs | A comma-separated list of POC Handles serving the `Tech` function for the network. A separate .tsv file is provided for each POC Handle, showing the history of the POC record. |
| Resource Abuse POCs | A comma-separated list of POC Handles serving the `Abuse` function for the network. A separate .tsv file is provided for each POC Handle, showing the history of the POC record. |
| Resource NOC POCs | A comma-separated list of POC Handles serving the `NOC` function for the network. A separate .tsv file is provided for each POC Handle, showing the history of the POC record. |

**Note:** On 28 July 2025, Autonomous System Originations (Origin AS) were retired as per [ARIN policy 2021-8](https://www.arin.net/vault/participate/policy/drafts/2021/2021_8/). The WhoWas report will only contain archived Origin AS data.”

Autonomous System Number (ASN) Report

| Field Name | Description |
| --- | --- |
| Action | The action taken on the AS record. As previously explained, the value will be one of the following: `Created`,`Modified`,`Registration Removed` |
| Action Date | The month, day, and year for the listed action, in `MM/DD/YY` format (with no zero-padding) |
| Number | In most cases, this is a single ASN. In cases where the AS Handle represents a range, it is in the format of `<start AS Number>–<end AS Number>`. Spaces are *not* present between the beginning number, the dash ("-"), and the end number. The number(s) will *not* be preceded by AS. |
| Name | The ARIN-registered ASN name for the ASN |
| Handle | The Handle, or identifier, for the ASN |
| Organization | The Org Handle to which the ASN was issued. A separate .tsv file is provided for each Org Handle, showing the history of the organization record. |
| Registration Date | The month, day, and year the ASN record was registered, in `MM/DD/YY` format (with no zero-padding) |
| Comments | Public comments |
| ASN Tech POCs | A comma-separated list of POC Handles serving the `Tech` function for the ASN. A separate .tsv file is provided for each POC Handle, showing the history of the POC record. |
| ASN Abuse POCs | A comma-separated list of POC Handles serving the `Abuse` function for the ASN. A separate .tsv file is provided for each POC Handle, showing the history of the POC record. |
| ASN NOC POCs | A comma-separated list of POC Handles serving the `NOC` function for the ASN. A separate .tsv file is provided for each POC Handle, showing the history of the POC record. |

Organization (Org ID) Report

| Field Name | Description |
| --- | --- |
| Action | The action taken on the organization record. As previously explained, the value will be one of the following: `Created`,`Modified`,`Registration Removed` |
| Action Date | The month, day, and year for the listed action, in `MM/DD/YY` format (with no zero-padding) |
| Org Name | The Organization name registered with ARIN |
| Org Handle | The Handle, or unique identifier, for the organization in ARIN’s database |
| Address | The street address of the organization |
| City | The city of the organization |
| State/Province | The two-letter abbreviation of the state or province of the organization |
| Postal Code | The postal code of the organization |
| Country | The ISO-3166 two-character country code of the organization |
| Registration Date | The month, day, and year the organization record was registered, in `MM/DD/YY` format (with no zero-padding) |
| Comments | Public comments |
| Referral Server | The publicly-accessible rwhois server for the organization |
| Org Admin POC | The POC Handle of the POC serving the `Admin` function for the organization. A separate .tsv file is provided for the POC Handle, showing the history of the POC record. |
| Org Tech POCs | A comma-separated list of POC Handles serving the `Tech` function for the organization. A separate .tsv file is provided for each POC Handle, showing the history of the POC record. |
| Org Abuse POCs | A comma-separated list of POC Handles serving the `Abuse` function for the organization. A separate .tsv file is provided for each POC Handle, showing the history of the POC record. |
| Org NOC POCs | A comma-separated list of POC Handles serving the `NOC` function for the organization. A separate .tsv file is provided for each POC Handle, showing the history of the POC record. |

Customer Reports

| Field Name | Description |
| --- | --- |
| Action | The action taken on the customer organization record. As previously explained above, the value will be one of the following: `Created`,`Modified`,`Registration Removed` |
| Action Date | The month, day, and year for the listed action, in `MM/DD/YY` format (with no zero-padding) |
| Name | The customer organization name registered with ARIN. In cases where the customer organization is marked as private, the report displays Private Customer. |
| Handle | The Handle, or unique identifier, for the customer organization in ARIN’s database |
| Address | The street address of the customer organization. In cases where the customer organization is marked as private, the report displays Private Residence. |
| City | The city of the customer organization. |
| State/Province | The two-letter abbreviation of the state or province of the customer organization |
| Postal Code | The postal code of the customer organization |
| Country | The ISO-3166 two-character country code of the customer organization |
| Registration Date | The month, day, and year the customer organization record was registered, in `MM/DD/YY` format (with no zero-padding) |

Point of Contact (POC) Report

| Field Name | Description |
| --- | --- |
| Action | The action taken on the POC record. As previously explained, the value will be one of: `Created`,`Modified`,`Registration Removed` |
| Action Date | The month, day, and year for the listed action, in `MM/DD/YY` format (with no zero-padding) |
| First Name | POC’s first name |
| Middle Name | POC’s middle name |
| Last Name | POC’s last name, or, if a role account, the role account name |
| POC Handle | The Handle, or unique identifier, for the POC in ARIN’s database |
| Company | The name of the POC’s company |
| Address | The street address of the POC |
| City | The city of the POC |
| State/Province | The two-letter abbreviation of the state or province of the POC |
| Postal Code | The postal code of the POC |
| Country | The ISO-3166 two-character country code of the POC |
| Registration Date | The month, day, and year the POC record was registered, in `MM/DD/YY` format (with no zero-padding) |
| Comments | Public comments |
| Phone | A comma-separated list of phone numbers associated with the POC. This field contains the phone number, followed by the type in parenthesis. The type can be: `Office`,`Fax`,`Mobile` |
| Email Address | A comma-separated list of email addresses associated with the POC |

## Additional Information

Some helpful hints for understanding several peculiarities you might see in the data in a WhoWas report:

### ARIN recycles ASNs and NETs when possible.

For example, ARIN may register *AS123* to one customer, who uses it for a few years and then returns it to ARIN when they no longer need it. Later, ARIN will issue *AS123* to a new customer. This may happen numerous times, so you can expect to see many `CREATED` and `REGISTRATION REMOVED` rows for any given NET or ASN.

### ASNs and NETs are sometimes removed for a short period of time.

Typically, ASNs and NETs leave the database for one of three reasons:

1. They are returned by the registrant
2. They are revoked by ARIN due to non-payment of fees; or
3. ARIN reclaims the registrations due to fraud

Sometimes (especially with revocations due to non-payment), the registration will eventually be reinstated. In the WhoWas data, this will be displayed with individual actions marked `CREATED` and `REGISTRATION REMOVED`.

### ARIN’s Whois has database constraints that require all Org IDs to have one Admin POC, at least one Tech POC, and at least one Abuse POC.

Many older objects have no useful POC data, so ARIN has placed the POC: `CKN23-ARIN` on the record to complete the missing data. `CKN23-ARIN` is the placeholder object denoting that ARIN does not have a bona fide contact for the Org ID.
