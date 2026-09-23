# New POCs are linked to the account owning ARIN_API_KEY.
resource "arin_poc" "example" {
  contact_type   = "ROLE"
  company_name   = "Example Networks"
  last_name      = "Network Operations"
  country_code   = "US"
  subdivision    = "VA"
  city           = "Chantilly"
  postal_code    = "20151"
  street_address = ["123 Example Street"]
  emails         = ["noc@example.net"]
  phones = [
    { type = "O", number = "+1-202-555-0100" },
  ]
  comments = ["Network operations contact"]
}
