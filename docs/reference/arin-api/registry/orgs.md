# Organization Identifiers (Org IDs)

Source: https://www.arin.net/resources/guide/account/records/org/

Retrieved: 2026-09-22T22:50:54.340959+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## Introduction

An Organization ID (Org ID) represents a business, nonprofit corporation, or government entity in the ARIN database. This includes individuals that [meet the legal requirements](https://www.arin.net/resources/guide/request/individual_request/) of the jurisdiction where they conduct business. (Note that the term `Org Handle` is also sometimes used to refer to the Org ID, which is the unique identifier for organization records in all of the services ARIN provides.) IP addresses and AS numbers (ASNs) directly issued by ARIN must be associated with an Org ID. ARIN’s longstanding business practice is to require any organization requesting Internet number resources to be an active business entity legally formed within the ARIN service region.

The Org ID is defined by a legal name, postal address, and Points of Contact. Internet Service Providers (ISPs) and other direct allocation holders may also reassign or reallocate IP addresses to an Org ID. An organization can establish more than one Org ID, and is responsible for all the fees associated with the resources held by each one. Unless you have a specific reason to maintain multiple accounts, it is recommended that you consolidate resources under one Org ID to simplify management.

## Methods to Create Org IDs

You use an [ARIN Online account](https://account.arin.net/public/accountSetup.xhtml) to establish and manage your Org ID records. Org IDs can also be created, as part of scripted automation, by submitting your request via REST calls using the [RESTful Provisioning System](https://www.arin.net/resources/registry/regrws/).

If you are submitting your Org ID request via a REST command, you will need to include an [API key](https://www.arin.net/reference/materials/security/api_keys/) to authorize processing. You can use the same API key you generated to establish your Point of Contact (POC). Visit [API Keys](https://www.arin.net/reference/materials/security/api_keys/) for information on how to create and manage API keys.

**Note**: New Org IDs may only be created by an authorized contact representing an entity that ARIN is able to validate. Org IDs cannot be created upon the request of third parties.

## Creating an Org ID

**Note**: An [Organization Create](https://www.arin.net/resources/fees/fee_schedule/#transactional-fees) fee must be paid prior to processing these requests.

### Method 1: ARIN Online

After you’ve established your POC or linked your user account to an existing POC, select the **Organization Identifiers** link on the dashboard or choose **Your Records** > **Organization Identifiers** in the navigation menu. In the **Org Actions** menu, choose **Create Org ID** and follow these steps:

1. Enter the Org Info. You will need to provide a legally registered name with your Org ID request. You may also specify a “doing-business-as” (d/b/a) name, if you have one. (Note that this legal name and, if one was specified, d/b/a name listed must match the legal name and d/b/a name on your signed [Registration Services Agreement](https://www.arin.net/about/corporate/agreements/#registration-services-agreement) that you will complete before ARIN can approve your Org ID.)
2. Enter your address.
3. Choose the Points of Contact (POCs) for your Org ID. Your POC must have already been created; see [Creating a POC](https://www.arin.net/resources/guide/account/records/poc/#creating-a-poc) for more information.
4. Enter additional optional information, such as public comments.

After you submit your Org ID request, ARIN will issue a ticket number. ARIN staff will review your request and respond within two business days. Before ARIN approves your request, you will need to submit a signed [Registration Services Agreement](https://www.arin.net/about/corporate/agreements/#registration-services-agreement).

During the approval process, ARIN will send responses through a notification to you in your ARIN Online account. You may be asked to supply additional documentation to verify your organization is an actively registered legal entity.

Canadian customers must provide a tax identification number (e.g. Canada Revenue Agency Business Number [CRA BN], Quebec Enterprise Number [NEQ]) during the Org ID create process.

### Method 2: RESTful Web Service

Visit our [RESTful Methods](https://www.arin.net/resources/registry/regrws/methods/) page for more information and to view the [API documentation for ARIN’s RESTful web service](https://www.arin.net/resources/registry/regrws/).

## Modifying an Org ID

To modify an Org ID, you must first link your ARIN Online user account to one of the Admin or Tech POC handles associated with the Org ID.

### Method 1: ARIN Online

After your user account is linked to the appropriate POC, you can modify the Org ID by performing these steps:

1. Choose **Your Records** > **Organization Identifiers** in the navigation menu.
2. Choose the **Org Handle** that you want to modify. The **Organization Record** page is displayed.
3. Choose **Actions** and select **Modify** to access the **Modify Organization Record** page.
4. Enter your changes and choose **Submit**. Your changes will take effect immediately.

### Method 2: RESTful Web Service

Visit our [RESTful Methods](https://www.arin.net/resources/registry/regrws/methods/) page for more information and to view the [API documentation for ARIN’s RESTful web service.](https://www.arin.net/resources/registry/regrws/)

## Associating Points of Contact (POCs) with an Org ID in ARIN Online

After you create your Org ID and add the initial POCs to the Org ID, you can add or replace POCs that are associated with the Org ID. To change the POCs associated with your Org ID, perform these steps:

1. Choose **Your Records** > **Organization Identifiers** in the navigation menu.
2. Choose the **Org Handle** that you want to modify. The **Organization Record** page is displayed.
3. Under Organization Points of Contact, the POCs that are currently associated with the Org ID are displayed. To add or remove them, choose **Manage**.
4. If adding a POC, the associated POCs are displayed. To search for a different POC, choose the **Find POCs** tab to search for the existing POC that you want to add by POC Handle, first or last name, or another search field.
5. From the results list, next to the POC you want to add, choose **Use this POC**.
6. Choose **Submit**. Your changes will take effect immediately.

## Changing Your Organization Name

### ARIN Online Only

If your organization changes its legal name, you can update your Org ID by performing the following steps:

1. Choose **Your Records** > **Organization Identifiers** in the navigation menu.
2. Choose the **Org Handle** that you want to modify. The **Organization Record** page is displayed.
3. Choose **Actions** and select **Request Name Change**.
4. Enter the new organization name. Name change requests will be ticketed for tracking and you will receive a notification in ARIN Online within two business days. If approved, you may need to sign a new [Registration Services Agreement (RSA)](https://www.arin.net/about/corporate/agreements/rsa.pdf) under the new name before ARIN will process the name change request.

**Note** : If the name change is part of a merger, acquisition, reorganization, or similar scenario, you cannot use this option. Instead, follow the instructions for submitting a [transfer request](https://www.arin.net/resources/registry/transfers/).

## Deleting an Org ID

The Admin or Tech POC can delete an Org ID if the organization is not associated with any resources or membership and has no open tickets.

### Method 1: ARIN Online

If your user account is linked to a POC with the appropriate permissions to add or delete an Org ID, you can delete an Org ID by performing the following steps:

1. Choose **Your Records** > **Organization Identifiers** in the navigation menu.
2. Choose the **Org Handle** that you want to delete. The **Organization Record** page is displayed.
3. Choose **Actions** and select **Remove**.
4. Confirm the removal. The change will take effect immediately and will be reflected shortly in ARIN’s Whois.

### Method 2: RESTful Web Service

Visit our [RESTful Methods](https://www.arin.net/resources/registry/regrws/methods/) page for more information and to view the [API documentation for ARIN’s RESTful web service.](https://www.arin.net/resources/registry/regrws/)

## Recovering an Org ID

**Note**: An [Organization Recovery](https://www.arin.net/resources/fees/fee_schedule/#transactional-fees) fee must be paid prior to processing these requests.

### ARIN Online Only

To modify an Org ID, you must link your ARIN Online user account to one of the Admin POCs or Tech POCs associated with the Org ID. If you are unable to link your user account to one of the Admin/Tech POCs (for example, the Admin or Tech POCs have left the organization), you will need to “recover” your Org ID. Select **Your Records** > **Organization Identifiers** in the navigation menu. In the **Org Actions** menu, choose **Recover Org ID** to start the Org ID recovery process.

After you submit your Org ID recovery request, ARIN will issue a ticket number. ARIN staff will review your request and respond within two business days with a notification to you in your ARIN Online account. You may be asked to supply additional documentation to verify you are authorized to recover the Org ID. You may also be asked to submit a signed [Registration Services Agreement](https://www.arin.net/about/corporate/agreements/#registration-services-agreement) before your recovery request is approved, if there is no signed RSA for your organization.
