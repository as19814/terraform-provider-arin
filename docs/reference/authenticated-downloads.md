# Authenticated download audit

The Bulk Whois and invalid-POC families now have a streaming client and mock
coverage. `arin_bulk_whois` and `arin_invalid_pocs` provide Terraform reads with mock coverage. OT&E routing and
authentication have been probed, but successful content retrieval has not been
verified with the available account.

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

The original HEAD probe made no GET requests, agreement submissions or account
changes. The probe
follows no redirects, reads no response body and prints no authenticated URL or
exception message. It is pinned to OT&E and does not fall back to a production
key. Reproduce after setting `ARIN_OTE_API_KEY` locally:

```sh
python3 tools/probe-downloads/probe.py
```

Exit zero means the HTTP observations completed, including denied requests. It
does not mean the account has download access or that these data sources exist.

## Streaming client and native GET evidence

`DownloadTo` selects `bulk_whois` or `invalid_pocs`. Bulk selections accept any
unique combination of asns, nets, orgs and pocs, with zip/xml/txt formats. Empty
selection means all objects; zip uses the documented complete-archive URL.
Invalid-POC downloads use the fixed nvpr ZIP endpoint. Selectors are validated
locally and cannot inject paths or query parameters.

The client defaults to the production or OT&E accountws origin matching its
registration origin. Custom registration origins require an explicit client
`DownloadBaseURL`. The configured API key is sent as the documented query
parameter, not as a Reg-RWS Authorization header. Redirects remain disabled.
Download failures never include response bodies, Location headers, transport
error text or authenticated URLs. HTTP 401/403 remain errors, not empty files.

Downloads stream to a caller-owned writer with a required positive size limit.
The limit is independent of the registration API's 4 MiB response cap. Successful
metadata includes size, SHA-256, media type and an optional response filename.
That filename is metadata only; it never selects an output path. Unsupported
media types/content encodings, unsafe filenames, empty responses, oversized
content and interrupted streams return errors. Any partial output must be
discarded; file callers should write to a temporary file and rename only after
success. The digest describes received bytes, not an upstream-signed integrity
claim. This transport does not extract ZIPs or validate report XML/ZIP contents.

Mocks cover deterministic URL selection, query-key authentication, redirects,
fixed-length/chunked limits, truncated bodies, writer failures, header validation,
cancellation, error redaction and streaming beyond 4 MiB. On 2026-09-23,
`TestOTEDownloadClientLifecycle` sent GETs for `bulkwhois/asns.xml` and `nvpr` to
OT&E. Both returned HTTP 403. Those native success cases are explicitly skipped,
not counted as successful artifact retrieval. No report was submitted and no
account settings or agreements were changed.

## Documented surface and remaining implementation

[Bulk Whois documentation](https://www.arin.net/reference/research/bulkwhois/)
describes a complete archive and combinations of `asns`, `pocs`, `orgs` and
`nets`, with ZIP, XML and text formats. The
[invalid-POC report](https://www.arin.net/resources/guide/account/records/poc/validation/readme/)
is a ZIP containing reports and schemas. Both require approved Bulk Whois access.
The documented download authentication uses an API key query parameter.

The user confirmed there is no existing OT&E Bulk Whois approval. No agreement
or enrollment request has been submitted. Next steps are approved sandbox access and
successful retrieval with validation of the returned report/schema formats.
Selection, transport bounds and error redaction have client mock coverage; native
format support and large-report behavior remain unverified.
An authorization failure must never become an empty successful data source.
Neither family is classified as unsupported by OT&E.

## Terraform data sources

`arin_bulk_whois` accepts an optional `objects` set and `format` (zip, xml or txt).
`arin_invalid_pocs` selects the fixed invalid-POC ZIP. Both accept `max_bytes`
(default 64 MiB) and `include_content` (default true). They return the canonical
selection ID, optional filename, media type, byte count, SHA-256 and sensitive
base64 content. With `include_content=false`, content stays null and the client
streams to a discard writer while computing metadata. This avoids storing the
artifact in Terraform state but still downloads all bytes on every refresh.

No local files are created. Content mode buffers bytes and stores base64 in state,
so callers should choose limits appropriate for their report size and state
backend. The provider's request timeout applies. `download_base_url` and
`ARIN_DOWNLOAD_BASE_URL` configure the separate account service; otherwise its
origin follows production/OT&E `base_url`. Custom registration origins need an
explicit download origin. No enrollment or report-generation request is made.

Terraform mocks cover content, metadata-only reads, input defaults, changed
remote artifacts, clean plans, environment and explicit origins, authorization
failure, size limits, invalid selection, redirects and unexpected media types.
The user-confirmed lack of account approval blocks successful native Terraform
download evidence; HTTP 403 is never represented as an empty result.
