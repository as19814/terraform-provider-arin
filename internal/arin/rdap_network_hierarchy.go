package arin

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"slices"
	"strings"
)

func validateRDAPNetworkRelation(relation string, active bool) error {
	if !slices.Contains([]string{"top", "up", "down", "bottom"}, relation) {
		return errors.New("relation must be top, up, down or bottom")
	}
	if active && (relation == "down" || relation == "bottom") {
		return errors.New("ARIN supports active_only only for top and up network searches")
	}
	return nil
}

func (c *Client) searchRDAPNetworkHierarchy(ctx context.Context, query, relation string, active bool) (map[string]any, error) {
	first, last, err := rdapNetworkQuery(query)
	if err != nil {
		return nil, err
	}
	if err := validateRDAPNetworkRelation(relation, active); err != nil {
		return nil, err
	}
	// The canonical address/prefix validator excludes query delimiters and path traversal.
	path := "/registry/ips/rirSearch1/rdap-" + relation + "/" + query
	if active {
		path += "?status=active"
	}
	body, err := c.get(ctx, c.rdapBaseURL, path, "application/rdap+json", false)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.rdapNotFound || ((relation == "down" || relation == "bottom") && apiErr.rdapEmptyNetworks)) {
			return map[string]any{"networks": []any{}}, nil
		}
		return nil, err
	}
	if err := checkRDAPCompleteness(body); err != nil {
		return nil, err
	}
	var envelope struct {
		Class   string             `json:"objectClassName"`
		Results *[]json.RawMessage `json:"ipSearchResults"`
		Error   json.RawMessage    `json:"errorCode"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Error != nil {
		return nil, errors.New("ARIN returned invalid network hierarchy JSON or an error in a successful response")
	}
	var results []json.RawMessage
	if relation == "top" || relation == "up" {
		if envelope.Class != "ip network" || envelope.Results != nil {
			return nil, errors.New("ARIN returned an unexpected single-network hierarchy response")
		}
		results = []json.RawMessage{body}
	} else {
		if envelope.Class != "" || envelope.Results == nil {
			return nil, errors.New("ARIN returned an unexpected network hierarchy collection")
		}
		results = *envelope.Results
	}
	records := make([]any, 0, len(results))
	seen := map[string]bool{}
	for _, raw := range results {
		record, err := decodeRDAPNetwork(raw)
		if err != nil {
			return nil, err
		}
		start, _ := netip.ParseAddr(record["start_address"].(string))
		end, _ := netip.ParseAddr(record["end_address"].(string))
		contains := start.Compare(first) <= 0 && end.Compare(last) >= 0
		within := start.Compare(first) >= 0 && end.Compare(last) <= 0
		equal := start == first && end == last
		valid := false
		switch relation {
		case "top":
			valid = contains
		case "up":
			valid = contains && !equal
		case "down":
			valid = within && !equal
		// Bottom can include an enclosing registration alongside more-specific children.
		// Multi-CIDR registrations can overlap a query without being wholly contained.
		case "bottom":
			valid = start.Compare(last) <= 0 && end.Compare(first) >= 0
		}
		if first.BitLen() != start.BitLen() || !valid {
			return nil, errors.New("ARIN returned a network outside the requested hierarchy")
		}
		handle := strings.ToUpper(record["handle"].(string))
		if seen[handle] {
			return nil, errors.New("ARIN returned duplicate network hierarchy handles")
		}
		seen[handle] = true
		compact, err := json.Marshal(raw)
		if err != nil {
			return nil, errors.New("ARIN returned invalid network hierarchy record JSON")
		}
		record["rdap_json"] = string(compact)
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b any) int {
		return strings.Compare(strings.ToUpper(a.(map[string]any)["handle"].(string)), strings.ToUpper(b.(map[string]any)["handle"].(string)))
	})
	return map[string]any{"networks": records}, nil
}
