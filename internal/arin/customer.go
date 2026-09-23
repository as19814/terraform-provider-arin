package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Customer contains the complete registration payload. The parent network is a
// create-only argument and is not returned by the customer API.
type Customer struct {
	Handle, Name, ParentOrgHandle, RegistrationDate                                           string
	CountryCode, CountryName, CountryCode3, CountryCallingCode, City, Subdivision, PostalCode string
	StreetAddress, Comments                                                                   []string
	Private                                                                                   bool
}
type registrationCountryXML struct {
	Code2 string `xml:"code2"`
}
type customerXML struct {
	XMLName          xml.Name               `xml:"http://www.arin.net/regrws/core/v1 customer"`
	Name             string                 `xml:"customerName"`
	Country          registrationCountryXML `xml:"iso3166-1"`
	Handle           string                 `xml:"handle,omitempty"`
	StreetAddress    []irrLine              `xml:"streetAddress>line"`
	City             string                 `xml:"city"`
	Subdivision      string                 `xml:"iso3166-2"`
	PostalCode       string                 `xml:"postalCode"`
	Comments         *irrLinesXML           `xml:"comment,omitempty"`
	ParentOrgHandle  string                 `xml:"parentOrgHandle,omitempty"`
	RegistrationDate string                 `xml:"registrationDate,omitempty"`
	Private          bool                   `xml:"privateCustomer"`
}

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

// ValidateCustomerContext checks the parent network and optional customer identity.
func ValidateCustomerContext(parent, handle string) error {
	if !handlePattern.MatchString(parent) || (handle != "" && !handlePattern.MatchString(handle)) {
		return errors.New("invalid parent network or customer handle")
	}
	return nil
}

func (r Customer) Validate() error {
	if strings.TrimSpace(r.Name) == "" || strings.ContainsAny(r.Name, "\r\n") {
		return errors.New("customer name must be a nonempty single line")
	}
	if !countryCodePattern.MatchString(r.CountryCode) {
		return errors.New("country_code must be an uppercase two-letter country code")
	}
	if len(r.StreetAddress) == 0 {
		return errors.New("street_address must contain at least one line")
	}
	if (r.CountryCode == "US" || r.CountryCode == "CA") && (r.Subdivision == "" || r.PostalCode == "") {
		return errors.New("US and CA customers require subdivision and postal_code")
	}
	for _, lines := range [][]string{r.StreetAddress, r.Comments} {
		for _, line := range lines {
			if strings.TrimSpace(line) == "" || strings.ContainsAny(line, "\r\n") {
				return errors.New("address and comments must contain nonempty individual lines")
			}
		}
	}
	return nil
}
func (r Customer) marshal() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	p := customerXML{Name: r.Name, Country: registrationCountryXML{r.CountryCode}, Handle: r.Handle, City: r.City, Subdivision: r.Subdivision, PostalCode: r.PostalCode, ParentOrgHandle: r.ParentOrgHandle, RegistrationDate: r.RegistrationDate, Private: r.Private, Comments: xmlPolicy(r.Comments)}
	for i, line := range r.StreetAddress {
		p.StreetAddress = append(p.StreetAddress, irrLine{i, line})
	}
	return xml.Marshal(p)
}
func decodeCustomer(body []byte, handle string) (*Customer, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "customer" || root.Name.Space != "http://www.arin.net/regrws/core/v1" {
		return nil, errors.New("ARIN returned an unexpected customer payload")
	}
	allowed := map[string]bool{"customerName": true, "iso3166-1": true, "handle": true, "streetAddress": true, "city": true, "iso3166-2": true, "postalCode": true, "comment": true, "parentOrgHandle": true, "registrationDate": true, "privateCustomer": true}
	for _, child := range root.Children {
		if child.Name.Space != root.Name.Space || !allowed[child.Name.Local] {
			return nil, errors.New("ARIN returned unsupported customer fields; refusing a partial representation")
		}
	}
	values, err := decodeFields(root, customerFields)
	if err != nil {
		return nil, err
	}
	str := func(k string) string { v, _ := values[k].(string); return v }
	r := &Customer{Handle: str("handle"), Name: str("name"), ParentOrgHandle: str("parent_org_handle"), RegistrationDate: str("registration_date"), CountryCode: str("country_code"), CountryName: str("country_name"), CountryCode3: str("country_code3"), CountryCallingCode: str("country_calling_code"), City: str("city"), Subdivision: str("subdivision"), PostalCode: str("postal_code")}
	private, ok := values["private_customer"].(bool)
	if !ok || !handlePattern.MatchString(r.Handle) || r.Name == "" || r.RegistrationDate == "" || r.CountryCode == "" || (handle != "" && r.Handle != handle) {
		return nil, errors.New("ARIN returned an incomplete or mismatched customer")
	}
	r.Private = private
	for _, v := range values["street_address"].([]any) {
		r.StreetAddress = append(r.StreetAddress, v.(string))
	}
	for _, v := range values["comments"].([]any) {
		r.Comments = append(r.Comments, v.(string))
	}
	return r, nil
}
func (c *Client) GetCustomer(ctx context.Context, handle string) (*Customer, error) {
	if !handlePattern.MatchString(handle) {
		return nil, errors.New("invalid customer handle")
	}
	body, err := c.get(ctx, c.baseURL, "/rest/customer/"+url.PathEscape(handle), "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodeCustomer(body, handle)
}
func (c *Client) CreateCustomer(ctx context.Context, parent string, r Customer) (*Customer, error) {
	if !handlePattern.MatchString(parent) {
		return nil, errors.New("invalid parent network handle")
	}
	// The server owns identity and parent organization; do not accept these on create.
	r.Handle = ""
	r.RegistrationDate = ""
	r.ParentOrgHandle = ""
	return c.writeCustomer(ctx, http.MethodPost, "/rest/net/"+url.PathEscape(parent)+"/customer", r)
}
func (c *Client) UpdateCustomer(ctx context.Context, r Customer) (*Customer, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	current, err := c.GetCustomer(ctx, r.Handle)
	if err != nil {
		return nil, err
	}
	r.RegistrationDate = current.RegistrationDate
	r.ParentOrgHandle = current.ParentOrgHandle
	return c.writeCustomer(ctx, http.MethodPut, "/rest/customer/"+url.PathEscape(r.Handle), r)
}
func (c *Client) writeCustomer(ctx context.Context, method, path string, r Customer) (*Customer, error) {
	body, err := r.marshal()
	if err != nil {
		return nil, err
	}
	response, err := c.request(ctx, method, c.baseURL, path, "application/xml", true, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return nil, errors.New("ARIN did not confirm completed customer creation/update; verify before retrying")
	}
	out, err := decodeCustomer(response.Body, r.Handle)
	if err != nil {
		return nil, fmt.Errorf("ARIN accepted the customer write but its result could not be verified: %w", err)
	}
	return out, nil
}
func (c *Client) DeleteCustomer(ctx context.Context, handle string) error {
	if !handlePattern.MatchString(handle) {
		return errors.New("invalid customer handle")
	}
	response, err := c.request(ctx, http.MethodDelete, c.baseURL, "/rest/customer/"+url.PathEscape(handle), "application/xml", true, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if response.StatusCode != 200 && response.StatusCode != 204 {
		return errors.New("ARIN did not confirm completed customer deletion; refresh before retrying")
	}
	return nil
}
