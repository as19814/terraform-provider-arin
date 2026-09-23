# Requires existing delegated enrollment and a private persistent journal directory.
# Supply signed objects and their matching updated manifest together.
resource "arin_rpki_publication_bundle" "example" {
  endpoint                = "https://publication.example.net/service/publisher"
  publisher_handle        = "publisher"
  journal_directory       = pathexpand("~/.local/state/arin-rpki")
  signing_key_file        = pathexpand("~/.config/arin-rpki/signing-key.pem")
  signing_certificate_pem = file("signing-ee.pem")
  signing_ca_pem          = file("local-ca.pem")
  signing_crls_pem        = file("local-crls.pem")
  peer_ca_pem             = file("repository-ca.pem")
  objects = {
    "rsync://repository.example.net/module/publisher/issuer.crl"   = filebase64("issuer.crl")
    "rsync://repository.example.net/module/publisher/child.cer"    = filebase64("child.cer")
    "rsync://repository.example.net/module/publisher/manifest.mft" = filebase64("manifest.mft")
  }
}
