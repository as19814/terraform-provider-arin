package arin

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

const messageNamespace = "http://www.arin.net/regrws/messages/v1"

type TicketAttachmentReference struct{ ID, Filename string }

// TicketMessage is an existing server message. Attachments are references and
// must be read through the attachment endpoint, never through response URLs.
type TicketMessage struct {
	ID, CreatedDate, Subject, Category string
	Text                               []string
	Attachments                        []TicketAttachmentReference
}

func validateMessageID(id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n < 1 || strconv.FormatInt(n, 10) != id {
		return errors.New("message ID must be a canonical positive integer")
	}
	return nil
}

func decodeTicketMessage(body []byte, expected string) (*TicketMessage, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "message" || root.Name.Space != registrationNamespace {
		return nil, errors.New("ARIN returned an unexpected message payload")
	}
	ids := nodesAt(root, "messageId")
	if len(ids) != 1 || ids[0].Name.Space != messageNamespace || len(ids[0].Children) != 0 {
		return nil, errors.New("ARIN returned an ambiguous or missing message ID")
	}
	id := strings.TrimSpace(ids[0].Text)
	if validateMessageID(id) != nil || expected != "" && expected != id {
		return nil, errors.New("ARIN returned an invalid or different message ID")
	}
	// Keep this trustworthy identity if decoding later fields fails.
	message := &TicketMessage{ID: id}
	values, err := decodeFields(root, messageFields)
	if err != nil {
		return message, err
	}
	message.CreatedDate = netString(values, "created_date")
	message.Subject = netString(values, "subject")
	message.Category = netString(values, "category")
	if message.Category != "NONE" && message.Category != "JUSTIFICATION" {
		return message, errors.New("ARIN returned an invalid message category")
	}
	if lines, ok := values["text"].([]any); ok {
		for _, line := range lines {
			message.Text = append(message.Text, line.(string))
		}
	}
	if refs, ok := values["attachment_references"].([]any); ok {
		for _, raw := range refs {
			ref := raw.(map[string]any)
			id, filename := netString(ref, "attachment_id"), netString(ref, "filename")
			if id == "" || filename == "" {
				return message, errors.New("ARIN returned an incomplete attachment reference")
			}
			message.Attachments = append(message.Attachments, TicketAttachmentReference{id, filename})
		}
	}
	return message, nil
}

func (c *Client) GetTicketMessage(ctx context.Context, number, id string) (*TicketMessage, error) {
	if err := ValidateTicketNumber(number); err != nil {
		return nil, err
	}
	if err := validateMessageID(id); err != nil {
		return nil, err
	}
	body, err := c.get(ctx, c.baseURL, "/rest/ticket/"+url.PathEscape(number)+"/message/"+id, "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodeTicketMessage(body, id)
}
