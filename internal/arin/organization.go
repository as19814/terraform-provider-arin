package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// RegisteredOrganization is the full writable organization record. The older
// Organization type is a minimal discovery result and must not be used for PUT.
type RegisteredOrganization struct {
	Handle, Name, DBAName, RegistrationDate                 string
	CountryCode, CountryName, City, Subdivision, PostalCode string
	TaxID, RWhoisURL                                        string
	AcceptReassignments                                     *bool
	StreetAddress, Comments                                 []string
	POCs                                                    []OrgPOC
}

type organizationXML struct {
	XMLName             xml.Name               `xml:"http://www.arin.net/regrws/core/v1 org"`
	Country             registrationCountryXML `xml:"iso3166-1"`
	StreetAddress       []irrLine              `xml:"streetAddress>line"`
	City                string                 `xml:"city"`
	Subdivision         string                 `xml:"iso3166-2"`
	PostalCode          string                 `xml:"postalCode"`
	Comments            *irrLinesXML           `xml:"comment,omitempty"`
	RegistrationDate    string                 `xml:"registrationDate,omitempty"`
	Handle              string                 `xml:"handle,omitempty"`
	Name                string                 `xml:"orgName"`
	DBAName             string                 `xml:"dbaName,omitempty"`
	TaxID               string                 `xml:"taxId,omitempty"`
	RWhoisURL           string                 `xml:"orgUrl,omitempty"`
	AcceptReassignments *bool                  `xml:"acceptReassignments,omitempty"`
	POCs                []NetPOC               `xml:"pocLinks>pocLinkRef"`
}

func (o RegisteredOrganization) Validate() error {
	if err := (Customer{Name: o.Name, CountryCode: o.CountryCode, Subdivision: o.Subdivision, PostalCode: o.PostalCode, StreetAddress: o.StreetAddress, Comments: o.Comments}).Validate(); err != nil {
		return fmt.Errorf("invalid organization name/address: %w", err)
	}
	for _, s := range []string{o.DBAName, o.City, o.Subdivision, o.PostalCode, o.TaxID, o.RWhoisURL} {
		if strings.ContainsAny(s, "\r\n") {
			return errors.New("organization scalar fields must be single lines")
		}
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, p := range o.POCs {
		if !handlePattern.MatchString(p.Handle) {
			return errors.New("invalid organization POC handle")
		}
		if p.Function != "AD" {
			if err := ValidateOrgPOCFunction(p.Function); err != nil {
				return err
			}
		}
		key := p.Handle + "/" + p.Function
		if seen[key] {
			return errors.New("duplicate organization POC association")
		}
		seen[key] = true
		counts[p.Function]++
	}
	if counts["AD"] != 1 || counts["T"] == 0 || counts["AB"] == 0 {
		return errors.New("organization requires exactly one Admin, at least one Tech and at least one Abuse POC")
	}
	return nil
}
func (o RegisteredOrganization) marshal() ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	p := organizationXML{Country: registrationCountryXML{o.CountryCode}, City: o.City, Subdivision: o.Subdivision, PostalCode: o.PostalCode, Comments: xmlPolicy(o.Comments), RegistrationDate: o.RegistrationDate, Handle: o.Handle, Name: o.Name, DBAName: o.DBAName, TaxID: o.TaxID, RWhoisURL: o.RWhoisURL, AcceptReassignments: o.AcceptReassignments}
	for i, s := range o.StreetAddress {
		p.StreetAddress = append(p.StreetAddress, irrLine{i, s})
	}
	for _, link := range o.POCs {
		p.POCs = append(p.POCs, NetPOC{Handle: link.Handle, Function: link.Function})
	}
	return xml.Marshal(p)
}

func decodeRegisteredOrganization(body []byte, handle string) (*RegisteredOrganization, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	return decodeRegisteredOrganizationNode(root, handle)
}
func decodeRegisteredOrganizationNode(root *xmlNode, handle string) (*RegisteredOrganization, error) {
	if root.Name.Local != "org" || root.Name.Space != registrationNamespace {
		return nil, errors.New("ARIN returned an unexpected organization payload")
	}
	allowed := map[string]bool{}
	for _, p := range []string{"handle", "orgName", "dbaName", "registrationDate", "taxId", "orgUrl", "acceptReassignments", "iso3166-1", "iso3166-1/code2", "iso3166-1/code3", "iso3166-1/name", "iso3166-1/e164", "iso3166-2", "city", "postalCode", "streetAddress", "streetAddress/line", "comment", "comment/line", "pocLinks", "pocLinks/pocLinkRef"} {
		allowed[p] = true
	}
	repeated := map[string]bool{"streetAddress/line": true, "comment/line": true, "pocLinks/pocLinkRef": true}
	var walk func(*xmlNode, string) error
	walk = func(n *xmlNode, path string) error {
		attrs := map[string]bool{}
		for _, a := range n.Attrs {
			if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
				continue
			}
			valid := a.Name.Space == "" && (((path == "streetAddress/line" || path == "comment/line") && a.Name.Local == "number") || (path == "pocLinks/pocLinkRef" && (a.Name.Local == "handle" || a.Name.Local == "function" || a.Name.Local == "description")))
			if !valid || attrs[a.Name.Local] {
				return errors.New("ARIN returned unsupported organization attributes")
			}
			attrs[a.Name.Local] = true
		}
		counts := map[string]int{}
		for _, child := range n.Children {
			p := child.Name.Local
			if path != "" {
				p = path + "/" + p
			}
			if child.Name.Space != registrationNamespace || !allowed[p] {
				return errors.New("ARIN returned unsupported organization fields; refusing a partial representation")
			}
			counts[p]++
			if counts[p] > 1 && !repeated[p] {
				return errors.New("ARIN returned duplicate organization fields")
			}
			if err := walk(child, p); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return nil, err
	}
	v, err := decodeFields(root, orgFields)
	if err != nil {
		return nil, err
	}
	s := func(k string) string { return netString(v, k) }
	o := &RegisteredOrganization{Handle: s("handle"), Name: s("name"), DBAName: s("dba_name"), RegistrationDate: s("registration_date"), CountryCode: s("country_code"), CountryName: s("country_name"), City: s("city"), Subdivision: s("subdivision"), PostalCode: s("postal_code"), TaxID: s("tax_id"), RWhoisURL: s("rwhois_url")}
	if b, ok := v["accept_reassignments"].(bool); ok {
		o.AcceptReassignments = &b
	}
	for k, target := range map[string]*[]string{"street_address": &o.StreetAddress, "comments": &o.Comments} {
		for _, item := range v[k].([]any) {
			*target = append(*target, item.(string))
		}
	}
	for _, item := range v["poc_links"].([]any) {
		p := item.(map[string]any)
		o.POCs = append(o.POCs, OrgPOC{Handle: netString(p, "handle"), Function: netString(p, "function"), Description: netString(p, "description")})
	}
	if !handlePattern.MatchString(o.Handle) || o.RegistrationDate == "" || (handle != "" && handle != o.Handle) {
		return nil, errors.New("ARIN returned incomplete or mismatched organization identity")
	}
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("ARIN returned invalid organization: %w", err)
	}
	return o, nil
}

// OrganizationWriteResult retains a ticket even when decoding the accompanying
// organization fails. Callers must persist it before reporting diagnostics.
type OrganizationWriteResult struct {
	Organization                                 *RegisteredOrganization
	TicketNumber, TicketStatus, TicketResolution string
}

func decodeOrganizationWriteResult(body []byte, handle string) (*OrganizationWriteResult, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	result := &OrganizationWriteResult{}
	if root.Name.Space != registrationNamespace {
		return result, errors.New("ARIN returned an unexpected organization response namespace")
	}
	nodes := []*xmlNode{root}
	if root.Name.Local == "ticketedRequest" {
		nodes = root.Children
	}
	seen := map[string]bool{}
	var decodeErr error
	for _, n := range nodes {
		if n.Name.Space != registrationNamespace || seen[n.Name.Local] {
			decodeErr = errors.New("ARIN returned invalid organization response fields")
			continue
		}
		seen[n.Name.Local] = true
		switch n.Name.Local {
		case "org":
			result.Organization, err = decodeRegisteredOrganizationNode(n, handle)
			if err != nil {
				decodeErr = err
			}
		case "ticket":
			v, e := decodeFields(n, ticketFields)
			if e != nil {
				decodeErr = e
				continue
			}
			result.TicketNumber = netString(v, "ticket_number")
			result.TicketStatus = netString(v, "ticket_status")
			result.TicketResolution = netString(v, "resolution")
		default:
			decodeErr = errors.New("ARIN returned unsupported organization response fields")
		}
	}
	if decodeErr != nil {
		return result, decodeErr
	}
	if result.Organization == nil && result.TicketNumber == "" {
		return result, errors.New("ARIN returned neither an organization nor a ticket")
	}
	return result, nil
}
func (c *Client) GetRegisteredOrganization(ctx context.Context, handle string) (*RegisteredOrganization, error) {
	if !handlePattern.MatchString(handle) {
		return nil, errors.New("invalid organization handle")
	}
	body, err := c.get(ctx, c.baseURL, "/rest/org/"+url.PathEscape(handle), "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodeRegisteredOrganization(body, handle)
}
func (c *Client) CreateOrganization(ctx context.Context, o RegisteredOrganization) (*OrganizationWriteResult, error) {
	o.Handle = ""
	o.RegistrationDate = ""
	return c.writeOrganization(ctx, http.MethodPost, "/rest/org", o)
}
func (c *Client) UpdateOrganization(ctx context.Context, o RegisteredOrganization) (*OrganizationWriteResult, error) {
	if !handlePattern.MatchString(o.Handle) {
		return nil, errors.New("invalid organization handle")
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	unlock, err := lockOrganization(ctx, c.baseURL, o.Handle)
	if err != nil {
		return nil, err
	}
	defer unlock()
	old, err := c.GetRegisteredOrganization(ctx, o.Handle)
	if err != nil {
		return nil, err
	}
	if old.Name != o.Name || old.DBAName != o.DBAName {
		return nil, errors.New("organization name and DBA name are immutable")
	}
	o.RegistrationDate = old.RegistrationDate
	return c.writeOrganization(ctx, http.MethodPut, "/rest/org/"+url.PathEscape(o.Handle), o)
}
func (c *Client) writeOrganization(ctx context.Context, method, path string, o RegisteredOrganization) (*OrganizationWriteResult, error) {
	body, err := o.marshal()
	if err != nil {
		return nil, err
	}
	response, err := c.request(ctx, method, c.baseURL, path, "application/xml", true, body)
	if err != nil {
		return nil, err
	}
	result, err := decodeOrganizationWriteResult(response.Body, o.Handle)
	if err != nil {
		return result, fmt.Errorf("organization write result could not be verified: %w", err)
	}
	if response.StatusCode != 200 && response.StatusCode != 201 && response.StatusCode != 202 {
		return result, errors.New("ARIN did not confirm organization write acceptance")
	}
	if response.StatusCode == 202 && result.TicketNumber == "" {
		return result, errors.New("ARIN accepted an asynchronous organization write without a recovery ticket")
	}
	return result, nil
}
func (c *Client) DeleteOrganization(ctx context.Context, handle string) (*OrganizationWriteResult, error) {
	if !handlePattern.MatchString(handle) {
		return nil, errors.New("invalid organization handle")
	}
	unlock, err := lockOrganization(ctx, c.baseURL, handle)
	if err != nil {
		return nil, err
	}
	defer unlock()
	response, err := c.request(ctx, http.MethodDelete, c.baseURL, "/rest/org/"+url.PathEscape(handle), "application/xml", true, nil)
	if IsNotFound(err) {
		return &OrganizationWriteResult{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := &OrganizationWriteResult{}
	if len(response.Body) > 0 {
		result, err = decodeOrganizationWriteResult(response.Body, handle)
		if err != nil {
			return result, err
		}
	}
	if response.StatusCode != 200 && response.StatusCode != 202 && response.StatusCode != 204 {
		return result, errors.New("ARIN did not confirm organization deletion acceptance")
	}
	if result.TicketNumber != "" {
		return result, nil
	}
	if response.StatusCode == 202 {
		return result, errors.New("ARIN accepted an asynchronous organization deletion without a recovery ticket")
	}
	_, err = c.GetRegisteredOrganization(ctx, handle)
	if IsNotFound(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	return result, errors.New("ARIN still returns the deleted organization")
}
