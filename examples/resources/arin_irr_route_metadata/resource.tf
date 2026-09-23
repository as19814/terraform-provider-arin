# Replace the example prefix and ASN with resources owned by your organization.
resource "arin_roa" "announcements" {
  org_handle           = "EXAMPLE-1"
  name                 = "Customer announcements"
  asn                  = 19814
  prefixes             = { "192.0.2.0/24" = 24 }
  auto_link            = true
  delete_linked_routes = true
}

resource "arin_irr_route_set" "customer" {
  name           = "RS-CUSTOMER"
  org_handle     = "EXAMPLE-1"
  description    = ["Customer route membership"]
  members_by_ref = ["MNT-EXAMPLE-1"]
}

resource "arin_irr_route_metadata" "example" {
  prefix              = "192.0.2.0/24"
  origin_as           = "AS${arin_roa.announcements.asn}"
  org_handle          = arin_roa.announcements.org_handle
  expected_roa_handle = arin_roa.announcements.handle
  description         = ["Customer IPv4 route"]
  remarks             = ["Routing contact: noc@example.net"]
  member_of           = [arin_irr_route_set.customer.name]
}

# Removing only the metadata resource leaves the route and ROA intact.
# For an existing unlinked route, omit expected_roa_handle or set it to "".
