package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

const registrationNamespace = "http://www.arin.net/regrws/core/v1"

// RegisteredNet is an authenticated NET record, separate from public RDAP discovery.
type RegisteredNet struct {
	Handle, Name, ParentNetHandle, OrgHandle, CustomerHandle, RegistrationDate string
	Version                                                                    int
	Blocks                                                                     []RegisteredNetBlock
	Comments, OriginASNs                                                       []string
	POCs                                                                       []NetPOC
}
type RegisteredNetBlock struct {
	Type         string `xml:"type"`
	Description  string `xml:"description,omitempty"`
	StartAddress string `xml:"startAddress"`
	EndAddress   string `xml:"endAddress"`
	CIDRLength   int    `xml:"cidrLength"`
}
type NetPOC struct {
	Handle      string `xml:"handle,attr"`
	Function    string `xml:"function,attr"`
	Description string `xml:"description,attr,omitempty"`
}
type registeredNetXML struct {
	XMLName          xml.Name             `xml:"http://www.arin.net/regrws/core/v1 net"`
	Version          int                  `xml:"version"`
	Comments         *irrLinesXML         `xml:"comment,omitempty"`
	RegistrationDate string               `xml:"registrationDate,omitempty"`
	OrgHandle        string               `xml:"orgHandle,omitempty"`
	Handle           string               `xml:"handle,omitempty"`
	Blocks           []RegisteredNetBlock `xml:"netBlocks>netBlock"`
	CustomerHandle   string               `xml:"customerHandle,omitempty"`
	ParentNetHandle  string               `xml:"parentNetHandle"`
	Name             string               `xml:"netName"`
	OriginASNs       []string             `xml:"originASes>originAS"`
	POCs             []NetPOC             `xml:"pocLinks>pocLinkRef"`
}

func (n RegisteredNet) marshal() ([]byte, error) {
	origins := make([]string, len(n.OriginASNs))
	for i, asn := range n.OriginASNs {
		origins[i] = strings.TrimPrefix(asn, "AS")
	}
	return xml.Marshal(registeredNetXML{Version: n.Version, Comments: xmlPolicy(n.Comments), RegistrationDate: n.RegistrationDate, OrgHandle: n.OrgHandle, Handle: n.Handle, Blocks: n.Blocks, CustomerHandle: n.CustomerHandle, ParentNetHandle: n.ParentNetHandle, Name: n.Name, OriginASNs: origins, POCs: n.POCs})
}

// NetAssignment creates a reassignment (customer or org) or reallocation (org).
// Multiple CIDR blocks must form the minimal cover of one contiguous range.
type NetAssignment struct {
	ParentNetHandle, Name, CustomerHandle, OrgHandle string
	Reallocate                                       bool
	Prefixes, Comments                               []string
	// OriginASNs is retained for explicit validation of retired input. Nonempty
	// values are rejected because ARIN no longer stores the NET Origin AS field.
	OriginASNs []string
}

var netNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 -]*$`)

func validateNetMetadata(name string, comments, origins []string) error {
	if len(origins) != 0 {
		return errors.New("NET Origin AS was retired by ARIN in July 2025; use IRR routes or RPKI instead")
	}
	if !netNamePattern.MatchString(name) {
		return errors.New("network name must contain only letters, digits, spaces and hyphens")
	}
	for _, line := range comments {
		if strings.TrimSpace(line) == "" || strings.ContainsAny(line, "\r\n") {
			return errors.New("comments must contain nonempty individual lines")
		}
	}
	return nil
}
func (a NetAssignment) Validate() error {
	if !handlePattern.MatchString(a.ParentNetHandle) {
		return errors.New("invalid parent network handle")
	}
	if (a.CustomerHandle == "") == (a.OrgHandle == "") {
		return errors.New("exactly one of customer_handle and org_handle is required")
	}
	recipient := a.CustomerHandle
	if recipient == "" {
		recipient = a.OrgHandle
	}
	if !handlePattern.MatchString(recipient) {
		return errors.New("invalid recipient handle")
	}
	if a.Reallocate && a.CustomerHandle != "" {
		return errors.New("reallocations require an organization recipient")
	}
	if err := validateNetMetadata(a.Name, a.Comments, a.OriginASNs); err != nil {
		return err
	}
	if len(a.Prefixes) == 0 {
		return errors.New("at least one network prefix is required")
	}
	var prefixes []netip.Prefix
	for _, raw := range a.Prefixes {
		p, err := netip.ParsePrefix(raw)
		if err != nil || p != p.Masked() || p.String() != raw || p.Addr().Is4In6() {
			return errors.New("prefixes must be canonical IPv4 or IPv6 CIDRs without host bits")
		}
		if p.Addr().Is6() && p.Bits() > 64 {
			return errors.New("IPv6 assignments must be at least /64")
		}
		for _, other := range prefixes {
			if p.Addr().BitLen() != other.Addr().BitLen() {
				return errors.New("all prefixes must have the same address family")
			}
			if p.Overlaps(other) {
				return errors.New("network prefixes must not overlap")
			}
		}
		prefixes = append(prefixes, p)
	}
	slices.SortFunc(prefixes, func(a, b netip.Prefix) int { return a.Addr().Compare(b.Addr()) })
	for i := 1; i < len(prefixes); i++ {
		prior, current := prefixes[i-1], prefixes[i]
		if prefixEnd(prior).Next() != current.Addr() {
			return errors.New("NET prefixes must describe one contiguous address range")
		}
		if prior.Bits() == current.Bits() && prior.Bits() > 0 &&
			netip.PrefixFrom(prior.Addr(), prior.Bits()-1).Masked() == netip.PrefixFrom(current.Addr(), current.Bits()-1).Masked() {
			return errors.New("merge adjacent sibling prefixes into their canonical parent CIDR")
		}
	}
	return nil
}
func prefixEnd(p netip.Prefix) netip.Addr {
	raw := p.Masked().Addr().AsSlice()
	for bit := p.Bits(); bit < len(raw)*8; bit++ {
		raw[bit/8] |= byte(1 << uint(7-bit%8))
	}
	address, _ := netip.AddrFromSlice(raw)
	return address
}
func (a NetAssignment) net() RegisteredNet {
	n := RegisteredNet{Name: a.Name, ParentNetHandle: a.ParentNetHandle, OrgHandle: a.OrgHandle, CustomerHandle: a.CustomerHandle, Comments: a.Comments, OriginASNs: a.OriginASNs, Version: 4}
	kind := "S"
	if a.Reallocate {
		kind = "A"
	}
	for _, raw := range a.Prefixes {
		p := netip.MustParsePrefix(raw)
		if p.Addr().Is6() {
			n.Version = 6
		}
		n.Blocks = append(n.Blocks, RegisteredNetBlock{Type: kind, StartAddress: p.Addr().String(), EndAddress: prefixEnd(p).String(), CIDRLength: p.Bits()})
	}
	return n
}
func validateNetTree(root *xmlNode) error {
	allowed := map[string]bool{}
	for _, p := range []string{"version", "comment", "comment/line", "registrationDate", "orgHandle", "handle", "netBlocks", "netBlocks/netBlock", "netBlocks/netBlock/type", "netBlocks/netBlock/description", "netBlocks/netBlock/startAddress", "netBlocks/netBlock/endAddress", "netBlocks/netBlock/cidrLength", "customerHandle", "parentNetHandle", "netName", "originASes", "originASes/originAS", "pocLinks", "pocLinks/pocLinkRef"} {
		allowed[p] = true
	}
	var walk func(*xmlNode, string) error
	walk = func(node *xmlNode, path string) error {
		counts := map[string]int{}
		for _, attr := range node.Attrs {
			if attr.Name.Space == "xmlns" || (attr.Name.Space == "" && attr.Name.Local == "xmlns") {
				continue
			}
			if attr.Name.Space == "" && ((path == "comment/line" && attr.Name.Local == "number") || (path == "pocLinks/pocLinkRef" && slices.Contains([]string{"handle", "function", "description"}, attr.Name.Local))) {
				continue
			}
			return errors.New("ARIN returned unsupported NET attributes; refusing a partial representation")
		}
		for _, child := range node.Children {
			p := child.Name.Local
			if path != "" {
				p = path + "/" + p
			}
			if child.Name.Space != registrationNamespace || !allowed[p] {
				return errors.New("ARIN returned unsupported NET fields; refusing a partial representation")
			}
			counts[p]++
			if counts[p] > 1 && p != "comment/line" && p != "netBlocks/netBlock" && p != "originASes/originAS" && p != "pocLinks/pocLinkRef" {
				return errors.New("ARIN returned duplicate NET fields")
			}
			if err := walk(child, p); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root, "")
}
func decodeRegisteredNetNode(root *xmlNode, handle string) (*RegisteredNet, error) {
	if root.Name.Local != "net" || root.Name.Space != registrationNamespace {
		return nil, errors.New("ARIN returned an unexpected NET payload")
	}
	if err := validateNetTree(root); err != nil {
		return nil, err
	}
	v, err := decodeFields(root, netFields)
	if err != nil {
		return nil, err
	}
	str := func(k string) string { s, _ := v[k].(string); return s }
	n := &RegisteredNet{Handle: str("handle"), Name: str("name"), ParentNetHandle: str("parent_net_handle"), OrgHandle: str("org_handle"), CustomerHandle: str("customer_handle"), RegistrationDate: str("registration_date")}
	version, _ := v["ip_version"].(int64)
	n.Version = int(version)
	if !handlePattern.MatchString(n.Handle) || (handle != "" && n.Handle != handle) || n.Name == "" || n.RegistrationDate == "" || (n.Version != 4 && n.Version != 6) || (n.OrgHandle == "") == (n.CustomerHandle == "") {
		return nil, errors.New("ARIN returned an incomplete or mismatched NET")
	}
	for _, k := range []string{"comments", "origin_asns"} {
		for _, item := range v[k].([]any) {
			if k == "comments" {
				n.Comments = append(n.Comments, item.(string))
			} else {
				// NET responses use numeric origins even when the write used ASn.
				asn := "AS" + strings.TrimPrefix(item.(string), "AS")
				if err := ValidateIRRRouteID("192.0.2.0/24," + asn); err != nil {
					return nil, errors.New("ARIN returned an invalid NET origin ASN")
				}
				n.OriginASNs = append(n.OriginASNs, asn)
			}
		}
	}
	for _, item := range v["net_blocks"].([]any) {
		b := item.(map[string]any)
		number, ok := b["cidr_length"].(int64)
		if !ok {
			return nil, errors.New("missing NET block CIDR length")
		}
		block := RegisteredNetBlock{Type: netString(b, "type"), Description: netString(b, "description"), StartAddress: netString(b, "start_address"), EndAddress: netString(b, "end_address"), CIDRLength: int(number)}
		start, e1 := netip.ParseAddr(block.StartAddress)
		end, e2 := netip.ParseAddr(block.EndAddress)
		p := netip.PrefixFrom(start, block.CIDRLength)
		if e1 != nil || e2 != nil || !p.IsValid() || p != p.Masked() || end != prefixEnd(p) || (start.Is4() != (n.Version == 4)) || block.Type == "" {
			return nil, errors.New("ARIN returned an invalid NET block")
		}
		n.Blocks = append(n.Blocks, block)
	}
	if len(n.Blocks) == 0 {
		return nil, errors.New("ARIN returned no NET blocks")
	}
	for _, item := range v["poc_links"].([]any) {
		p := item.(map[string]any)
		n.POCs = append(n.POCs, NetPOC{Handle: netString(p, "handle"), Function: netString(p, "function"), Description: netString(p, "description")})
	}
	return n, nil
}
func decodeRegisteredNet(body []byte, handle string) (*RegisteredNet, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	return decodeRegisteredNetNode(root, handle)
}
func (c *Client) GetRegisteredNet(ctx context.Context, handle string) (*RegisteredNet, error) {
	if !handlePattern.MatchString(handle) {
		return nil, errors.New("invalid network handle")
	}
	body, err := c.get(ctx, c.baseURL, "/rest/net/"+url.PathEscape(handle), "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodeRegisteredNet(body, handle)
}

// NetWriteResult can represent an accepted ticket without a completed network.
// Callers must persist TicketNumber and reconcile it, never retry the write blindly.
type NetWriteResult struct {
	Net                                          *RegisteredNet
	TicketNumber, TicketStatus, TicketResolution string
}

func decodeNetWriteResult(body []byte) (*NetWriteResult, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "ticketedRequest" || root.Name.Space != registrationNamespace {
		return nil, errors.New("ARIN returned an unexpected ticketed NET response")
	}
	result := &NetWriteResult{}
	seen := map[string]bool{}
	for _, child := range root.Children {
		if child.Name.Space != registrationNamespace || seen[child.Name.Local] {
			return result, errors.New("ARIN returned invalid ticketed NET fields")
		}
		seen[child.Name.Local] = true
		switch child.Name.Local {
		case "net":
			// Continue to collect a following ticket even if the NET is malformed.
			// That ticket is needed to reconcile an accepted write safely.
			result.Net, err = decodeRegisteredNetNode(child, "")
		case "ticket":
			v, e := decodeFields(child, ticketFields)
			if e != nil {
				return result, e
			}
			result.TicketNumber, _ = v["ticket_number"].(string)
			result.TicketStatus, _ = v["ticket_status"].(string)
			result.TicketResolution, _ = v["resolution"].(string)
			if result.TicketNumber == "" {
				return result, errors.New("ARIN returned an incomplete NET ticket")
			}
		default:
			return result, errors.New("ARIN returned unsupported ticketed NET fields")
		}
	}
	if err != nil {
		return result, err
	}
	if result.Net == nil && result.TicketNumber == "" {
		return result, errors.New("ARIN returned neither a network nor a ticket")
	}
	return result, nil
}
func (c *Client) CreateNetAssignment(ctx context.Context, a NetAssignment) (*NetWriteResult, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	n := a.net()
	body, err := n.marshal()
	if err != nil {
		return nil, err
	}
	action := "reassign"
	if a.Reallocate {
		action = "reallocate"
	}
	response, err := c.request(ctx, http.MethodPut, c.baseURL, "/rest/net/"+url.PathEscape(a.ParentNetHandle)+"/"+action, "application/xml", true, body)
	if err != nil {
		return nil, err
	}
	result, err := decodeNetWriteResult(response.Body)
	if err != nil {
		return result, fmt.Errorf("NET assignment result could not be verified; reconcile before retrying: %w", err)
	}
	if result.Net != nil {
		actual := result.Net
		if !sameNetAssignment(*actual, n) {
			return result, errors.New("ARIN returned a different NET assignment; reconcile before retrying")
		}
	}
	return result, nil
}

// UpdateRegisteredNet preserves immutable fields by fetching the current record.
// POCs are passed explicitly; nil preserves current links, an empty slice clears them.
func (c *Client) UpdateRegisteredNet(ctx context.Context, handle, name string, comments, origins []string, pocs []NetPOC) (*RegisteredNet, error) {
	if err := validateNetMetadata(name, comments, origins); err != nil {
		return nil, err
	}
	for _, p := range pocs {
		if !handlePattern.MatchString(p.Handle) || !slices.Contains([]string{"T", "AB", "N", "R", "D"}, p.Function) {
			return nil, errors.New("invalid NET POC link")
		}
	}
	n, err := c.GetRegisteredNet(ctx, handle)
	if err != nil {
		return nil, err
	}
	n.Name = name
	n.Comments = comments
	n.OriginASNs = origins
	if pocs != nil {
		n.POCs = pocs
	}
	body, err := n.marshal()
	if err != nil {
		return nil, err
	}
	response, err := c.request(ctx, http.MethodPut, c.baseURL, "/rest/net/"+url.PathEscape(handle), "application/xml", true, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		return nil, errors.New("ARIN did not confirm completed NET modification")
	}
	updated, err := decodeRegisteredNet(response.Body, handle)
	if err != nil {
		return nil, err
	}
	if !sameNetAssignment(*updated, *n) || updated.RegistrationDate != n.RegistrationDate {
		return nil, errors.New("ARIN changed immutable NET fields during metadata update")
	}
	return updated, nil
}
func (c *Client) DeleteNetAssignment(ctx context.Context, handle string) (*NetWriteResult, error) {
	n, err := c.GetRegisteredNet(ctx, handle)
	if IsNotFound(err) {
		return &NetWriteResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	for _, b := range n.Blocks {
		if b.Type != "S" && b.Type != "A" {
			return nil, errors.New("only reassigned or reallocated NET records can be deleted")
		}
	}
	response, err := c.request(ctx, http.MethodDelete, c.baseURL, "/rest/net/"+url.PathEscape(handle), "application/xml", true, nil)
	if IsNotFound(err) {
		return &NetWriteResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	result, err := decodeNetWriteResult(response.Body)
	if err != nil {
		return result, fmt.Errorf("NET deletion result could not be verified; reconcile before retrying: %w", err)
	}
	if result.Net != nil && result.Net.Handle != handle {
		return result, errors.New("ARIN returned a mismatched deleted NET")
	}
	return result, nil
}

func netString(v map[string]any, k string) string { s, _ := v[k].(string); return s }

// sameNetAssignment compares immutable recipient and address-space identity.
// Block descriptions are server-generated display text, not identity.
func sameNetAssignment(a, b RegisteredNet) bool {
	blocks := func(n RegisteredNet) []string {
		out := []string{}
		for _, block := range n.Blocks {
			out = append(out, fmt.Sprintf("%s/%d:%s", block.StartAddress, block.CIDRLength, block.Type))
		}
		slices.Sort(out)
		return out
	}
	return a.Version == b.Version && a.ParentNetHandle == b.ParentNetHandle &&
		a.CustomerHandle == b.CustomerHandle && a.OrgHandle == b.OrgHandle &&
		slices.Equal(blocks(a), blocks(b))
}

// FindNetAssignment reconciles a previously submitted assignment using exact
// range lookups. It never adopts an unrelated registration in the same range.
func (c *Client) FindNetAssignment(ctx context.Context, a NetAssignment) (*RegisteredNet, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	// mostSpecificNet matches the registration's complete range, not each
	// constituent CIDR. Multi-block records need one bounding-range lookup.
	first := netip.MustParsePrefix(a.Prefixes[0])
	start, end := first.Addr(), prefixEnd(first)
	for _, raw := range a.Prefixes[1:] {
		p := netip.MustParsePrefix(raw)
		if p.Addr().Compare(start) < 0 {
			start = p.Addr()
		}
		if last := prefixEnd(p); last.Compare(end) > 0 {
			end = last
		}
	}
	body, err := c.get(ctx, c.baseURL, "/rest/net/mostSpecificNet/"+url.PathEscape(start.String())+"/"+url.PathEscape(end.String()), "application/xml", true)
	if IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n, err := decodeRegisteredNet(body, "")
	if err != nil {
		return nil, err
	}
	if !sameNetAssignment(*n, a.net()) || n.Name != a.Name {
		return nil, errors.New("the requested range contains a different registration; import or investigate it before retrying")
	}
	return n, nil
}

type RegistrationTicket struct{ Number, Status, Resolution string }

func (t RegistrationTicket) Terminal() bool {
	return slices.Contains([]string{"RESOLVED", "CLOSED"}, t.Status)
}

func (t RegistrationTicket) Failed() bool {
	return slices.Contains([]string{"RESOLVED", "CLOSED"}, t.Status) &&
		slices.Contains([]string{"DENIED", "ABANDONED", "WITHDRAWN", "UNSUCCESSFUL", "DUPLICATE"}, t.Resolution)
}
func (c *Client) GetRegistrationTicket(ctx context.Context, number string) (*RegistrationTicket, error) {
	for _, spec := range RegistrationReads() {
		if spec.Name != "ticket_summary" {
			continue
		}
		v, err := c.ReadRegistration(ctx, spec, map[string]string{"ticket_number": number})
		if err != nil {
			return nil, err
		}
		if netString(v, "ticket_number") != number {
			return nil, errors.New("ARIN returned a mismatched ticket")
		}
		return &RegistrationTicket{Number: number, Status: netString(v, "ticket_status"), Resolution: netString(v, "resolution")}, nil
	}
	return nil, errors.New("ticket summary API is unavailable")
}
