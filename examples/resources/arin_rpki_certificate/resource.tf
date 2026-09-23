# Requires existing delegated enrollment and BPKI/resource certificates.
# Keep the three private directories persistent across Terraform runs.
resource "arin_rpki_certificate" "example" {
  endpoint                = "https://parent.example/provisioning"
  child_handle            = "assigned-child"
  parent_handle           = "parent"
  journal_directory       = "/private/arin/exchanges"
  signing_key_file        = "/private/arin/bpki-ee.key"
  signing_certificate_pem = file("/private/arin/bpki-ee.pem")
  signing_ca_pem          = file("/private/arin/bpki-ca.pem")
  signing_crls_pem        = file("/private/arin/bpki.crl.pem")
  peer_ca_pem             = file("/private/arin/parent-bpki.pem")

  class_name = "assigned-class"
  csr_pem    = file("/private/arin/resource-ca.csr.pem")

  resource_anchor_pem        = file("/private/arin/resource-anchor.pem")
  issuer_chain_pem           = file("/private/arin/issuers-to-anchor.pem")
  rrdp_notifications         = ["https://repository.example/notification.xml"]
  rrdp_cache_directory       = "/private/arin/rrdp"
  manifest_history_directory = "/private/arin/manifests"
}
