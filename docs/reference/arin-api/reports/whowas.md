# Historical Whois Data (WhoWas)

Source: https://www.arin.net/reference/research/whowas/

Retrieved: 2026-09-22T22:50:53.635084+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

ARIN’s WhoWas service provides authorized users access to historical registration information for a given IP address or ASN. This information is provided to allow users to research the historical registration information of specific Internet number resources as part of the normal course of business in providing related network and Internet services. WhoWas is not intended to be used to support research that requires high-volume querying, and excessive querying can result in termination of access to the service.

WhoWas functionality is available to ARIN Online users by choosing Downloads & Services from the navigation menu. Access to WhoWas data is available by request and must be approved by ARIN staff. Users must agree to comply with the [WhoWas Terms of Use (ToU)](https://www.arin.net/reference/research/whowas/tou/) before they can request WhoWas reports.

Users can request a WhoWas Report for any IP address or Autonomous System Number (ASN), not just resources issued to their Organization. Because the WhoWas report contains the entire public history of the IP address or ASN, it is much larger and more complex than a Whois query response. Each IP address or ASN could have been a part of multiple networks/AS ranges over time and have been issued to multiple organizations, each with their own associated Points of Contact (POCs).

Please read the [WhoWas ReadMe](https://www.arin.net/reference/research/whowas/readme/) to learn more about interpreting the results.

WhoWas Reports only contain the data that would have been publicly viewable in Whois.

## Requesting WhoWas Access

Your user account does not need to be linked to a POC or associated with an Organization Identifier (Org ID) to request WhoWas access, but you do need an ARIN Online account. Perform these steps:

1. Log in to ARIN Online.
2. Choose **Downloads & Services** from the navigation menu.
3. In the **WhoWas** section, choose **Submit Request for WhoWas Access**. ARIN Online creates a ticket for you. Notifications about your tickets can be accessed by choosing the icon next to your name, or choose **Tickets** from the navigation menu to see a list of your tickets.

This is a one-time request to grant authorization to access WhoWas reports; you will not have to repeat this step to request reports once access is granted.

### Understanding the Review Process

ARIN staff will review your request and respond within two business days. You will be asked to provide additional information to verify your WhoWas access request. The questions include:

* What is your intended use of this WhoWas data?
* How often do you project needing to query for WhoWas data?
* Will your organization be using the data to accomplish advertising or marketing goals or marketing research goals?

ARIN will conduct this correspondence through the notifications to you in your ARIN Online account. Upon approval, your account will be authorized to request reports.

For questions about your request, communicate via the Ask ARIN feature of your online account or call ARIN’s Registration Services Help Desk at +1.703.227.0660.

## Requesting a WhoWas Report

Once authorized, you can request a WhoWas report for an IP Address or ASN. Perform these steps:

1. Log in to ARIN Online.
2. Choose **Downloads & Services** from the navigation menu.
3. In the **WhoWas** section, choose **Request WhoWas Report**.
4. If this is the first time you have requested a WhoWas Report, accept the Terms of Use Agreement.

* *You will not be asked to do this on subsequent requests unless ARIN releases a new version of the WhoWas ToU. If a new ToU is enacted, you will be required to accept the new version before requesting another WhoWas report.*

5. Choose the tab to search by **IP Address** or **ASN**. The search will not accept handles or CIDR notation; you must enter the actual number. You cannot request a WhoWas report for an Org ID or POC. Reports are only available for IP addresses and ASNs administered by ARIN, and this does include legacy addresses.

You can also request WhoWas reports using Reg-RWS. Please refer to [Registration Records and Reg-RWS](https://www.arin.net/resources/registry/regrws/quickstart/) page for details.

## Accessing a WhoWas Report

You will receive a notification in ARIN Online when your WhoWas report is finished processing. The report will be attached to your ticketed request in a .zip file for you to download. You will be notified if there are no results for the requested address or ASN, and you may submit questions to Ask ARIN via the link in the response if you have questions about the results.

## Reading a WhoWas Report

For any IP address or ASN, the WhoWas report will contain a separate .tsv file for any Net Handle (direct or reassignment) that the IP Address was ever a part of or any AS Handle that the ASN was ever a part of, any Org Handles (including Customer Orgs) ever associated with those Net or AS Handles, and any POC Handles associated directly with the Net or AS Handles or with the Org Handles. All of these .tsv files are combined into a .zip file, which includes:

* a ReadMe file
* a summary file (lists the Net Handles or AS Handles, Org, Customer and POC Handles for which individual reports are returned)
* individual reports providing the historical registration information for the requested IP Address or ASN
* individual reports providing the historical registration information for the requested resource’s associated Organization and POC records

For detailed information on the content and format of the WhoWas report, please see the [WhoWas ReadMe documentation](https://www.arin.net/reference/research/whowas/readme/). Create a ticket using Ask ARIN in your ARIN Online account to submit any questions about the WhoWas service and reports or the data output.
