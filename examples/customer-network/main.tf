terraform {
  required_providers {
    arin = {
      source = "as19814/arin"
    }
  }
}

# Set ARIN_API_KEY in the environment. These defaults target OT&E.
provider "arin" {
  base_url = "https://reg.ote.arin.net"
}

variable "parent_net_handle" {
  type        = string
  description = "An allocation you control in OT&E."
}

variable "prefix" {
  type        = string
  description = "An available IPv4 or IPv6 CIDR within the parent allocation."
}

resource "arin_customer" "recipient" {
  parent_net_handle = var.parent_net_handle
  name              = "Example Customer"
  country_code      = "US"
  city              = "Chantilly"
  subdivision       = "VA"
  postal_code       = "20151"
  street_address    = ["123 Test Street"]
  private_customer  = true
}

resource "arin_net" "assignment" {
  parent_net_handle = arin_customer.recipient.parent_net_handle
  customer_handle   = arin_customer.recipient.id
  name              = "EXAMPLE-CUSTOMER-NET"
  prefixes          = [var.prefix]
}

output "customer_handle" {
  value = arin_customer.recipient.id
}

output "net_handle" {
  value = arin_net.assignment.id
}
