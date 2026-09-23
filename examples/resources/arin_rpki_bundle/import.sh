# Import exactly these existing members, not the entire organization inventory.
terraform import arin_rpki_bundle.example '{"org_handle":"EXAMPLE-1","name":"customer-routing","roas":{"ipv4":"ROA-V4-HANDLE","ipv6":"ROA-V6-HANDLE"},"aspas":[19814]}'
