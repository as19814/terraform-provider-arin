# Import this existing linked route before applying.
# The owning ROA is managed outside this Terraform resource.
resource "arin_irr_linked_route" "example" {
  prefix              = "192.0.2.0/24"
  origin_as           = "AS19814"
  org_handle          = "EXAMPLE-1"
  expected_roa_handle = "ROA-HANDLE"
  description         = ["Independently managed customer route"]
  remarks             = ["Routing contact: noc@example.net"]
}

# Destroy deletes the IRR route but leaves its ROA authorization intact.
# Use arin_irr_route_metadata when Terraform also manages the owning ROA/bundle.
