# Published schema archive

Source: <https://www.arin.net/resources/registry/whois/rws/whoisrws-relaxng-compact-schemas.zip>

The [Whois-RWS API guide](../lookup/whois-rws-api.md) links this archive as its Relax NG schemas. The actual downloaded contents include registration and RPKI payloads, not just public lookup structures:

- `NetPayload.rnc` uses the namespace `http://www.arin.net/regrws/core/v1`.
- `RoaSpecPayload.rnc` uses `http://www.arin.net/regrws/rpki/v1`.
- There are customer, organization, POC, delegation, ticket, transaction, IRR, and ROA definitions.

The archive is preserved unchanged. `extracted/` contains its files for searching and inspection. The refresh script regenerates these copies from the archive.

Treat this as supplemental evidence. For example, `RoaSpecPayload.rnc` includes validity dates and does not include the current guide's `autoLink` field. The archive contains no ASPA-named schema. These differences mean it should not be used unchanged to generate a client for the current RPKI API.

Compare with [current Reg-RWS payloads](../reg-rws/payloads.md) and the [current RPKI guide](../rpki/api.md), then validate behavior in OT&E. This is an observed documentation inconsistency, not a claim that ARIN currently enforces these archived definitions.
