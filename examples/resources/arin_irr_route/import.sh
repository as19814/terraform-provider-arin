# Import existing simple routes using their canonical prefix and origin ASN.
terraform import arin_irr_route.example '192.0.2.0/24,AS64496'
terraform import arin_irr_route.example_v6 '2001:db8::/48,AS64496'
