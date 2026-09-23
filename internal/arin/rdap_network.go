package arin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
)

var rdapNetworkFields = []Field{
	required(text("handle", "")), text("name", ""), text("network_type", ""), required(text("ip_version", "")), required(text("start_address", "")), required(text("end_address", "")), text("parent_handle", ""), text("country", ""), stringsField("cidrs", ""), stringsField("org_handles", ""), stringsField("status", ""), objects("events", "", text("action", ""), text("date", "")),
}

// rdapNetworkQuery requires canonical forms so the lookup identity is stable.
func rdapNetworkQuery(query string) (netip.Addr, netip.Addr, error) {
	if strings.Contains(query, "/") {
		prefix, err := netip.ParsePrefix(query)
		if err != nil || prefix != prefix.Masked() || prefix.Addr().Is4In6() || prefix.String() != query {
			return netip.Addr{}, netip.Addr{}, errors.New("query must be a canonical IPv4/IPv6 address or network prefix without host bits")
		}
		return prefix.Addr(), prefixLastAddress(prefix), nil
	}
	address, err := netip.ParseAddr(query)
	if err != nil || address.Is4In6() || address.Zone() != "" || address.String() != query {
		return netip.Addr{}, netip.Addr{}, errors.New("query must be a canonical IPv4/IPv6 address or network prefix without host bits")
	}
	return address, address, nil
}
func (c *Client) readRDAPNetwork(ctx context.Context, query string) (map[string]any, error) {
	first, last, err := rdapNetworkQuery(query)
	if err != nil {
		return nil, err
	}
	body, err := c.get(ctx, c.rdapBaseURL, "/registry/ip/"+query, "application/rdap+json", false)
	if err != nil {
		return nil, err
	}
	if err := checkRDAPCompleteness(body); err != nil {
		return nil, err
	}
	var n struct {
		rdapNetwork
		Parent   *string         `json:"parentHandle"`
		Country  *string         `json:"country"`
		Entities []rdapEntityRef `json:"entities"`
		Status   []string        `json:"status"`
		Events   []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
	}
	if json.Unmarshal(body, &n) != nil {
		return nil, errors.New("ARIN returned invalid network JSON")
	}
	start, e1 := netip.ParseAddr(n.StartAddress)
	end, e2 := netip.ParseAddr(n.EndAddress)
	if n.ObjectClassName != "ip network" || n.Handle == "" || e1 != nil || e2 != nil || start.Is4In6() || end.Is4In6() || start.Zone() != "" || end.Zone() != "" || start.BitLen() != end.BitLen() || start.Compare(end) > 0 || (n.IPVersion != "v4" && n.IPVersion != "v6") || (n.IPVersion == "v4") != start.Is4() {
		return nil, errors.New("ARIN returned an invalid network registration")
	}
	if first.BitLen() != start.BitLen() || first.Compare(start) < 0 || last.Compare(end) > 0 {
		return nil, errors.New("ARIN returned a network that does not contain the requested address or prefix")
	}
	cidrs := []string{}
	prefixes := []netip.Prefix{}
	for _, block := range n.CIDRs {
		address := block.V4Prefix
		if n.IPVersion == "v6" {
			address = block.V6Prefix
		}
		if block.Length == nil || (block.V4Prefix != "" && block.V6Prefix != "") {
			return nil, errors.New("ARIN returned an invalid network CIDR")
		}
		prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", address, *block.Length))
		if err != nil || prefix != prefix.Masked() || prefix.Addr().Is4In6() || prefix.Addr().BitLen() != start.BitLen() || prefix.Addr().Compare(start) < 0 || prefixLastAddress(prefix).Compare(end) > 0 {
			return nil, errors.New("ARIN returned an invalid network CIDR")
		}
		prefixes = append(prefixes, prefix)
		cidrs = append(cidrs, prefix.String())
	}
	if len(prefixes) > 0 {
		slices.SortFunc(prefixes, func(a, b netip.Prefix) int { return a.Addr().Compare(b.Addr()) })
		next := start
		for _, prefix := range prefixes {
			if prefix.Addr() != next {
				return nil, errors.New("ARIN returned overlapping or incomplete network CIDRs")
			}
			next = prefixLastAddress(prefix).Next()
		}
		if next != end.Next() {
			return nil, errors.New("ARIN returned incomplete network CIDRs")
		}
	}
	orgs := []string{}
	for _, entity := range n.Entities {
		if incompleteNotices(entity.Notices) || incompleteNotices(entity.Remarks) {
			return nil, errors.New("ARIN returned a truncated network entity")
		}
		if slices.Contains(entity.Roles, "registrant") {
			if entity.Handle == "" {
				return nil, errors.New("ARIN returned a registrant without a handle")
			}
			orgs = append(orgs, entity.Handle)
		}
	}
	slices.SortFunc(n.Events, func(a, b struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	}) int {
		return strings.Compare(a.Action+"/"+a.Date, b.Action+"/"+b.Date)
	})
	events := make([]any, 0, len(n.Events))
	for _, event := range n.Events {
		events = append(events, map[string]any{"action": event.Action, "date": event.Date})
	}
	var parent, country any
	if n.Parent != nil {
		parent = *n.Parent
	}
	if n.Country != nil {
		country = *n.Country
	}
	return map[string]any{"handle": n.Handle, "name": n.Name, "network_type": n.Type, "ip_version": n.IPVersion, "start_address": start.String(), "end_address": end.String(), "parent_handle": parent, "country": country, "cidrs": anyStrings(cidrs), "org_handles": anyStrings(orgs), "status": anyStrings(n.Status), "events": events}, nil
}
