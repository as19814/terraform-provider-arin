# Requires approved Bulk Whois access and ARIN_API_KEY.
# Content is base64 ZIP data and is stored as sensitive Terraform state.
data "arin_invalid_pocs" "example" {
  max_bytes = 67108864
}
