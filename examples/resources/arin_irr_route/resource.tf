# Substitute prefixes registered to your organization; evaluate writes in OT&E.
resource "arin_irr_route" "example" {
  prefix      = "192.0.2.0/24"
  origin_as   = "AS64496"
  org_handle  = "EXAMPLE-1"
  description = ["Example IPv4 route"]
  remarks     = ["Managed with Terraform"]
  member_of   = [arin_irr_route_set.example.name]
}

resource "arin_irr_route" "example_v6" {
  prefix      = "2001:db8::/48"
  origin_as   = "AS64496"
  org_handle  = "EXAMPLE-1"
  description = ["Example IPv6 route"]
}

# Authorize this organization's routes to join a route set by reference.
resource "arin_irr_route_set" "example" {
  name           = "RS-EXAMPLE"
  org_handle     = "EXAMPLE-1"
  description    = ["Example routes"]
  members_by_ref = ["MNT-EXAMPLE-1"]
}
