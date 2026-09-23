# Replace these prefixes and ASN with resources authorized by your organization.
# Each prefix maps to the longest prefix length the origin may announce.
resource "arin_roa" "example" {
  org_handle = "EXAMPLE-1"
  name       = "Customer announcements"
  asn        = 19814
  prefixes = {
    "192.0.2.0/24"  = 24
    "2001:db8::/32" = 48
  }
  auto_link            = false
  delete_linked_routes = false
}

# Updates replace the ARIN handle in one atomic transaction.
# Use asn = 0 with auto_link = false for an AS0 authorization.
