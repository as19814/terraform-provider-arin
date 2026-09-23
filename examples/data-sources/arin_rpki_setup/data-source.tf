data "arin_rpki_setup" "parent" {
  xml = file("${path.module}/parent-response.xml")
}
