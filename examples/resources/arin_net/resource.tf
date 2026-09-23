# Replace the parent and prefix with your organization's address space.
resource "arin_customer" "recipient" {
  parent_net_handle = "NET-192-0-2-0-1"
  name              = "Example Customer"
  country_code      = "US"
  city              = "Chantilly"
  subdivision       = "VA"
  postal_code       = "20151"
  street_address    = ["123 Test Street"]
  private_customer  = true
}

resource "arin_net" "example" {
  parent_net_handle = arin_customer.recipient.parent_net_handle
  customer_handle   = arin_customer.recipient.id
  name              = "EXAMPLE-CUSTOMER-NET"
  prefixes          = ["192.0.2.0/29"]
  comments          = ["Example customer assignment"]
}

# For detailed reassignment, replace customer_handle with org_handle.
# For reallocation, use org_handle and set reallocate = true.

# Optional: configure supporting correspondence to send only on destroy.
# Apply this policy before removing the resource from configuration.
# removal_messages = [{
#   subject     = "Remove downstream assignment"
#   text        = ["This customer's assignment is no longer in use."]
#   category    = "JUSTIFICATION"
#   attachments = { "evidence.txt" = filebase64("evidence.txt") }
# }]
