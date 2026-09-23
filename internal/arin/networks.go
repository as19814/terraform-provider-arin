package arin

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"slices"
	"strings"
)

// Network is a public registration, not proof that an API key can modify it.
type Network struct {
	Handle       string
	Name         string
	IPVersion    string
	Type         string
	StartAddress string
	EndAddress   string
	CIDRs        []string
	RDAPJSON     string
}

type rdapNotice struct {
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Description []string `json:"description"`
}

type rdapNetwork struct {
	Handle          string `json:"handle"`
	Name            string `json:"name"`
	IPVersion       string `json:"ipVersion"`
	Type            string `json:"type"`
	StartAddress    string `json:"startAddress"`
	EndAddress      string `json:"endAddress"`
	ObjectClassName string `json:"objectClassName"`
	Entities        []struct {
		Handle string   `json:"handle"`
		Roles  []string `json:"roles"`
	} `json:"entities"`
	CIDRs []struct {
		V4Prefix string `json:"v4prefix"`
		V6Prefix string `json:"v6prefix"`
		Length   *int   `json:"length"`
	} `json:"cidr0_cidrs"`
	Notices []rdapNotice `json:"notices"`
	Remarks []rdapNotice `json:"remarks"`
}

func incompleteNotices(notices []rdapNotice) bool {
	for _, n := range notices {
		text := strings.ToLower(n.Title + " " + n.Type + " " + strings.Join(n.Description, " "))
		if strings.Contains(text, "truncat") || strings.Contains(text, "incomplete") {
			return true
		}
	}
	return false
}

// ListOrganizationNetworks discovers registrations via public RDAP. No API key
// is sent. Associated contacts are filtered out by checking the registrant role.
func (c *Client) ListOrganizationNetworks(ctx context.Context, handle string) ([]Network, error) {
	if !handlePattern.MatchString(handle) {
		return nil, errors.New("organization handle must contain only letters, digits, and hyphens, starting with a letter or digit")
	}
	if c.rdapBaseURL == "" {
		return nil, errors.New("rdap_base_url is required for network discovery when base_url is a custom origin")
	}
	query := url.Values{"handle": []string{handle}}
	body, err := c.get(ctx, c.rdapBaseURL, "/registry/ips/reverse_search/entity?"+query.Encode(), "application/rdap+json", false)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.rdapNotFound {
		// Confirm an entity exists before accepting its empty inventory.
		if _, entityErr := c.rdapEntity(ctx, handle); entityErr != nil {
			return nil, entityErr
		}
		return []Network{}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := checkRDAPCompleteness(body); err != nil {
		return nil, err
	}
	var result struct {
		Networks *[]json.RawMessage `json:"ipSearchResults"`
	}
	if json.Unmarshal(body, &result) != nil || result.Networks == nil {
		return nil, errors.New("ARIN returned an invalid or unexpected RDAP search response")
	}
	networks := make([]Network, 0, len(*result.Networks))
	seen := map[string]bool{}
	for _, raw := range *result.Networks {
		record, err := decodeRDAPNetwork(raw)
		if err != nil {
			return nil, err
		}
		belongs := false
		for _, org := range record["org_handles"].([]any) {
			if strings.EqualFold(org.(string), handle) {
				belongs = true
			}
		}
		if !belongs {
			continue
		}
		key := strings.ToUpper(record["handle"].(string))
		if seen[key] {
			return nil, errors.New("ARIN returned duplicate network handles")
		}
		seen[key] = true
		cidrs := []string{}
		for _, cidr := range record["cidrs"].([]any) {
			cidrs = append(cidrs, cidr.(string))
		}
		networks = append(networks, Network{Handle: record["handle"].(string), Name: record["name"].(string), IPVersion: record["ip_version"].(string), Type: record["network_type"].(string), StartAddress: record["start_address"].(string), EndAddress: record["end_address"].(string), CIDRs: cidrs, RDAPJSON: record["rdap_json"].(string)})
	}
	slices.SortFunc(networks, func(a, b Network) int { return strings.Compare(strings.ToUpper(a.Handle), strings.ToUpper(b.Handle)) })
	return networks, nil
}

func prefixLastAddress(prefix netip.Prefix) netip.Addr {
	bytes := prefix.Addr().AsSlice()
	for bit := prefix.Bits(); bit < len(bytes)*8; bit++ {
		bytes[bit/8] |= 1 << (7 - bit%8)
	}
	address, _ := netip.AddrFromSlice(bytes)
	return address
}
