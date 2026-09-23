# Import an existing organization first. Creation may require ARIN staff review.
# This resource owns the complete POC collection, including the Admin contact.
resource "arin_org" "example" {
  name                 = "Example Networks"
  country_code         = "US"
  street_address       = ["123 Example Street"]
  city                 = "Chantilly"
  subdivision          = "VA"
  postal_code          = "20151"
  accept_reassignments = true
  poc_links = [
    { handle = "EXAMPLE-ARIN", function = "AD" },
    { handle = "EXAMPLE-ARIN", function = "T" },
    { handle = "EXAMPLE-ARIN", function = "AB" },
  ]
}
