# Whois-RWS

Source: https://www.arin.net/resources/registry/whois/rws/

Retrieved: 2026-09-22T22:50:52.738168+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## About Whois-RWS

ARIN’s Whois-RWS is a public resource that allows a user to retrieve information about IP number resources, organizations, and Points of Contact (POCs) registered with ARIN. It pulls this information directly from ARIN’s database, which contains IPv4 and IPv6 addresses, Autonomous System Numbers (ASNs), organizations, customer reassignments, and related POCs.

ARIN provides a Representational State Transfer (REST)ful interface for Whois queries (Whois-RWS). RESTful systems usually utilize URLs that are sent using an Application Programming Interface (API). ARIN’s Whois-RWS can be accessed through its web interface with a browser or through command-line scripts.

Whois-RWS is still supported, but additional Whois functionality is available through Registration Data Access Protocol (RDAP). RDAP was developed by the IETF to enable querying registration data and to eventually replace the WHOIS protocol. ARIN provides Whois/RDAP through a [web search interface](https://www.arin.net/resources/registry/whois/rdap/#using-the-whois-rdap-web-interface). RDAP can also be accessed using [clients](https://www.arin.net/resources/registry/whois/rdap/#rdap-client) or [query URLs](https://www.arin.net/resources/registry/whois/rdap/#rdap-urls) that can be used in scripts.

## Accessing Whois-RWS

ARIN provides the following methods of accessing Whois-RWS:

* [Whois-RWS web interface](http://whois.arin.net/ui/): Enter your query term in the **Search Whois-RWS** field.
* [Whois-RWS API](https://www.arin.net/resources/registry/whois/rws/api/): You can write programs or scripts that use ARIN’s Whois-RWS API.

You can also access Whois-RWS using a [command-line interface](https://www.arin.net/resources/registry/whois/rws/cli/).

### Tips and Helpful Information for Searching in Whois-RWS

* To guarantee matching only a single record, look it up by its handle using a handle-only search. In the record summary line, the handle is shown in parentheses after the name.
* When using a handle to conduct a search for POC information, be sure to add the -ARIN extension.
* Queries that return more than 256 results will stop displaying data after the limit has been reached for each record type. You may want to narrow your search criteria or add flags to your query to limit the results.
* To search on an individual’s name, you can enter the last name, or to further restrict results, use the last name and first name, separated by a comma. (For example: Smith, John.)

## Additional Help using Whois-RWS

* Visit [Frequently Asked Questions about Whois-RWS](https://www.arin.net/resources/registry/whois/rws/faq/).
* For operational problems with Whois, please log in to your ARIN Online account and create an [Ask ARIN](https://account.arin.net/public/communication/message/beginQuestion.xhtml) ticket, or [contact the ARIN Help Desk](https://www.arin.net/resources/guide/helpdesk/) with the appropriate details.
* To report inaccurate information in Whois, you can [file a report](https://www.arin.net/resources/registry/whois/inaccuracy_reporting/) online.
