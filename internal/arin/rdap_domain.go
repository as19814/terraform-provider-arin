package arin

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"strings"
)

var rdapDomainFields = []Field{
	required(text("name", "")), text("handle", ""), text("unicode_name", ""), stringsField("status", ""), stringsField("org_handles", ""), text("network_handle", ""),
	objects("nameservers", "", required(text("name", "")), stringsField("ipv4_addresses", ""), stringsField("ipv6_addresses", "")),
	{Name: "zone_signed", Kind: BoolKind, Description: "Published zoneSigned flag. Null if omitted; this provider does not validate DNS signatures."}, {Name: "delegation_signed", Kind: BoolKind, Description: "Published delegationSigned flag indicating DS records in the parent. Null if omitted."}, {Name: "max_sig_life", Kind: IntKind, Description: "Published maximum signature lifetime in seconds. Null if omitted."},
	objects("ds_records", "", integer("key_tag", ""), integer("algorithm", ""), integer("digest_type", ""), text("digest", "")),
	objects("key_records", "", integer("flags", ""), integer("protocol", ""), integer("algorithm", ""), text("public_key", "")),
	objects("events", "", text("action", ""), text("date", "")),
	{Name: "rdap_json", Kind: StringKind, Description: "Complete domain JSON, including embedded network/entity records, DNSSEC metadata, links and extensions. No links are followed."},
}

func rdapDomainName(name string) (string, error) {
	normalized := strings.ToLower(strings.TrimSuffix(name, "."))
	if strings.IndexFunc(name, func(r rune) bool { return r > 127 }) >= 0 || !validDNSHost(normalized) || !(strings.HasSuffix(normalized, ".in-addr.arpa") || strings.HasSuffix(normalized, ".ip6.arpa") || normalized == "in-addr.arpa" || normalized == "ip6.arpa") {
		return "", errors.New("name must be an ASCII reverse DNS domain in in-addr.arpa or ip6.arpa, with an optional trailing dot")
	}
	return normalized + ".", nil
}

func (c *Client) readRDAPDomain(ctx context.Context, name string) (map[string]any, error) {
	normalized, err := rdapDomainName(name)
	if err != nil {
		return nil, err
	}
	body, err := c.get(ctx, c.rdapBaseURL, "/registry/domain/"+url.PathEscape(normalized), "application/rdap+json", false)
	if err != nil {
		return nil, err
	}
	record, err := decodeRDAPDomain(body)
	if err != nil {
		return nil, err
	}
	if record["name"] != normalized {
		return nil, errors.New("ARIN returned a mismatched domain")
	}
	return record, nil
}

// Check notices in embedded RDAP objects as well as the domain envelope.
func checkRDAPDomainTree(body json.RawMessage, depth int) error {
	if depth > 64 {
		return errors.New("ARIN domain nesting exceeded the limit")
	}
	if err := validateEntityTree(body, 0); err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil {
		return errors.New("ARIN returned invalid domain JSON")
	}
	for _, key := range []string{"nameservers", "dsData", "keyData"} {
		if raw, ok := object[key]; ok {
			var children []json.RawMessage
			if json.Unmarshal(raw, &children) != nil {
				return errors.New("ARIN returned invalid domain children")
			}
			for _, child := range children {
				if err := checkRDAPDomainTree(child, depth+1); err != nil {
					return err
				}
			}
		}
	}
	for _, key := range []string{"network", "secureDNS"} {
		if raw, ok := object[key]; ok && string(raw) != "null" {
			if err := checkRDAPDomainTree(raw, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func optionalRDAPValue[T any](value *T) any {
	if value == nil {
		return nil
	}
	return *value
}
func rdapIntegerInRange(value *int64, max int64) bool {
	return value == nil || (*value >= 0 && *value <= max)
}

func decodeRDAPDomain(body []byte) (map[string]any, error) {
	if err := checkRDAPDomainTree(body, 0); err != nil {
		return nil, err
	}
	var domain struct {
		Class       string          `json:"objectClassName"`
		Name        string          `json:"ldhName"`
		Handle      *string         `json:"handle"`
		UnicodeName *string         `json:"unicodeName"`
		Status      []string        `json:"status"`
		Entities    []rdapEntityRef `json:"entities"`
		Network     *struct {
			Handle *string `json:"handle"`
			Class  string  `json:"objectClassName"`
		} `json:"network"`
		Nameservers []struct {
			Class       string `json:"objectClassName"`
			Name        string `json:"ldhName"`
			IPAddresses struct {
				V4 []string `json:"v4"`
				V6 []string `json:"v6"`
			} `json:"ipAddresses"`
		} `json:"nameservers"`
		SecureDNS struct {
			ZoneSigned       *bool  `json:"zoneSigned"`
			DelegationSigned *bool  `json:"delegationSigned"`
			MaxSigLife       *int64 `json:"maxSigLife"`
			DSData           []struct {
				KeyTag     *int64  `json:"keyTag"`
				Algorithm  *int64  `json:"algorithm"`
				DigestType *int64  `json:"digestType"`
				Digest     *string `json:"digest"`
			} `json:"dsData"`
			KeyData []struct {
				Flags     *int64  `json:"flags"`
				Protocol  *int64  `json:"protocol"`
				Algorithm *int64  `json:"algorithm"`
				PublicKey *string `json:"publicKey"`
			} `json:"keyData"`
		} `json:"secureDNS"`
		Events []rdapEvent `json:"events"`
	}
	if json.Unmarshal(body, &domain) != nil || domain.Class != "domain" {
		return nil, errors.New("ARIN returned an invalid domain registration")
	}
	name, err := rdapDomainName(domain.Name)
	if err != nil {
		return nil, errors.New("ARIN returned an invalid domain name")
	}
	nameservers := []any{}
	seen := map[string]bool{}
	for _, ns := range domain.Nameservers {
		host := strings.ToLower(strings.TrimSuffix(ns.Name, "."))
		if ns.Class != "nameserver" || !validDNSHost(host) || seen[host] {
			return nil, errors.New("ARIN returned invalid or duplicate domain nameservers")
		}
		seen[host] = true
		addresses := map[string][]string{"ipv4_addresses": {}, "ipv6_addresses": {}}
		for family, values := range map[string][]string{"ipv4_addresses": ns.IPAddresses.V4, "ipv6_addresses": ns.IPAddresses.V6} {
			for _, value := range values {
				address, e := netip.ParseAddr(value)
				if e != nil || address.Is4In6() || address.Zone() != "" || address.Is4() != (family == "ipv4_addresses") {
					return nil, errors.New("ARIN returned an invalid nameserver address")
				}
				addresses[family] = append(addresses[family], address.String())
			}
		}
		nameservers = append(nameservers, map[string]any{"name": host, "ipv4_addresses": anyStrings(addresses["ipv4_addresses"]), "ipv6_addresses": anyStrings(addresses["ipv6_addresses"])})
	}
	slices.SortFunc(nameservers, func(a, b any) int {
		return strings.Compare(a.(map[string]any)["name"].(string), b.(map[string]any)["name"].(string))
	})
	orgs := []string{}
	for _, entity := range domain.Entities {
		if slices.Contains(entity.Roles, "registrant") {
			if entity.Handle == "" {
				return nil, errors.New("ARIN returned a domain registrant without a handle")
			}
			orgs = append(orgs, entity.Handle)
		}
	}
	var networkHandle any
	if domain.Network != nil {
		if domain.Network.Class != "ip network" {
			return nil, errors.New("ARIN returned an invalid embedded network")
		}
		networkHandle = optionalRDAPValue(domain.Network.Handle)
	}
	dns := domain.SecureDNS
	if dns.MaxSigLife != nil && *dns.MaxSigLife < 0 {
		return nil, errors.New("ARIN returned a negative signature lifetime")
	}
	dsRecords := []any{}
	for _, ds := range dns.DSData {
		if !rdapIntegerInRange(ds.KeyTag, 65535) || !rdapIntegerInRange(ds.Algorithm, 255) || !rdapIntegerInRange(ds.DigestType, 255) {
			return nil, errors.New("ARIN returned invalid DS metadata")
		}
		if ds.Digest != nil {
			if decoded, err := hex.DecodeString(*ds.Digest); err != nil || len(decoded) == 0 {
				return nil, errors.New("ARIN returned an invalid DS digest")
			}
		}
		dsRecords = append(dsRecords, map[string]any{"key_tag": optionalRDAPValue(ds.KeyTag), "algorithm": optionalRDAPValue(ds.Algorithm), "digest_type": optionalRDAPValue(ds.DigestType), "digest": optionalRDAPValue(ds.Digest)})
	}
	keyRecords := []any{}
	for _, key := range dns.KeyData {
		if !rdapIntegerInRange(key.Flags, 65535) || !rdapIntegerInRange(key.Protocol, 255) || !rdapIntegerInRange(key.Algorithm, 255) {
			return nil, errors.New("ARIN returned invalid DNSKEY metadata")
		}
		keyRecords = append(keyRecords, map[string]any{"flags": optionalRDAPValue(key.Flags), "protocol": optionalRDAPValue(key.Protocol), "algorithm": optionalRDAPValue(key.Algorithm), "public_key": optionalRDAPValue(key.PublicKey)})
	}
	for _, records := range [][]any{dsRecords, keyRecords} {
		slices.SortFunc(records, func(a, b any) int {
			left, _ := json.Marshal(a)
			right, _ := json.Marshal(b)
			return strings.Compare(string(left), string(right))
		})
	}
	slices.SortFunc(domain.Events, func(a, b rdapEvent) int { return strings.Compare(a.Action+"/"+a.Date, b.Action+"/"+b.Date) })
	events := []any{}
	for _, event := range domain.Events {
		events = append(events, map[string]any{"action": event.Action, "date": event.Date})
	}
	raw, err := json.Marshal(json.RawMessage(body))
	if err != nil {
		return nil, fmt.Errorf("invalid domain JSON: %w", err)
	}
	return map[string]any{"name": name, "handle": optionalRDAPValue(domain.Handle), "unicode_name": optionalRDAPValue(domain.UnicodeName), "status": anyStrings(domain.Status), "org_handles": anyStrings(orgs), "network_handle": networkHandle, "nameservers": nameservers, "zone_signed": optionalRDAPValue(dns.ZoneSigned), "delegation_signed": optionalRDAPValue(dns.DelegationSigned), "max_sig_life": optionalRDAPValue(dns.MaxSigLife), "ds_records": dsRecords, "key_records": keyRecords, "events": events, "rdap_json": string(raw)}, nil
}
