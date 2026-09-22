# Securing DNS (DNSSEC)

Source: https://www.arin.net/resources/manage/dnssec/

Retrieved: 2026-09-22T22:50:53.364000+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## Understanding DNS

*Domain Name System (DNS)* is the hierarchical naming system for all resources connected to the Internet or a private network, including websites, mail servers and application servers. Domain names are human-friendly identifiers (names) that are paired with the computer-friendly IP addresses (numbers). Domain names allow users to find a specific domain or object on the Internet, and direct their Internet-capable devices to it. For instance, the domain name `www.example.com` translates to the addresses `192.0.32.10` (IPv4) and `2606:2800:220:1:248:1893:25c8:1946` (IPv6). Both identify the same website, but `www.example.com` is easier for a person to remember and use.

Domain names are categorized hierarchically within DNS servers, or nameservers. When you access a web site or send an email, your device uses a nameserver to look up that particular domain name. This process is formally referred to as DNS name resolution. Nameservers exist for each domain, preventing the need for a single location or server to be in charge of any and all DNS information and any changes made to it.

*Reverse DNS*, or *reverse resolution*, is a system that provides the name of a domain or object when a user or device initially provides an IP address. Visit the [Reverse DNS](https://www.arin.net/resources/manage/reverse/) page for more information.

While DNS is invaluable to the Internet community, it is not without vulnerability. Internet criminals are capable of creating false DNS records, which may trick users into visiting websites or downloading malicious software.

In 2011, the FBI’s [Operation Ghost Click](https://www.fbi.gov/news/stories/international-cyber-ring-that-infected-millions-of-computers-dismantled) took down a group of Estonian and Russian hackers who used a DNS Changer botnet that hijacked over 4 million computers worldwide and made the group $14 million from fraudulent advertising revenue.

DNSSEC protects the Internet from these kinds of attacks using public-key cryptography. It allows users to validate that the DNS records they receive came from the correct source. *Domain Name System Security (DNSSEC)* provides verification of the name and IP address data so Internet traffic reaches the proper destination.

### DNS Zones

The DNS structure is hierarchically divided into segments called zones. Each DNS zone is responsible for a variety of tasks, such as defining that zone’s naming hierarchy and reg-istration procedures, and operating DNS servers to store that naming hierarchy. DNS zones may be separated by locality, organization, department, or by specific individuals registered to use a domain or sub-domain.

The original “base” zone is referred to as the “parent” zone, e.g. example.com; the sepa-rated subdomain is referred to as “child” zone, e.g. sub.example.com. With each subdivi-sion of a DNS zone, the “child” zones must contain records that provide referral infor-mation to other DNS servers, so that users querying the DNS can find the domain(s) with-in that zone. These records are called Nameserver (NS) records. In order for delegations to function properly, every parent zone must contain NS records for each of its child zones.

### DNS Resource Records

There are many types of records within the DNS that specify information about a given resource. These records might contain the resource’s IP address, nameserver information, geographic location, etc. The Internet Assigned Numbers Authority (IANA) provides [a list of DNS record types](https://www.iana.org/assignments/dns-parameters/dns-parameters.xhtml#dns-parameters-4).

DNSSEC is enabled with the addition of the following DNS record types:

* **RRSIG (Resource Record Signature)**: This record is provided by a DNS server whenever the DNS receives a query from a DNS resolver (the program responsible for initiating and sequencing DNS queries) for information about a particular resource.
* **DNSKEY (DNS Key)**: This record contains the cryptographic keys used to sign zone file records (records describing the contents and structure of a DNS zone). DNSSEC involves using DNSKEY records to cryptographically verify RRSIG records and ensure that outgoing Internet traffic is always sent to the correct place.
* **DS (Delegation Signer)** This record indicates that a certain child zone is digitally signed and that the key used to sign that zone’s Resource Record set is recognized as valid. These records are crucial to the chain of trust model DNSSEC is designed for. Each parent domain’s DS record is used to verify the DNSKEY record in its subdomain, from the top of the DNS hierarchy down.
* **NSEC (Next Secure)**: This record’s sole purpose is to prove that no records exist between two other records, preventing malicious parties from inserting false records into a DNSSEC-protected zone.

### Choosing How to Deploy DNSSEC

DNSSEC deployment is complex, and there are a variety of deployment strategies available depending on your organization’s objectives. General DNSSEC deployment models include:

* **Do it yourself:** As the name implies, this method consists of your organization finding the scripts, tools and software required to DNSSEC-enable your zone and configure them properly so that they work for your environment. This method has a higher learning curve and research requirement, but is more easily audited if you keep records of what you do.
* **Hosted solutions:** Many organizations offer DNSSEC deployment and management services for organizations that may not have sufficient manpower or training to do so in-house. This option eliminates the high learning curve, but may cost more.
* **Automated:** Some organizations may want to secure their DNS zones, but not have the time or manpower to take on a manual deployment of DNSSEC. There are automated DNSSEC deployment/maintenance products available that will handle secure key signing and re-signing, key rollover, and will allow online dynamic content and automatic record keeping.

Which deployment method will work best for your organization? Consider the following:

Does your organization have

* The expertise and training to properly enable DNSSEC?
* The staff and resources to deploy and maintain DNSSEC?
* The rigorous process discipline to keep DNSSEC working?

Also consider these issues:

* What level of security does your organization want; Is your organization prepared to implement it?
* Will your organization’s number and size of zones and how often they change considerably complicate your DNSSEC maintainence?
* Does your organization need detailed logs and records for the resources you use for potential auditing purposes?

Assess your needs against the questions and issues listed above to get a better idea of which method best suits your organization.

## Managing Reverse DNS and DNSSEC for Resources Issued by ARIN

ARIN’s [RESTful provisioning system (Reg-RWS)](https://www.arin.net/resources/registry/regrws/methods/#delegations) provides delegation management tools to individually manage reverse DNS within IPv4 and IPv6 networks once your zones are DNSSEC-enabled. You will need an [ARIN Online account](https://www.arin.net/resources/guide/account/) with an [API Key](https://www.arin.net/reference/materials/security/api_keys/) to use Reg-RWS. ARIN members may choose to DNSSEC-enable their reverse zones by submitting DS Records to ARIN. These DS records point to DNSKEY Resource Records that are held in the zone being maintained.

**Are you a source organization involved in a transfer to specified recipients within the ARIN region (NRPM 8.3) or an inter-RIR transfer (NRPM 8.4)?** See our [routing security best practices and source pre-transfer checklist](https://www.arin.net/resources/registry/transfers/#requirements-for-the-source-organization-current-registrant) to ensure a clean transition.

### Managing Reverse DNS Using ARIN Online

To manage reverse DNS for your resources using ARIN Online:

1. Log in to ARIN Online.
2. On the **Account Manager** dashboard, choose **Networks [NETs]** or select **IP Addresses** > **Manage Networks**
   in the left-hand navigation menu.
3. On the **Manage Networks** page, enter the network in the search field and select **Search**
   then choose the Net Handle for the network you want to modify.
4. On the **View & Manage Network** page, select **Manage Reverse DNS** in the Reverse DNS Information table.

A list is shown of the reverse DNS zones that you have permission to modify, the nameservers delegated to that zone, any registered DS resource record key tags, and the names of any organizations with shared authority over a zone.

#### Adding or Modifying Nameservers

The nameserver is the directory (usually managed by your hosting company) that translates domain names to IP addresses. To change the nameservers for the DNS zone or add new nameservers:

1. In the **Manage Reverse DNS** page, select the zones you want to change and choose **Modify Nameservers**.
2. Specify the hostnames (not the IP addresses) of the nameservers that should be authoritative for all of the reverse DNS delegations listed in the **Selected Delegations** field. Modifications are applied to all listed delegations, and any previous nameservers are deleted.
3. For each nameserver, enter a Time To Live (TTL) value. If a TTL is not specified, a default of 86400 seconds is used.
4. (Optional) Choose **Add Nameserver** to add additional nameservers.
5. Choose **Apply to All**. Changes are reflected immediately in the ARIN database, but may take 24 hours to be visible in DNS.

### Adding or Deleting DS Records Using ARIN Online

DS records are used to secure delegated zones. They contain the information used to verify the authenticity of the zones, such as encrypted information about the key used to sign the DNS record.

ARIN accepts DS information in space-delimited text format with the following fields:

* Zone: Optional; this field is ignored
* Class: Optional; default value = IN
* Time to Live: Optional; default value = 86400 seconds
* Resource Record Type: Delegation signer; value = DS
* Key Tag: 2-byte integer
* Algorithm: 1-byte integer; 5, 7, 8, 10, 13, 14, 15 or 16
* Digest Type: 1-byte integer; 1, 2, 3, or 4
* Digest: hex-encoded digest

The format would appear similar to the following example (without the explanatory header row):

![image containing a sample DS record](https://www.arin.net/resources/manage/dnssec/ds_record.png)

To manage DS records using ARIN Online:

1. In the **Manage Reverse DNS** page, select the zones you want to change and choose **Modify DS Records**.
2. A list of current DS records is displayed. You can delete records or upload new records. To upload new records:

* In the **Add DS Records** section, either paste the text of the DS record into the DS Records field, or choose a .txt file that contains the DS record and upload it.
* After the text of the DS Record is shown in the field, choose **Parse DS Record**. After the record is parsed, or checked for validity, it appears in the list of records.
* When you finish adding the DS records, choose **Apply to All**. Changes are reflected immediately in the ARIN database, but may take 24 hours to be visible in DNS.
