package arin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
)

var rdapDomainsByNameserverSpec = ReadSpec{Name: "rdap_domains_by_nameserver", Public: true, Collection: true, Output: "domains", Description: "Find public reverse-domain registrations using an exact nameserver name. Returns complete domain records sorted by name, including DNSSEC and raw JSON. No API key is sent. No matches produce an empty list; partial results and referrals are rejected. ARIN does not support wildcards for this query.", Inputs: []Input{input("nameserver", "dns_host", "ns1.arin.net", "Exact ASCII nameserver hostname. Case and an optional trailing dot are normalized; no wildcards.")}, Fields: rdapDomainFields}

func rdapNameserverName(name string) (string, error) {
	normalized := strings.ToLower(strings.TrimSuffix(name, "."))
	if strings.IndexFunc(name, func(r rune) bool { return r > 127 }) >= 0 || !validDNSHost(normalized) {
		return "", errors.New("nameserver must be an ASCII hostname without wildcards, with an optional trailing dot")
	}
	return normalized, nil
}
func (c *Client) searchRDAPDomainsByNameserver(ctx context.Context, name string) (map[string]any, error) {
	normalized, err := rdapNameserverName(name)
	if err != nil {
		return nil, err
	}
	path := "/registry/domains?" + url.Values{"nsLdhName": []string{normalized}}.Encode()
	body, err := c.get(ctx, c.rdapBaseURL, path, "application/rdap+json", false)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.rdapNotFound || apiErr.rdapEmptyDomains) {
			return map[string]any{"domains": []any{}}, nil
		}
		return nil, err
	}
	if err := checkRDAPCompleteness(body); err != nil {
		return nil, err
	}
	var response struct {
		Results *[]json.RawMessage `json:"domainSearchResults"`
	}
	if json.Unmarshal(body, &response) != nil || response.Results == nil {
		return nil, errors.New("ARIN returned an unexpected nameserver domain search response")
	}
	records := make([]any, 0, len(*response.Results))
	seen := map[string]bool{}
	for _, raw := range *response.Results {
		record, err := decodeRDAPDomain(raw)
		if err != nil {
			return nil, err
		}
		name := record["name"].(string)
		if seen[name] {
			return nil, errors.New("ARIN returned duplicate domain search names")
		}
		seen[name] = true
		matched := false
		for _, ns := range record["nameservers"].([]any) {
			if ns.(map[string]any)["name"] == normalized {
				matched = true
			}
		}
		if !matched {
			return nil, errors.New("ARIN returned a domain without the requested nameserver")
		}
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b any) int {
		return strings.Compare(a.(map[string]any)["name"].(string), b.(map[string]any)["name"].(string))
	})
	return map[string]any{"domains": records}, nil
}
