package arin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
)

func rdapResourceSearchSpec(name, output string, fields []Field) ReadSpec {
	return ReadSpec{Name: name, Public: true, Collection: true, Output: output, Description: "Search public ARIN " + output + " by registration handle/name or by an associated entity's handle/name/email. Supports a trailing wildcard and optional abuse, noc or technical role filters for entity searches. Results include associated records, not just direct ownership. No API key is sent. Confirmed no matches return an empty list; partial results and referrals are rejected.", Inputs: []Input{
		input("search_by", "name", "entity_handle", "Search field: handle, name, entity_handle, entity_name, or entity_email."),
		input("query", "rdap_search", "FT-684", "Exact search term or term ending with one wildcard (*). ARIN applies name/email matching semantics."),
		optional(input("role", "name", "any", "Entity role filter: any (omit filter), abuse, noc, or technical. Only entity searches support a role filter."), "any"),
	}, Fields: fields}
}
func validateRDAPResourceSearch(by, query, role string) error {
	if !slices.Contains([]string{"handle", "name", "entity_handle", "entity_name", "entity_email"}, by) {
		return errors.New("search_by must be handle, name, entity_handle, entity_name or entity_email")
	}
	if !validRDAPSearch(query) {
		return errors.New("query must be nonempty text with at most one trailing wildcard and no control characters")
	}
	if (by == "handle" || by == "entity_handle") && query != "*" && !handlePattern.MatchString(strings.TrimSuffix(query, "*")) {
		return errors.New("handle search must contain letters, digits and hyphens, optionally followed by one wildcard")
	}
	if !slices.Contains([]string{"any", "abuse", "noc", "technical"}, role) {
		return errors.New("role must be any, abuse, noc or technical")
	}
	if role != "any" && !strings.HasPrefix(by, "entity_") {
		return errors.New("role filters require an entity search")
	}
	return nil
}

func (c *Client) searchRDAPResources(ctx context.Context, spec, by, query, role string) (map[string]any, error) {
	if err := validateRDAPResourceSearch(by, query, role); err != nil {
		return nil, err
	}
	endpoint, output, resultKey := "ips", "networks", "ipSearchResults"
	switch spec {
	case "rdap_networks":
	case "rdap_asns":
		endpoint, output, resultKey = "autnums", "asns", "autnumSearchResults"
	default:
		return nil, errors.New("unsupported RDAP resource search")
	}
	path := "/registry/" + endpoint
	parameter := by
	if strings.HasPrefix(by, "entity_") {
		path += "/reverse_search/entity"
		parameter = strings.TrimPrefix(by, "entity_")
		if parameter == "name" {
			parameter = "fn"
		}
	}
	params := url.Values{parameter: []string{query}}
	if role != "any" {
		params.Set("role", role)
	}
	body, err := c.get(ctx, c.rdapBaseURL, path+"?"+params.Encode(), "application/rdap+json", false)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.rdapNotFound {
			return map[string]any{output: []any{}}, nil
		}
		return nil, err
	}
	if err := checkRDAPCompleteness(body); err != nil {
		return nil, err
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || envelope == nil {
		return nil, errors.New("ARIN returned invalid resource search JSON")
	}
	if _, ok := envelope["errorCode"]; ok {
		return nil, errors.New("ARIN returned an error in a successful resource search response")
	}
	var results []json.RawMessage
	if json.Unmarshal(envelope[resultKey], &results) != nil || results == nil {
		return nil, errors.New("ARIN returned an unexpected resource search response")
	}
	records := make([]any, 0, len(results))
	seen := map[string]bool{}
	for _, raw := range results {
		var record map[string]any
		if spec == "rdap_networks" {
			record, err = decodeRDAPNetwork(raw)
		} else {
			record, err = decodeRDAPASN(raw)
		}
		if err != nil {
			return nil, err
		}
		handle := strings.ToUpper(record["handle"].(string))
		if seen[handle] {
			return nil, errors.New("ARIN returned duplicate resource search handles")
		}
		seen[handle] = true
		if by == "handle" {
			target := strings.ToUpper(strings.TrimSuffix(query, "*"))
			matches := handle == target
			if strings.HasSuffix(query, "*") {
				matches = strings.HasPrefix(handle, target)
			}
			if !matches {
				return nil, errors.New("ARIN returned a registration outside the requested handle search")
			}
		}

		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b any) int {
		return strings.Compare(strings.ToUpper(a.(map[string]any)["handle"].(string)), strings.ToUpper(b.(map[string]any)["handle"].(string)))
	})
	return map[string]any{output: records}, nil
}

func decodeRDAPASN(body []byte) (map[string]any, error) {
	if err := validateEntityTree(body, 0); err != nil {
		return nil, err
	}
	var asn rdapASN
	if json.Unmarshal(body, &asn) != nil {
		return nil, errors.New("ARIN returned invalid ASN JSON")
	}
	record, err := asn.record()
	if err != nil {
		return nil, err
	}
	compact, err := json.Marshal(json.RawMessage(body))
	if err != nil {
		return nil, errors.New("ARIN returned invalid ASN JSON")
	}
	record["rdap_json"] = string(compact)
	return record, nil
}
