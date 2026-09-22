package arin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if IsNotFound(err) {
		// A reverse search with no matches returns 404. Confirm the entity exists
		// before treating it as an organization with an empty network inventory.
		entityBody, entityErr := c.get(ctx, c.rdapBaseURL, "/registry/entity/"+url.PathEscape(handle), "application/rdap+json", false)
		if entityErr != nil {
			return nil, entityErr
		}
		var entity struct {
			Handle          string `json:"handle"`
			ObjectClassName string `json:"objectClassName"`
		}
		if json.Unmarshal(entityBody, &entity) != nil || !strings.EqualFold(entity.Handle, handle) || entity.ObjectClassName != "entity" {
			return nil, errors.New("ARIN returned an invalid RDAP entity response")
		}
		return []Network{}, nil
	}
	if err != nil {
		return nil, err
	}
	var result struct {
		Networks *[]rdapNetwork `json:"ipSearchResults"`
		Notices  []rdapNotice   `json:"notices"`
		Links    []struct {
			Rel string `json:"rel"`
		} `json:"links"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Networks == nil {
		return nil, errors.New("ARIN returned an invalid or unexpected RDAP search response")
	}
	if incompleteNotices(result.Notices) {
		return nil, errors.New("ARIN truncated the network search; refusing to return an incomplete inventory")
	}
	for _, link := range result.Links {
		if strings.EqualFold(link.Rel, "next") {
			return nil, errors.New("ARIN returned a paginated network search; pagination is not supported, so no partial inventory was returned")
		}
	}
	networks := make([]Network, 0, len(*result.Networks))
	seen := make(map[string]bool)
	for _, n := range *result.Networks {
		if incompleteNotices(n.Notices) || incompleteNotices(n.Remarks) {
			return nil, errors.New("ARIN returned a truncated network object; refusing to return an incomplete inventory")
		}
		registrant := false
		for _, entity := range n.Entities {
			if strings.EqualFold(entity.Handle, handle) && slices.Contains(entity.Roles, "registrant") {
				registrant = true
				break
			}
		}
		if !registrant {
			continue
		}
		start, e1 := netip.ParseAddr(n.StartAddress)
		end, e2 := netip.ParseAddr(n.EndAddress)
		if n.Handle == "" || n.Name == "" || n.ObjectClassName != "ip network" || e1 != nil || e2 != nil || start.BitLen() != end.BitLen() || start.Compare(end) > 0 ||
			(n.IPVersion != "v4" && n.IPVersion != "v6") || (n.IPVersion == "v4") != start.Is4() {
			return nil, errors.New("ARIN returned an invalid network registration")
		}
		if seen[n.Handle] {
			return nil, errors.New("ARIN returned duplicate network handles")
		}
		seen[n.Handle] = true
		network := Network{Handle: n.Handle, Name: n.Name, IPVersion: n.IPVersion, Type: n.Type, StartAddress: start.String(), EndAddress: end.String(), CIDRs: []string{}}
		for _, block := range n.CIDRs {
			address := block.V4Prefix
			if n.IPVersion == "v6" {
				address = block.V6Prefix
			}
			if block.Length == nil {
				return nil, errors.New("ARIN returned a CIDR without a prefix length")
			}
			prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", address, *block.Length))
			if err != nil || prefix.Addr().BitLen() != start.BitLen() || prefix != prefix.Masked() || prefix.Addr().Compare(start) < 0 || prefix.Addr().Compare(end) > 0 || prefixLastAddress(prefix).Compare(end) > 0 {
				return nil, errors.New("ARIN returned an invalid network CIDR")
			}
			network.CIDRs = append(network.CIDRs, prefix.String())
		}
		slices.Sort(network.CIDRs)
		network.CIDRs = slices.Compact(network.CIDRs)
		networks = append(networks, network)
	}
	slices.SortFunc(networks, func(a, b Network) int { return strings.Compare(a.Handle, b.Handle) })
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
