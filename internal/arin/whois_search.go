package arin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

type whoisSearchSpec struct {
	endpoint, target, output string
	filters                  []string
}

var whoisSearches = []whoisSearchSpec{
	{"orgs", "org", "orgs", []string{"q", "handle", "name", "dba"}},
	{"customers", "customer", "customers", []string{"q", "handle", "name"}},
	{"pocs", "poc", "pocs", []string{"q", "handle", "domain", "first", "middle", "last", "company", "city"}},
	{"asns", "asn", "asns", []string{"q", "handle", "name"}},
	{"nets", "net", "networks", []string{"q", "handle", "name"}},
}

func whoisSearch(name string) (whoisSearchSpec, bool) {
	for _, s := range whoisSearches {
		if name == "whois_"+s.endpoint {
			return s, true
		}
	}
	return whoisSearchSpec{}, false
}

func WhoisSearchReads() []ReadSpec {
	specs := []ReadSpec{}
	for _, s := range whoisSearches {
		record := whoisRecordSpec(s.target)
		fields := []Field{}
		for _, f := range record.Fields {
			if f.Name != "whois_xml" {
				fields = append(fields, f)
			}
		}
		fields = append(fields, text("reference_name", ""))
		example, _ := json.Marshal(map[string]string{"handle": record.Inputs[0].Example})
		specs = append(specs, ReadSpec{
			Name: "whois_" + s.endpoint, Public: true, Collection: true, Root: s.endpoint, Output: s.output, Fields: fields,
			Inputs:         []Input{input("filters", "whois_filters", string(example), "Nonempty map of search predicates. Allowed keys: "+strings.Join(s.filters, ", ")+". q performs a general text search across handles and names. Predicates are combined with AND. Values match case-insensitively; a single trailing * requests a prefix match. Unknown keys, empty values and other wildcard positions are rejected locally."), record.Inputs[1]},
			ResponseFields: []Field{{Name: "whois_xml", Kind: StringKind, Description: "Complete search response XML. Null for a recognized Whois no-results HTTP 404. Links are not followed."}},
			Description:    "Search public Whois-RWS " + s.endpoint + " using one or more supported filters. References are returned by default; show_details requests full records. Typed fields and complete XML are retained. No API key is sent. Partial results and referrals are errors; recognized no-results responses produce an empty list.",
		})
	}
	return specs
}

func whoisSearchFilters(name, encoded string) (map[string]string, error) {
	spec, ok := whoisSearch(name)
	if !ok {
		return nil, errors.New("unsupported Whois search")
	}
	var filters map[string]string
	if json.Unmarshal([]byte(encoded), &filters) != nil || len(filters) == 0 {
		return nil, errors.New("filters must be a nonempty map of strings")
	}
	for key, value := range filters {
		if !slices.Contains(spec.filters, key) {
			return nil, fmt.Errorf("unsupported Whois filter %q; allowed keys: %s", key, strings.Join(spec.filters, ", "))
		}
		if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.Contains(strings.TrimSuffix(value, "*"), "*") {
			return nil, fmt.Errorf("invalid Whois filter %q: use nonempty text with at most one trailing wildcard", key)
		}
	}
	return filters, nil
}

func (c *Client) readWhoisSearch(ctx context.Context, spec ReadSpec, search whoisSearchSpec, p map[string]string) (map[string]any, error) {
	filters, err := whoisSearchFilters(spec.Name, p["filters"])
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	path := "/rest/" + search.endpoint
	for _, key := range keys {
		// Matrix values need every delimiter escaped, including semicolons and equals.
		path += ";" + key + "=" + strings.ReplaceAll(url.QueryEscape(filters[key]), "+", "%20")
	}
	if p["show_details"] == "true" {
		path += "?showDetails=true"
	}
	body, err := c.get(ctx, c.whoisBaseURL, path, "application/xml", false)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.whoisNoMatches {
			return map[string]any{spec.Output: []any{}, "whois_xml": nil}, nil
		}
		return nil, err
	}
	return decodeWhoisCollection(body, spec, whoisRelation{target: search.target, root: spec.Root}, "", p["show_details"] == "true")
}
