# Network Modifications

Source: https://www.arin.net/resources/registry/manage/netmod/

Retrieved: 2026-09-22T22:50:54.614716+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

You can modify your Network Record (NET) to change the network name, Origin Autonomous System (AS), Resource Point of Contact (POC) handles, and public comments on an IP address block.

**Note:** Changes to [reverse DNS delegation](https://www.arin.net/resources/manage/reverse/) cannot be made with a network modification.

Network modifications must be submitted by a POC with the appropriate permissions, as defined in the following table:

POC Types and Permissions

| Action | Admin | Tech | Routing |
| --- | --- | --- | --- |
| Modify Network Name | ✅ | ✅ |  |
| Modify Public Comments | ✅ | ✅ | ✅ |
| Modify Resource POCs | ✅ | ✅ |  |

## Using Your ARIN Online Account

To modify a NET using ARIN Online:

1. Select **IP Addresses** > **Manage Networks** from the navigation menu, then select the Net Handle of the NET you need to update.
2. From the **Actions** menu, choose **Modify**.
3. Enter the updated information and choose **Submit**. Your changes will be visible immediately in your account, and ARIN’s Whois will reflect the update shortly thereafter.

## Using RESTful Calls

To automate large numbers of network modifications, you can use RESTful calls.

**Note:** To modify a NET through a RESTful call, you must first [generate an API key](https://www.arin.net/reference/materials/security/api_keys/).

### RESTful Calls

ARIN offers a RESTful web service that you can use to modify network records. This provisioning system allows for more secure interactions with ARIN’s database and provides even stronger authentication on automated submissions. Visit [Automating Record Management with Reg-RWS](https://www.arin.net/resources/registry/regrws/) for more information.
