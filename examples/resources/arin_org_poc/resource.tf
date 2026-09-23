# Both records must exist. Import this association if already present.
resource "arin_org_poc" "example" {
  org_handle = "EXAMPLE-1"
  poc_handle = "EXAMPLE-ARIN"
  function   = "N"
}
