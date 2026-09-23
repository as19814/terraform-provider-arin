package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"slices"
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

// TicketMessageSubmission preserves an attempted append even when its response
// is lost. A nil Message means the caller must reconcile the ticket's messages;
// it is not permission to repeat POST. Confirmed requires a successful fresh GET.
type TicketMessageSubmission struct {
	TicketNumber string
	Message      *TicketMessage
	Confirmed    bool
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

// AddTicketMessage appends once to an existing non-closed ticket. It does not
// retry, poll, follow attachment links, or infer identity from matching text.
// Callers must persist submission intent before invoking this operation.
func (c *Client) AddTicketMessage(ctx context.Context, number string, message RegistrationMessage) (*TicketMessageSubmission, error) {
	if err := ValidateTicketNumber(number); err != nil {
		return nil, err
	}
	messages, err := registrationMessages([]RegistrationMessage{message})
	if err != nil {
		return nil, err
	}
	payload := struct {
		XMLName xml.Name `xml:"http://www.arin.net/regrws/core/v1 message"`
		registrationMessageXML
	}{registrationMessageXML: messages.Messages[0]}
	body, err := xml.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("message exceeds the client payload limit")
	}
	ticket, err := c.GetTicket(ctx, number)
	if err != nil {
		return nil, err
	}
	if ticket.Status == "CLOSED" {
		return nil, errors.New("messages cannot be added to a closed ticket")
	}
	receipt := &TicketMessageSubmission{TicketNumber: number}
	response, err := c.request(ctx, http.MethodPost, c.baseURL, "/rest/ticket/"+url.PathEscape(number)+"/message", "application/xml", true, body)
	if err != nil {
		return receipt, err
	}
	receipt.Message, err = decodeTicketMessage(response.Body, "")
	if err != nil {
		return receipt, err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return receipt, errors.New("ARIN did not confirm message submission")
	}
	verified, err := c.GetTicketMessage(ctx, number, receipt.Message.ID)
	if err != nil {
		return receipt, err
	}
	receipt.Message = verified
	category := message.Category
	if category == "" {
		category = "NONE"
	}
	if verified.Subject != message.Subject || verified.Category != category || !slices.Equal(verified.Text, message.Text) {
		return receipt, errors.New("ARIN message content differs from the submitted message")
	}
	filenames := make([]string, 0, len(message.Attachments))
	actualFilenames := make([]string, 0, len(verified.Attachments))
	for _, attachment := range message.Attachments {
		filenames = append(filenames, attachment.Filename)
	}
	for _, attachment := range verified.Attachments {
		actualFilenames = append(actualFilenames, attachment.Filename)
	}
	slices.Sort(filenames)
	slices.Sort(actualFilenames)
	if !slices.Equal(filenames, actualFilenames) {
		return receipt, errors.New("ARIN message attachment references differ from the submission")
	}
	receipt.Confirmed = true
	return receipt, nil
}
