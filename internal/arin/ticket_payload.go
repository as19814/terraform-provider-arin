package arin

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
)

// closedTicketPayload changes only the direct status element's text. All other
// bytes, including namespaces, dates, sharing metadata and message references,
// survive unchanged. Never rebuild a full PUT from the summary-only Ticket type.
func closedTicketPayload(body []byte, number string) ([]byte, *Ticket, error) {
	ticket, err := decodeTicket(body, number)
	if err != nil {
		return nil, ticket, err
	}
	if ticket.Status != "RESOLVED" && ticket.Status != "CLOSED" {
		return nil, ticket, errors.New("only a RESOLVED ticket can be closed")
	}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	depth, count := 0, 0
	var start, end int64
	inStatus := false
	for {
		before := decoder.InputOffset()
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, ticket, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			depth++
			if inStatus {
				return nil, ticket, errors.New("ticket status contains nested content")
			}
			if depth == 2 && token.Name.Local == "messages" {
				return nil, ticket, errors.New("full ticket update requires message references, not embedded messages")
			}
			if depth == 2 && token.Name.Local == "webTicketStatus" {
				if token.Name.Space != registrationNamespace {
					return nil, ticket, errors.New("ticket status has an unexpected namespace")
				}
				count++
				inStatus = true
				start = decoder.InputOffset()
			}
		case xml.EndElement:
			if inStatus && depth == 2 {
				end = before
				inStatus = false
			}
			depth--
		}
	}
	if count != 1 || end <= start {
		return nil, ticket, errors.New("ticket status is missing or ambiguous")
	}
	out := append([]byte{}, body[:start]...)
	out = append(out, []byte("CLOSED")...)
	out = append(out, body[end:]...)
	return out, ticket, nil
}

// CloseTicketWithPayload uses the full-ticket modification endpoint. It reads a
// fresh payload with message references, preserves its server-owned fields and
// changes only RESOLVED to CLOSED. It neither appends correspondence nor retries.
func (c *Client) CloseTicketWithPayload(ctx context.Context, number string) (*Ticket, error) {
	if err := ValidateTicketNumber(number); err != nil {
		return nil, err
	}
	path := "/rest/ticket/" + url.PathEscape(number) + "?msgRefs=true"
	body, err := c.get(ctx, c.baseURL, path, "application/xml", true)
	if err != nil {
		return nil, err
	}
	payload, current, err := closedTicketPayload(body, number)
	if err != nil {
		return current, err
	}
	if current.Status == "CLOSED" {
		return current, nil
	}
	response, err := c.request(ctx, http.MethodPut, c.baseURL, path, "application/xml", true, payload)
	if err != nil {
		return current, err
	}
	result, err := decodeTicket(response.Body, number)
	if err != nil {
		return result, err
	}
	if response.StatusCode != 200 || result.Status != "CLOSED" {
		return result, errors.New("ARIN did not confirm full-ticket closure")
	}
	verified, err := c.GetTicket(ctx, number)
	if err != nil {
		return result, err
	}
	if verified.Status != "CLOSED" {
		return result, errors.New("ARIN ticket is not closed after full-ticket update")
	}
	return verified, nil
}
