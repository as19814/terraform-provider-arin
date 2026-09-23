# Requires an existing delegated publication identity and enrollment.
# Keep the key file and persistent journal directory private.
data "arin_rpki_publication" "example" {
  endpoint                = "https://publication.example.net/service/publisher"
  publisher_handle        = "publisher"
  journal_directory       = pathexpand("~/.local/state/arin-rpki")
  signing_key_file        = pathexpand("~/.config/arin-rpki/signing-key.pem")
  signing_certificate_pem = file("signing-ee.pem")
  signing_ca_pem          = file("local-ca.pem")
  signing_crls_pem        = file("local-crls.pem")
  peer_ca_pem             = file("repository-ca.pem")
}
