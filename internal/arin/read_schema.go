package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ValueKind describes API output independently of Terraform.
type ValueKind string

const (
	StringKind  ValueKind = "string"
	IntKind     ValueKind = "int"
	BoolKind    ValueKind = "bool"
	StringsKind ValueKind = "strings"
	IntsKind    ValueKind = "ints"
	ObjectsKind ValueKind = "objects"
)

type Field struct {
	Name, Path, Description      string
	Kind                         ValueKind
	Fields                       []Field
	Required, Sensitive, Ordered bool
}

type Input struct {
	Name, Kind, Description, Default, Example string
}

// ReadSpec is an explicit allowlist of read-only registration operations.
// No caller-supplied URL or HTTP method is accepted.
type ReadSpec struct {
	Public                                bool
	Name, Description, Root, Item, Output string
	Inputs                                []Input
	Fields                                []Field
	Path                                  func(map[string]string) string
	Collection, Binary, Sensitive         bool
	// Select identifies an individual object using a supported collection endpoint.
	SelectInput, SelectField string
}

type xmlNode struct {
	Name     xml.Name
	Attrs    []xml.Attr
	Text     string
	Children []*xmlNode
}

func parseXML(body []byte) (*xmlNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var stack []*xmlNode
	var root *xmlNode
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("ARIN returned malformed XML")
		}
		switch t := token.(type) {
		case xml.StartElement:
			if len(stack) >= 64 {
				return nil, errors.New("ARIN XML exceeded the nesting limit")
			}
			node := &xmlNode{Name: t.Name, Attrs: t.Attr}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("ARIN returned multiple XML roots")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			} else if strings.TrimSpace(string(t)) != "" {
				return nil, errors.New("ARIN returned unexpected XML text")
			}
		case xml.Directive:
			return nil, errors.New("ARIN returned an unsupported XML directive")
		}
	}
	if root == nil {
		return nil, errors.New("ARIN returned empty XML")
	}
	return root, nil
}

func arinNamespace(ns string) bool {
	return ns == "http://www.arin.net/regrws/core/v1" || ns == "http://www.arin.net/regrws/rpki/v1" || ns == "http://www.arin.net/regrws/messages/v1" || ns == "http://www.arin.net/regrws/shared-ticket/v1" || ns == "http://www.arin.net/regrws/ttl/v1"
}

func nodesAt(node *xmlNode, path string) []*xmlNode {
	nodes := []*xmlNode{node}
	if path == "." || path == "" {
		return nodes
	}
	for _, part := range strings.Split(path, "/") {
		var next []*xmlNode
		for _, n := range nodes {
			if strings.HasPrefix(part, "@") {
				for _, a := range n.Attrs {
					if a.Name.Local == part[1:] && (a.Name.Space == "" || arinNamespace(a.Name.Space)) {
						next = append(next, &xmlNode{Text: a.Value})
					}
				}
			} else {
				for _, child := range n.Children {
					if child.Name.Local == part && arinNamespace(child.Name.Space) {
						next = append(next, child)
					}
				}
			}
		}
		nodes = next
	}
	return nodes
}

func decodeFields(node *xmlNode, fields []Field) (map[string]any, error) {
	values := make(map[string]any, len(fields))
	for _, field := range fields {
		nodes := nodesAt(node, field.Path)
		if field.Ordered {
			// ARIN multiline text is ordered by its numeric line attribute.
			numbered := make(map[*xmlNode]int64)
			seen := make(map[int64]bool)
			for _, n := range nodes {
				attrs := nodesAt(n, "@number")
				if len(attrs) != 1 {
					return nil, fmt.Errorf("ARIN returned an unnumbered line for %s", field.Name)
				}
				number, err := strconv.ParseInt(attrs[0].Text, 10, 64)
				if err != nil || number < 0 || seen[number] {
					return nil, fmt.Errorf("ARIN returned invalid line numbering for %s", field.Name)
				}
				seen[number] = true
				numbered[n] = number
			}
			slices.SortFunc(nodes, func(a, b *xmlNode) int {
				if numbered[a] < numbered[b] {
					return -1
				}
				if numbered[a] > numbered[b] {
					return 1
				}
				return 0
			})
		}
		var value any
		switch field.Kind {
		case StringsKind, IntsKind, ObjectsKind:
			list := make([]any, 0, len(nodes))
			for _, n := range nodes {
				var item any
				var err error
				if field.Kind == ObjectsKind {
					item, err = decodeFields(n, field.Fields)
				} else if field.Kind == IntsKind {
					item, err = parseScalar(n.Text, IntKind)
				} else {
					item = n.Text
				}
				if err != nil {
					return nil, fmt.Errorf("invalid ARIN field %s: %w", field.Name, err)
				}
				list = append(list, item)
			}
			if !field.Ordered {
				slices.SortStableFunc(list, func(a, b any) int { ja, _ := json.Marshal(a); jb, _ := json.Marshal(b); return bytes.Compare(ja, jb) })
			}
			value = list
		default:
			if len(nodes) > 1 {
				return nil, fmt.Errorf("ARIN returned duplicate scalar field %s", field.Name)
			}
			if len(nodes) == 1 {
				var err error
				value, err = parseScalar(nodes[0].Text, field.Kind)
				if err == nil && field.Kind == StringKind && value != "" {
					value, err = normalizeReadString(field.Name, value.(string))
				}
				if err != nil {
					return nil, fmt.Errorf("invalid ARIN field %s: %w", field.Name, err)
				}
			}
			if field.Required && (value == nil || value == "") {
				return nil, fmt.Errorf("ARIN omitted required field %s", field.Name)
			}
		}
		values[field.Name] = value
	}
	return values, nil
}

func parseScalar(text string, kind ValueKind) (any, error) {
	switch kind {
	case StringKind:
		return text, nil
	case IntKind:
		v, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			return nil, errors.New("expected an integer")
		}
		return v, nil
	case BoolKind:
		switch strings.TrimSpace(text) {
		case "true", "1":
			return true, nil
		case "false", "0":
			return false, nil
		}
		return nil, errors.New("expected a boolean")
	default:
		return nil, errors.New("unknown field kind")
	}
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]*$`)
var enumPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func (s ReadSpec) Validate(params map[string]string) error {
	for _, input := range s.Inputs {
		value := params[input.Name]
		if value == "" {
			return fmt.Errorf("%s is required", input.Name)
		}
		valid := false
		switch input.Kind {
		case "handle":
			valid = handlePattern.MatchString(value)
		case "name":
			valid = namePattern.MatchString(value) && !strings.Contains(value, "..")
		case "enum":
			valid = enumPattern.MatchString(value)
		case "bool":
			valid = value == "true" || value == "false"
		case "opaque":
			valid = value != "." && value != ".." && !strings.ContainsAny(value, "/\\?#%") && strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) < 0
		case "id":
			_, err := strconv.ParseUint(value, 10, 64)
			valid = err == nil
		case "asn":
			n, err := strconv.ParseUint(value, 10, 32)
			valid = err == nil && n > 0
		case "cidr":
			prefix, err := netip.ParsePrefix(value)
			valid = err == nil && prefix == prefix.Masked() && !prefix.Addr().Is4In6()
		case "ip_network":
			_, _, err := rdapNetworkQuery(value)
			valid = err == nil
		case "ip":
			addr, err := netip.ParseAddr(value)
			valid = err == nil && !addr.Is4In6() && addr.Zone() == ""
		}
		if !valid {
			return fmt.Errorf("%s is not a valid %s", input.Name, input.Kind)
		}
	}
	if params["start_address"] != "" {
		start, _ := netip.ParseAddr(params["start_address"])
		end, _ := netip.ParseAddr(params["end_address"])
		if start.BitLen() != end.BitLen() || start.Compare(end) > 0 {
			return errors.New("start_address and end_address must form an ordered range in the same address family")
		}
	}
	return nil
}

// ReadRegistration issues exactly one documented read request. It cannot create
// reports, update records, follow response links, or fetch arbitrary URLs.
func (c *Client) ReadRegistration(ctx context.Context, spec ReadSpec, params map[string]string) (map[string]any, error) {
	if err := spec.Validate(params); err != nil {
		return nil, err
	}
	if spec.Public {
		return c.readPublic(ctx, spec, params)
	}
	if spec.Binary {
		response, err := c.fetch(ctx, c.baseURL, spec.Path(params), "application/octet-stream", true)
		if err != nil {
			return nil, err
		}
		var filename any
		if _, parameters, err := mime.ParseMediaType(response.Header.Get("Content-Disposition")); err == nil {
			if name, ok := parameters["filename"]; ok {
				filename = name
			}
		}
		return map[string]any{"content_base64": base64.StdEncoding.EncodeToString(response.Body), "size_bytes": int64(len(response.Body)), "filename": filename, "content_type": response.Header.Get("Content-Type"), "sha256": fmt.Sprintf("%x", sha256.Sum256(response.Body))}, nil
	}
	body, err := c.get(ctx, c.baseURL, spec.Path(params), "application/xml", true)
	if err != nil {
		return nil, err
	}
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if !arinNamespace(root.Name.Space) {
		return nil, errors.New("ARIN returned an unexpected XML namespace")
	}
	var nodes []*xmlNode
	if spec.Collection || spec.SelectInput != "" {
		if root.Name.Local != "collection" {
			return nil, errors.New("ARIN returned an unexpected collection payload")
		}
		for _, child := range root.Children {
			if child.Name.Local != spec.Item || !arinNamespace(child.Name.Space) {
				return nil, errors.New("ARIN returned unexpected collection content; refusing a partial result")
			}
			nodes = append(nodes, child)
		}
	} else {
		if root.Name.Local != spec.Root {
			return nil, errors.New("ARIN returned an unexpected object payload")
		}
		nodes = []*xmlNode{root}
	}
	records := make([]any, 0, len(nodes))
	for _, node := range nodes {
		record, err := decodeFields(node, spec.Fields)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if spec.SelectInput != "" {
		var found map[string]any
		for _, record := range records {
			candidate := record.(map[string]any)
			if fmt.Sprint(candidate[spec.SelectField]) == params[spec.SelectInput] {
				if found != nil {
					return nil, errors.New("ARIN returned duplicate object identities")
				}
				found = candidate
			}
		}
		if found == nil {
			return nil, &APIError{StatusCode: 404, Code: "E_OBJECT_NOT_FOUND", Message: "The requested object was not present in the organization collection"}
		}
		return found, nil
	}
	if spec.Collection {
		slices.SortStableFunc(records, func(a, b any) int { ja, _ := json.Marshal(a); jb, _ := json.Marshal(b); return bytes.Compare(ja, jb) })
		return map[string]any{spec.Output: records}, nil
	}
	return records[0].(map[string]any), nil
}

func segment(params map[string]string, key string) string { return url.PathEscape(params[key]) }

// Reg-RWS permits zero-padded IPv4 octets. Interpret them as decimal, never octal.
func parseRegistrationAddress(raw string) (netip.Addr, error) {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, ".") && !strings.Contains(raw, ":") {
		parts := strings.Split(raw, ".")
		if len(parts) != 4 {
			return netip.Addr{}, errors.New("invalid IPv4 address")
		}
		for i, part := range parts {
			if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
				return netip.Addr{}, errors.New("invalid IPv4 octet")
			}
			n, err := strconv.ParseUint(part, 10, 8)
			if err != nil {
				return netip.Addr{}, errors.New("invalid IPv4 octet")
			}
			parts[i] = strconv.FormatUint(n, 10)
		}
		raw = strings.Join(parts, ".")
	}
	address, err := netip.ParseAddr(raw)
	if err != nil || address.Is4In6() || address.Zone() != "" {
		return netip.Addr{}, errors.New("invalid IP address")
	}
	return address, nil
}
func normalizeReadString(name, value string) (string, error) {
	switch name {
	case "start_address", "end_address":
		addr, err := parseRegistrationAddress(value)
		if err != nil {
			return "", err
		}
		return addr.String(), nil
	case "prefix":
		pieces := strings.Split(strings.TrimSpace(value), "/")
		if len(pieces) != 2 {
			return "", errors.New("invalid CIDR")
		}
		address, err := parseRegistrationAddress(pieces[0])
		if err != nil {
			return "", err
		}
		bits, err := strconv.Atoi(pieces[1])
		if err != nil || bits < 0 || bits > address.BitLen() {
			return "", errors.New("invalid CIDR length")
		}
		prefix := netip.PrefixFrom(address, bits)
		if prefix != prefix.Masked() {
			return "", errors.New("noncanonical network address")
		}
		return prefix.String(), nil
	default:
		return value, nil
	}
}
