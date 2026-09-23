package arin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

var asnFields = []Field{
	rdapJSONField, text("asn_type", "Registration type returned by ARIN."),
	required(text("handle", "")), text("name", ""), required(integer("start_asn", "")), required(integer("end_asn", "")), text("country", ""), stringsField("org_handles", ""), stringsField("status", ""),
	objects("events", "", text("action", ""), text("date", "")),
}

func PublicReads() []ReadSpec {
	return []ReadSpec{
		rdapHelpSpec, rdapDomainsByNameserverSpec,
		{Name: "rdap_network_hierarchy", Public: true, Collection: true, Output: "networks", Description: "Search public IPv4/IPv6 network hierarchy: top (least-specific covering network), up (strict parent), down (immediate children), or bottom (most-specific networks, including enclosing registrations where needed). All relations return a list sorted by handle; confirmed no matches return an empty list. No API key is sent. Partial results and referrals are rejected.", Inputs: []Input{input("query", "ip_network", "192.0.2.0/24", "Canonical IPv4/IPv6 address or network prefix without host bits."), input("relation", "name", "up", "Hierarchy relation: top, up, down or bottom."), optional(input("active_only", "bool", "false", "Apply ARIN status=active filtering. Supported only for top and up; ARIN determines which records are active."), "false")}, Fields: rdapNetworkFields},
		rdapResourceSearchSpec("rdap_networks", "networks", rdapNetworkFields),
		rdapResourceSearchSpec("rdap_asns", "asns", asnFields),
		{Name: "rdap_domains", Public: true, Collection: true, Output: "domains", Description: "Search public reverse-domain hierarchy: top (least-specific covering domain), up (parent), down (immediate children), or bottom (most-specific domains, including an enclosing domain when needed). Returns a list for all relations, including an empty list for confirmed no matches. No API key is sent; incomplete results and referrals are rejected.", Inputs: []Input{input("name", "rdap_domain", "2.0.192.in-addr.arpa.", "Reverse DNS domain to search relative to; case and an optional trailing dot are normalized."), input("relation", "name", "up", "Hierarchy relation: top, up, down or bottom."), optional(input("active_only", "bool", "false", "Apply ARIN status=active filtering. Supported only for top and up; ARIN determines which records are active."), "false")}, Fields: rdapDomainFields},
		{Name: "rdap_domain", Public: true, Description: "Read a public reverse-domain registration from ARIN RDAP, including nameservers, published DNSSEC data and complete JSON. No API key is sent. Forward domains, referrals and partial responses are not supported. This reads registration data, not live DNS or DNSSEC validation results.", Inputs: []Input{input("name", "rdap_domain", "2.0.192.in-addr.arpa.", "Reverse DNS domain in in-addr.arpa or ip6.arpa. Case and an optional trailing dot are normalized for lookup.")}, Fields: rdapDomainFields},
		{Name: "rdap_entities", Public: true, Collection: true, Output: "entities", Description: "Search public RDAP entities by handle or name, including organizations, POCs and customer entities returned by ARIN. Supports one trailing wildcard. No API key is sent. Results use ARIN search semantics and are sorted by handle; partial results, duplicates and referrals are rejected. A structured RDAP no-match response produces an empty list.", Inputs: []Input{input("search_by", "name", "handle", "Search field: handle or name (mapped to RDAP fn)."), input("query", "rdap_search", "EXAMPLE-*", "Exact search term or a term ending in one wildcard (*). Name matching is performed by ARIN, including its name-component matching rules.")}, Fields: rdapEntityFields},
		{Name: "rdap_entity", Public: true, Description: "Look up a public organization or POC entity by handle. No API key is sent. Returns contact fields and complete jCard/RDAP JSON, including embedded records and extensions. Public data may omit private registration fields. Partial results and referrals are rejected.", Inputs: []Input{input("handle", "handle", "EXAMPLE-1", "Organization or POC handle.")}, Fields: rdapEntityFields},
		{Name: "rdap_network", Public: true, Description: "Look up the public ARIN network registration containing an IPv4/IPv6 address or canonical prefix. No API key is sent. Returns registration data, not proof of authority to modify the network. Referrals and partial results are rejected.", Inputs: []Input{input("query", "ip_network", "192.0.2.1", "Canonical IPv4/IPv6 address or network prefix without host bits.")}, Fields: rdapNetworkFields},
		{Name: "asn", Public: true, Description: "Read an ASN registration through public ARIN RDAP. No API key is sent. This is distinct from an IRR aut-num object.", Inputs: []Input{input("asn", "asn", "19814", "Autonomous system number.")}, Fields: asnFields},
		{Name: "asns", Public: true, Collection: true, Output: "asns", Description: "List ASN registrations where an organization is the direct registrant using public RDAP. No API key is needed. Incomplete results are rejected.", Inputs: []Input{orgInput}, Fields: asnFields},
		{Name: "org_pocs", Public: true, Collection: true, Output: "pocs", Description: "List public contact handles and roles linked directly to an ARIN organization. Use arin_poc to read contact details through Reg-RWS. No API key is needed.", Inputs: []Input{orgInput}, Fields: []Field{required(text("handle", "")), stringsField("roles", "")}},
	}
}

type rdapEntityRef struct {
	Handle  string       `json:"handle"`
	Roles   []string     `json:"roles"`
	Notices []rdapNotice `json:"notices"`
	Remarks []rdapNotice `json:"remarks"`
}
type rdapASN struct {
	Handle   string          `json:"handle"`
	Name     string          `json:"name"`
	Class    string          `json:"objectClassName"`
	Start    *int64          `json:"startAutnum"`
	End      *int64          `json:"endAutnum"`
	Country  *string         `json:"country"`
	Type     *string         `json:"type"`
	Status   []string        `json:"status"`
	Entities []rdapEntityRef `json:"entities"`
	Events   []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
	Notices []rdapNotice `json:"notices"`
	Remarks []rdapNotice `json:"remarks"`
}

func anyStrings(values []string) []any {
	copy := slices.Clone(values)
	slices.Sort(copy)
	copy = slices.Compact(copy)
	result := make([]any, 0, len(copy))
	for _, v := range copy {
		result = append(result, v)
	}
	return result
}
func (n rdapASN) record() (map[string]any, error) {
	if n.Class != "autnum" || n.Handle == "" || n.Start == nil || n.End == nil || *n.Start < 0 || *n.End > 4294967295 || *n.Start > *n.End {
		return nil, errors.New("ARIN returned an invalid ASN registration")
	}
	if incompleteNotices(n.Notices) || incompleteNotices(n.Remarks) {
		return nil, errors.New("ARIN returned a truncated ASN registration")
	}
	var orgs []string
	for _, entity := range n.Entities {
		if incompleteNotices(entity.Notices) || incompleteNotices(entity.Remarks) {
			return nil, errors.New("ARIN returned a truncated ASN entity")
		}
		if slices.Contains(entity.Roles, "registrant") {
			if entity.Handle == "" {
				return nil, errors.New("ARIN returned an ASN registrant without a handle")
			}
			orgs = append(orgs, entity.Handle)
		}
	}
	events := make([]any, 0, len(n.Events))
	slices.SortFunc(n.Events, func(a, b struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	}) int {
		return strings.Compare(a.Action+"/"+a.Date, b.Action+"/"+b.Date)
	})
	for _, event := range n.Events {
		events = append(events, map[string]any{"action": event.Action, "date": event.Date})
	}
	var country any
	if n.Country != nil {
		country = *n.Country
	}
	return map[string]any{"asn_type": optionalRDAPValue(n.Type), "handle": n.Handle, "name": n.Name, "start_asn": *n.Start, "end_asn": *n.End, "country": country, "org_handles": anyStrings(orgs), "status": anyStrings(n.Status), "events": events}, nil
}

// Error payloads still need truncation and pagination checks before a 404 can mean no matches.
func checkRDAPErrorCompleteness(body []byte) error {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || envelope == nil {
		return errors.New("ARIN returned invalid RDAP error JSON")
	}
	delete(envelope, "errorCode")
	filtered, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return checkRDAPCompleteness(filtered)
}

func checkRDAPCompleteness(body []byte) error {
	var envelope struct {
		Error   json.RawMessage `json:"errorCode"`
		Notices []rdapNotice    `json:"notices"`
		Remarks []rdapNotice    `json:"remarks"`
		Links   []struct {
			Rel string `json:"rel"`
		} `json:"links"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return errors.New("ARIN returned invalid RDAP JSON")
	}
	if envelope.Error != nil {
		return errors.New("ARIN returned an RDAP error payload instead of a registration")
	}
	if incompleteNotices(envelope.Notices) || incompleteNotices(envelope.Remarks) {
		return errors.New("ARIN truncated the RDAP response; refusing a partial result")
	}
	for _, link := range envelope.Links {
		if strings.EqualFold(link.Rel, "next") {
			return errors.New("ARIN returned a paginated RDAP response; refusing a partial result")
		}
	}
	return nil
}
func (c *Client) rdapEntity(ctx context.Context, handle string) ([]rdapEntityRef, error) {
	body, err := c.get(ctx, c.rdapBaseURL, "/registry/entity/"+url.PathEscape(handle), "application/rdap+json", false)
	if err != nil {
		return nil, err
	}
	if err = validateEntityTree(body, 0); err != nil {
		return nil, err
	}
	var entity struct {
		Handle   string          `json:"handle"`
		Class    string          `json:"objectClassName"`
		Entities []rdapEntityRef `json:"entities"`
	}
	if json.Unmarshal(body, &entity) != nil || entity.Class != "entity" || !strings.EqualFold(entity.Handle, handle) {
		return nil, errors.New("ARIN returned an unexpected RDAP entity")
	}
	return entity.Entities, nil
}
func (c *Client) readPublic(ctx context.Context, spec ReadSpec, p map[string]string) (map[string]any, error) {
	if c.rdapBaseURL == "" {
		return nil, errors.New("rdap_base_url is required for public reads when base_url is a custom origin")
	}
	if spec.Name == "rdap_domains_by_nameserver" {
		return c.searchRDAPDomainsByNameserver(ctx, p["nameserver"])
	}
	if spec.Name == "rdap_help" {
		return c.readRDAPHelp(ctx)
	}
	if spec.Name == "rdap_networks" || spec.Name == "rdap_asns" {
		return c.searchRDAPResources(ctx, spec.Name, p["search_by"], p["query"], p["role"])
	}
	if spec.Name == "rdap_network_hierarchy" {
		return c.searchRDAPNetworkHierarchy(ctx, p["query"], p["relation"], p["active_only"] == "true")
	}
	if spec.Name == "rdap_domains" {
		return c.searchRDAPDomains(ctx, p["name"], p["relation"], p["active_only"] == "true")
	}
	if spec.Name == "rdap_domain" {
		return c.readRDAPDomain(ctx, p["name"])
	}
	if spec.Name == "rdap_entities" {
		return c.searchRDAPEntities(ctx, p["search_by"], p["query"])
	}
	if spec.Name == "rdap_entity" {
		return c.readRDAPEntity(ctx, p["handle"])
	}
	if spec.Name == "rdap_network" {
		return c.readRDAPNetwork(ctx, p["query"])
	}
	if spec.Name == "org_pocs" {
		entities, err := c.rdapEntity(ctx, p["org_handle"])
		if err != nil {
			return nil, err
		}
		records := make([]any, 0, len(entities))
		slices.SortFunc(entities, func(a, b rdapEntityRef) int {
			return strings.Compare(strings.ToUpper(a.Handle), strings.ToUpper(b.Handle))
		})
		seen := map[string]bool{}
		for _, e := range entities {
			if incompleteNotices(e.Notices) || incompleteNotices(e.Remarks) {
				return nil, errors.New("ARIN returned a truncated contact reference")
			}
			key := strings.ToUpper(e.Handle)
			if key == "" || seen[key] {
				return nil, errors.New("ARIN returned missing or duplicate contact handles")
			}
			seen[key] = true
			records = append(records, map[string]any{"handle": e.Handle, "roles": anyStrings(e.Roles)})
		}
		return map[string]any{"pocs": records}, nil
	}
	path := "/registry/autnum/" + url.PathEscape(p["asn"])
	if spec.Name == "asns" {
		path = "/registry/autnums/reverse_search/entity?" + url.Values{"handle": []string{p["org_handle"]}}.Encode()
	}
	body, err := c.get(ctx, c.rdapBaseURL, path, "application/rdap+json", false)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.rdapNotFound && spec.Name == "asns" {
		if _, entityErr := c.rdapEntity(ctx, p["org_handle"]); entityErr != nil {
			return nil, entityErr
		}
		return map[string]any{"asns": []any{}}, nil
	}
	if err != nil {
		return nil, err
	}
	if err = checkRDAPCompleteness(body); err != nil {
		return nil, err
	}
	if spec.Name == "asn" {
		record, err := decodeRDAPASN(body)
		if err != nil {
			return nil, err
		}
		requested, _ := strconv.ParseInt(p["asn"], 10, 64)
		if requested < record["start_asn"].(int64) || requested > record["end_asn"].(int64) {
			return nil, errors.New("ARIN returned a mismatched ASN registration")
		}
		return record, nil
	}
	var response struct {
		Results *[]json.RawMessage `json:"autnumSearchResults"`
	}
	if json.Unmarshal(body, &response) != nil || response.Results == nil {
		return nil, errors.New("ARIN returned an unexpected ASN search response")
	}
	records := make([]any, 0, len(*response.Results))
	seen := map[string]bool{}
	for _, n := range *response.Results {
		record, err := decodeRDAPASN(n)
		if err != nil {
			return nil, err
		}
		belongs := false
		for _, org := range record["org_handles"].([]any) {
			if strings.EqualFold(org.(string), p["org_handle"]) {
				belongs = true
			}
		}
		if !belongs {
			continue
		}
		key := strings.ToUpper(record["handle"].(string))
		if seen[key] {
			return nil, errors.New("ARIN returned duplicate ASN handles")
		}
		seen[key] = true
		records = append(records, record)
	}
	slices.SortFunc(records, func(a, b any) int {
		return strings.Compare(strings.ToUpper(a.(map[string]any)["handle"].(string)), strings.ToUpper(b.(map[string]any)["handle"].(string)))
	})
	return map[string]any{"asns": records}, nil
}
