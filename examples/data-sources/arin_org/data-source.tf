data "arin_org" "example" {
  handle = "EXAMPLE-1"
}

output "organization_name" {
  value = data.arin_org.example.name
}
