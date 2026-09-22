# Internet Routing Registry (IRR)

Source: https://www.arin.net/resources/manage/irr/

Retrieved: 2026-09-22T22:50:51.498008+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## Understanding Internet Routing Registries

Internet Routing Registries (IRRs) contain information  -  submitted and maintained by ISPs or other entities  -  about Autonomous System Numbers (ASNs) and routing IP number prefixes. IRRs can be used by ISPs to develop routing plans. For example, ISPs who use Border Gateway Protocol (BGP) can create Access Control Lists to permit or deny traffic in their networks based on route registry information.

The global IRR is comprised of a network of distributed databases maintained by Regional Internet Registries (RIRs) such as ARIN, service providers, and third parties. Some of these databases contain only routing information for a particular region, network, or ISP. Other IRR databases mirror specific IRR databases, and contain IRR information from multiple databases. One of the IRR maintainers provides a [list of routing registries](http://www.irr.net/docs/list.html).

![A world map illustrating connections between the five Regional Internet Registries and other Internet Routing Registries](https://www.arin.net/about/welcome/region/images/NRO_rirmap.png)

## Overview of the ARIN IRR

The ARIN IRR is a searchable database of public routing policy information for networks in the ARIN region. IRR information is contained in a separate database from ARIN’s public Whois information. As part of the global IRR, the ARIN IRR provides a registry of Internet routing objects for resources in the ARIN region.

There are two main functions of the IRR: getting routing data from ARIN users, and publishing that data to entities (such as customers, other IRRs, and [aggregators](https://www.arin.net/resources/manage/irr/irr_faq/#aggregator)) who retrieve it for use in routing decisions for their networks. You do not have to submit your own routing data to ARIN’s IRR to be able to query and retrieve routing data from the ARIN IRR. The following graphic illustrates the functions of ARIN’s IRR.

![An illustration of how information flows through Internet Routing Registries](https://www.arin.net/resources/images/irr_overview_final.png)

The ARIN IRR stores information in Routing Policy Specification Language (RPSL) objects. These objects are submitted to the ARIN IRR by resource holders such as ISPs and retrieved by other IRRs when ISPs in their region are requesting ARIN routing information.

ARIN’s IRR is integrated with ARIN Online. The IRR-online system is connected to ARIN’s IRR database. The database provides IRR information via a Near Real-Time Monitoring (NRTM) service, and is also accessible using Whois on port 43.

The following illustration provides an overview of the components of ARIN’s IRR.

![illustration of how the pieces of ARIN’s IRR work together](https://www.arin.net/resources/manage/irr/ARIN-irr-overview.png)

ARIN’s IRR provides two methods for entering IRR data. Some of the differences between IRR-online and the IRR RESTful API are summarized in the following table.

Feature Differences Between IRR Methods of Access

| Feature/Item | Supported in IRR-online | Supported in IRR RESTful API |
| --- | --- | --- |
| Bulk add/update capability | No | Multiple REST commands can be used in scripts or software; only one object create / read / update / delete per REST call is supported at this time |
| Email notifications when objects are updated | No | No |
| Maintainer object | No (objects are assigned to Orgs and managed through linked ARIN Online accounts) | No (objects are assigned to Orgs and managed through linked ARIN Online users with a valid API key) |
| Proxy registration | Yes, via Routing POCs | Yes, via Routing POCs |

More information about how to submit and retrieve IRR data is provided in the following sections.

**Are you a source organization involved in a transfer to specified recipients within the ARIN region (NRPM 8.3) or an inter-RIR transfer (NRPM 8.4)?** See our [routing security best practices and source pre-transfer checklist](https://www.arin.net/resources/registry/transfers/#requirements-for-the-source-organization-current-registrant) to ensure a clean transition.

## Submitting Routing Information

Data can be submitted to ARIN’s IRR using the methods described in the following sections. Currently, ARIN allows you to submit data using the graphical user interface (GUI) and using the RESTful interface.

ARIN has categorized IRR objects as *simple* and *advanced*. *Simple* objects are those created by:

* using the ARIN Online GUI to create objects individually by entering data in each field and then submitting each object
* using the RESTful API to create objects by submitting IRR data in XML format (which the system then converts in the back end to RPSL)

*Advanced* objects are those created by:

* using the RESTful API to create objects by submitting IRR data in RPSL format

Because ARIN is implementing IRR functionality in stages, the distinction between simple and advanced objects is important when viewing, updating, and deleting objects. It does not affect the IRR record that is retrieved from ARIN by other users, which is in RPSL format.

### Object Permissions

Currently, viewing, editing, and deleting of some objects is limited depending on the method used originally to create the object; refer to the following table. The system will return a warning if you try an operation that is not permitted–for example, using a REST command with an XML payload to view an object originally created with a REST command using RPSL (an advanced object).

IRR Object Permissions

| IRR Object Creation Method | REST (RPSL) Permissions | REST (XML) Permissions | ARIN Online Permissions |
| --- | --- | --- | --- |
| simple (created in IRR-online or with RESTful/XML) | none | create, view, edit, delete | create, view, edit, delete |
| advanced (created with RESTful/RPSL or migrated\* from email templates) | create, view, edit, delete | none | view and delete only |

`*` Migrated objects have been imported from email templates that have passed the newer, stricter validation requirements for IRR objects and are indicated by a note on the object page in the ARIN Online graphical user interface (GUI).

### IRR-online

ARIN’s IRR-online is available from the main navigation menu in ARIN Online. To learn how to submit and manage routing information in IRR-online, visit the [ARIN IRR-online User Guide](https://www.arin.net/resources/manage/irr/userguide).

### IRR RESTful API

ARIN provides a RESTful Application Programming Interface (API) that allows users to submit routing information via Representational State Transfer (REST) commands in scripts or software. This provides support for automating bulk data entry of route information. Visit the [ARIN IRR RESTful API User Guide](https://www.arin.net/resources/manage/irr/irr-restful) for more information on how to use these commands.

## Querying ARIN’s IRR

ARIN provides multiple ways to query the IRR database.

### Near Real Time Mirroring (NRTM)

ARIN’s IRR database provides an NRTM stream:

* ARIN: This stream contains “authorized” objects, or objects in the ARIN IRR database that are validated. “Validated” means that prefixes in route objects and Autonomous System Numbers (ASNs) in aut-num objects are assigned to the valid maintaining Org ID and are covered by a Registration Services Agreement (RSA). For routes and ASNs, the Internet numbers claimed in the IRR object are registered to that same valid Org ID.

You will need to establish an NRTM session to gain access to ARIN’s IRR object.

### File Download

ARIN’s IRR information can be obtained from <https://ftp.arin.net/pub/rr/>. The database source files can also be downloaded directly from <https://ftp.arin.net/pub/rr/arin.db.gz>.

**Note:** The source files are in compressed format (.gz).

### Whois Port 43

You can also use a command-line interface command in a program such as Terminal to enter query commands. The ARIN IRR uses the Internet Routing Registry Daemon (IRRd) Version 4 database server. IRRd supports two types of Whois queries: *IRRd-style queries* and *RIPE-style queries*. You cannot mix styles in a single query. Each style has different flags and arguments.

Some common IRR query examples are listed in the following table, and longer examples are provided after the table.

IRR Query Commands

| Query | IRRd-Style Example | RIPE-Style Example |
| --- | --- | --- |
| objects of a specified maintainer | `whois -h rr.arin.net \!oMNT-EXAMPLECORP` | none |
| as-set | `whois -h rr.arin.net \!iAS-TUSCONHUB` | `whois -h rr.arin.net AS-TUSCONHUB` |
| route-set | `whois -h rr.arin.net \!RS-ARIZONA` | `whois -h rr.arin.net RS-ARIZONA` |
| route | `whois -h rr.arin.net \!r192.0.2.0/24` | `whois -h rr.arin.net 192.0.2.0/24` |

For a full list of querying instructions for both IRRd-style and RIPE-style queries, view [IRRd: Running queries - Whois protocol reference](https://irrd.readthedocs.io/en/stable/users/queries/whois/).

**IRRd-Style Query Example**

IRRd-style queries use arguments to retrieve certain types of data. These arguments are preceded by the exclamation point. When using the terminal window to enter commands, you’ll need to precede the exclamation point with a backslash `(\)` or enclose the exclamation point and arguments within single quotation marks.

For example, to search for all members of an `as-set` you would enter the `as-set` name in the command like this:

terminal$ `whois -h rr.arin.net \!iAS-EXAMPLE`  
A24  
AS-EXP AS-WESTB AS65536

An explanation of the results is provided here:

* `A<length>`: A24: The number of bytes in the response, including new lines after the content.
* `<as-set> <as-set> <asn>`: AS-EXP AS-WESTB AS65536: This `as-set` (“as-example”) includes two other `as-sets`, AS-EXP and AS-WESTB, and the ASN AS65536.

**RIPE-Style Query Example**

To search for an object such as a `route6` object, enter the command like this:

terminal$ `whois -h rr.arin.net 2001:db8::/32`

Results:
route6: 2001:db8::/32  
descr: Example Corporation  
origin: AS65536  
member-of: rs-example  
mnt-by: MNT-EXAMPLECORP  
changed: `tech@example.com` 20180810  
source: ARIN
