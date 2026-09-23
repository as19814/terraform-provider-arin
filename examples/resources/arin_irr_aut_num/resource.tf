# Substitute an ASN registered to your organization and evaluate writes in OT&E.
resource "arin_irr_aut_num" "example" {
  as_number        = "AS64496"
  as_name          = "EXAMPLE-AS"
  org_handle       = "EXAMPLE-1"
  description      = ["Example routing policy"]
  import_policy    = ["from AS64497 accept ANY"]
  export_policy    = ["to AS64497 announce AS64496"]
  mp_import_policy = ["afi ipv6.unicast from AS64497 accept ANY"]
  mp_export_policy = ["afi ipv6.unicast to AS64497 announce AS64496"]
}
