# Evaluate writes in OT&E using your organization handle.
resource "arin_irr_route_set" "example" {
  name           = "RS-EXAMPLE"
  org_handle     = "EXAMPLE-1"
  description    = ["Example routes"]
  members        = ["192.0.2.0/24", "198.51.100.0/24^+"]
  mp_members     = ["2001:db8::/32"]
  members_by_ref = ["MNT-EXAMPLE-1"]
}
