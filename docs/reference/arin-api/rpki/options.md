# RPKI Deployment Options

Source: https://www.arin.net/resources/manage/rpki/options/

Retrieved: 2026-09-22T22:50:51.848381+00:00

Publisher: American Registry for Internet Numbers (ARIN).

Local reading copy; formatting and punctuation normalized. Upstream is authoritative.

---

## Available RPKI Services

ARIN offers services supporting two models of RPKI deployment: **Hosted** and **Delegated**. Only direct resource holders can participate in RPKI. Any downstream organization must have their upstream provider submit Route Origin Authorizations (ROAs) on their behalf.

With Hosted RPKI, ARIN hosts a Certificate Authority and signs all ROAs for resources allocated within the ARIN region. With Delegated RPKI, ARIN will produce a delegated resource certificate at your request that can be used in your own Certificate Authority to sign your ROAs. You can maintain your own repository and publication server, or you can choose to use ARIN’s Repository Publication Service. In both models, ARIN serves as the trust anchor certifying that the rightful resource holder is the one who created the ROA.
  
  

## Hosted RPKI

Hosted RPKI is an solution in which ARIN hosts a Certificate Authority and signs all ROAs for resources within the ARIN region. The majority of RPKI users at ARIN utilize the Hosted service.

[Hosted RPKI](https://www.arin.net/resources/manage/rpki/options/hosted)

## Delegated RPKI

Delegated RPKI is an solution in which a Regional Internet Registry’s direct resource holders may request their own delegated resource certificates and host their own Certificate Authority. Delegated RPKI participants may then sign ROAs and issue resource certificates for their customers.

[Delegated RPKI](https://www.arin.net/resources/manage/rpki/options/delegated)

## Repository Publication Service (RPS)

This solution is recommended for organizations who choose the Delegated RPKI option but do not want to maintain a high-availability repository and run their own publication server.

[Repository Publication Service](https://www.arin.net/resources/manage/rpki/options/rps)

### Which one is right for my organization?

If your organization is just getting started with RPKI, you may want to choose the Hosted option; it is the easiest to use because the only thing your organization needs to do is create ROAs to cover your resources. ARIN is responsible for serving as the Certificate Authority and publishing the high-availability repository, as well as providing tools for easy creation and management. Over 95 percent of ARIN RPKI deployments use the Hosted service. Delegated RPKI is an option for an organization that wants to maintain cryptographic control and independence, but you should have in-depth knowledge about RPKI and the resources  -  both human and technological  -  to run a Certificate Authority and publication server.

| RPKI Option Comparison |  |
| --- | --- |
| [Hosted RPKI](https://www.arin.net/resources/manage/rpki/options/hosted/) | - Easiest to use.   - Organization only needs to create ROAs.   - Little to no coding required from participants.   - Certificate Authority functionality work taken care of by ARIN.   - Data security via a hardware security module.   - High availability repository provided by ARIN.   - Over 95 percent of ARIN RPKI deployments use the Hosted service. |
| [Delegated RPKI](https://www.arin.net/resources/manage/rpki/options/delegated/) | - For organizations that want to maintain cryptographic control and independence.   - Direct resource holders who may request their own delegated resource certificates and are responsible for maintaining their own Certificate Authority.   - May then sign ROAs and issue resource certificates to customers.   - Organizations are responsible for ensuring their resource certificates and ROAs are available to other entities.   - Should have in-depth knowledge about RPKI and the (human and technical) resources to run a Certificate Authority and publication server.   - ARIN is the nominated Trust Anchor. |
| [Repository Publication Service (RPS)](https://www.arin.net/resources/manage/rpki/options/rps/) | - A common term for this type of service is “Hybrid RPKI.”   - Organizations that host their own RPKI Certificate Authority may request ARIN maintain their high-availability repository.   - Should have in-depth knowledge about RPKI and the (human and technical) resources to run a Certificate Authority. |
