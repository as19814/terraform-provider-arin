# Application Programming Interface (API) Keys

Source: https://www.arin.net/reference/materials/security/api_keys/

Retrieved: 2026-09-22T22:50:52.821117+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

An Application Programming Interface (API) key is a secret code that you can use to identify yourself to ARIN when you interact with us. You create an API key in ARIN Online, and then use the key in interactions with ARIN outside of ARIN Online. Multiple interactions may be performed with the same API key, or you can create multiple API keys to locally track specific requests or to access reports. The API key does not expire, but can be deactivated at any time.

Your API key is used in some interactions with ARIN, including the following uses:

* To conduct RESTful registration actions with ARIN [(Reg-RWS)](https://www.arin.net/resources/registry/regrws/)
* Modifying networks using [RESTful calls](https://www.arin.net/resources/registry/manage/netmod/#using-restful-calls)
* [Securing DNS](https://www.arin.net/resources/manage/dnssec/)
* Resource Certification [(RPKI)](https://www.arin.net/resources/manage/rpki/)
* [Reverse DNS](https://www.arin.net/resources/manage/reverse/)
* To access reports and downloads, such as [Bulk Whois](https://www.arin.net/reference/research/bulkwhois/)

## Creating and Managing API Keys

To create an API key:

1. Log in to your [ARIN Online account](https://www.arin.net/resources/guide/account/) and select **Settings** from the menu under your name.
2. In the **Security Info** section, from the **Actions** menu, choose **Manage API Keys**.
3. In the **Generate API Key** section, choose **Create API Key**. The API key you just created will be shown in the **Generated API Key** field.

**This will be the only time that your full API Key will be shown to you.** Save the API Key in a password management tool, print it out, or write it down and store it in a safe place.

4. The API Key will be added to the table under **Manage Your API Keys,** but will only be identifiable by the Key Prefix and date of creation.

To deactivate an API key:

1. Log in to your [ARIN Online account](https://www.arin.net/resources/guide/account/) and select **Settings** from the menu under your name.
2. In the **Security Info** section, from the **Actions** menu, choose **Manage API Keys**.
3. In the **Manage API Keys** page, under **Manage Your API Keys**, find the key you want to deactivate and choose **Deactivate**. If the deactivation is successful, you receive a message and the key is moved to the list of inactive API keys.

## Using API Keys

### RESTful Calls

[ARIN’s RESTful Provisioning system](https://www.arin.net/resources/registry/regrws/) leverages modern application interfaces and provides even stronger authentication. RESTful calls require the use of an API key. In order for your RESTful call to be considered, your ARIN Online account must be linked to a Point of Contact (POC) that has authority to make the request.

### Report Downloads

Reports are available to authorized users by choosing **Downloads and Services** in ARIN Online. However, if you have an API key, a RESTful HTTP request containing your API key can be used to automate the retrieval of restricted access reports. This means you do not have to log in to your ARIN Online account to download the report. For more information, visit:

* [Bulk Whois Data](https://www.arin.net/reference/research/bulkwhois/#downloading-using-an-api-key)
* [Report of Resources with No Valid POCs](https://www.arin.net/resources/guide/account/records/poc/validation/readme/#downloading-using-an-api-key)
* [WhoWas NET Report](https://www.arin.net/resources/registry/regrws/methods/#request-whowas-net-report)
* [WhoWas ASN Report](https://www.arin.net/resources/registry/regrws/methods/#request-whowas-asn-report)
