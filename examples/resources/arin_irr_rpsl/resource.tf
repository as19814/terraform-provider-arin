# Use contacts registered to your organization, and evaluate writes in OT&E.
resource "arin_irr_rpsl" "example" {
  object_type = "as-set"
  name        = "AS-EXAMPLE"
  org_handle  = "EXAMPLE-1"
  rpsl        = <<-RPSL
    as-set: AS-EXAMPLE
    descr: Example advanced AS set
    admin-c: ADMIN-ARIN
    tech-c: TECH-ARIN
    mnt-by: MNT-EXAMPLE-1
    members: AS64496
    source: ARIN
  RPSL
}
