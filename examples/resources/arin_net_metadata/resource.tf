# The NET must already exist. This also supports direct allocations.
# Destroy stops managing metadata and leaves the network and its values intact.
resource "arin_net_metadata" "example" {
  handle   = "NET-192-0-2-0-1"
  name     = "EXAMPLE-NET"
  comments = ["Network operations: https://example.net"]

  poc_links = [
    {
      handle   = "TECH-EXAMPLE"
      function = "T"
    },
    {
      handle   = "ABUSE-EXAMPLE"
      function = "AB"
    },
  ]
}

# Omit name, comments or poc_links to preserve their current values.
# Use comments = [] or poc_links = [] to clear a collection.
# With arin_net, configure only poc_links here to avoid overlapping ownership.
