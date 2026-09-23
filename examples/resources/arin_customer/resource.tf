# Use a parent network registered to your organization.
# This creates the recipient only, without reassigning address space.
resource "arin_customer" "example" {
  parent_net_handle = "NET-192-0-2-0-1"
  name              = "Example Customer"
  country_code      = "US"
  city              = "Chantilly"
  subdivision       = "VA"
  postal_code       = "20151"
  street_address    = ["123 Test Street", "Suite 2"]
  private_customer  = true
}
