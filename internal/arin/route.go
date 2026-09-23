package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
)

// IRRRoute owns simple route metadata. Registration and POC links are server-owned.
type IRRRoute struct {
	Prefix, OriginAS, OrgHandle, NetHandle              string
	Description, Remarks, MemberOf                      []string
	POCs                                                []IRRPOC
	CreationDate, LastModifiedDate, AutoLinkedROAHandle string
}

func (r IRRRoute) ID() string { return r.Prefix + "," + r.OriginAS }

func ValidateIRRRouteID(id string) error {
	parts := strings.Split(id, ",")
	if len(parts) != 2 {
		return errors.New("route ID must be canonical CIDR,AS<number>, for example 192.0.2.0/24,AS64496")
	}
	p, err := netip.ParsePrefix(parts[0])
	if err != nil || p.Addr().Is4In6() || p != p.Masked() || p.String() != parts[0] {
		return errors.New("prefix must be a canonical IPv4 or IPv6 network CIDR without host bits")
	}
	if !strings.HasPrefix(parts[1], "AS") {
		return errors.New("origin_as must be AS followed by an ASN")
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(parts[1], "AS"), 10, 32)
	if err != nil || n == 0 || "AS"+strconv.FormatUint(n, 10) != parts[1] {
		return errors.New("origin_as must be canonical AS1 through AS4294967295")
	}
	return nil
}
func (r IRRRoute) Validate() error {
	if err := ValidateIRRRouteID(r.ID()); err != nil {
		return err
	}
	if !handlePattern.MatchString(r.OrgHandle) || r.OrgHandle != strings.ToUpper(r.OrgHandle) {
		return errors.New("org_handle must be an uppercase ARIN organization handle")
	}
	if len(r.Description) == 0 {
		return errors.New("description must contain at least one line")
	}
	for _, lines := range [][]string{r.Description, r.Remarks} {
		for _, line := range lines {
			if line == "" || strings.TrimSpace(line) != line || strings.ContainsAny(line, "\r\n") {
				return errors.New("description and remarks require nonempty single lines without surrounding whitespace")
			}
		}
	}
	for _, name := range r.MemberOf {
		if err := ValidateRouteSetName(name); err != nil {
			return fmt.Errorf("member_of: %w", err)
		}
	}
	return nil
}
func routePath(id string) (string, error) {
	if err := ValidateIRRRouteID(id); err != nil {
		return "", err
	}
	parts := strings.Split(id, ",")
	return "/rest/irr/route/" + parts[0] + "/" + parts[1], nil
}

type routeXML struct {
	XMLName     xml.Name     `xml:"http://www.arin.net/regrws/core/v1 route"`
	OrgHandle   string       `xml:"orgHandle"`
	Description []irrLine    `xml:"description>line"`
	Remarks     *irrLinesXML `xml:"remarks,omitempty"`
	Source      string       `xml:"source"`
	Prefix      string       `xml:"prefix"`
	OriginAS    string       `xml:"originAS"`
	MemberOf    []irrMember  `xml:"memberOf>routeSetRef"`
}

func (r IRRRoute) marshal() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	p := routeXML{OrgHandle: r.OrgHandle, Source: "ARIN", Prefix: r.Prefix, OriginAS: r.OriginAS}
	for i, line := range r.Description {
		p.Description = append(p.Description, irrLine{i, line})
	}
	if len(r.Remarks) > 0 {
		p.Remarks = &irrLinesXML{}
		for i, line := range r.Remarks {
			p.Remarks.Lines = append(p.Remarks.Lines, irrLine{i, line})
		}
	}
	for _, name := range r.MemberOf {
		p.MemberOf = append(p.MemberOf, irrMember{name})
	}
	return xml.Marshal(p)
}
func decodeIRRRoute(body []byte, id string) (*IRRRoute, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "route" || root.Name.Space != "http://www.arin.net/regrws/core/v1" {
		return nil, errors.New("ARIN returned an unexpected route payload; only simple XML routes are supported")
	}
	allowed := map[string]bool{"orgHandle": true, "description": true, "remarks": true, "creationDate": true, "lastModifiedDate": true, "source": true, "pocLinks": true, "prefix": true, "originAS": true, "netHandle": true, "version": true, "autoLinkedRoaHandle": true, "memberOf": true}
	for _, child := range root.Children {
		if child.Name.Space != root.Name.Space || !allowed[child.Name.Local] {
			return nil, errors.New("ARIN returned unsupported route fields; refusing to manage a partial representation")
		}
		if child.Name.Local == "memberOf" {
			if strings.TrimSpace(child.Text) != "" || len(child.Attrs) != 0 {
				return nil, errors.New("ARIN returned unsupported route membership")
			}
			for _, ref := range child.Children {
				if ref.Name.Space != root.Name.Space || ref.Name.Local != "routeSetRef" || len(ref.Children) != 0 || strings.TrimSpace(ref.Text) != "" || len(ref.Attrs) != 1 || ref.Attrs[0].Name.Space != "" || ref.Attrs[0].Name.Local != "name" {
					return nil, errors.New("ARIN returned unsupported route membership reference")
				}
				if err := ValidateRouteSetName(ref.Attrs[0].Value); err != nil {
					return nil, fmt.Errorf("ARIN returned invalid route membership: %w", err)
				}
			}
		}
	}
	values, err := decodeFields(root, routeFields)
	if err != nil {
		return nil, err
	}
	str := func(key string) string { v, _ := values[key].(string); return v }
	r := &IRRRoute{Prefix: str("prefix"), OriginAS: str("origin_as"), OrgHandle: str("org_handle"), NetHandle: str("net_handle"), CreationDate: str("creation_date"), LastModifiedDate: str("last_modified_date"), AutoLinkedROAHandle: str("auto_linked_roa_handle")}
	if r.ID() != id || r.OrgHandle == "" || r.NetHandle == "" || str("source") != "ARIN" {
		return nil, errors.New("ARIN returned an incomplete or mismatched route")
	}
	for _, v := range values["description"].([]any) {
		r.Description = append(r.Description, v.(string))
	}
	for _, v := range values["remarks"].([]any) {
		r.Remarks = append(r.Remarks, v.(string))
	}
	for _, v := range values["member_of"].([]any) {
		r.MemberOf = append(r.MemberOf, v.(string))
	}
	for _, v := range values["poc_links"].([]any) {
		p := v.(map[string]any)
		h, _ := p["handle"].(string)
		f, _ := p["function"].(string)
		if h == "" || f == "" {
			return nil, errors.New("ARIN returned an incomplete route POC link")
		}
		r.POCs = append(r.POCs, IRRPOC{Handle: h, Function: f, Description: netString(p, "description")})
	}
	return r, nil
}
func (r IRRRoute) CheckManaged() error {
	if r.AutoLinkedROAHandle != "" {
		return errors.New("route is linked to a ROA and must be managed through its RPKI lifecycle")
	}
	return nil
}
func (c *Client) GetIRRRoute(ctx context.Context, id string) (*IRRRoute, error) {
	path, err := routePath(id)
	if err != nil {
		return nil, err
	}
	body, err := c.get(ctx, c.baseURL, path, "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodeIRRRoute(body, id)
}
func (c *Client) CreateIRRRoute(ctx context.Context, r IRRRoute) (*IRRRoute, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if _, err := c.GetIRRRoute(ctx, r.ID()); err == nil {
		return nil, errors.New("route already exists; import it before managing it")
	} else if !IsNotFound(err) {
		return nil, err
	}
	return c.writeIRRRoute(ctx, http.MethodPost, r)
}
func (c *Client) UpdateIRRRoute(ctx context.Context, r IRRRoute) (*IRRRoute, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	current, err := c.GetIRRRoute(ctx, r.ID())
	if err != nil {
		return nil, err
	}
	if err := current.CheckManaged(); err != nil {
		return nil, err
	}
	if current.OrgHandle != r.OrgHandle {
		return nil, errors.New("route organization differs from configuration; refusing update")
	}
	return c.writeIRRRoute(ctx, http.MethodPut, r)
}
func (c *Client) writeIRRRoute(ctx context.Context, method string, r IRRRoute) (*IRRRoute, error) {
	body, err := r.marshal()
	if err != nil {
		return nil, err
	}
	path, err := routePath(r.ID())
	if err != nil {
		return nil, err
	}
	response, err := c.request(ctx, method, c.baseURL, path, "application/xml", true, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return nil, errors.New("ARIN did not confirm a completed route write; refresh before retrying")
	}
	out, err := decodeIRRRoute(response.Body, r.ID())
	if err != nil {
		return nil, fmt.Errorf("ARIN accepted the route write but its result could not be verified: %w", err)
	}
	if out.OrgHandle != r.OrgHandle {
		return nil, errors.New("ARIN returned a mismatched route organization after the write")
	}
	if err := out.CheckManaged(); err != nil {
		return nil, err
	}
	return out, nil
}
func (c *Client) DeleteIRRRoute(ctx context.Context, id string) error {
	current, err := c.GetIRRRoute(ctx, id)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := current.CheckManaged(); err != nil {
		return err
	}
	path, err := routePath(id)
	if err != nil {
		return err
	}
	response, err := c.request(ctx, http.MethodDelete, c.baseURL, path, "application/xml", true, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if response.StatusCode != 200 && response.StatusCode != 204 {
		return errors.New("ARIN did not confirm completed route deletion; refresh before retrying")
	}
	return nil
}
