package arin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const rpkiUpDownNamespace = "http://www.apnic.net/specs/rescerts/up-down/"

var errRPKIUpDown = errors.New("invalid RPKI provisioning message")

type rpkiUpDownClient struct {
	Exchange      rpkiHTTPExchange
	Child, Parent string
}

type rpkiUpDownError struct{ Code int }

func (e *rpkiUpDownError) Error() string {
	return fmt.Sprintf("RPKI provisioning request rejected (status %d)", e.Code)
}

func upDownToken(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' }), " ")
}
func upDownLabel(s string) bool {
	return validRegistrationText(s) && utf8.RuneCountInString(s) > 0 && utf8.RuneCountInString(s) <= 1024
}
func upDownSKI(s string) (string, error) {
	s = upDownToken(s)
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	if err != nil {
		b, err = base64.URLEncoding.Strict().DecodeString(s)
	}
	if err != nil || len(b) != 20 {
		return "", errRPKIUpDown
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (c rpkiUpDownClient) Revoke(ctx context.Context, class, ski string) error {
	child, parent, class := upDownToken(c.Child), upDownToken(c.Parent), upDownToken(class)
	ski, err := upDownSKI(ski)
	if err != nil || !upDownLabel(child) || !upDownLabel(parent) || !upDownLabel(class) || c.Exchange.MediaType != "application/rpki-updown" {
		return errRPKIUpDown
	}
	type key struct {
		Class string `xml:"class_name,attr"`
		SKI   string `xml:"ski,attr"`
	}
	request, err := xml.Marshal(struct {
		XMLName   xml.Name `xml:"http://www.apnic.net/specs/rescerts/up-down/ message"`
		Version   string   `xml:"version,attr"`
		Sender    string   `xml:"sender,attr"`
		Recipient string   `xml:"recipient,attr"`
		Type      string   `xml:"type,attr"`
		Key       key      `xml:"key"`
	}{Version: "1", Sender: child, Recipient: parent, Type: "revoke", Key: key{class, ski}})
	if err != nil {
		return errRPKIUpDown
	}
	exchange := c.Exchange
	scope, _ := json.Marshal([]string{child, parent})
	exchange.PeerScope = string(scope)
	var rejected *rpkiUpDownError
	_, err = exchange.exchange(ctx, "updown-revoke", request, func(_, reply []byte) error {
		var err error
		rejected, err = validateUpDownRevoke(reply, child, parent, class, ski)
		return err
	})
	if err != nil {
		return err
	}
	if rejected != nil {
		return rejected
	}
	return nil
}

func upDownAttrs(n *xmlNode, name string, allowed ...string) (map[string]string, error) {
	if n.Name.Space != rpkiUpDownNamespace || n.Name.Local != name {
		return nil, errRPKIUpDown
	}
	attrs := make(map[string]string)
	for _, a := range n.Attrs {
		if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
			continue
		}
		ok := false
		for _, k := range allowed {
			if a.Name.Space == "" && a.Name.Local == k {
				ok = true
			}
		}
		if _, exists := attrs[a.Name.Local]; !ok || exists {
			return nil, errRPKIUpDown
		}
		attrs[a.Name.Local] = a.Value
	}
	return attrs, nil
}

func validateUpDownRevoke(body []byte, child, parent, class, ski string) (*rpkiUpDownError, error) {
	if len(body) > 4<<20 {
		return nil, errRPKIUpDown
	}
	root, err := parseXML(body)
	if err != nil {
		return nil, errRPKIUpDown
	}
	a, err := upDownAttrs(root, "message", "version", "sender", "recipient", "type")
	if err != nil || a["version"] != "1" || upDownToken(a["sender"]) != parent || upDownToken(a["recipient"]) != child || strings.Trim(root.Text, " \t\r\n") != "" {
		return nil, errRPKIUpDown
	}
	switch a["type"] {
	case "revoke_response":
		if len(root.Children) != 1 {
			return nil, errRPKIUpDown
		}
		k := root.Children[0]
		attrs, err := upDownAttrs(k, "key", "class_name", "ski")
		if err != nil || len(k.Children) != 0 || strings.Trim(k.Text, " \t\r\n") != "" || upDownToken(attrs["class_name"]) != class {
			return nil, errRPKIUpDown
		}
		got, err := upDownSKI(attrs["ski"])
		if err != nil || got != ski {
			return nil, errRPKIUpDown
		}
		return nil, nil
	case "error_response":
		return parseUpDownError(root)
	default:
		return nil, errRPKIUpDown
	}
}

func parseUpDownError(root *xmlNode) (*rpkiUpDownError, error) {
	if len(root.Children) == 0 {
		return nil, errRPKIUpDown
	}
	status := root.Children[0]
	if _, err := upDownAttrs(status, "status"); err != nil || len(status.Children) != 0 {
		return nil, errRPKIUpDown
	}
	code, err := strconv.Atoi(strings.Trim(status.Text, " \t\r\n"))
	if err != nil {
		return nil, errRPKIUpDown
	}
	switch code {
	// These statuses leave the outcome uncertain. Preserve pending state.
	case 1101, 1104:
		return nil, errRPKIUpDown
	case 1102, 1103, 1201, 1202, 1203, 1204, 1301, 1302, 2001:
	default:
		return nil, errRPKIUpDown
	}
	english := false
	for _, n := range root.Children[1:] {
		if n.Name.Space != rpkiUpDownNamespace || n.Name.Local != "description" || len(n.Children) != 0 || utf8.RuneCountInString(n.Text) > 1024 {
			return nil, errRPKIUpDown
		}
		lang := ""
		for _, a := range n.Attrs {
			if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
				continue
			}
			if a.Name.Space != "http://www.w3.org/XML/1998/namespace" || a.Name.Local != "lang" || lang != "" {
				return nil, errRPKIUpDown
			}
			lang = a.Value
		}
		// XML Schema language: alphabetic first segment, then alphanumeric segments.
		parts := strings.Split(lang, "-")
		for i, p := range parts {
			if len(p) < 1 || len(p) > 8 {
				return nil, errRPKIUpDown
			}
			for _, r := range p {
				if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9')) {
					return nil, errRPKIUpDown
				}
			}
		}
		if strings.EqualFold(lang, "en-US") {
			english = true
		}
	}
	if len(root.Children) > 1 && !english {
		return nil, errRPKIUpDown
	}
	return &rpkiUpDownError{Code: code}, nil
}
