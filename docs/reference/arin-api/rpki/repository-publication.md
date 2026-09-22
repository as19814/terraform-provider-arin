# Repository Publication Service (RPS)

Source: https://www.arin.net/resources/manage/rpki/options/rps/

Retrieved: 2026-09-22T22:50:54.525251+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## What is ARIN’s Repository Publication Service (RPS)?

As stated in [RFC8181](https://datatracker.ietf.org/doc/html/rfc8181), in order to make participation in the Resource Public Key Infrastructure (RPKI) easier, it is helpful to have a few consolidated repositories for RPKI objects, thus saving every participant from the cost of maintaining a new service. Similarly, Relying Parties using the RPKI objects will find it faster and more reliable to retrieve the necessary set from a smaller number of repositories.

These consolidated RPKI object repositories will in many cases be outside the administrative scope of the organization issuing a given RPKI object. A common term for this type of service is ‘Hybrid RPKI.’

## Why Use a Remote RPKI Publication Repository?

In some cases, outsourcing operation of the repository will be an explicit goal: some resource holders who strongly wish to control their own RPKI private keys may lack the resources to operate a 24/7 repository or may simply not wish to do so.

The operator of an RPKI publication repository may well be an Internet registry which issues certificates to its customers, but it need not be; conceptually, operation of an RPKI publication repository is separate from operation of an RPKI Certificate Authority (CA).

Even in cases where a resource holder operates both a certificate engine and a publication repository, it can be useful to separate the two functions, as they have somewhat different operational and security requirements.

### How to Configure a Certificate Authority to use RPS

You must obtain a Certificate Authority software that supports RFC8181, [A Publication Protocol for the Resource Public Key Infrastructure (RPKI)](http://datatracker.ietf.org/doc/rfc8181), such as Krill or Dragon Research Lab’s RPKI Toolkit.

The steps below show how an organization configures the Krill Certificate Authority software to use the ARIN Publication Service:

1. In Krill, choose the **Repository** tab to get the Publisher Request XML it has generated. Copy the XML to the clipboard.

![Screen for getting Publisher Request](https://www.arin.net/resources/manage/rpki/images/Krill-get-publisherRequest.png)

2. Log in to ARIN Online and:
   1. Select **Routing Security**, then **RPKI** from the navigation menu.
   2. In the ‘Your Organizations’ window, select **View Details** for the delegated organization for which you want to configure RPS.
   3. In the top navigation menu, choose **Publication Repository**.
   4. Paste the XML copied from Krill into the Publisher Request XML field and choose **Submit**.
   5. ARIN will generate and display a Repository Response in XML format. Use **Copy** to copy the response into the clipboard.

![Screen for ARIN Online Publication Repository](https://www.arin.net/resources/manage/rpki/images/ARIN-RPS-initial.png)

3. Open the Krill software and choose the **Repository** tab.

![Screen for putting Publisher Request in the configuration](https://www.arin.net/resources/manage/rpki/images/Krill-put-publisher-response-here_larger.png)

4. Paste the response you copied to clipboard into the appropriate section and choose **Confirm**.

### Creating Route Origin Authorizations (ROAs)

After you have received a delegated resource certificate from ARIN, you (or organizations under you whose resources you are certifying) will use a Certificate Authority (CA) package such as Krill to create ROAs. ROAs are stored in the RPKI repository along with a CRL (Certificate Revocation List). A CRL is a list of resource certificates that have been revoked and should not be relied upon. A CRL is always issued by the CA which issues the corresponding certificates. The publication server (either yours or ARIN’s, if you are using the ARIN Publication Service for Delegated RPKI) will provide these objects to requesting entities.

## Additional Information

For additional information on RPKI, including Delegated RPKI, visit the following resources:

* RPKI Documentation at [https://rpki.readthedocs.io](https://rpki.readthedocs.io/en/latest/)
* [Krill - a free, open source RPKI daemon](https://krill.docs.nlnetlabs.nl/en/stable/index.html)
* Internet Engineering Task Force (IETF) Requests for Comments (RFCs) relevant to Delegated RPKI:
  + [3986: Uniform Resource Identifier (URI): Generic Syntax](http://datatracker.ietf.org/doc/rfc3986)
  + [5280: Internet X.509 Public Key Infrastructure Certificate and Certificate Revocation List (CRL) Profile](http://datatracker.ietf.org/doc/rfc5280)
  + [6481: A Profile for Resource Certificate Repository Structure](http://datatracker.ietf.org/doc/rfc6481)
  + [6486: Manifests for the Resource Public Key Infrastructure (RPKI)](http://datatracker.ietf.org/doc/rfc6486)
  + [6492: A Protocol for Provisioning Resource Certificates](http://datatracker.ietf.org/doc/rfc6492)
  + [8183: An Out-of-Band Setup Protocol for Resource Public Key Infrastructure (RPKI) Production Services](https://datatracker.ietf.org/doc/rfc8183)
