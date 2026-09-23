package arin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

func validRDAPSearch(query string) bool {
	return utf8.ValidString(query) && strings.TrimSpace(query) != "" && strings.IndexFunc(query, unicode.IsControl) < 0 && strings.Count(query, "*") <= 1 && (!strings.Contains(query, "*") || strings.HasSuffix(query, "*"))
}
func validateRDAPEntitySearch(by, query string) error {
	if by != "handle" && by != "name" {
		return errors.New("search_by must be handle or name")
	}
	if !validRDAPSearch(query) {
		return errors.New("query must be nonempty text with at most one trailing wildcard and no control characters")
	}
	if by == "handle" && query != "*" && !handlePattern.MatchString(strings.TrimSuffix(query, "*")) {
		return errors.New("handle search must contain letters, digits and hyphens, optionally followed by one wildcard")
	}
	return nil
}

func (c *Client) searchRDAPEntities(ctx context.Context, by, query string) (map[string]any, error) {
	if err := validateRDAPEntitySearch(by, query); err != nil {
		return nil, err
	}
	parameter := by
	if by == "name" {
		parameter = "fn"
	}
	body, err := c.get(ctx, c.rdapBaseURL, "/registry/entities?"+url.Values{parameter: []string{query}}.Encode(), "application/rdap+json", false)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.rdapNotFound {
			return map[string]any{"entities": []any{}}, nil
		}
		return nil, err
	}
	if err := checkRDAPCompleteness(body); err != nil {
		return nil, err
	}
	var response struct {
		Results *[]json.RawMessage `json:"entitySearchResults"`
	}
	if json.Unmarshal(body, &response) != nil || response.Results == nil {
		return nil, errors.New("ARIN returned an unexpected entity search response")
	}
	records := make([]map[string]any, 0, len(*response.Results))
	seen := map[string]bool{}
	for _, raw := range *response.Results {
		record, err := decodeRDAPEntity(raw)
		if err != nil {
			return nil, err
		}
		handle := strings.ToUpper(record["handle"].(string))
		if seen[handle] {
			return nil, errors.New("ARIN returned duplicate entity search results")
		}
		seen[handle] = true
		if by == "handle" {
			target := strings.ToUpper(strings.TrimSuffix(query, "*"))
			matches := handle == target
			if strings.HasSuffix(query, "*") {
				matches = strings.HasPrefix(handle, target)
			}
			if !matches {
				return nil, errors.New("ARIN returned an entity outside the requested handle search")
			}
		}
		// ARIN name searches also match name components, so a formatted-name prefix
		// comparison would reject valid results. Leave name matching to the service.
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b map[string]any) int {
		return strings.Compare(strings.ToUpper(a["handle"].(string)), strings.ToUpper(b["handle"].(string)))
	})
	results := make([]any, 0, len(records))
	for _, record := range records {
		results = append(results, record)
	}
	return map[string]any{"entities": results}, nil
}
