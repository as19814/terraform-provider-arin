# Use an ASN held by your organization and its actual provider ASNs.
# Import first if the ASPA already exists. Provider changes are atomic.
resource "arin_aspa" "example" {
  org_handle    = "EXAMPLE-1"
  customer_asn  = 19814
  provider_asns = [13335, 15169]
}

# An ASN with no providers can use provider_asns = [0].
# AS0 cannot be combined with other provider ASNs.
