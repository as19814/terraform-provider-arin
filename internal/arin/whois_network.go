package arin

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"slices"
	"strings"
)

func WhoisNetworkReads() []ReadSpec {
	net := whoisRecordSpec("net")
	flags := []Input{net.Inputs[1], optional(input("show_arin", "bool", "true", "Include ARIN's own allocation records for unallocated space. Setting false may produce a missing-record error or an empty hierarchy result."), "true")}
	fields := []Field{}
	for _, f := range net.Fields {
		if f.Name != "whois_xml" {
			fields = append(fields, f)
		}
	}
	return []ReadSpec{
		{Name: "whois_ip", Public: true, Root: "net", Fields: net.Fields, Inputs: append([]Input{input("address", "ip", "192.0.2.1", "IPv4 or IPv6 address to look up.")}, flags...), Description: "Look up the public Whois network containing an IPv4 or IPv6 address. Typed fields and complete XML are retained. No API key is sent. Missing records, partial results and referrals remain errors."},
		{Name: "whois_cidr", Public: true, Root: "net", Fields: net.Fields, Inputs: append([]Input{input("prefix", "cidr", "192.0.2.0/24", "Canonical IPv4 or IPv6 CIDR to look up. ARIN may return a larger multi-block registration containing this allocation segment.")}, flags...), Description: "Look up a public Whois network by its CIDR allocation segment. This is an exact CIDR lookup, not a most-specific containing-network search. A missing record is an error; use whois_cidr_networks with relation less for enclosing registrations. Complete XML is retained and no API key is sent."},
		{Name: "whois_cidr_networks", Public: true, Collection: true, Root: "nets", Output: "networks", Fields: fields, Inputs: append([]Input{input("prefix", "cidr", "192.0.2.0/24", "Canonical IPv4 or IPv6 CIDR."), input("relation", "opaque", "less", "Use less for enclosing networks or more for more-specific networks. Results may include the queried registration or a containing multi-block registration.")}, flags...), ResponseFields: []Field{{Name: "whois_xml", Kind: StringKind, Description: "Complete Whois hierarchy response XML. Null for a recognized no-results HTTP 404. Links are not followed."}}, Description: "List public Whois networks less or more specific than a CIDR. References are returned by default; show_details requests full records. Confirmed no matches produce an empty list; partial results and referrals remain errors. Complete XML is retained and no API key is sent."},
	}
}

func (c *Client) readWhoisNetwork(ctx context.Context, spec ReadSpec, p map[string]string) (map[string]any, error) {
	var first, last netip.Addr
	path := "/rest/"
	if spec.Name == "whois_ip" {
		first, _ = netip.ParseAddr(p["address"])
		last = first
		path += "ip/" + url.PathEscape(first.String())
	} else {
		prefix, _ := netip.ParsePrefix(p["prefix"])
		first, last = prefix.Addr(), prefixLastAddress(prefix)
		path += "cidr/" + prefix.String()
		if spec.Collection {
			path += "/" + p["relation"]
		}
	}
	query := url.Values{}
	if p["show_details"] == "true" {
		query.Set("showDetails", "true")
	}
	if p["show_arin"] == "false" {
		query.Set("showARIN", "false")
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	body, err := c.get(ctx, c.whoisBaseURL, path, "application/xml", false)
	if err != nil {
		var apiErr *APIError
		if spec.Collection && errors.As(err, &apiErr) && apiErr.whoisNoMatches {
			return map[string]any{spec.Output: []any{}, "whois_xml": nil}, nil
		}
		return nil, err
	}
	if spec.Collection {
		result, err := decodeWhoisCollection(body, spec, whoisRelation{target: "net", root: "nets"}, "", p["show_details"] == "true")
		if err != nil {
			return nil, err
		}
		for _, raw := range result[spec.Output].([]any) {
			if err := validateWhoisNetworkMatch(raw.(map[string]any), first, last, p["relation"]); err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "net" || (root.Name.Space != whoisNamespace && root.Name.Space != strings.Replace(whoisNamespace, "https:", "http:", 1)) {
		return nil, errors.New("ARIN returned an unexpected Whois network response")
	}
	if err := normalizeWhoisTree(root); err != nil {
		return nil, err
	}
	record, err := decodeWhoisRecord(root, whoisRecordSpec("net"))
	if err != nil {
		return nil, err
	}
	if err := validateWhoisNetworkMatch(record, first, last, "less"); err != nil {
		return nil, err
	}
	record["whois_xml"] = string(body)
	return record, nil
}

func validateWhoisNetworkMatch(record map[string]any, first, last netip.Addr, relation string) error {
	startText, ok1 := record["start_address"].(string)
	endText, ok2 := record["end_address"].(string)
	start, e1 := netip.ParseAddr(startText)
	end, e2 := netip.ParseAddr(endText)
	if !ok1 || !ok2 || e1 != nil || e2 != nil || start.BitLen() != first.BitLen() || end.BitLen() != first.BitLen() {
		return errors.New("ARIN returned a Whois network without a matching address family and range")
	}
	if start.Compare(last) > 0 || end.Compare(first) < 0 || (relation == "less" && (start.Compare(first) > 0 || end.Compare(last) < 0)) {
		return errors.New("ARIN returned a Whois network outside the requested range")
	}
	// Full records can contain disjoint blocks. Their outer range alone cannot
	// prove that a queried address belongs to the registration.
	blocks, _ := record["net_blocks"].([]any)
	if len(blocks) > 0 {
		type addressRange struct{ start, end netip.Addr }
		ranges := make([]addressRange, 0, len(blocks))
		for _, raw := range blocks {
			block := raw.(map[string]any)
			a, _ := netip.ParseAddr(block["start_address"].(string))
			b, _ := netip.ParseAddr(block["end_address"].(string))
			ranges = append(ranges, addressRange{a, b})
		}
		slices.SortFunc(ranges, func(a, b addressRange) int { return a.start.Compare(b.start) })
		cursor := first
		for _, block := range ranges {
			if relation == "more" {
				if block.start.Compare(last) <= 0 && block.end.Compare(first) >= 0 {
					return nil
				}
				continue
			}
			if block.end.Compare(cursor) < 0 {
				continue
			}
			if block.start.Compare(cursor) > 0 {
				break
			}
			if block.end.Compare(last) >= 0 {
				return nil
			}
			cursor = block.end.Next()
		}
		return errors.New("ARIN returned Whois network blocks that do not match the requested range")
	}
	return nil
}
