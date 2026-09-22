# Managing Resource Records

Source: https://www.arin.net/resources/registry/manage/

Retrieved: 2026-09-22T22:50:53.452980+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## API Keys

An Application Programming Interface (API) Key is a shared secret that can be used to identify yourself in your interactions with ARIN. When submitting calls into ARIN’s RESTful Provisioning system, and for some other transactions, you will need an API key. [Learn how to create, manage and use your API keys](https://www.arin.net/reference/materials/security/api_keys/) to authenticate your transactions.

## Reassignments and Reallocations

When managing IP address space, organizations often need to delegate portions of their allocations to other parties. ARIN supports this through different types of actions: Simple Reassignments, Detailed Reassignments, and Reallocations.

### Simple Reassignments

A simple reassignment allows a direct registrant of IP addresses to show that a portion of their address space is being used by another party. These are commonly used to demonstrate utilization for justification purposes. However, the receiving entity does not possess any rights or capability over the reassignment.

Key characteristics:

* The organization name in the reassignment is entered as free text and is not vetted by ARIN.
* The listed name typically refers to a customer of the direct registrant.
* The receiving organization cannot manage the reassigned block.
* The direct registrant retains full control and can modify or delete the record at any time.

### Detailed Reassignments

A detailed reassignment provides more flexibility and allows the receiving organization to manage the reassigned IP addresses directly. These are appropriate when the receiving organization requires more autonomy.

Key characteristics:

* Requires an Organization Identifier (Org ID) and a Registration Services Agreement (RSA) with ARIN.
* Allows the receiving organization to add contacts and manage the reassigned IP addresses in ARIN Online.
* The receiving organization can interact with ARIN but lacks the ability to reassign space further.
* Management of the record is shared between the direct registrant and the receiving organization, allowing both parties to add, remove, or modify Internet Routing Registry (IRR) objects and reverse DNS.
* The direct registrant retains the ability to reclaim the reassignment at any time.

### Reallocations

A reallocation is a formal delegation of IP address space to another organization for independent management. This method is appropriate for Internet Service Providers (ISPs) or organizations that will further delegate address space to their customers, and will create a fully independent relationship between ARIN and the receiving organization.

Key characteristics:

* Requires the receiving organization to have an Org ID and an RSA.
* The receiving organization gets full management rights and can create reallocation/reassignment records to other organizations.
* The IP addresses are fully delegated to the receiving organization, and they are responsible for their use.
* Reallocations are appropriate when the receiving party will manage and subdelegate IP address space.
* Management of the record is shared between the direct registrant and the receiving organization, allowing both parties to add, remove, or modify IRR objects and reverse DNS.
* The direct registrant retains the ability to reclaim the reallocation at any time.

Organizations that receive IPv4 or IPv6 address space allocations from ARIN directly or as a downstream customer of another organization must provide reassignment/reallocation information via either Referral Whois (RWhois)or a distributed service in compliance with [ARIN policy](https://www.arin.net/participate/policy/nrpm/#3-2-distributed-information-service-use-requirements).

* [Reporting Reassignments via RWhois](https://www.arin.net/resources/registry/reassignments/rwhois/#reporting-reassignments-via-rwhois)
* [Requesting Removal of Stale Reassignment / Reallocation Records](https://www.arin.net/resources/registry/reassignments/stale_records/)
* [RESTful Provisioning (Reg-RWS) Information](https://www.arin.net/resources/registry/regrws/)
  + [Reassigning NETs with Reg-RWS](https://www.arin.net/resources/registry/regrws/methods/#reassign-net)
  + [Reallocating NETs with Reg-RWS](https://www.arin.net/resources/registry/regrws/methods/#reallocate-net)

## Resource Modifications

A Network Modification request is used to change the network name, POC handles, and public comments on an IP address block. Changes to reverse DNS delegation cannot be made with a network modification.

ASN Modifications can be done in ARIN Online.

* [Modifying an IPv4 or IPv6 Network](https://www.arin.net/resources/registry/manage/netmod/)
  + [Modifying Networks Using ARIN Online (Recommended)](https://www.arin.net/resources/registry/manage/netmod/#using-your-arin-online-account)
* [RESTful Provisioning (Reg-RWS)](https://www.arin.net/resources/registry/regrws/)

## Returning Resources

ARIN welcomes the return of any unused or unneeded resources so they may be added back to our pool of available Internet number resources for issuance to other organizations.

To learn more about returning your resources, visit [Understanding the Return and Revocation Process](https://www.arin.net/resources/registry/manage/return_revoke/).

## Reverse DNS

Reverse DNS is used to determine the domain name that is associated with a given IP address using the Domain Name System (DNS). Reverse resolution is accomplished using pointer (PTR) records that are rooted in the `in-addr.arpa` and `ip6.arpa` domains. ARIN requires organizations to maintain their `in-addr.arpa` and `ip6.arpa` domain records. You can maintain these records using ARIN Online or ARIN’s Reg-RWS.

* [Reverse DNS](https://www.arin.net/resources/manage/reverse/)
* [Securing DNS (DNSSEC)](https://www.arin.net/resources/manage/dnssec/)
* [RESTful Provisioning (Automating Record Management with Reg-RWS)](https://www.arin.net/resources/registry/regrws/)

## Internet Routing Registry (IRR)

ARIN’s Internet Routing Registry (IRR) service allows network operators to submit, maintain, and retrieve router configuration information abstracted from the languages and syntaxes utilized by router configuration software. [Internet Routing Registry](https://www.arin.net/resources/manage/irr/) provides a detailed explanation of how to access and utilize this information to support your organization.

## Resource Public Key Infrastructure (RPKI)

RPKI is an opt-in service that allows users to certify their Internet number resources to help secure Internet routing. Internet routing is vulnerable to hijacking and the provisioning/use of certificates is one of the first steps required to make routing more secure. Widespread RPKI adoption will help simplify IP address holder verification and routing decision-making throughout the ARIN region. More information is provided at [Resource Certification (RPKI)](https://www.arin.net/resources/manage/rpki/).

## Legacy Number Resources

The Legacy Registration Services Agreement (Legacy RSA or LRSA) has been offered to those organizations and individuals in the ARIN service region who hold legacy Internet number resources not covered by any other Registration Services Agreement (RSA) with ARIN. Legacy holders who sign up have been guaranteed the same services provided to other organizations who have a signed RSA with ARIN. Legacy holders will still enjoy this benefit; however, this will be now done by executing a new RSA document that combines the Legacy RSA and the RSA into a single document. This combined RSA document is also now being offered to non-legacy holders.

There is an annual maintenance fee for Legacy holders who sign the combined RSA. The annual invoice will be sent to the designated billing Point of Contact (POC) approximately 60 days before it is due.

If you have questions, please visit the [Legacy Resource Holder FAQ](https://www.arin.net/about/corporate/agreements/rsa_faq/#legacy) section of the [Registration Services Agreement FAQ](https://www.arin.net/about/corporate/agreements/rsa_faq/) or [Legacy Resource Services](https://www.arin.net/resources/guide/legacy/services/).
