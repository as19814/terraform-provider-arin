data "arin_rpki_setup_request" "child" {
  message_type = "child_request"
  handle       = "example-child"
  bpki_ta_pem  = file("${path.module}/bpki-ca.pem")
}
