# Reverse DNS

Source: https://www.arin.net/resources/manage/reverse/

Retrieved: 2026-09-22T22:50:53.279611+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

Domain Name System (DNS) translates hostnames into IP addresses; this process is known as forward resolution. DNS allows a user to enter `www.example.com` into the browser and receive an IP address for the server dedicated to that website. A lesser-known task that DNS performs is determining the hostname from an IPv4 address; this is commonly called reverse resolution or reverse DNS delegation. Reverse resolution is normally used by servers to find the human-friendly name associated with an IP address. The information for an IP address and the domain that it points to are located in pointer (PTR) records.

Reverse DNS is also used for functions such as:

* Network troubleshooting and testing
* Checking domain names for suspicious information, such as overly generic reverse DNS names, dialup users or dynamically-assigned addresses in an attempt to limit email spam
* Screening spam/phishing groups who forge domain information
* Data logging and analysis within web servers

The reverse DNS database is rooted under two specific domains: `in-addr.arpa` for IPv4, and `ip6.arpa` for IPv6. Each IP address associated with a domain has a record within at least one of these domains, known as a pointer (PTR) record. ARIN requires organizations to maintain their PTR records for their associated networks in order to keep reverse DNS running smoothly.

## Managing Reverse DNS Delegations

ARIN’s delegation management tools enable you to individually manage each reverse delegation within both IPv4 and IPv6 networks. Delegations can be managed in IPv4 on bit boundaries (/8, /16 or /24s), and IPv6 networks can be managed on nibble boundaries (every 4 bits of the IPv6 address). ARIN supports delegations for CIDR-aligned blocks for size /24 and larger. However, delegation sizes are determined by the sizes of the CIDR blocks that make up a given Direct Allocation of network resources from ARIN. For each CIDR block in the Direct Allocation, the largest possible delegations will be created.

For example, in IPv4, you could have a /23 network issued by ARIN that is comprised of two /24 delegations. In this example, you are able to delegate one set of nameservers to the first delegation and another set of nameservers to the second delegation. To use another example, if you have a /16 network, you would have one /16 delegation and would be able to manage nameservers for only that /16.

### Modifying Delegations

You have two options for managing your delegations/reverse DNS:

* [Using your ARIN Online account](https://www.arin.net/resources/manage/dnssec/#managing-reverse-dns-using-arin-online) (Good for users who need to certify small numbers of delegations)
* [Using the RESTful web service](https://www.arin.net/resources/registry/regrws/methods/#delegations) (Better for users who need to certify large numbers of delegations)

To manage DNS information through the RESTful web service, you first need to [obtain an API key](https://www.arin.net/reference/materials/security/api_keys/).

### Who Can Manage DNS Delegations?

Organizations who are direct and indirect resource holders can perform DNS management. Resource holders who directly receive space from ARIN will be able to manage their delegations. ARIN also allows organizations that receive space from an ISP to jointly manage this space with their ISP via SWIP. This is called shared authority.

**Note:** Organizations who receive space from their ISP via reassignments/reallocations will not have shared authority if the reassignment/reallocation is from the ISP’s /16 or larger. All DNS management would be performed by the ISP.

If you are an ISP and you delegate addresses via reassignment/reallocation to your customers, in ARIN’s Whois, you may see their organization name listed as an Authorized Organization. This indicates that they share the authority to manage the reverse DNS zone. They can log in to their ARIN Online account and manage DNS delegations, but only for the addresses you’ve delegated to them via reassignment/reallocation.

**Important**: As customers disconnect from you, it’s imperative that you protect your records by promptly removing any reassignments or reallocations to them to remove their shared authority rights for your reverse zones.

## Security

ARIN provides a method to secure reverse records. [Domain Name System Security (DNSSEC)](https://www.arin.net/resources/manage/dnssec/) is used to protect DNS information by digitally signing records using secure cryptography. Once your reverse zone is secured, you need to indicate to the parent (in this case, ARIN) that your zone is DNSSEC enabled. This signal to enable DNSSEC is done by using Delegation Signer (DS) records. You can also manage DS resource records for each delegation through ARIN Online or using the RESTful provisioning system.
