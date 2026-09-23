# The object must have been created using RPSL, not the XML interface.
data "arin_irr_rpsl" "example" {
  object_type = "as-set"
  name        = "AS-EXAMPLE"
}

output "advanced_as_set" {
  value = data.arin_irr_rpsl.example.rpsl
}
