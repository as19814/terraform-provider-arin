# Use an existing POC. Do not overlap with arin_poc email management.
resource "arin_poc_email" "example" {
  poc_handle = "EXAMPLE-ARIN"
  email      = "abuse@example.net"
}
