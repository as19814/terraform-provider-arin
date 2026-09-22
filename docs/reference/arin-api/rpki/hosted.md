# Hosted RPKI

Source: https://www.arin.net/resources/manage/rpki/options/hosted/

Retrieved: 2026-09-22T22:50:51.931738+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## What is Hosted RPKI?

Hosted Resource Public Key Infrastructure (RPKI) is an infrastructure in which ARIN hosts a Certificate Authority and signs all Route Origin Authorizations (ROAs) for resources within the ARIN region. Only direct resource holders can participate in RPKI. Reallocated and reassigned net resources cannot be added to an organization’s RPKI certificate. Any downstream organization must have their upstream provider submit ROAs on their behalf.

Hosted RPKI’s benefits include:

* Ease of use
* Little to no coding required from participants
* Certificate Authority functionality work taken care of by ARIN
* Data security via a hardware security module
* Functioning repository provided by ARIN

In Hosted RPKI, ARIN issues you a certificate that means you are authorized to submit routing information for your resources. For example, you can specify that all traffic for a certain IP address that you manage should originate from a specified Autonomous System Number (ASN).

You then add your routing information in ARIN Online, and that information is propagated every few minutes to ARIN’s RPKI repository. Other organizations then use ARIN’s RPKI information to determine authorized routes for traffic on the Internet.

**IMPORTANT:** The Internet number resources you want to certify with RPKI must be covered by an [ARIN Agreement](https://www.arin.net/about/corporate/agreements/#registration-services-agreement).

## Configuring Hosted RPKI in ARIN Online

Configuring Hosted RPKI requires the following steps. Choose the links to obtain additional information about each step.

1. Log in to ARIN Online and select **Routing Security** from the navigation menu.

![Navigating to Routing Security](https://www.arin.net/resources/manage/rpki/images/ROAS_1_2B.png)

2. On the ‘Routing Security Dashboard’ page, under “Your Organizations,” select **Sign Up for RPKI** for the organization for which you want to configure Hosted RPKI.

![Sign up for RPKI](https://www.arin.net/resources/manage/rpki/images/ROAS_3B.png)

3. On the ‘Manage RPKI’ page, under “Choose Between Two Models of RPKI,” select **Sign Up for Hosted** to make your resource certificate request.

![Choose between two models of RPKI](https://www.arin.net/resources/manage/rpki/images/ROAS_4C.png)

4. In the top bar of the ‘Manage RPKI’ page, select **Hosted Certificate** to begin your certificate request.
5. After you submit your request, you will be returned to the ‘Routing Security Dashboard’ page. Select **Manage RPKI.**

![Manage RPKI - Manage ROAs](https://www.arin.net/resources/manage/rpki/images/ROAS_5B.png)

7. On the ‘RPKI: ROAs’ page, you can begin creating ROAs for your resources by selecting **Create ROA.**

![Create ROA](https://www.arin.net/resources/manage/rpki/images/ROAS_6C.png)

1. After entering the required information, select **Next Step.** Verify the information in your ROA is correct, choose whether to create a [matching IRR route object](https://www.arin.net/resources/manage/rpki/roas/#irr-auto-manager), and select **Submit.**

![Enter ROA Information](https://www.arin.net/resources/manage/rpki/images/ROAS_7D.png)
  
![Confirm ROA Information](https://www.arin.net/resources/manage/rpki/images/ROAS_8D.png)

You will be returned to the ‘RPKI: ROAs’ page, where you will receive confirmation that your ROA has been created, and your ROA will be listed in the “Route Origin Authorizations” table.

![New ROA Created](https://www.arin.net/resources/manage/rpki/images/ROAS_9C.png)

### VIDEO: Creating a ROA

### What is a Resource Certificate?

A resource certificate provides cryptographic validation that a collection of Internet number resources (IPv4 addresses, IPv6 addresses, and ASNs) belong to you as the authorized resource holder. These certificates contain no identifying information about the holder of the resources.

#### Accessing Your Resource Certificates

To view the information on your resource certificate from the ‘Manage RPKI’ page:

1. Log in to ARIN Online and select **Routing Security,** then **RPKI** from the navigation menu.
2. Select **View Details** for the organization whose resource certificate you wish to see.
3. Select **Certified Resources** from the top menu.

## Managing RPKI Resources

1. Log in to ARIN Online and select **Routing Security**, then **RPKI** from the navigation menu.
2. In the ‘Your Organization’ window, select **View Details** for the organization for which you want to manage RPKI resources.
3. You can perform the following actions:

* View, create, modify, and delete ROAs
* View your certified resources

## Using the Operational Test and Evaluation (OT&E) Environment

ARIN has created an RPKI instance within its OT&E for those wishing to experiment with RPKI without affecting production data. For more information, see the [OT&E page](https://www.arin.net/reference/tools/testing/).
