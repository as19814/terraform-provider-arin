package arin

import (
	"context"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

const rpkiPublicationNamespace = "http://www.hactrn.net/uris/rpki/publication-spec/"
const rpkiPublicationListQuery = `<msg xmlns="http://www.hactrn.net/uris/rpki/publication-spec/" version="4" type="query"><list/></msg>`

var errRPKIPublicationReply = errors.New("invalid RPKI publication reply")

type rpkiPublicationObject struct{ URI, SHA256 string }

// Remote diagnostic text and echoed PDUs are deliberately excluded from errors.
type rpkiPublicationError struct {
	Codes []string
	// OperationIndexes aligns with Codes for batches; -1 identifies a generic error.
	OperationIndexes []int
}

func (e *rpkiPublicationError) Error() string {
	return "RPKI publication server rejected request: " + strings.Join(e.Codes, ", ")
}

type rpkiPublicationClient struct{ Exchange rpkiHTTPExchange }

func (c rpkiPublicationClient) List(ctx context.Context) ([]rpkiPublicationObject, error) {
	if c.Exchange.MediaType != "application/rpki-publication" {
		return nil, errors.New("publication client requires publication media type")
	}
	var objects []rpkiPublicationObject
	var rejected *rpkiPublicationError
	_, err := c.Exchange.exchange(ctx, "publication-list", []byte(rpkiPublicationListQuery), func(_, reply []byte) error {
		var err error
		objects, rejected, err = parseRPKIPublicationList(reply)
		return err
	})
	if err != nil {
		return nil, err
	}
	// An authenticated, valid protocol rejection completes the journal too.
	if rejected != nil {
		return nil, rejected
	}
	return objects, nil
}

func publicationAttrs(n *xmlNode, name string, allowed ...string) (map[string]string, error) {
	if n.Name.Space != rpkiPublicationNamespace || n.Name.Local != name {
		return nil, errRPKIPublicationReply
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
				break
			}
		}
		if _, duplicate := attrs[a.Name.Local]; !ok || duplicate {
			return nil, errRPKIPublicationReply
		}
		attrs[a.Name.Local] = a.Value
	}
	return attrs, nil
}

func publicationEmpty(n *xmlNode, name string) bool {
	_, err := publicationAttrs(n, name)
	return err == nil && len(n.Children) == 0 && strings.Trim(n.Text, " \t\r\n") == ""
}

func publicationURI(s string) bool {
	if utf8.RuneCountInString(s) > 4096 || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	for _, r := range s {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "rsync" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && !strings.Contains(s, "#") && u.Opaque == "" && strings.HasPrefix(u.Path, "/") && len(u.Path) > 1
}

func parseRPKIPublicationList(body []byte) ([]rpkiPublicationObject, *rpkiPublicationError, error) {
	if len(body) > 4<<20 {
		return nil, nil, errRPKIPublicationReply
	}
	root, err := parseXML(body)
	if err != nil {
		return nil, nil, errRPKIPublicationReply
	}
	attrs, err := publicationAttrs(root, "msg", "type", "version")
	if err != nil || attrs["type"] != "reply" || attrs["version"] != "4" || strings.Trim(root.Text, " \t\r\n") != "" {
		return nil, nil, errRPKIPublicationReply
	}
	objects := make([]rpkiPublicationObject, 0, len(root.Children))
	seen := make(map[string]bool)
	var codes []string
	for _, child := range root.Children {
		switch child.Name.Local {
		case "list":
			a, err := publicationAttrs(child, "list", "uri", "hash")
			if err != nil || len(codes) != 0 || len(child.Children) != 0 || strings.Trim(child.Text, " \t\r\n") != "" || !publicationURI(a["uri"]) || len(a["hash"]) != 64 || seen[a["uri"]] {
				return nil, nil, errRPKIPublicationReply
			}
			if _, err := hex.DecodeString(a["hash"]); err != nil {
				return nil, nil, errRPKIPublicationReply
			}
			seen[a["uri"]] = true
			objects = append(objects, rpkiPublicationObject{URI: a["uri"], SHA256: strings.ToLower(a["hash"])})
		case "report_error":
			code, err := publicationListError(child)
			if err != nil || len(objects) != 0 {
				return nil, nil, errRPKIPublicationReply
			}
			codes = append(codes, code)
		default:
			return nil, nil, errRPKIPublicationReply
		}
	}
	if len(codes) != 0 {
		return nil, &rpkiPublicationError{Codes: codes}, nil
	}
	return objects, nil, nil
}

func publicationListError(n *xmlNode) (string, error) {
	return publicationError(n, func(tag string, failed *xmlNode) bool {
		return strings.Trim(tag, " \t\r\n") == "" && (failed == nil || publicationEmpty(failed, "list"))
	})
}

func publicationError(n *xmlNode, matches func(string, *xmlNode) bool) (string, error) {
	a, err := publicationAttrs(n, "report_error", "error_code", "tag")
	if err != nil || strings.Trim(n.Text, " \t\r\n") != "" {
		return "", errRPKIPublicationReply
	}
	if !matches(a["tag"], nil) {
		return "", errRPKIPublicationReply
	}
	switch a["error_code"] {
	case "xml_error", "permission_failure", "bad_cms_signature", "object_already_present", "no_object_present", "no_object_matching_hash", "consistency_problem", "other_error":
	default:
		return "", errRPKIPublicationReply
	}
	textSeen, failedSeen := false, false
	for _, c := range n.Children {
		if _, err := publicationAttrs(c, c.Name.Local); err != nil {
			return "", errRPKIPublicationReply
		}
		switch c.Name.Local {
		case "error_text":
			if textSeen || failedSeen || len(c.Children) != 0 || utf8.RuneCountInString(c.Text) > 512000 {
				return "", errRPKIPublicationReply
			}
			textSeen = true
		case "failed_pdu":
			if failedSeen || strings.Trim(c.Text, " \t\r\n") != "" || len(c.Children) != 1 || !matches(a["tag"], c.Children[0]) {
				return "", errRPKIPublicationReply
			}
			failedSeen = true
		default:
			return "", errRPKIPublicationReply
		}
	}
	return a["error_code"], nil
}
