# Registry Data Description

Source: https://www.arin.net/reference/materials/data/

Retrieved: 2026-09-22T22:50:53.108084+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## Introduction

ARIN’s best-known and certainly one of the most important functions maintained for the Internet community is the directory service information referred to as Whois. The Whois data is available through services known as [Whois-RWS](https://www.arin.net/resources/registry/whois/) and [Registration Data Access Protocol (RDAP)](https://www.arin.net/resources/registry/whois/rdap/). These public directory services are driven by a large relational database with classes of objects representing organizations, individuals, and resources, all of which interconnect to create meaningful, searchable information. This relational database is referred to as the ARIN Registry.

The data in the Whois directory service is, by definition, publicly available. ARIN also collects additional data to carry out its operational and legal duties and while that information is stored within the ARIN Registry – this information is generally not public.

The ARIN Mission Statement highlights the main service areas that ARIN covers and the operational justification for data collection:

> *ARIN supports the operation of the Internet through the management of Internet number resources throughout its service region; coordinates the development of policies by the community for the management of Internet Protocol number resources; and advances the Internet through informational outreach. ARIN will continue to utilize an open, transparent multi-stakeholder process for registry policy development.*

## Summary

This document describes what data ARIN collects, who it collects it from, and how it manages that data.

All operational data that ARIN collects is to either fulfill requirements to provide services or to meet legal requirements necessary to carry out its mission in a responsible fashion. Specifically, the services for which ARIN collects operational data include:

* Provide public, transparent registry information via RDAP and Whois-RWS that is authoritative and accurate for Internet number resources managed by ARIN.
* Provisioning the [Reverse Domain Name System](https://www.arin.net/resources/manage/reverse/)
* Publishing routing policies by network operators ([Internet Routing Registry](https://www.arin.net/resources/manage/irr/) / [RPKI](https://www.arin.net/resources/manage/rpki/))
* Facilitating [Internet operations and coordination](https://www.nro.net/technical-coordination/) and [security and management services](https://www.arin.net/resources/manage/).
* Enabling publicly available data to enable research and analysis of Internet Number Resource usage throughout the ARIN region in the form of [statistics](https://www.arin.net/reference/research/statistics/), [Bulk Whois](https://www.arin.net/reference/research/bulkwhois/), and [WhoWas](https://www.arin.net/reference/research/whowas/).

Billing information in the form of payment sources from customers is not kept by ARIN, but instead is provided by customers at the time of payment to our third-party payment processing vendor. At no time does ARIN collect or store this information.

ARIN may also collect some information in order to support its [Policy Development Process (PDP)](https://www.arin.net/participate/policy/pdp/), the [ARIN Consultation and Suggestion Process (ACSP)](https://www.arin.net/participate/community/acsp/process/), [elections](https://www.arin.net/participate/oversight/elections/) for the Board of Trustees, Advisory Council, and NRO Number Council, registrations for ARIN [events and meetings](https://www.arin.net/participate/meetings/), and various [mailing lists](https://www.arin.net/participate/community/mailing_lists/) and surveys established for communication within the ARIN community. This information is generally limited to names, email addresses, and contact information, but in the case of elections it may include biographical and other personal information. This information is not stored within the ARIN Registry database and is instead distributed through the various systems established to support these operational and community functions.

The focus of this document is to focus on the requirements surrounding only the operational data used in the ARIN Registry database and the public directory services.

### Principles and Goals of Data Collection and Management

The collection and management of customer data at ARIN should follow these principles and goals:

1. **Be efficient:** collect the absolute minimum amount of data necessary for the operation and legal requirements of ARIN and what is necessary to serve the ARIN community.
2. **Be secure:** protect the data customers provide to ARIN. This includes safeguarding data that must be kept private and managing access to data within ARIN systems to allow only authorized access.
3. **Be accurate:** To ensure the integrity of the ARIN directory service and to help protect against fraud, provide customers the tools necessary to keep their data accurate and up to date as easily as possible.
4. **Be transparent:** Make information about data collection and usage publicly available and update the community as necessary when changes occur.
5. **Be reliable:** Much of the data customers provide ARIN is to assist with coordination as it pertains to Internet operators. To that end, the consistent and reliable availability of the public data ARIN provides should be of paramount importance.

## Data Required by ARIN for Operational Requirements

As stated, the ARIN Registry is comprised of classes of objects that represent organizations, individuals, and Internet Number Resources. The classes of objects include:

* Organization Identifiers (Org ID): this is a “container” that holds all the other classes of objects and identifies your organization.
* Points of Contact (POC): This object identifies people or a role (group of people) within an organization that is responsible for the day-to-day management of an organization’s delegated Internet number resources.
* Autonomous System Numbers (ASNs): These represent an Autonomous System (AS) which are represented by networks that adhere to a single routing policy.
* Networks (NETs): These are IPv4 or IPv6 address blocks listed within ARIN Whois.
* Customers: These include the POCs and Resources for organizations that are associated with ARIN through a provider organization such as an Internet Service Provider (ISP). They do not have a direct relationship with ARIN but are listed in ARIN’s Whois.
* Delegations: These are entries necessary for Reverse DNS.

To support these objects, ARIN collects data at the time of a request for resources to process a request, or throughout the lifetime of a resource delegation to facilitate the management of those resources, as well as for publication of some public information to the directory service.

This data is collected via the [ARIN Online](https://www.arin.net/resources/guide/account/) secure portal and the [Registration RESTful Service (Reg-RWS)](https://www.arin.net/resources/registry/regrws//).

### Legal and Business Documentation

Throughout the course of a resource request, a request for the transfer of resources, or reviews of transactions, ARIN may request that customers provide operational or legal documentation in order to evaluate the request. This information is stored in the ARIN Registry and is not made available publicly. However, at the discretion of the customer, they may include internal or third-party contacts who may view this information as it is included within the ticket history.

## Data Requirements from Number Resource Policy

[ARIN’s Number Resource Policy Manual (NRPM)](https://www.arin.net/participate/policy/nrpm/) is the document created by the community that governs how ARIN delegates and manages Internet number resources. Within the document are some provisions that govern what data ARIN collects.

### Request Justification

Several sections of NRPM stipulate justification requirements for resource requests, generally involving proof of utilization of existing resources by an organization. ARIN collects this data in order to evaluate the request but does not publish it.

### Reassignment Information

Per Section 4.2.3.7.1. “Reassignment and Reallocation Information” of NRPM:

> *Each IPv4 reassignment or reallocation containing a /29 or more addresses shall be registered via SWIP or a directory services system which meets the standards set forth in section 3.2.*

> *Reassignment registrations must include each customer’s name, except where specifically exempted by this policy. Reassignment registrations shall only include point of contact (POC) information if either: (1) requested by the customer; or (2) the reassigned block is intended to be routed and announced outside of the provider’s network.*

> *Reallocation registrations must contain the customer’s organization name and appropriate point of contact (POC) information.*

This means that organizations not directly receiving resources from ARIN must still execute a Registration Services Agreement and provide contact information if they receive a large enough block of IPv4 addresses from an upstream provider.

### Privatizing POC Information

Per Section 3.3 of NRPM, organizations may designate some POCs as private (and thus not listed in ARIN directory services), as long as one POC is still listed.

### Residential Customer Privacy

In Section 4.2.3.7.3.2. of NRPM, organizations providing reassignment information may substitute their name for the downstream customer’s name, e.g., ‘Private Customer - XYZ Network’ and the downstream customer’s postal address may read ‘Private Residence’ if certain requirements are met regarding visible POCs.

## Legal Requirements for Data

### Registration Services Agreement

The [Registration Services Agreement (RSA) / Legacy Registration Services Agreement (LRSA)](https://www.arin.net/about/corporate/agreements/rsa.pdf) is a contractual agreement that governs the relationship between ARIN and its customers. Within the agreement, it stipulates the following regarding data requirements:

> *Section 3 (b): Responsibility for Directory Services Data. The Holder is responsible for the timely and accurate maintenance of directory services data (Whois) with respect to the Included Number Resources, as well as data concerning any organization to which Holder further sub-delegates the Included Number Resources.*

### Organization Vetting

While approving the creation of an Organization Identifier, transfer requests, or a review of an organization and its resources, ARIN may seek out information about an organization including business registrations, asset purchase agreements, etc. This information is kept confidential.

## Public and Internal Data

### ARIN’s Directory Services

The primary publicly available data ARIN provides is through its directory services Whois-RWS and RDAP.

**What Categories of Information are in ARIN’s Directory Services?**

Registration information about:

* Internet Protocol (IP) addresses and Autonomous System Numbers (ASNs) issued by ARIN
* IP addresses and ASNs issued by ARIN’s predecessor registries
* Organizations that hold these resources
* Points of Contact (POCs) for resources or organizations
* Customer reassignment information (from an ISP to its customers)
* Referential information for:
  + Other authoritative RIRs
  + Customer reassignment information put into an RWhois server by an ISP

**What types of information are not available in ARIN’s Directory Services?**

* Domain names and any associated information
* Authoritative information for:
  + Sub-delegations by ISPs using RWhois
  + Resources registered in other RIRs’ Whois
  + Customer reassignments smaller than /29 (per ARIN policy)
  + Some privatized residential customers (per ARIN policy)
* Routing information
* Accurate geographic location of the network
  + The main purpose of Whois is to record authorized users or assignees of an Internet resource. ARIN cannot guarantee that the address associated with an Internet resource record is the actual physical location of the network.

**What other types of data does ARIN provide via related services?**

ARIN may be able to provide current and historical information about:

* Upstream ISP or end user contact information (on direct allocations and assignments)
* Customer reassignment information smaller than /29
* Previous address holders after a transfer of the resources to another organization

In addition to the above, what types of information might ARIN share with law enforcement entities when given a subpoena?

* Any of the data outlined above
* Financial transaction records and billing POCs
* Banking information (if available)
* Other miscellaneous information provided by customers when seeking ARIN services

### Usage of Publicly Available Directory Service Data

The public data available through the ARIN Directory Service is restricted in use by the Internet community and general public for Internet operational or technical research purposes, including:

* Evaluating routing policies or assuring compliance with routing policies;
* Facilitating operational coordination between network operators (e.g., network problem resolution, outage notification, etc.);
* Providing reversed DNS and ENUM delegations;
* For use in conjunction with the normal course of business in providing network and Internet services, but only so long as such use does not republish, resell, or make publicly available data obtained from the ARIN directory service;
* Ensuring the uniqueness of Internet number resource usage;
* Conducting scientific research into network operations;
* Automated processing of abuse requests, research, and tracking of abuse issues in connection with the maintenance of Internet number resource registration data, and efforts to maintain accurate sub delegation records; and
* Identifying resources used or suspected of being used for unlawful or harassing purposes.

## Data Sources

Information in the ARIN Registry is collected from sources directly related to the usage of resources. This includes legacy data from predecessor registries, and current data from direct customers, and in some cases, the downstream customers of those organizations. ARIN generally does not update customer data but relies on customers to self-manage their data.

This information is in the form of identifying information about the organization and authorized Points of Contact (POC) for the organization or the resources it holds.

### Organizations

An organization, or a functional subset of an organization, e.g., a company may either be represented by a single Organization Identifier (Org ID) or multiple Org IDs. Org IDs are treated as independent entities, regardless of whether they are legally or functionally related to other Org IDs. All Internet number resources are associated at the Org ID level.

Within the ARIN Registry, the following information is collected and published in ARIN directory services:

* Organization Name
* DBA Name (if present, used as “Full Name” in directory services)
* Postal Address
* Affiliations with designated POCs and delegated Internet number resources

In addition, ARIN may request, collect, or populate the ARIN Registry with additional information regarding organizations to assist with reviews of requests or resources. This information is not published to ARIN Directory Services.

### Individuals

Individuals exist within the ARIN Registry as Points of Contact (POC) associated with organizations or resources, and in relation to ARIN Online user accounts. [ARIN’s Privacy Policy](https://www.arin.net/about/privacy/) also describes how an individuals data is handled within the context of using ARIN services.

#### Points of Contact

A POC is a specific person or role account that is associated with an IPv4, IPv6, or Autonomous System Number (ASN) record in the ARIN Registry. Registered POCs are the only ones authorized to make changes to an organization’s registration records. A POC can be specified as an Admin, Tech, Abuse, Network Operations Center (NOC), Routing, or Domain Name System (DNS) contact for an organization.

ARIN requires valid POC information for several reasons, including but not limited to:

* Individuals affiliated with an organization that requested or holds Internet number resources (IP addresses and/or ASNs) from ARIN and need to manage those resources
* Organizations and their designated POCs that received Internet number resources through a reassignment from an upstream provider
* Organizations and their designated POCs that hold legacy number resources (IPv4 addresses or ASNs) that are in the ARIN Registry and available through ARIN directory services.

Data collected from individuals for POC records includes:

* First Name
* Middle Name
* Last Name
* Postal Address
* Company Name
* Email Address
* Phone Number

#### ARIN Online Users

All ARIN accounts are managed through [ARIN Online](https://account.arin.net/public/login). ARIN Online is a secure online portal through which individuals manage the records and resources associated with their organizations. Data from ARIN Online user accounts is not published in ARIN directory services.

Data collected from individuals for their ARIN Online user accounts includes:

* First Name
* Middle Name
* Last Name
* Postal Address
* Company Name
* Email Address
* Security Questions and Responses
* Phone Number

## Data Retention

ARIN keeps all records related to operational and legal requirements of resource delegations in perpetuity. In the event of modifications or deletions of objects, the information that existed at the time of modification/deletion is no longer available publicly but still exists within the ARIN Registry database.

## Data Accuracy

One of ARIN’s core responsibilities is to maintain a registry of unique Internet number resources (IPv4 and IPv6 addresses and Autonomous System Numbers) and provide [accurate registration information](https://www.arin.net/reference/materials/accuracy/) about these resources, including their associated organization and contact information.

An accurate registry contributes to the overall operability and stability of the Internet in several ways.

### Change Monitoring

As stated earlier, information in ARIN’s Registry is held in perpetuity to ensure accurate and auditable records. However, with the bankruptcy of an organization, the dissolution of a relationship between an individual associated with a customer organization, or the death of an individual, ARIN must work to safeguard relevant data, while providing potential access to successors. This is necessary to ensure proper stewardship of Internet Number Resources (INR) as well as to protect against fraud and abuse.
