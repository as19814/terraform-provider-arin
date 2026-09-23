# Use an OT&E provider configuration while evaluating write behavior.
resource "arin_irr_as_set" "peers" {
  name        = "AS-EXAMPLE-PEERS"
  org_handle  = "EXAMPLE-1"
  description = ["Peers of the example network"]
  members     = ["AS64496", "AS64497"]
  remarks     = ["Managed with Terraform"]
}
