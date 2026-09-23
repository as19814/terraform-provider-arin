package arin

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

const RPKISetupNamespace = "http://www.hactrn.net/uris/rpki/rpki-setup/"

// RPKISetup contains out-of-band configuration, not authenticated enrollment.
// URLs and referral CMS tokens are returned without following or trusting them.
type RPKISetup struct {
	Tag                                                    *string
	Type, ChildHandle, ParentHandle, PublisherHandle       string
	ServiceURI, SIABase, RRDPNotificationURI               string
	CertificatePEM, CertificateSHA256, NotBefore, NotAfter string
	Offer                                                  bool
	Referrals                                              []RPKISetupReferral
}
type RPKISetupReferral struct{ Referrer, ContactURI, AuthorizationBase64 string }

var setupHandle = regexp.MustCompile(`^[A-Za-z0-9/_-]*$`)

func setupAttrs(n *xmlNode, required, optional []string) (map[string]string, error) {
	attrs := map[string]string{}
	for _, a := range n.Attrs {
		if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
			continue
		}
		if a.Name.Space != "" || (!slices.Contains(required, a.Name.Local) && !slices.Contains(optional, a.Name.Local)) {
			return nil, errors.New("unexpected setup attribute")
		}
		if _, exists := attrs[a.Name.Local]; exists {
			return nil, errors.New("duplicate setup attribute")
		}
		attrs[a.Name.Local] = a.Value
	}
	for _, name := range required {
		if _, ok := attrs[name]; !ok {
			return nil, fmt.Errorf("missing setup attribute %s", name)
		}
	}
	for name, value := range attrs {
		switch {
		case name == "version":
			if value != "1" {
				return nil, errors.New("unsupported setup version")
			}
		case strings.HasSuffix(name, "handle") || name == "referrer":
			if len(value) > 255 || !setupHandle.MatchString(value) {
				return nil, errors.New("invalid setup handle")
			}
		case name == "tag":
			attrs[name] = strings.Join(strings.FieldsFunc(value, func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' }), " ")
			if len([]rune(attrs[name])) > 1024 {
				return nil, errors.New("setup tag exceeds limit")
			}
		default:
			if len([]rune(value)) > 4096 {
				return nil, errors.New("setup URI exceeds limit")
			}
			u, err := url.Parse(value)
			if err != nil || u.Scheme == "" || strings.ContainsAny(value, "\r\n\t ") {
				return nil, errors.New("invalid setup URI")
			}
			if name == "service_uri" && (u.Host == "" || (u.Scheme != "http" && u.Scheme != "https")) {
				return nil, errors.New("setup service URI must use HTTP or HTTPS")
			}
			if name == "sia_base" && (u.Scheme != "rsync" || u.Host == "") {
				return nil, errors.New("setup SIA base must use rsync")
			}
		}
	}
	return attrs, nil
}
func setupBase64(n *xmlNode) ([]byte, error) {
	if len(n.Children) > 0 {
		return nil, errors.New("nested setup binary content")
	}
	text := strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "").Replace(n.Text)
	if len(text) > 4*((512000+2)/3) {
		return nil, errors.New("setup binary content exceeds limit")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(text)
	if err != nil || len(data) == 0 || len(data) > 512000 {
		return nil, errors.New("invalid setup base64 content")
	}
	return data, nil
}

// ParseRPKISetup parses the four RFC 8183 configuration messages. It checks the
// certificate's CA properties and self-signature but cannot authenticate its
// source. Certificate validity dates are exposed rather than checked against a
// clock, so historical setup documents remain readable.
func ParseRPKISetup(document []byte) (RPKISetup, error) {
	var out RPKISetup
	if len(document) > 4<<20 {
		return out, errors.New("setup XML exceeds 4 MiB")
	}
	root, err := parseXML(document)
	if err != nil {
		return out, errors.New("invalid setup XML")
	}
	if root.Name.Space != RPKISetupNamespace || strings.TrimSpace(root.Text) != "" {
		return out, errors.New("unexpected setup root or text")
	}
	required, optional := []string{"version"}, []string{"tag"}
	certName := ""
	switch root.Name.Local {
	case "child_request":
		required = append(required, "child_handle")
		certName = "child_bpki_ta"
	case "parent_response":
		required = append(required, "child_handle", "parent_handle", "service_uri")
		certName = "parent_bpki_ta"
	case "publisher_request":
		required = append(required, "publisher_handle")
		certName = "publisher_bpki_ta"
	case "repository_response":
		required = append(required, "publisher_handle", "service_uri", "sia_base")
		optional = append(optional, "rrdp_notification_uri")
		certName = "repository_bpki_ta"
	default:
		return out, errors.New("expected child_request, parent_response, publisher_request or repository_response")
	}
	attrs, err := setupAttrs(root, required, optional)
	if err != nil {
		return out, err
	}
	out = RPKISetup{Type: root.Name.Local, ChildHandle: attrs["child_handle"], ParentHandle: attrs["parent_handle"], PublisherHandle: attrs["publisher_handle"], ServiceURI: attrs["service_uri"], SIABase: attrs["sia_base"], RRDPNotificationURI: attrs["rrdp_notification_uri"], Referrals: []RPKISetupReferral{}}
	if tag, ok := attrs["tag"]; ok {
		out.Tag = &tag
	}
	for i, n := range root.Children {
		if n.Name.Space != RPKISetupNamespace {
			return RPKISetup{}, errors.New("unexpected setup child namespace")
		}
		switch {
		case i == 0 && n.Name.Local == certName:
			if _, err = setupAttrs(n, nil, nil); err != nil {
				return RPKISetup{}, err
			}
			der, e := setupBase64(n)
			if e != nil {
				return RPKISetup{}, e
			}
			cert, e := x509.ParseCertificate(der)
			if e != nil || !cert.IsCA || !cert.BasicConstraintsValid || !bytes.Equal(cert.RawIssuer, cert.RawSubject) || cert.CheckSignatureFrom(cert) != nil {
				return RPKISetup{}, errors.New("setup BPKI certificate must be a self-signed CA")
			}
			for _, ext := range cert.Extensions {
				if ext.Id.String() == "1.3.6.1.5.5.7.1.7" || ext.Id.String() == "1.3.6.1.5.5.7.1.8" {
					return RPKISetup{}, errors.New("BPKI certificate must not contain RPKI resource extensions")
				}
			}
			digest := sha256.Sum256(der)
			out.CertificateSHA256 = hex.EncodeToString(digest[:])
			out.CertificatePEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
			out.NotBefore = cert.NotBefore.UTC().Format(time.RFC3339)
			out.NotAfter = cert.NotAfter.UTC().Format(time.RFC3339)
		case n.Name.Local == "offer" && out.Type == "parent_response" && i == 1:
			if _, err = setupAttrs(n, nil, nil); err != nil || len(n.Children) > 0 || strings.TrimSpace(n.Text) != "" {
				return RPKISetup{}, errors.New("invalid publication offer")
			}
			out.Offer = true
		case n.Name.Local == "referral" && (out.Type == "parent_response" || out.Type == "publisher_request"):
			opts := []string{}
			if out.Type == "parent_response" {
				opts = append(opts, "contact_uri")
			}
			a, e := setupAttrs(n, []string{"referrer"}, opts)
			if e != nil {
				return RPKISetup{}, e
			}
			token, e := setupBase64(n)
			if e != nil {
				return RPKISetup{}, e
			}
			out.Referrals = append(out.Referrals, RPKISetupReferral{Referrer: a["referrer"], ContactURI: a["contact_uri"], AuthorizationBase64: base64.StdEncoding.EncodeToString(token)})
		default:
			return RPKISetup{}, errors.New("unexpected or duplicate setup element")
		}
	}
	if out.CertificatePEM == "" {
		return RPKISetup{}, errors.New("missing setup BPKI certificate")
	}
	return out, nil
}
