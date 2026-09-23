# One submission is recorded in state. Refresh does not request another report.
resource "arin_report_request" "associations" {
  report_type = "associations"
}

resource "arin_report_request" "reassignments" {
  report_type = "reassignment"
  target      = "NET-192-0-2-0-1"
}

# WhoWas requires account authorization before requesting historical data.
resource "arin_report_request" "history" {
  report_type = "who_was_asn"
  target      = "19814"
}

# ticket_number can be used with ticket, message and attachment data sources.
# Destroy forgets the receipt; it does not delete or cancel the report ticket.
