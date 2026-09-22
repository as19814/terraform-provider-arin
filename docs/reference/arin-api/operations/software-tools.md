# Community Software Repository

Source: https://www.arin.net/reference/tools/software/

Retrieved: 2026-09-22T22:50:53.197470+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

ARIN furnishes [a software repository](http://projects.arin.net/) as a service to the community, to promote tools that are related to ARIN’s mission. These tools include command-line clients, Java applications, Ruby scripts, and other clients that interface with ARIN’s Application Programming Interfaces (APIs), including our [Whois RESTful Web Service (Whois-RWS)](https://www.arin.net/resources/registry/whois/rws/api/), the [Registration RESTful Web Service (Reg-RWS)](https://www.arin.net/resources/registry/regrws/), and Resource Certification, also known as [Resource Public Key Infrastructure (RPKI)](https://www.arin.net/resources/manage/rpki/).

These tools are furnished “as is” and maintained by members in the community. If you would like ARIN to host your tools for the community to use, please contact [projects@arin.net](mailto:projects@arin.net) with a short description of the project and how it is relevant to ARIN’s mission. We strongly encourage community members to develop, test, and submit similar clients and tools for projects.arin.net, as our APIs and services continue to evolve.

## ArinWhois.NET

[ARIN-Whois.NET](https://github.com/MaxHorstmann/ArinWhois.NET) is a .NET client for ARIN’s Whois RESTful Web Service, the API to accessing ARIN’s Whois data. This client programmatically accesses ARIN’s RESTful API with a set of statically-typed helper classes.

## RDAP Bootstrap

[RDAP Bootstrap](https://github.com/arineng/rdap_bootstrap_server) is a web application that acts as an RDAP. This web application is meant for use in conjunction with RDAP clients without their own bootstrap knowledge. This bootstrap server will redirect RDAP clients to the most appropriate authoritative RDAP server.

RDAP is a specification from the IETF that is designed as a replacement for Whois data services for Domain Name Registries (DNRs) and Regional Internet Registries (RIRs). Unlike Whois, RDAP is an HTTP-based Representational State Transfer (REST)-style protocol with responses specified in JSON. Whois is a text based protocol, utilizing a specialized protocol and port. Whois defines no queries or responses, and as a result the interaction with DNRs and RIRs can vary significantly.

RDAP is specified in RFCs 7480, 7481, 7482, 7483, 7484, and 7485.

For more information, visit [Registration Data Access Protocol (RDAP)](https://www.arin.net/resources/registry/whois/rdap/).

## RPKI Up/Down Login

ARIN has implemented an RPKI instance within its [Operational Test and Evaluation environment (OT&E)](https://www.arin.net/reference/tools/testing/), which offers the opportunity to experiment with different facets of RPKI and ROA requesting in an environment with a production-like repository and UI, without any impact on production data.

## ARINcli

[ARINcli](https://github.com/arineng/arincli) is a set of command-line scripts, written in Ruby, that utilize ARIN’s Whois-RWS and Reg-RWS. Whois-RWS is ARIN’s Whois/NICNAME RESTful web service for exposing IP network and ASN registration data to the public (note that this service pre-dates the IETF WEIRDS/RDAP service and is not yet compatible with that specification). Reg-RWS is ARIN’s registration RESTful web service available to customers of ARIN.

ARINcli contains [the following scripts](http://projects.arin.net/arincli_html/arincli.7.html):

* arininfo: This script queries Whois data that follows ARIN’s data model more closely than most available Whois command-line clients. Features of this script include verbosity settings, data caching, tree-like record structure display, and intelligent sorting.
* poc: This script queries ARIN’s Registration RESTful Web Service (Reg-RWS) and returns organized, templated results that allow you to Manage Point of Contact records (POCs). This script allows you to create, modify, and delete any POC you have authority over without needing to use ARIN’s website or a REST client.
* arinreports: This script is used to request WhoWas reports, association reports and reassignment reports from ARIN directly. The resulting report is attached to a ticket that you may find and access via command-line as well.
* ticket: This script allows you to access, reply to, and interact with attachments on ARIN tickets.
* rdns: This script is used to modify reverse DNS delegations from the command line, removing a significant amount of work normally required to manage reverse DNS, especially for DNS Security (DNSSEC) users. This script will parse your zone files and DS sets, determine delegations, display all information to you, and upload it to ARIN for you.
