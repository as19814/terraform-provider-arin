terraform {
  required_providers {
    arin = {
      source = "as19814/arin"
    }
  }
}

# Set ARIN_API_KEY in the environment.
provider "arin" {
  base_url        = "https://reg.ote.arin.net"
  timeout_seconds = 30
}
