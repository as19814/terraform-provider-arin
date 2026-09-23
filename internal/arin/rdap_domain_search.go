package arin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
)

func validateRDAPDomainRelation(relation string, active bool) error {
	if !slices.Contains([]string{"top", "up", "down", "bottom"}, relation) {
		return errors.New("relation must be top, up, down or bottom")
	}
	if active && (relation == "down" || relation == "bottom") {
		return errors.New("ARIN supports active_only only for top and up domain searches")
	}
	return nil
}

func (c *Client) searchRDAPDomains(ctx context.Context, name, relation string, active bool) (map[string]any, error) {
	name, err := rdapDomainName(name)
	if err != nil {
		return nil, err
	}
	if err := validateRDAPDomainRelation(relation, active); err != nil {
		return nil, err
	}
	path := "/registry/domains/rirSearch1/rdap-" + relation + "/" + url.PathEscape(name)
	if active {
		path += "?status=active"
	}
	body, err := c.get(ctx, c.rdapBaseURL, path, "application/rdap+json", false)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.rdapNotFound || ((relation == "down" || relation == "bottom") && apiErr.rdapEmptyDomains)) {
			return map[string]any{"domains": []any{}}, nil
		}
		return nil, err
	}
	if err := checkRDAPCompleteness(body); err != nil {
		return nil, err
	}
	var envelope struct {
		Class   string             `json:"objectClassName"`
		Results *[]json.RawMessage `json:"domainSearchResults"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil, errors.New("ARIN returned invalid domain search JSON")
	}
	var results []json.RawMessage
	if relation == "top" || relation == "up" {
		if envelope.Class != "domain" || envelope.Results != nil {
			return nil, errors.New("ARIN returned an unexpected single-domain search response")
		}
		results = []json.RawMessage{body}
	} else {
		if envelope.Class != "" || envelope.Results == nil {
			return nil, errors.New("ARIN returned an unexpected domain search collection")
		}
		results = *envelope.Results
	}
	records := make([]any, 0, len(results))
	seen := map[string]bool{}
	for _, raw := range results {
		record, err := decodeRDAPDomain(raw)
		if err != nil {
			return nil, err
		}
		returned := record["name"].(string)
		if seen[returned] {
			return nil, errors.New("ARIN returned duplicate domain search results")
		}
		seen[returned] = true
		ancestor := strings.HasSuffix(name, "."+returned)
		descendant := strings.HasSuffix(returned, "."+name)
		valid := false
		switch relation {
		case "top":
			valid = ancestor || returned == name
		case "up":
			valid = ancestor
		case "down":
			valid = descendant
		// ARIN includes a closest enclosing domain when leaves do not cover the query.
		case "bottom":
			valid = ancestor || descendant || returned == name
		}
		if !valid {
			return nil, errors.New("ARIN returned a domain outside the requested hierarchy")
		}
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b any) int {
		return strings.Compare(a.(map[string]any)["name"].(string), b.(map[string]any)["name"].(string))
	})
	return map[string]any{"domains": records}, nil
}
