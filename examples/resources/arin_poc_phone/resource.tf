# Do not overlap with arin_poc phone management.
# Extension changes delete and re-add this record.
resource "arin_poc_phone" "example" {
  poc_handle = "EXAMPLE-ARIN"
  type       = "F"
  number     = "+1-202-555-0101"
  extension  = "42"
}
