package arin

import (
	"encoding/base64"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// RegistrationMessage is caller-supplied correspondence. Generated message IDs
// and dates are deliberately absent from the write representation.
type RegistrationMessage struct {
	Subject     string
	Text        []string
	Category    string
	Attachments []RegistrationAttachment
}

type RegistrationAttachment struct {
	Filename string
	Data     []byte
}

type registrationMessagesXML struct {
	Messages []registrationMessageXML `xml:"message"`
}
type registrationMessageXML struct {
	Subject     string                      `xml:"subject,omitempty"`
	Text        *irrLinesXML                `xml:"text,omitempty"`
	Category    string                      `xml:"category"`
	Attachments *registrationAttachmentsXML `xml:"attachments,omitempty"`
}
type registrationAttachmentsXML struct {
	Attachments []registrationAttachmentXML `xml:"attachment"`
}
type registrationAttachmentXML struct {
	Data     string `xml:"data"`
	Filename string `xml:"filename"`
}

// encoding/xml replaces forbidden characters rather than reporting all of them.
// Reject them before submission so correspondence is never silently altered.
func validRegistrationText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if !(r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r <= 0xD7FF || r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF) {
			return false
		}
	}
	return true
}

func registrationMessages(messages []RegistrationMessage) (*registrationMessagesXML, error) {
	if len(messages) == 0 {
		return nil, nil
	}
	out := &registrationMessagesXML{}
	total := 0
	for _, message := range messages {
		if !validRegistrationText(message.Subject) || strings.ContainsAny(message.Subject, "\r\n") {
			return nil, errors.New("message subject must contain valid XML text on one line")
		}
		if strings.TrimSpace(message.Subject) == "" && len(message.Text) == 0 && len(message.Attachments) == 0 {
			return nil, errors.New("message must contain a subject, text or attachment")
		}
		category := message.Category
		if category == "" {
			category = "NONE"
		}
		if category != "NONE" && category != "JUSTIFICATION" {
			return nil, errors.New("message category must be NONE or JUSTIFICATION")
		}
		item := registrationMessageXML{Subject: message.Subject, Text: xmlPolicy(message.Text), Category: category}
		total += len(message.Subject)
		for _, line := range message.Text {
			if !validRegistrationText(line) || strings.ContainsAny(line, "\r\n") {
				return nil, errors.New("message text must contain valid XML text as individual lines")
			}
			total += len(line)
		}
		if len(message.Attachments) != 0 {
			item.Attachments = &registrationAttachmentsXML{}
		}
		for _, attachment := range message.Attachments {
			if strings.TrimSpace(attachment.Filename) == "" || !validRegistrationText(attachment.Filename) || strings.ContainsAny(attachment.Filename, "/\\") || strings.IndexFunc(attachment.Filename, unicode.IsControl) >= 0 || attachment.Filename == "." || attachment.Filename == ".." {
				return nil, errors.New("attachment filename must be a nonempty filename without path separators or controls")
			}
			// Bound raw content before allocating its base64 representation.
			if len(attachment.Data) > maxResponseBytes {
				return nil, errors.New("registration messages exceed the client payload limit")
			}
			total += base64.StdEncoding.EncodedLen(len(attachment.Data)) + len(attachment.Filename)
			if total > maxResponseBytes {
				return nil, errors.New("registration messages exceed the client payload limit")
			}
			item.Attachments.Attachments = append(item.Attachments.Attachments, registrationAttachmentXML{Data: base64.StdEncoding.EncodeToString(attachment.Data), Filename: attachment.Filename})
		}
		if total > maxResponseBytes {
			return nil, errors.New("registration messages exceed the client payload limit")
		}
		out.Messages = append(out.Messages, item)
	}
	return out, nil
}

// ValidateRegistrationMessages validates caller-supplied correspondence without
// making requests. Final encoded NET size is checked when building the request.
func ValidateRegistrationMessages(messages []RegistrationMessage) error {
	_, err := registrationMessages(messages)
	return err
}
