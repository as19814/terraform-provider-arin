package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
)

type POCPhone struct {
	Description string
	Type        string
	Number      string
	Extension   string
}
type POC struct {
	Handle, RegistrationDate                                                                  string
	ContactType, CompanyName, FirstName, MiddleName, LastName                                 string
	CountryCode, CountryName, CountryCode3, CountryCallingCode, City, Subdivision, PostalCode string
	StreetAddress, Comments, Emails                                                           []string
	Phones                                                                                    []POCPhone
}
type pocPhoneTypeXML struct {
	Code string `xml:"code"`
}
type pocPhoneXML struct {
	XMLName   xml.Name        `xml:"http://www.arin.net/regrws/core/v1 phone"`
	Type      pocPhoneTypeXML `xml:"type"`
	Number    string          `xml:"number"`
	Extension string          `xml:"extension,omitempty"`
}
type pocXML struct {
	XMLName          xml.Name               `xml:"http://www.arin.net/regrws/core/v1 poc"`
	Subdivision      string                 `xml:"iso3166-2"`
	Country          registrationCountryXML `xml:"iso3166-1"`
	Emails           []string               `xml:"emails>email"`
	StreetAddress    []irrLine              `xml:"streetAddress>line"`
	City             string                 `xml:"city"`
	PostalCode       string                 `xml:"postalCode"`
	Comments         *irrLinesXML           `xml:"comment,omitempty"`
	RegistrationDate string                 `xml:"registrationDate,omitempty"`
	Handle           string                 `xml:"handle,omitempty"`
	ContactType      string                 `xml:"contactType"`
	CompanyName      string                 `xml:"companyName"`
	FirstName        string                 `xml:"firstName,omitempty"`
	MiddleName       string                 `xml:"middleName,omitempty"`
	LastName         string                 `xml:"lastName"`
	Phones           []pocPhoneXML          `xml:"phones>phone"`
}

func ValidatePOCEmail(email string) error {
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || strings.ContainsAny(email, "\r\n") {
		return errors.New("POC email must be a plain email address")
	}
	return nil
}
func (p POCPhone) Validate() error {
	if p.Type != "O" && p.Type != "F" && p.Type != "M" {
		return errors.New("phone type must be O, F or M")
	}
	if strings.TrimSpace(p.Number) == "" || strings.ContainsAny(p.Number, "\r\n/;? #") {
		return errors.New("phone number must be nonempty and contain no whitespace or URL separators")
	}
	if strings.ContainsAny(p.Extension, "\r\n") {
		return errors.New("phone extension must be a single line")
	}
	return nil
}
func (p POC) Validate() error {
	if p.ContactType != "ROLE" && p.ContactType != "PERSON" {
		return errors.New("contact_type must be ROLE or PERSON")
	}
	if p.ContactType == "ROLE" && (strings.TrimSpace(p.CompanyName) == "" || p.FirstName != "") {
		return errors.New("ROLE POCs require company_name and an empty first_name")
	}
	if p.ContactType == "PERSON" && strings.TrimSpace(p.FirstName) == "" {
		return errors.New("PERSON POCs require first_name")
	}
	if err := (Customer{Name: p.LastName, CountryCode: p.CountryCode, Subdivision: p.Subdivision, PostalCode: p.PostalCode, StreetAddress: p.StreetAddress, Comments: p.Comments}).Validate(); err != nil {
		return fmt.Errorf("invalid POC name/address: %w", err)
	}
	for _, s := range []string{p.CompanyName, p.FirstName, p.MiddleName, p.City, p.Subdivision, p.PostalCode} {
		if strings.ContainsAny(s, "\r\n") {
			return errors.New("POC scalar fields must be single lines")
		}
	}
	if len(p.Emails) == 0 {
		return errors.New("POC requires at least one email")
	}
	seen := map[string]bool{}
	for _, e := range p.Emails {
		if err := ValidatePOCEmail(e); err != nil {
			return err
		}
		if seen[e] {
			return errors.New("duplicate POC email")
		}
		seen[e] = true
	}
	office := false
	seen = map[string]bool{}
	for _, ph := range p.Phones {
		if err := ph.Validate(); err != nil {
			return err
		}
		key := ph.Type + "/" + ph.Number
		if seen[key] {
			return errors.New("duplicate phone type/number")
		}
		seen[key] = true
		if ph.Type == "O" {
			office = true
		}
	}
	if !office {
		return errors.New("POC requires at least one office phone")
	}
	return nil
}
func phoneXML(p POCPhone) pocPhoneXML {
	return pocPhoneXML{Type: pocPhoneTypeXML{Code: p.Type}, Number: p.Number, Extension: p.Extension}
}
func (p POC) marshal() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	payload := pocXML{Subdivision: p.Subdivision, Country: registrationCountryXML{Code2: p.CountryCode}, Emails: p.Emails, City: p.City, PostalCode: p.PostalCode, Comments: xmlPolicy(p.Comments), RegistrationDate: p.RegistrationDate, Handle: p.Handle, ContactType: p.ContactType, CompanyName: p.CompanyName, FirstName: p.FirstName, MiddleName: p.MiddleName, LastName: p.LastName}
	for i, line := range p.StreetAddress {
		payload.StreetAddress = append(payload.StreetAddress, irrLine{i, line})
	}
	for _, ph := range p.Phones {
		payload.Phones = append(payload.Phones, phoneXML(ph))
	}
	return xml.Marshal(payload)
}
func decodePOC(body []byte, handle string) (*POC, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "poc" || root.Name.Space != registrationNamespace {
		return nil, errors.New("ARIN returned an unexpected POC payload")
	}
	allowed := map[string]bool{}
	for _, p := range []string{"handle", "registrationDate", "contactType", "companyName", "firstName", "middleName", "lastName", "iso3166-1", "iso3166-1/code2", "iso3166-1/code3", "iso3166-1/name", "iso3166-1/e164", "iso3166-2", "city", "postalCode", "streetAddress", "streetAddress/line", "comment", "comment/line", "emails", "emails/email", "phones", "phones/phone", "phones/phone/type", "phones/phone/type/code", "phones/phone/type/description", "phones/phone/number", "phones/phone/extension"} {
		allowed[p] = true
	}
	repeated := map[string]bool{"streetAddress/line": true, "comment/line": true, "emails/email": true, "phones/phone": true}
	var walk func(*xmlNode, string) error
	walk = func(n *xmlNode, path string) error {
		for _, a := range n.Attrs {
			if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
				continue
			}
			if (path == "streetAddress/line" || path == "comment/line") && a.Name.Space == "" && a.Name.Local == "number" {
				continue
			}
			return errors.New("ARIN returned unsupported POC attributes")
		}
		counts := map[string]int{}
		for _, child := range n.Children {
			p := child.Name.Local
			if path != "" {
				p = path + "/" + p
			}
			if child.Name.Space != registrationNamespace || !allowed[p] {
				return errors.New("ARIN returned unsupported POC fields; refusing a partial representation")
			}
			counts[p]++
			if counts[p] > 1 && !repeated[p] {
				return errors.New("ARIN returned duplicate POC fields")
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
	v, err := decodeFields(root, pocFields)
	if err != nil {
		return nil, err
	}
	s := func(k string) string { return netString(v, k) }
	p := &POC{Handle: s("handle"), RegistrationDate: s("registration_date"), ContactType: s("contact_type"), CompanyName: s("company_name"), FirstName: s("first_name"), MiddleName: s("middle_name"), LastName: s("last_name"), CountryCode: s("country_code"), CountryName: s("country_name"), CountryCode3: s("country_code3"), CountryCallingCode: s("country_calling_code"), City: s("city"), Subdivision: s("subdivision"), PostalCode: s("postal_code")}
	for key, target := range map[string]*[]string{"street_address": &p.StreetAddress, "comments": &p.Comments, "emails": &p.Emails} {
		for _, x := range v[key].([]any) {
			*target = append(*target, x.(string))
		}
	}
	for _, x := range v["phones"].([]any) {
		ph := x.(map[string]any)
		p.Phones = append(p.Phones, POCPhone{Description: netString(ph, "description"), Type: netString(ph, "type"), Number: netString(ph, "number"), Extension: netString(ph, "extension")})
	}
	if !handlePattern.MatchString(p.Handle) || p.RegistrationDate == "" || (handle != "" && handle != p.Handle) {
		return nil, errors.New("ARIN returned incomplete or mismatched POC identity")
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("ARIN returned invalid POC: %w", err)
	}
	return p, nil
}
func ValidatePOCHandle(handle string) error {
	if !handlePattern.MatchString(handle) {
		return errors.New("invalid POC handle")
	}
	return nil
}
func (c *Client) GetPOC(ctx context.Context, handle string) (*POC, error) {
	if !handlePattern.MatchString(handle) {
		return nil, errors.New("invalid POC handle")
	}
	body, err := c.get(ctx, c.baseURL, "/rest/poc/"+url.PathEscape(handle), "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodePOC(body, handle)
}

// CreatePOC always links the new POC to the authenticated account so it remains manageable.
func (c *Client) CreatePOC(ctx context.Context, p POC) (*POC, error) {
	p.Handle = ""
	p.RegistrationDate = ""
	return c.writePOC(ctx, http.MethodPost, "/rest/poc;makeLink=true", p)
}
func (c *Client) UpdatePOC(ctx context.Context, p POC) (*POC, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	old, err := c.GetPOC(ctx, p.Handle)
	if err != nil {
		return nil, err
	}
	if old.ContactType != p.ContactType || old.FirstName != p.FirstName || old.MiddleName != p.MiddleName || old.LastName != p.LastName {
		return nil, errors.New("POC contact type and first, middle and last names are immutable")
	}
	p.RegistrationDate = old.RegistrationDate
	return c.writePOC(ctx, http.MethodPut, "/rest/poc/"+url.PathEscape(p.Handle), p)
}
func (c *Client) writePOC(ctx context.Context, method, path string, p POC) (*POC, error) {
	body, err := p.marshal()
	if err != nil {
		return nil, err
	}
	response, err := c.request(ctx, method, c.baseURL, path, "application/xml", true, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return nil, errors.New("ARIN did not confirm completed POC write; verify before retrying")
	}
	out, err := decodePOC(response.Body, p.Handle)
	if err != nil {
		return nil, fmt.Errorf("ARIN accepted the POC write but its result could not be verified: %w", err)
	}
	return out, nil
}
func (c *Client) DeletePOC(ctx context.Context, handle string) error {
	if !handlePattern.MatchString(handle) {
		return errors.New("invalid POC handle")
	}
	response, err := c.request(ctx, http.MethodDelete, c.baseURL, "/rest/poc/"+url.PathEscape(handle), "application/xml", true, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if response.StatusCode != 200 && response.StatusCode != 204 {
		return errors.New("ARIN did not confirm completed POC deletion")
	}
	_, err = c.GetPOC(ctx, handle)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("ARIN still returns the deleted POC")
}
