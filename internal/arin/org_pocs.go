package arin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type OrgPOC struct{ Handle, Function, Description string }

func ValidateOrgPOCFunction(function string) error {
	switch function {
	case "T", "N", "AB", "R", "D":
		return nil
	case "AD":
		return errors.New("Admin POC changes require a full organization update")
	default:
		return errors.New("POC function must be T, N, AB, R or D")
	}
}
func (c *Client) GetOrganizationPOCs(ctx context.Context, org string) ([]OrgPOC, error) {
	if !handlePattern.MatchString(org) {
		return nil, errors.New("invalid organization handle")
	}
	body, err := c.get(ctx, c.baseURL, "/rest/org/"+url.PathEscape(org), "application/xml", true)
	if err != nil {
		return nil, err
	}
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "org" || root.Name.Space != registrationNamespace {
		return nil, errors.New("ARIN returned an unexpected organization payload")
	}
	containers := 0
	for _, n := range root.Children {
		if n.Name.Local != "pocLinks" {
			continue
		}
		containers++
		if n.Name.Space != registrationNamespace {
			return nil, errors.New("ARIN returned an unexpected POC link namespace")
		}
		for _, link := range n.Children {
			if link.Name.Local != "pocLinkRef" || link.Name.Space != registrationNamespace || len(link.Children) != 0 {
				return nil, errors.New("ARIN returned unsupported POC link content")
			}
			attrs := map[string]bool{}
			for _, a := range link.Attrs {
				if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
					continue
				}
				if a.Name.Space != "" || (a.Name.Local != "handle" && a.Name.Local != "function" && a.Name.Local != "description") || attrs[a.Name.Local] {
					return nil, errors.New("ARIN returned unsupported POC link attributes")
				}
				attrs[a.Name.Local] = true
			}
		}
	}
	if containers != 1 {
		return nil, errors.New("ARIN did not return one complete organization POC collection")
	}
	values, err := decodeFields(root, orgFields)
	if err != nil {
		return nil, err
	}
	if netString(values, "handle") != org {
		return nil, errors.New("ARIN returned a mismatched organization")
	}
	result := []OrgPOC{}
	seen := map[string]bool{}
	for _, item := range values["poc_links"].([]any) {
		v := item.(map[string]any)
		p := OrgPOC{Handle: netString(v, "handle"), Function: netString(v, "function"), Description: netString(v, "description")}
		if !handlePattern.MatchString(p.Handle) {
			return nil, errors.New("ARIN returned an invalid organization POC handle")
		}
		if p.Function != "AD" {
			if err := ValidateOrgPOCFunction(p.Function); err != nil {
				return nil, fmt.Errorf("ARIN returned an unsupported POC role: %w", err)
			}
		}
		key := p.Handle + "/" + p.Function
		if seen[key] {
			return nil, errors.New("ARIN returned duplicate organization POC links")
		}
		seen[key] = true
		result = append(result, p)
	}
	return result, nil
}
func (c *Client) AddOrganizationPOC(ctx context.Context, org, poc, function string) ([]OrgPOC, error) {
	if !handlePattern.MatchString(org) || !handlePattern.MatchString(poc) {
		return nil, errors.New("invalid organization or POC handle")
	}
	if err := ValidateOrgPOCFunction(function); err != nil {
		return nil, err
	}
	out, err := c.mutateOrgPOCs(ctx, http.MethodPut, "/rest/org/"+url.PathEscape(org)+"/poc/"+url.PathEscape(poc)+";pocFunction="+function, org)
	if err != nil {
		return nil, err
	}
	for _, p := range out {
		if p.Handle == poc && p.Function == function {
			return out, nil
		}
	}
	return nil, errors.New("ARIN did not return the added organization POC link")
}

// RemoveOrganizationPOC removes exactly one association. OT&E rejects the
// documented handle-only and function-only deletion routes.
func (c *Client) RemoveOrganizationPOC(ctx context.Context, org, poc, function string) ([]OrgPOC, error) {
	if !handlePattern.MatchString(org) || !handlePattern.MatchString(poc) {
		return nil, errors.New("organization and POC handles are required")
	}
	if err := ValidateOrgPOCFunction(function); err != nil {
		return nil, err
	}
	path := "/rest/org/" + url.PathEscape(org) + "/poc/" + url.PathEscape(poc) + ";pocFunction=" + function
	out, err := c.mutateOrgPOCs(ctx, http.MethodDelete, path, org)
	if err != nil {
		return nil, err
	}
	for _, p := range out {
		if p.Handle == poc && p.Function == function {
			return nil, errors.New("ARIN still returns the removed organization POC link")
		}
	}
	return out, nil
}
func (c *Client) mutateOrgPOCs(ctx context.Context, method, path, org string) ([]OrgPOC, error) {
	unlock, err := lockOrganization(ctx, c.baseURL, org)
	if err != nil {
		return nil, err
	}
	defer unlock()
	response, err := c.request(ctx, method, c.baseURL, path, "application/xml", true, nil)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 && response.StatusCode != 204 {
		return nil, errors.New("ARIN did not confirm completed organization POC modification")
	}
	return c.GetOrganizationPOCs(ctx, org)
}
