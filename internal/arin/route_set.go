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
	"strconv"
	"strings"
)

// RouteSet is the complete writable representation of an XML (simple) IRR route set.
// Dates, source, and POC links are server-owned. POCs are read-only output.
type RouteSet struct {
	Name, OrgHandle                                        string
	Description, Remarks, Members, MembersByRef, MPMembers []string
	POCs                                                   []IRRPOC
	CreationDate, LastModifiedDate                         string
}
type routeSetXML struct {
	XMLName      xml.Name     `xml:"http://www.arin.net/regrws/core/v1 routeSet"`
	Description  []irrLine    `xml:"description>line"`
	OrgHandle    string       `xml:"orgHandle"`
	Remarks      *irrLinesXML `xml:"remarks,omitempty"`
	Source       string       `xml:"source"`
	Members      []irrMember  `xml:"members>member"`
	MembersByRef []irrMember  `xml:"membersByRef>memberByRef"`
	MPMembers    []irrMember  `xml:"mpMembers>mpMember"`
	Name         string       `xml:"name"`
}

var routeSetNamePattern = regexp.MustCompile(`^(?:AS[0-9]+:|(?:AS|RS)-[A-Z0-9][A-Z0-9_-]*:)*RS-[A-Z0-9][A-Z0-9_-]*$`)

func ValidateRouteSetName(name string) error {
	if !routeSetNamePattern.MatchString(name) {
		return errors.New("route set name must be uppercase, start with RS-, or be a colon-separated hierarchy ending in RS- followed by a name")
	}
	return nil
}
func (s RouteSet) Validate() error {
	if err := ValidateRouteSetName(s.Name); err != nil {
		return err
	}
	if !handlePattern.MatchString(s.OrgHandle) || s.OrgHandle != strings.ToUpper(s.OrgHandle) {
		return errors.New("org_handle must be an uppercase ARIN organization handle")
	}
	if len(s.Description) == 0 {
		return errors.New("description must contain at least one line")
	}
	for _, lines := range [][]string{s.Description, s.Remarks} {
		for _, line := range lines {
			if strings.TrimSpace(line) != line || line == "" || strings.ContainsAny(line, "\r\n") {
				return errors.New("description and remarks must contain nonempty individual lines without surrounding whitespace")
			}
		}
	}
	for _, m := range s.Members {
		if err := validateRouteSetMember(m, false); err != nil {
			return fmt.Errorf("members: %w", err)
		}
	}
	for _, m := range s.MPMembers {
		if err := validateRouteSetMember(m, true); err != nil {
			return fmt.Errorf("mp_members: %w", err)
		}
	}
	for _, m := range s.MembersByRef {
		if m != "ANY" && (!strings.HasPrefix(m, "MNT-") || !handlePattern.MatchString(strings.TrimPrefix(m, "MNT-")) || m != strings.ToUpper(m)) {
			return errors.New("members_by_ref must contain ANY or uppercase MNT- organization handles")
		}
	}

	return nil
}
func (s RouteSet) marshal() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	p := routeSetXML{Name: s.Name, OrgHandle: s.OrgHandle, Source: "ARIN"}
	for i, v := range s.Description {
		p.Description = append(p.Description, irrLine{i, v})
	}
	if len(s.Remarks) > 0 {
		p.Remarks = &irrLinesXML{}
		for i, v := range s.Remarks {
			p.Remarks.Lines = append(p.Remarks.Lines, irrLine{i, v})
		}
	}
	for _, v := range s.Members {
		p.Members = append(p.Members, irrMember{v})
	}
	for _, v := range s.MembersByRef {
		p.MembersByRef = append(p.MembersByRef, irrMember{v})
	}

	for _, v := range s.MPMembers {
		p.MPMembers = append(p.MPMembers, irrMember{v})
	}
	return xml.Marshal(p)
}
func decodeRouteSet(body []byte, name string) (*RouteSet, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "routeSet" || root.Name.Space != "http://www.arin.net/regrws/core/v1" {
		return nil, errors.New("ARIN returned an unexpected route set payload; only simple XML objects are supported")
	}
	// Refuse extensions we cannot round-trip instead of silently dropping them on PUT.
	allowed := map[string]bool{"name": true, "orgHandle": true, "source": true, "description": true, "remarks": true, "pocLinks": true, "members": true, "membersByRef": true, "mpMembers": true, "creationDate": true, "lastModifiedDate": true}
	for _, child := range root.Children {
		if child.Name.Space != root.Name.Space || !allowed[child.Name.Local] {
			return nil, errors.New("ARIN returned unsupported route set fields; refusing to manage a partial representation")
		}
	}
	values, err := decodeFields(root, setFields)
	if err != nil {
		return nil, err
	}
	str := func(key string) string { v, _ := values[key].(string); return v }
	if str("name") != name || str("org_handle") == "" || str("source") != "ARIN" {
		return nil, errors.New("ARIN returned an incomplete or mismatched route set")
	}
	list := func(key string) []string {
		out := []string{}
		for _, v := range values[key].([]any) {
			out = append(out, v.(string))
		}
		return out
	}
	s := &RouteSet{Name: str("name"), OrgHandle: str("org_handle"), Description: list("description"), Remarks: list("remarks"), Members: list("members"), MembersByRef: list("members_by_ref"), MPMembers: list("mp_members"), CreationDate: str("creation_date"), LastModifiedDate: str("last_modified_date")}
	for _, v := range values["poc_links"].([]any) {
		p := v.(map[string]any)
		h, _ := p["handle"].(string)
		f, _ := p["function"].(string)
		if h == "" || (f != "AD" && f != "T" && f != "R") {
			return nil, errors.New("ARIN returned an unsupported IRR POC link")
		}
		s.POCs = append(s.POCs, IRRPOC{h, f})
	}
	return s, nil
}
func (c *Client) GetRouteSet(ctx context.Context, name string) (*RouteSet, error) {
	if err := ValidateRouteSetName(name); err != nil {
		return nil, err
	}
	body, err := c.get(ctx, c.baseURL, "/rest/irr/route-set/"+url.PathEscape(name), "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodeRouteSet(body, name)
}
func (c *Client) CreateRouteSet(ctx context.Context, s RouteSet) (*RouteSet, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	// Existing objects must be imported, never silently adopted or overwritten.
	if _, err := c.GetRouteSet(ctx, s.Name); err == nil {
		return nil, errors.New("route set already exists; import it before managing it")
	} else if !IsNotFound(err) {
		return nil, err
	}
	return c.writeRouteSet(ctx, http.MethodPost, "/rest/irr/route-set?orgHandle="+url.QueryEscape(s.OrgHandle), s)
}
func (c *Client) UpdateRouteSet(ctx context.Context, s RouteSet) (*RouteSet, error) {
	return c.writeRouteSet(ctx, http.MethodPut, "/rest/irr/route-set/"+url.PathEscape(s.Name), s)
}

func (c *Client) writeRouteSet(ctx context.Context, method, path string, s RouteSet) (*RouteSet, error) {
	payload, err := s.marshal()
	if err != nil {
		return nil, err
	}
	response, err := c.request(ctx, method, c.baseURL, path, "application/xml", true, payload)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return nil, errors.New("ARIN did not return the completed route set; verify its status before retrying")
	}
	result, err := decodeRouteSet(response.Body, s.Name)
	if err != nil {
		return nil, fmt.Errorf("ARIN accepted the write but its result could not be verified: %w", err)
	}
	if result.OrgHandle != s.OrgHandle {
		return nil, errors.New("ARIN returned a mismatched route set organization after the write")
	}
	return result, nil
}
func (c *Client) DeleteRouteSet(ctx context.Context, name string) error {
	if err := ValidateRouteSetName(name); err != nil {
		return err
	}
	response, err := c.request(ctx, http.MethodDelete, c.baseURL, "/rest/irr/route-set/"+url.PathEscape(name), "application/xml", true, nil)
	if IsNotFound(err) {
		return nil
	}
	if err == nil && response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		return errors.New("ARIN did not confirm completed deletion; refresh before retrying")
	}
	return err
}

// Prefix range operators are preserved as RPSL expressions. CIDR components must
// be canonical, and IPv6 is only valid in mp_members.
func validateRouteSetMember(member string, ipv6 bool) error {
	parts := strings.Split(member, "^")
	if len(parts) > 2 {
		return errors.New("invalid prefix range operator")
	}
	base := parts[0]
	bits := 0
	maxBits := 128
	if ValidateRouteSetName(base) != nil {
		p, err := netip.ParsePrefix(base)
		if err != nil || p.Addr().Is4In6() || p.Masked() != p || p.String() != base {
			return errors.New("member must be a canonical prefix or uppercase route-set name")
		}
		if p.Addr().Is6() && !ipv6 {
			return errors.New("IPv6 prefixes belong in mp_members")
		}
		bits = p.Bits()
		maxBits = p.Addr().BitLen()
	}
	if len(parts) == 1 {
		return nil
	}
	op := parts[1]
	if op == "+" || op == "-" {
		return nil
	}
	span := strings.Split(op, "-")
	if len(span) > 2 {
		return errors.New("invalid prefix range")
	}
	low, err := strconv.Atoi(span[0])
	if err != nil || strconv.Itoa(low) != span[0] || low < bits || low > maxBits {
		return errors.New("invalid prefix range lower bound")
	}
	if len(span) == 2 {
		high, err := strconv.Atoi(span[1])
		if err != nil || strconv.Itoa(high) != span[1] || high < low || high > maxBits {
			return errors.New("invalid prefix range upper bound")
		}
	}
	return nil
}
