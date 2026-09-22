# Referral Whois (RWhois)

Source: https://www.arin.net/resources/registry/reassignments/rwhois/

Retrieved: 2026-09-22T22:50:54.255910+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## What is Referral Whois (RWhois)?

RWhois (Referral Whois) is a directory services protocol which extends and enhances the Whois concept in a hierarchical and scalable fashion. It focuses on the distribution of “network objects,”–the data representing Internet resources or people–and uses the inherently hierarchical nature of these network objects (for example, domain names, IP networks, and email addresses) to more accurately discover the requested information.

RWhois synthesizes concepts from other, established Internet protocols to create a more useful way to find resources across the Internet. The RWhois protocol and architecture derive a great deal of structure from the Domain Name System (DNS) [[RFC 1034](https://www.rfc-editor.org/rfc/rfc1034.txt)] and borrow directory service concepts from other directory service efforts, primarily [X.500]. The protocol is also influenced by earlier established Internet protocols, such as the Simple Mail Transfer Protocol (SMTP) [[RFC 821](https://www.rfc-editor.org/rfc/rfc821.txt)] for response codes.

ARIN resource number policy [establishes requirements](https://www.arin.net/participate/policy/nrpm/#3-2-distributed-information-service-use-requirements) for Distributed Information Servers, such as RWhois, including that it be accessible to the public and ARIN staff. If you are interested in building and maintaining a RWhois server, ARIN’s RWhois code repository is available [on GitHub](https://github.com/arineng/rwhoisd).

Visit [RFC 2167](https://www.rfc-editor.org/rfc/rfc2167.txt) for more information.

## Reporting Reassignments via RWhois

ARIN requires organizations to submit information for all IPv4 reassignments of /29 and IPv6 reassignments of /47 or shorter prefix within seven days of the subdelegation. Organizations may only submit reassignment data for records within their allocated blocks.

As outlined in the [Number Resource Policy Manual (NRPM](https://www.arin.net/participate/policy/nrpm/#3-2-distributed-information-service-use-requirements)), ARIN has requirements for organizations that use distributed information servers, such as RWhois. While the exact details of these requirements are available in the NRPM, here is a summary:

* The server must always be operational and available to both the general public and ARIN staff. Reasonable downtime for server maintenance is allowed.
* Organizations may protect their distributed information server by restricting the number of queries allowed per time interval from a host or subnet.
* While the server must return reassignment information for the IP address queried, privacy protections for residential customers may be put in place following [ARIN’s residential customer privacy policy](https://www.arin.net/participate/policy/nrpm/#4-2-3-7-3-2-residential-customer-privacy).
* The distributed information service may return results for non-IP queries and may include optional attributes.
* The distributed information service must respond to a query with the minimal set of attributes per object as defined by ARIN staff, and results must by up-to-date on reassignment information.

## Examples

For IP network address requests, ARIN Customer Service Resource Analysts use the requirements outlined in ARIN policy to review reassignments from upstream providers to their customers. The RWhois attributes that ARIN reviews as part of the IP network address process are listed along with example network reassignments using RWhois 1.0 and RWhois 1.5.

### Version 1.5

```
network:ID:
network:Network-Name:
network:IP-Network:
network:Org-Name:
network:Street-Address:
network:City:
network:State:
network:Postal-Code:
network:Country-Code:
network:Tech-Contact:
network:Updated:
network:Updated-By:
network:Class-Name:network
```

### Version 1.0

```
network:Organization-Name:
network:Organization-Postal:
network:Organization-Postal:
network:Organization-Country-Code:
network:Handle:
network:Network-Name:
network:IP-Network:
network:Class-IP-Network:
network:Class-IP-Network-Block:
network:Admin-Contact:
network:Admin-Contact:
network:Admin-Contact:
network:Admin-Contact:
network:Updated:
```

### Version 1.5 Example

```
network:ID: NET-WIDGET
network:Network-Name: WIDGET
network:IP-Network: 10.1.1.0/24
network:Org-Name: Widget Corp
network:Street-Address: 211 Oak Drive
network:City: Pineville
network:State: WI
network:Postal-Code: 48888
network:Country-Code: US
network:Tech-Contact: BZ142-MYRWhois
network:Updated: 19991221
network:Updated-By: jo@myRWhois.net
network:Class-Name:network
```

### Version 1.0 Example

```
network:Organization-Name: Widget Corp
network:Organization-Postal: 211 Oak Drive
network:Organization-Postal: Pineville, WI 48888
network:Organization-Country-Code: US
network:Handle: NET-WIDGET
network:Network-Name: WIDGET
network:IP-Network: 10.1.1.0/24
network:Class-IP-Network: 10.1.1.0
network:Class-IP-Network-Block: 10.1.1.255
network:Admin-Contact: Smith, Jo
network:Admin-Contact: BZ142-MYRWhois
network:Admin-Contact: jsmith@widget.com
network:Admin-Contact: 444-444-1212
network:Updated: 21-Dec-1999
```
