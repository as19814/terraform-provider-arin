package arin

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type rpkiResourceClass struct {
	Name, CertificateURLs, ASN, IPv4, IPv6, SuggestedSIA string
	NotAfter                                             time.Time
	IssuerDER                                            []byte
	Certificates                                         []rpkiResourceCertificate
}
type rpkiResourceCertificate struct {
	URLs string
	DER  []byte
	// Nil and an explicitly empty requested resource set have distinct meanings.
	RequestedASN, RequestedIPv4, RequestedIPv6 *string
}

func (c rpkiUpDownClient) List(ctx context.Context) ([]rpkiResourceClass, error) {
	child, parent := upDownToken(c.Child), upDownToken(c.Parent)
	if !upDownLabel(child) || !upDownLabel(parent) || c.Exchange.MediaType != "application/rpki-updown" {
		return nil, errRPKIUpDown
	}
	request, err := xml.Marshal(struct {
		XMLName   xml.Name `xml:"http://www.apnic.net/specs/rescerts/up-down/ message"`
		Version   string   `xml:"version,attr"`
		Sender    string   `xml:"sender,attr"`
		Recipient string   `xml:"recipient,attr"`
		Type      string   `xml:"type,attr"`
	}{Version: "1", Sender: child, Recipient: parent, Type: "list"})
	if err != nil {
		return nil, errRPKIUpDown
	}
	exchange := c.Exchange
	scope, _ := json.Marshal([]string{child, parent})
	exchange.PeerScope = string(scope)
	var classes []rpkiResourceClass
	var rejected *rpkiUpDownError
	_, err = exchange.exchange(ctx, "updown-list", request, func(_, reply []byte) error {
		var err error
		classes, rejected, err = parseUpDownList(reply, child, parent)
		return err
	})
	if err != nil {
		return nil, err
	}
	if rejected != nil {
		return nil, rejected
	}
	return classes, nil
}

func parseUpDownList(body []byte, child, parent string) ([]rpkiResourceClass, *rpkiUpDownError, error) {
	if len(body) > 4<<20 {
		return nil, nil, errRPKIUpDown
	}
	root, err := parseXML(body)
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
	if a["type"] != "list_response" {
		return nil, nil, errRPKIUpDown
	}
	classes := make([]rpkiResourceClass, 0, len(root.Children))
	seen := make(map[string]bool)
	for _, n := range root.Children {
		class, err := parseUpDownClass(n)
		if err != nil || seen[class.Name] {
			return nil, nil, errRPKIUpDown
		}
		seen[class.Name] = true
		classes = append(classes, class)
	}
	return classes, nil, nil
}

func parseUpDownClass(n *xmlNode) (rpkiResourceClass, error) {
	var out rpkiResourceClass
	a, err := upDownAttrs(n, "class", "class_name", "cert_url", "resource_set_as", "resource_set_ipv4", "resource_set_ipv6", "resource_set_notafter", "suggested_sia_head")
	if err != nil || strings.Trim(n.Text, " \t\r\n") != "" {
		return out, errRPKIUpDown
	}
	out.Name = upDownToken(a["class_name"])
	if !upDownLabel(out.Name) || !upDownCertificateURLs(a["cert_url"]) {
		return out, errRPKIUpDown
	}
	for _, r := range []struct {
		name   string
		family int
	}{{"resource_set_as", 0}, {"resource_set_ipv4", 4}, {"resource_set_ipv6", 6}} {
		value, ok := a[r.name]
		if !ok || !upDownResources(value, r.family) {
			return out, errRPKIUpDown
		}
	}
	out.NotAfter, err = time.Parse("2006-01-02T15:04:05Z", a["resource_set_notafter"])
	if err != nil || out.NotAfter.Format("2006-01-02T15:04:05Z") != a["resource_set_notafter"] {
		return out, errRPKIUpDown
	}
	out.CertificateURLs, out.ASN, out.IPv4, out.IPv6 = a["cert_url"], a["resource_set_as"], a["resource_set_ipv4"], a["resource_set_ipv6"]
	if sia, ok := a["suggested_sia_head"]; ok {
		if len(sia) > 1024 || !publicationURI(sia) || !strings.HasSuffix(sia, "/") {
			return out, errRPKIUpDown
		}
		out.SuggestedSIA = sia
	}
	if len(n.Children) == 0 {
		return out, errRPKIUpDown
	}
	out.Certificates = make([]rpkiResourceCertificate, 0, len(n.Children)-1)
	for i, c := range n.Children {
		if i == len(n.Children)-1 {
			if _, err := upDownAttrs(c, "issuer"); err != nil {
				return out, errRPKIUpDown
			}
			out.IssuerDER, err = upDownCertificateDER(c)
			if err != nil {
				return out, err
			}
			break
		}
		attrs, err := upDownAttrs(c, "certificate", "cert_url", "req_resource_set_as", "req_resource_set_ipv4", "req_resource_set_ipv6")
		if err != nil || !upDownCertificateURLs(attrs["cert_url"]) {
			return out, errRPKIUpDown
		}
		cert := rpkiResourceCertificate{URLs: attrs["cert_url"]}
		cert.DER, err = upDownCertificateDER(c)
		if err != nil {
			return out, err
		}
		for _, r := range []struct {
			name   string
			family int
			dest   **string
		}{{"req_resource_set_as", 0, &cert.RequestedASN}, {"req_resource_set_ipv4", 4, &cert.RequestedIPv4}, {"req_resource_set_ipv6", 6, &cert.RequestedIPv6}} {
			if value, ok := attrs[r.name]; ok {
				if !upDownResources(value, r.family) {
					return out, errRPKIUpDown
				}
				*r.dest = &value
			}
		}
		out.Certificates = append(out.Certificates, cert)
	}
	return out, nil
}

func upDownCertificateDER(n *xmlNode) ([]byte, error) {
	if len(n.Children) != 0 {
		return nil, errRPKIUpDown
	}
	s := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, n.Text)
	if len(s) > base64.StdEncoding.EncodedLen(512000) {
		return nil, errRPKIUpDown
	}
	der, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil || len(der) < 4 || len(der) > 512000 {
		return nil, errRPKIUpDown
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil || !cert.IsCA || !cert.BasicConstraintsValid {
		return nil, errRPKIUpDown
	}
	// Parsing does not establish an RPKI validation path or resource entitlement.
	return der, nil
}

func upDownCertificateURLs(s string) bool {
	if len(s) < 10 || len(s) > 4096 {
		return false
	}
	rsync := false
	for _, part := range strings.Split(s, ",") {
		u, err := url.Parse(part)
		if err != nil || u.Scheme == "" || u.Hostname() == "" || u.User != nil || strings.ContainsAny(part, " \t\r\n") {
			return false
		}
		if u.Scheme == "rsync" {
			if !publicationURI(part) {
				return false
			}
			rsync = true
		}
	}
	return rsync
}

func upDownResources(s string, family int) bool {
	if len(s) > 512000 {
		return false
	}
	if s == "" {
		return true
	}
	var previousAS uint64
	var previousIP netip.Addr
	for i, item := range strings.Split(s, ",") {
		if family == 0 {
			parts := strings.Split(item, "-")
			if len(parts) > 2 {
				return false
			}
			values := make([]uint64, len(parts))
			for j, p := range parts {
				v, err := strconv.ParseUint(p, 10, 32)
				if err != nil || strconv.FormatUint(v, 10) != p {
					return false
				}
				values[j] = v
			}
			low, high := values[0], values[len(values)-1]
			if low > high || (i > 0 && low <= previousAS) {
				return false
			}
			previousAS = high
			continue
		}
		var low, high netip.Addr
		if strings.Contains(item, "/") {
			p, err := netip.ParsePrefix(item)
			if err != nil || p != p.Masked() {
				return false
			}
			low = p.Addr()
			raw := low.As16()
			bits := p.Bits()
			if low.Is4() {
				bits += 96
			}
			for bit := bits; bit < 128; bit++ {
				raw[bit/8] |= 1 << uint(7-bit%8)
			}
			high = netip.AddrFrom16(raw)
			if low.Is4() {
				high = high.Unmap()
			}
		} else {
			parts := strings.Split(item, "-")
			if len(parts) != 2 {
				return false
			}
			var err error
			low, err = netip.ParseAddr(parts[0])
			if err != nil {
				return false
			}
			high, err = netip.ParseAddr(parts[1])
			if err != nil {
				return false
			}
		}
		if low.Zone() != "" || high.Zone() != "" || low.Is4In6() || high.Is4In6() || low.Is4() != (family == 4) || high.Is4() != (family == 4) || low.Compare(high) > 0 || (previousIP.IsValid() && low.Compare(previousIP) <= 0) {
			return false
		}
		previousIP = high
	}
	return true
}
