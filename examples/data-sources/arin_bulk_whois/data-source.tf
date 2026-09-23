# Requires approved Bulk Whois access and ARIN_API_KEY.
data "arin_bulk_whois" "example" {
  objects         = ["asns", "nets"]
  format          = "xml"
  include_content = false
  max_bytes       = 268435456
}
