package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
)

func (c *Client) mutatePOCContact(ctx context.Context, method, path, handle string, body []byte) (*POC, error) {
	response, err := c.request(ctx, method, c.baseURL, path, "application/xml", true, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 && response.StatusCode != 204 {
		return nil, errors.New("ARIN did not confirm completed POC contact modification")
	}
	// Phone methods return a phone or payload list, whereas email methods return
	// the POC. Read the complete record to verify the resulting contact collection.
	return c.GetPOC(ctx, handle)
}
func (c *Client) AddPOCEmail(ctx context.Context, handle, email string) (*POC, error) {
	if err := ValidatePOCHandle(handle); err != nil {
		return nil, err
	}
	if err := ValidatePOCEmail(email); err != nil {
		return nil, err
	}
	p, err := c.mutatePOCContact(ctx, http.MethodPost, "/rest/poc/"+url.PathEscape(handle)+"/email/"+url.PathEscape(email), handle, nil)
	if err != nil {
		return nil, err
	}
	for _, e := range p.Emails {
		if e == email {
			return p, nil
		}
	}
	return nil, errors.New("ARIN did not return the added POC email")
}
func (c *Client) DeletePOCEmail(ctx context.Context, handle, email string) (*POC, error) {
	if err := ValidatePOCHandle(handle); err != nil {
		return nil, err
	}
	if err := ValidatePOCEmail(email); err != nil {
		return nil, err
	}
	p, err := c.mutatePOCContact(ctx, http.MethodDelete, "/rest/poc/"+url.PathEscape(handle)+"/email/"+url.PathEscape(email), handle, nil)
	if err != nil {
		return nil, err
	}
	for _, e := range p.Emails {
		if e == email {
			return nil, errors.New("ARIN still returns the deleted POC email")
		}
	}
	return p, nil
}
func (c *Client) AddPOCPhone(ctx context.Context, handle string, phone POCPhone) (*POC, error) {
	if err := ValidatePOCHandle(handle); err != nil {
		return nil, err
	}
	if err := phone.Validate(); err != nil {
		return nil, err
	}
	existing, err := c.GetPOC(ctx, handle)
	if err != nil {
		return nil, err
	}
	for _, old := range existing.Phones {
		if old.Type == phone.Type && old.Number == phone.Number {
			if old.Extension != phone.Extension {
				return nil, errors.New("the individual phone endpoint cannot change an existing extension; use a full POC update or remove and re-add the phone")
			}
			return existing, nil
		}
	}
	body, err := xml.Marshal(phoneXML(phone))
	if err != nil {
		return nil, err
	}
	p, err := c.mutatePOCContact(ctx, http.MethodPut, "/rest/poc/"+url.PathEscape(handle)+"/phone", handle, body)
	if err != nil {
		return nil, err
	}
	for _, ph := range p.Phones {
		if ph == phone {
			return p, nil
		}
	}
	return nil, errors.New("ARIN did not return the configured POC phone")
}

// DeletePOCPhones removes records matching both supplied selectors. An empty
// number matches every phone of the type; an empty type matches every type for
// that number. At least one selector is required.
func (c *Client) DeletePOCPhones(ctx context.Context, handle, number, kind string) (*POC, error) {
	if err := ValidatePOCHandle(handle); err != nil {
		return nil, err
	}
	if number == "" && kind == "" {
		return nil, errors.New("phone deletion requires a number or type")
	}
	if kind != "" && kind != "O" && kind != "F" && kind != "M" {
		return nil, errors.New("phone type must be O, F or M")
	}
	if number != "" {
		validationType := kind
		if validationType == "" {
			validationType = "O"
		}
		if err := (POCPhone{Type: validationType, Number: number}).Validate(); err != nil {
			return nil, err
		}
	}
	path := "/rest/poc/" + url.PathEscape(handle) + "/phone/" + url.PathEscape(number)
	if kind != "" {
		path += ";type=" + kind
	}
	p, err := c.mutatePOCContact(ctx, http.MethodDelete, path, handle, nil)
	if err != nil {
		return nil, err
	}
	for _, ph := range p.Phones {
		if (number == "" || ph.Number == number) && (kind == "" || ph.Type == kind) {
			return nil, errors.New("ARIN still returns a deleted POC phone")
		}
	}
	return p, nil
}
