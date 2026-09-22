# ARIN API documentation

Official ARIN documentation collected on 2026-09-22 for provider development. These are local reading copies of article bodies, including tables and request/response examples. Site navigation is omitted, links point to upstream, and punctuation is normalized. ARIN retains ownership of the source documentation. Consult the linked upstream sources when implementing or updating behavior.

## Management APIs

| Area | Local documentation | Coverage |
| --- | --- | --- |
| Reg-RWS | [Overview](reg-rws/overview.md), [quick start](reg-rws/quickstart.md) | Authentication, requests, formats, prerequisites |
| Reg-RWS operations | [Methods](reg-rws/methods.md) | Customers, networks, delegations, organizations, POCs, associations, reports, tickets, errors |
| Reg-RWS data model | [Payloads](reg-rws/payloads.md) | XML fields, namespaces, examples, server-generated and editable fields |
| IRR | [API guide](irr/api.md), [overview](irr/overview.md) | route/route6, aut-num, as-set, route-set, permissions, XML/RPSL, errors |
| RPKI | [API guide](rpki/api.md) | Transactional creation/deletion of ROAs and ASPAs, list operations, payloads, automatic IRR links |
| Reverse DNS | [Reverse DNS](dns/reverse.md), [DNSSEC](dns/dnssec.md) | Delegation nameservers and DS records; API methods/payloads are in Reg-RWS |

## Authentication and testing

- [API keys](operations/api-keys.md)
- [Operational Test and Evaluation environment (OT&E)](operations/ote.md)
- [OT&E trust anchor locator](rpki/arin_ote.tal)
- [Software releases](operations/software-releases.md)
- [Registry data relationships](operations/registry-data.md)
- [Community software and reference clients](operations/software-tools.md)

## Public lookup APIs and schemas

- [RDAP](lookup/rdap.md): query syntax, examples, bootstrap behavior, and links to the relevant IETF specifications.
- [Whois-RWS API](lookup/whois-rws-api.md) and [overview](lookup/whois-rws.md): legacy public lookup interface, formats, searches, and references.
- [Published Whois-RWS Relax NG schema archive](schemas/whoisrws-relaxng-compact-schemas.zip). Despite the link label and filename, the archive contains Reg-RWS and RPKI payload definitions. See [schema findings](schemas/README.md) and the extracted files in `schemas/extracted/`.

## Registration workflows

- [Managing resource records](registry/records.md)
- [Network modifications](registry/network-modifications.md)
- [Organizations](registry/orgs.md) and [POCs](registry/pocs.md)
- [Reassignments](registry/reassignments.md) and [RWhois](registry/rwhois.md)

## RPKI workflows and protocols

- [Overview](rpki/overview.md) and [service options](rpki/options.md)
- [Hosted RPKI](rpki/hosted.md)
- [ROAs and IRR Auto-Manager](rpki/roas.md)
- [ASPAs](rpki/aspa.md)
- [Delegated RPKI](rpki/delegated.md)
- [Repository Publication Service](rpki/repository-publication.md)

## Reports and downloads

- [WhoWas overview](reports/whowas.md) and [format/readme](reports/whowas-readme.md)
- [Bulk Whois](reports/bulk-whois.md), including authenticated downloads and data format
- [Resources with no valid POCs](reports/invalid-pocs.md)
- Associations and reassignment report request methods are included in [Reg-RWS methods](reg-rws/methods.md).

## Provenance and refresh

- [sources.json](sources.json): explicit download inventory.
- [manifest.json](manifest.json): requested/resolved URLs, UTC retrieval times, MIME types, byte counts, and SHA-256 hashes of upstream responses and saved files.
- [links.json](links.json): links discovered within the saved articles, including related specifications and examples.
- The script can extract inline images into `media/`; the current articles use upstream illustration links. Text, tables, and code examples are available locally, but remote illustrations need a network connection.

From the repository root:

```sh
python3 -m venv .venv
.venv/bin/pip install -r scripts/requirements.txt
.venv/bin/python scripts/sync_api_docs.py
```

The refresh script fetches only the curated public URLs. It fails on HTTP errors or missing article bodies rather than saving an error page as documentation. It rewrites snapshots and metadata; review changes before using refreshed API behavior. Update this index when adding sources.

## Scope and remaining boundaries

This collection covers the current public API guides and associated operating documentation found through ARIN's documentation hubs and their links. It is not a mirror of every historical release, meeting presentation, blog post, or account-only help page. External RFCs and third-party implementations are linked, not copied. No authenticated resource records or account data are included.

No OpenAPI specification or write XSD was identified in the reviewed guides. A linked Relax NG archive does contain registration/RPKI definitions, but some differ from current documented payloads. Use the current XML payload references and OT&E to resolve differences. Documentation examples are retained as published, not certified executable fixtures; apparent inconsistencies should be checked in OT&E.
