# Use an existing ticket that is RESOLVED or already CLOSED.
resource "arin_ticket_status" "example" {
  ticket_number = "20260923-X1"
  status        = "CLOSED"
}

# Destroy forgets local management; it does not reopen or delete the ticket.
