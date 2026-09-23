package arin

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"strings"
	"time"
)

type rpkiIssueRequest struct {
	Class                                      string
	CSRDER                                     []byte
	RequestedASN, RequestedIPv4, RequestedIPv6 *string
}

// Issue performs protocol validation only. Provider resources must use the
// resource-path entry point for certificate and allocation validation.
func (c rpkiUpDownClient) Issue(ctx context.Context, input rpkiIssueRequest) (*rpkiResourceClass, error) {
	return c.issue(ctx, input, nil)
}

func (c rpkiUpDownClient) issue(ctx context.Context, input rpkiIssueRequest, validate func(*rpkiResourceClass, time.Time) error) (*rpkiResourceClass, error) {
	child, parent := upDownToken(c.Child), upDownToken(c.Parent)
	if !upDownLabel(child) || !upDownLabel(parent) || c.Exchange.MediaType != "application/rpki-updown" {
		return nil, errRPKIUpDown
	}
	request, err := buildUpDownIssue(child, parent, input)
	if err != nil {
		return nil, err
	}
	exchange := c.Exchange
	scope, _ := json.Marshal([]string{child, parent})
	exchange.PeerScope = string(scope)
	clock := exchange.Clock
	if clock == nil {
		clock = time.Now
	}
	var result *rpkiResourceClass
	var rejected *rpkiUpDownError
	_, err = exchange.exchange(ctx, "updown-issue", request, func(query, reply []byte) error {
		var err error
		now := clock()
		result, rejected, err = validateUpDownIssue(query, reply, child, parent, now)
		if err == nil && rejected == nil && validate != nil {
			err = validate(result, now)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if rejected != nil {
		return nil, rejected
	}
	return result, nil
}

func buildUpDownIssue(child, parent string, input rpkiIssueRequest) ([]byte, error) {
	input.Class = upDownToken(input.Class)
	if !upDownLabel(input.Class) || len(input.CSRDER) < 4 || len(input.CSRDER) > 512000 {
		return nil, errRPKIUpDown
	}
	_, err := parseRPKICACSR(input.CSRDER)
	if err != nil {
		return nil, errRPKIUpDown
	}
	for _, r := range []struct {
		value  *string
		family int
	}{{input.RequestedASN, 0}, {input.RequestedIPv4, 4}, {input.RequestedIPv6, 6}} {
		if r.value != nil && !upDownResources(*r.value, r.family) {
			return nil, errRPKIUpDown
		}
	}
	type issue struct {
		Class string  `xml:"class_name,attr"`
		ASN   *string `xml:"req_resource_set_as,attr,omitempty"`
		IPv4  *string `xml:"req_resource_set_ipv4,attr,omitempty"`
		IPv6  *string `xml:"req_resource_set_ipv6,attr,omitempty"`
		CSR   string  `xml:",chardata"`
	}
	request, err := xml.Marshal(struct {
		XMLName   xml.Name `xml:"http://www.apnic.net/specs/rescerts/up-down/ message"`
		Version   string   `xml:"version,attr"`
		Sender    string   `xml:"sender,attr"`
		Recipient string   `xml:"recipient,attr"`
		Type      string   `xml:"type,attr"`
		Request   issue    `xml:"request"`
	}{Version: "1", Sender: child, Recipient: parent, Type: "issue", Request: issue{input.Class, input.RequestedASN, input.RequestedIPv4, input.RequestedIPv6, base64.StdEncoding.EncodeToString(input.CSRDER)}})
	if err != nil || len(request) > 4<<20 {
		return nil, errRPKIUpDown
	}
	return request, nil
}

func validateUpDownIssue(query, reply []byte, child, parent string, now time.Time) (*rpkiResourceClass, *rpkiUpDownError, error) {
	if len(query) > 4<<20 || len(reply) > 4<<20 {
		return nil, nil, errRPKIUpDown
	}
	request, err := parseXML(query)
	if err != nil || len(request.Children) != 1 {
		return nil, nil, errRPKIUpDown
	}
	wanted, err := upDownAttrs(request.Children[0], "request", "class_name", "req_resource_set_as", "req_resource_set_ipv4", "req_resource_set_ipv6")
	if err != nil {
		return nil, nil, errRPKIUpDown
	}
	csrDER, err := base64.StdEncoding.DecodeString(request.Children[0].Text)
	if err != nil {
		return nil, nil, errRPKIUpDown
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return nil, nil, errRPKIUpDown
	}
	root, err := parseXML(reply)
	if err != nil {
		return nil, nil, errRPKIUpDown
	}
	a, err := upDownAttrs(root, "message", "version", "sender", "recipient", "type")
	if err != nil || a["version"] != "1" || upDownToken(a["sender"]) != parent || upDownToken(a["recipient"]) != child || strings.Trim(root.Text, " \t\r\n") != "" {
		return nil, nil, errRPKIUpDown
	}
	if a["type"] == "error_response" {
		rejected, err := parseUpDownError(root)
		return nil, rejected, err
	}
	if a["type"] != "issue_response" || len(root.Children) != 1 {
		return nil, nil, errRPKIUpDown
	}
	class, err := parseUpDownClass(root.Children[0])
	if err != nil || class.Name != wanted["class_name"] || len(class.Certificates) != 1 {
		return nil, nil, errRPKIUpDown
	}
	issued := class.Certificates[0]
	for _, r := range []struct {
		name string
		got  *string
	}{{"req_resource_set_as", issued.RequestedASN}, {"req_resource_set_ipv4", issued.RequestedIPv4}, {"req_resource_set_ipv6", issued.RequestedIPv6}} {
		value, present := wanted[r.name]
		if present != (r.got != nil) || (present && value != *r.got) {
			return nil, nil, errRPKIUpDown
		}
	}
	cert, err := x509.ParseCertificate(issued.DER)
	if err != nil {
		return nil, nil, errRPKIUpDown
	}
	requestedSIA, err := rpkiCASIA(csr.Extensions)
	if err != nil {
		return nil, nil, err
	}
	issuedSIA, err := rpkiCASIA(cert.Extensions)
	if err != nil || !bytes.Equal(requestedSIA, issuedSIA) {
		return nil, nil, errRPKIUpDown
	}
	issuer, err := x509.ParseCertificate(class.IssuerDER)
	if err != nil {
		return nil, nil, errRPKIUpDown
	}
	if !bytes.Equal(cert.RawSubjectPublicKeyInfo, csr.RawSubjectPublicKeyInfo) || !bytes.Equal(cert.RawIssuer, issuer.RawSubject) || cert.CheckSignatureFrom(issuer) != nil {
		return nil, nil, errRPKIUpDown
	}
	if len(cert.AuthorityKeyId) > 0 && !bytes.Equal(cert.AuthorityKeyId, issuer.SubjectKeyId) {
		return nil, nil, errRPKIUpDown
	}
	for _, c := range []*x509.Certificate{cert, issuer} {
		if now.Before(c.NotBefore) || now.After(c.NotAfter) {
			return nil, nil, errRPKIUpDown
		}
	}
	// CMS authenticates the parent's assertion; these checks do not establish
	// the issuer's RPKI path or validate RFC 3779 resource containment.
	return &class, nil, nil
}
