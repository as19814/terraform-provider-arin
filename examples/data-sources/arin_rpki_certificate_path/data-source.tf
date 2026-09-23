# Validate an existing, published resource CA certificate without API credentials.
# Create both private directories beforehand and retain them across runs.
data "arin_rpki_certificate_path" "example" {
  certificate_pem     = file("/private/arin/existing-resource-ca.pem")
  resource_anchor_pem = file("/private/arin/resource-anchor.pem")

  # One URL per issuer, ordered from immediate issuer through the pinned anchor.
  rrdp_notifications         = ["https://repository.example/notification.xml"]
  rrdp_cache_directory       = "/private/arin/rrdp"
  manifest_history_directory = "/private/arin/manifests"
}

output "validated_issuer_chain" {
  value = data.arin_rpki_certificate_path.example.issuer_chain_pem
}

# For an arin_rpki_certificate using this same issuer path, set:
# issuer_chain_pem = data.arin_rpki_certificate_path.example.issuer_chain_pem
# Use an independently available certificate as input to avoid a dependency cycle.
