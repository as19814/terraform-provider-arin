# Discover the zone through arin_net_delegations.
# Import first if the zone already has DNS records.
# This resource owns all NS and DS records. Destroy clears both collections.
resource "arin_delegation" "example" {
  name = "2.0.192.in-addr.arpa."
  nameservers = [
    { name = "ns1.example.net", ttl = 3600 },
    { name = "ns2.example.net", ttl = 3600 },
  ]
  ds_records = []
}
