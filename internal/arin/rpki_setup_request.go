package arin

import (
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"strings"
)

// RPKISetupRequest generates local enrollment input. It never sends the request
// or creates a signing identity. The caller retains the corresponding CA key.
type RPKISetupRequest struct {
	Type, Handle, CertificatePEM string
	Tag                          *string
	Referrals                    []RPKISetupRequestReferral
}
type RPKISetupRequestReferral struct {
	Referrer            string `xml:"referrer,attr"`
	AuthorizationBase64 string `xml:",chardata"`
}
type setupRequestCertificateXML struct {
	XMLName xml.Name
	Text    string `xml:",chardata"`
}
type setupRequestXML struct {
	XMLName     xml.Name
	Namespace   string     `xml:"xmlns,attr"`
	Attrs       []xml.Attr `xml:",any,attr"`
	Certificate setupRequestCertificateXML
	Referrals   []RPKISetupRequestReferral `xml:"referral"`
}

func BuildRPKISetupRequest(req RPKISetupRequest) ([]byte, error) {
	var handleName, certName string
	switch req.Type {
	case "child_request":
		handleName = "child_handle"
		certName = "child_bpki_ta"
	case "publisher_request":
		handleName = "publisher_handle"
		certName = "publisher_bpki_ta"
	default:
		return nil, errors.New("setup request type must be child_request or publisher_request")
	}
	if req.Type == "child_request" && len(req.Referrals) > 0 {
		return nil, errors.New("child setup requests cannot contain publication referrals")
	}
	attrs := []xml.Attr{{Name: xml.Name{Local: "version"}, Value: "1"}, {Name: xml.Name{Local: handleName}, Value: req.Handle}}
	if req.Tag != nil {
		if len(*req.Tag) > 4<<20 {
			return nil, errors.New("setup tag input exceeds limit")
		}
		if !validRegistrationText(*req.Tag) {
			return nil, errors.New("setup tag contains invalid XML text")
		}
		attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "tag"}, Value: *req.Tag})
	}
	normalized, err := setupAttrs(&xmlNode{Attrs: attrs}, []string{"version", handleName}, []string{"tag"})
	if err != nil {
		return nil, err
	}
	for i := range attrs {
		attrs[i].Value = normalized[attrs[i].Name.Local]
	}
	if len(req.CertificatePEM) > 4<<20 {
		return nil, errors.New("setup certificate input exceeds limit")
	}
	input := strings.TrimSpace(req.CertificatePEM)
	if !strings.HasPrefix(input, "-----BEGIN CERTIFICATE-----\n") && !strings.HasPrefix(input, "-----BEGIN CERTIFICATE-----\r\n") {
		return nil, errors.New("expected a single PEM certificate")
	}
	if strings.Count(input, "-----BEGIN ") != 1 || strings.Count(input, "-----END ") != 1 {
		return nil, errors.New("expected exactly one PEM block")
	}
	block, rest := pem.Decode([]byte(input))
	if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) > 0 || len(strings.TrimSpace(string(rest))) > 0 {
		return nil, errors.New("expected a single PEM certificate without headers or extra content")
	}
	if len(block.Bytes) > 512000 {
		return nil, errors.New("setup certificate exceeds binary limit")
	}
	out := setupRequestXML{XMLName: xml.Name{Local: req.Type}, Namespace: RPKISetupNamespace, Attrs: attrs, Certificate: setupRequestCertificateXML{XMLName: xml.Name{Local: certName}, Text: base64.StdEncoding.EncodeToString(block.Bytes)}}
	size := len(out.Certificate.Text) + len(req.Handle)
	for _, ref := range req.Referrals {
		if _, err := setupAttrs(&xmlNode{Attrs: []xml.Attr{{Name: xml.Name{Local: "referrer"}, Value: ref.Referrer}}}, []string{"referrer"}, nil); err != nil {
			return nil, err
		}
		if len(ref.AuthorizationBase64) > 4<<20 {
			return nil, errors.New("setup referral input exceeds limit")
		}
		token, err := setupBase64(&xmlNode{Text: ref.AuthorizationBase64})
		if err != nil {
			return nil, err
		}
		item := RPKISetupRequestReferral{Referrer: ref.Referrer, AuthorizationBase64: base64.StdEncoding.EncodeToString(token)}
		size += len(item.AuthorizationBase64) + len(item.Referrer) + 64
		if size > 4<<20 {
			return nil, errors.New("setup request exceeds 4 MiB")
		}
		out.Referrals = append(out.Referrals, item)
	}
	document, err := xml.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, errors.New("could not encode setup request")
	}
	document = append([]byte(xml.Header), append(document, '\n')...)
	// The reader enforces certificate CA properties, self-signature and protocol
	// structure on generated requests just as it does on supplied documents.
	if _, err = ParseRPKISetup(document); err != nil {
		return nil, err
	}
	return document, nil
}
