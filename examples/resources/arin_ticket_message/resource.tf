# This sends correspondence to an existing ARIN ticket.
# Changing the content submits a new message; destroy only forgets the receipt.
resource "arin_ticket_message" "example" {
  ticket_number = "20260923-X1"
  subject       = "Requested information"
  text          = ["Additional information for this request."]
  category      = "JUSTIFICATION"
  attachments = {
    "evidence.txt" = base64encode("Supporting evidence")
  }
}
