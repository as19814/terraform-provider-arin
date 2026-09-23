# Authenticated download audit

The Bulk Whois and invalid-POC download families remain unimplemented. OT&E
routing and authentication have been probed, but successful content retrieval
has not been verified with the available account.

## Native evidence

On 2026-09-23, HTTPS HEAD requests to `accountws.ote.arin.net` returned:

| Path | No key | Authorization header | API key query parameter |
| --- | --- | --- | --- |
| `/public/rest/downloads/bulkwhois` | 401 | 401 | 403 |
| `/public/rest/downloads/bulkwhois/asns.xml` | 401 | 401 | 403 |
| `/public/rest/downloads/nvpr` | 401 | 401 | 403 |
| `/public/rest/downloads/terraform-provider-arin-nonexistent` | 404 | 404 | 404 |

The query used the same OT&E key that passed registration lifecycle tests. The
nonexistent path is a routing control. These results distinguish the download
routes from a generic catch-all response; they do not prove successful GET
behavior, available report contents, or support for every selector/format.
The 403 responses indicate an access barrier for this account. Header auth
cannot be assumed to work merely because it works on Reg-RWS.

No GET requests, agreement submissions or account changes were made. The probe
follows no redirects, reads no response body and prints no authenticated URL or
exception message. It is pinned to OT&E and does not fall back to a production
key. Reproduce after setting `ARIN_OTE_API_KEY` locally:

```sh
python3 tools/probe-downloads/probe.py
```

Exit zero means the HTTP observations completed, including denied requests. It
does not mean the account has download access or that these data sources exist.

## Documented surface and remaining implementation

[Bulk Whois documentation](https://www.arin.net/reference/research/bulkwhois/)
describes a complete archive and combinations of `asns`, `pocs`, `orgs` and
`nets`, with ZIP, XML and text formats. The
[invalid-POC report](https://www.arin.net/resources/guide/account/records/poc/validation/readme/)
is a ZIP containing reports and schemas. Both require approved Bulk Whois access.
The documented download authentication uses an API key query parameter.

Next steps are to obtain evidence of approved sandbox access, verify successful
GET responses and included schemas, then implement bounded downloads with mock
and native Terraform tests. Selection, formats, content integrity, credential
redaction and large-report behavior still require implementation and validation.
An authorization failure must never become an empty successful data source.
Neither family is classified as unsupported by OT&E.
