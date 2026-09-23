# Manage distinct nameservers with separate resources.
# Do not also manage this zone with arin_delegation.
resource "arin_delegation_nameserver" "example" {
  delegation = "2.0.192.in-addr.arpa."
  name       = "ns1.example.net"
  ttl        = 3600
}
