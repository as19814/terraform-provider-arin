# Use an existing ticket that is RESOLVED or already CLOSED.
resource "arin_ticket_status" "example" {
  ticket_number = "20260923-X1"
  status        = "CLOSED"
  # Optional: use the full-ticket PUT endpoint instead of the default status endpoint.
  # update_method = "payload"
}

# Destroy forgets local management; it does not reopen or delete the ticket.
