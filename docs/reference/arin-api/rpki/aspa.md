# Autonomous System Provider Authorizations (ASPAs)

Source: https://www.arin.net/resources/manage/rpki/aspa/

Retrieved: 2026-09-22T22:50:51.763570+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## Autonomous System Provider Authorization (ASPA) Overview

An ASPA is a cryptographically signed object that allows holders of an Autonomous System Number (ASN) to authorize other ASNs as their provider networks. ASPAs may only be generated for ASNs listed on the customer’s resource certificate.

Similar to a Route Origin Authorization (ROA), which contains only one Origin AS, an ASPA object may only contain one Customer ASN. If your organization has multiple ASNs, a unique ASPA object must be created for each one.

### Creating an ASPA in ARIN Online

1. Log in to ARIN Online and select **Routing Security** from the navigation menu.
2. In the ‘Your Organization’ window, select **Manage RPKI** for the organization for which you want to add an ASPA.
3. In the top menu, select **ASPAs**.
4. Above the ‘Autonomous System Provider Authorizations’ window, select **Create ASPA**.
5. In the ‘Create an Autonomous System Provider Authorization (ASPA)’ window, complete the required fields, then select **Create ASPA**.
6. In the ‘Review ASPA’ window, review and submit your ASPA request by selecting **Submit**.

## Viewing Your ASPAs

You can view your ASPAs using these methods:

### Using the API

Visit [ARIN’s RPKI RESTful API User Guide](https://www.arin.net/resources/manage/rpki/rpki-restful) to view a list of ASPAs for an organization. (Note that you will need an [ARIN Online account](https://www.arin.net/resources/guide/account/) with an [API Key](https://www.arin.net/reference/materials/security/api_keys/) to use Reg-RWS.)

### Using ARIN Online

1. Log in to ARIN Online and select **Routing Security** from the navigation menu.
2. In the ‘Routing Security Dashboard’ window, select **Manage RPKI.**
3. Select ‘ASPAs’ in the top menu to view those created for the organization.

You can view your ASPAs for another organization by using the drop-down menu in the upper left to select a different Org ID and selecting **ASPAs** in the top menu.

## Verifying Your ASPAs Are Active

The RPKI repository is updated every few minutes. To verify that your resources are active, you’ll need to use an RPKI validator and obtain ARIN’s RPKI repository. Visit [Using ARIN’s RPKI Repository for Routing](https://www.arin.net/resources/manage/rpki/#rpki-routing) for more information.

## Modifying an ASPA

You can modify your ASPAs using one of the following methods:

### Using the API

Visit [ARIN’s RPKI RESTful API User Guide](https://www.arin.net/resources/manage/rpki/rpki-restful) to modify an ASPA (note that you will need an [ARIN Online account](https://www.arin.net/resources/guide/account/) with an [API Key](https://www.arin.net/reference/materials/security/api_keys/) to use Reg-RWS).

### Using ARIN Online

1. Log in to ARIN Online and select **Routing Security** from the navigation menu.
2. In the ‘Your Organization’ window, select **Manage RPKI** to view those created for the organization.
3. Select the ASPA object you wish to modify.
4. In the ‘Autonomous System Provider Authorizations (ASPAs)’ window, select **Modify.**
5. Make changes to the ‘Set of Provier ASes,’ then select **submit.** Changes will take effect in the RPKI database immediately and will be reflected in the public RPKI repository within 24 hours.

## Removing an ASPA

You can delete your ASPAs using one of the following methods:

### Using the API

Visit [ARIN’s RPKI RESTful API User Guide](https://www.arin.net/resources/manage/rpki/rpki-restful) to delete an ASPA (note that you will need an [ARIN Online account](https://www.arin.net/resources/guide/account/) with an [API Key](https://www.arin.net/reference/materials/security/api_keys/) to use Reg-RWS).

### Using ARIN Online

1. Log in to ARIN Online and select **Routing Security** from the navigation menu.
2. In the ‘Your Organization’ window, select **Manage RPKI** to view those created for the organization.
3. Select the ASPA object you wish to modify.
4. In the ‘Autonomous System Provider Authorizations (ASPAs)’ window, select **Remove.**
5. Choose **Remove** again to confirm the removal. Changes will take effect in the RPKI database immediately and will be reflected in the public RPKI repository within 24 hours.
