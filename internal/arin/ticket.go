package arin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Ticket contains metadata only. It is not a full-record PUT representation:
// messages, attachments and sharing fields remain available through read APIs.
type Ticket struct {
	Number, Type, Status, Resolution                   string
	CreatedDate, UpdatedDate, ResolvedDate, ClosedDate string
}

func ValidateTicketNumber(number string) error {
	if len(number) < 10 || number[8] != '-' || !handlePattern.MatchString(number) {
		return errors.New("ticket number must use YYYYMMDD-NUM format")
	}
	for _, r := range number[:8] {
		if r < '0' || r > '9' {
			return errors.New("ticket number must use YYYYMMDD-NUM format")
		}
	}
	return nil
}
func decodeTicket(body []byte, expected string) (*Ticket, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "ticket" || root.Name.Space != registrationNamespace {
		return nil, errors.New("ARIN returned an unexpected ticket payload")
	}
	// Preserve a trustworthy generated identity even if later fields are invalid.
	numbers := nodesAt(root, "ticketNo")
	if len(numbers) != 1 || numbers[0].Name.Space != registrationNamespace || len(numbers[0].Children) != 0 {
		return nil, errors.New("ARIN returned an ambiguous or missing ticket number")
	}
	number := strings.TrimSpace(numbers[0].Text)
	if err := ValidateTicketNumber(number); err != nil {
		return nil, errors.New("ARIN returned an invalid ticket number")
	}
	if expected != "" && number != expected {
		return nil, errors.New("ARIN returned a different ticket")
	}
	ticket := &Ticket{Number: number}
	values, err := decodeFields(root, ticketFields)
	if err != nil {
		return ticket, err
	}
	ticket.Type = netString(values, "ticket_type")
	ticket.Status = netString(values, "ticket_status")
	ticket.Resolution = netString(values, "resolution")
	ticket.CreatedDate = netString(values, "created_date")
	ticket.UpdatedDate = netString(values, "updated_date")
	ticket.ResolvedDate = netString(values, "resolved_date")
	ticket.ClosedDate = netString(values, "closed_date")
	if !enumPattern.MatchString(ticket.Type) || ticket.Type == "ANY" || !enumPattern.MatchString(ticket.Status) || ticket.Status == "ANY" || ticket.Status == "ANY_OPEN" {
		return ticket, errors.New("ARIN returned incomplete ticket metadata")
	}
	return ticket, nil
}
func (c *Client) GetTicket(ctx context.Context, number string) (*Ticket, error) {
	if err := ValidateTicketNumber(number); err != nil {
		return nil, err
	}
	body, err := c.get(ctx, c.baseURL, "/rest/ticket/"+url.PathEscape(number)+"/summary", "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodeTicket(body, number)
}

// CloseTicket implements the documented RESOLVED -> CLOSED transition. It does
// not withdraw open requests or reopen closed tickets. A closed ticket is a no-op.
func (c *Client) CloseTicket(ctx context.Context, number string) (*Ticket, error) {
	current, err := c.GetTicket(ctx, number)
	if err != nil {
		return current, err
	}
	if current.Status == "CLOSED" {
		return current, nil
	}
	if current.Status != "RESOLVED" {
		return current, errors.New("only a RESOLVED ticket can be closed")
	}
	response, err := c.request(ctx, http.MethodPut, c.baseURL, "/rest/ticket/"+url.PathEscape(number)+"/ticketStatus/CLOSED?msgRefs=true", "application/xml", true, nil)
	if err != nil {
		return current, err
	}
	result, err := decodeTicket(response.Body, number)
	if err != nil {
		return result, err
	}
	if response.StatusCode != 200 || result.Status != "CLOSED" {
		return result, errors.New("ARIN did not confirm ticket closure")
	}
	verified, err := c.GetTicket(ctx, number)
	if err != nil {
		return result, err
	}
	if verified.Status != "CLOSED" {
		return result, errors.New("ARIN ticket is not closed after the update")
	}
	return verified, nil
}
