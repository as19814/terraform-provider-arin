# Replace the example prefixes and ASNs with resources owned by your organization.
# Every change to this managed group is submitted in one atomic transaction.
resource "arin_rpki_bundle" "example" {
  org_handle = "EXAMPLE-1"
  name       = "customer-routing"
  roas = {
    ipv4 = {
      name                 = "Customer IPv4"
      asn                  = 19814
      prefixes             = { "192.0.2.0/24" = 24 }
      auto_link            = false
      delete_linked_routes = false
    }
    ipv6 = {
      name     = "Customer IPv6"
      asn      = 19814
      prefixes = { "2001:db8::/32" = 48 }
    }
  }
  aspas = {
    "19814" = [13335, 3356]
  }
}

# Use {} for either category when only ROAs or only ASPAs are needed.
# Existing objects require manifest import. Do not manage these members elsewhere.
