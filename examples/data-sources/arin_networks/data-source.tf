data "arin_networks" "ours" {
  org_handle = "FT-684"
}

output "networks" {
  value = data.arin_networks.ours.networks
}

output "ipv4_networks" {
  value = {
    for handle, network in data.arin_networks.ours.networks :
    handle => network if network.ip_version == "v4"
  }
}
