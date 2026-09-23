package arin

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

var rdapHelpSpec = ReadSpec{Name: "rdap_help", Public: true, Description: "Read ARIN RDAP service metadata, including conformance identifiers, advertised reverse-search properties and complete help JSON. No API key is sent. Advertised capabilities do not guarantee that every query is supported; individual requests remain authoritative.", Fields: []Field{
	stringsField("conformance", "Sorted conformance identifiers advertised by the server."),
	objects("reverse_search_properties", "Advertised reverse-search property combinations, sorted by resource, related resource and property.", required(text("resource_type", "Searchable resource type, such as ips or autnums.")), required(text("related_resource_type", "Related resource type, such as entity.")), required(text("property", "Search property, such as handle, fn, email or role."))),
	{Name: "rdap_json", Kind: StringKind, Description: "Complete RDAP help JSON, preserving notices, contact details, links and extensions. Links are not followed."},
}}

func (c *Client) readRDAPHelp(ctx context.Context) (map[string]any, error) {
	body, err := c.get(ctx, c.rdapBaseURL, "/registry/help", "application/rdap+json", false)
	if err != nil {
		return nil, err
	}
	if err := checkRDAPCompleteness(body); err != nil {
		return nil, err
	}
	var response struct {
		Conformance []string `json:"rdapConformance"`
		Properties  []struct {
			Resource string `json:"searchableResourceType"`
			Related  string `json:"relatedResourceType"`
			Property string `json:"property"`
		} `json:"reverse_search_properties"`
	}
	if json.Unmarshal(body, &response) != nil || !slices.Contains(response.Conformance, "rdap_level_0") {
		return nil, errors.New("ARIN returned invalid RDAP help metadata")
	}
	for _, v := range response.Conformance {
		if strings.TrimSpace(v) == "" {
			return nil, errors.New("ARIN returned an empty RDAP conformance identifier")
		}
	}
	properties := make([]any, 0, len(response.Properties))
	seen := map[string]bool{}
	for _, p := range response.Properties {
		if strings.TrimSpace(p.Resource) == "" || strings.TrimSpace(p.Related) == "" || strings.TrimSpace(p.Property) == "" {
			return nil, errors.New("ARIN returned an incomplete reverse-search property")
		}
		key := p.Resource + "\x00" + p.Related + "\x00" + p.Property
		if seen[key] {
			return nil, errors.New("ARIN returned duplicate reverse-search properties")
		}
		seen[key] = true
		properties = append(properties, map[string]any{"resource_type": p.Resource, "related_resource_type": p.Related, "property": p.Property})
	}
	slices.SortFunc(properties, func(a, b any) int {
		x, y := a.(map[string]any), b.(map[string]any)
		for _, field := range []string{"resource_type", "related_resource_type", "property"} {
			if n := strings.Compare(x[field].(string), y[field].(string)); n != 0 {
				return n
			}
		}
		return 0
	})
	compact, err := json.Marshal(json.RawMessage(body))
	if err != nil {
		return nil, errors.New("ARIN returned invalid RDAP help JSON")
	}
	return map[string]any{"conformance": anyStrings(response.Conformance), "reverse_search_properties": properties, "rdap_json": string(compact)}, nil
}
