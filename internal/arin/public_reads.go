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
	required(text("handle", "")), text("name", ""), required(integer("start_asn", "")), required(integer("end_asn", "")), text("country", ""), stringsField("org_handles", ""), stringsField("status", ""),
	objects("events", "", text("action", ""), text("date", "")),
}

func PublicReads() []ReadSpec {
	return []ReadSpec{
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
		if slices.Contains(entity.Roles, "registrant") {
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
	return map[string]any{"handle": n.Handle, "name": n.Name, "start_asn": *n.Start, "end_asn": *n.End, "country": country, "org_handles": anyStrings(orgs), "status": anyStrings(n.Status), "events": events}, nil
}

func checkRDAPCompleteness(body []byte) error {
	var envelope struct {
		Notices []rdapNotice `json:"notices"`
		Remarks []rdapNotice `json:"remarks"`
		Links   []struct {
			Rel string `json:"rel"`
		} `json:"links"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return errors.New("ARIN returned invalid RDAP JSON")
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
	if err = checkRDAPCompleteness(body); err != nil {
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
		slices.SortFunc(entities, func(a, b rdapEntityRef) int { return strings.Compare(a.Handle, b.Handle) })
		for _, e := range entities {
			if incompleteNotices(e.Notices) || incompleteNotices(e.Remarks) {
				return nil, errors.New("ARIN returned a truncated contact reference")
			}
			if e.Handle == "" {
				return nil, errors.New("ARIN returned a contact without a handle")
			}
			records = append(records, map[string]any{"handle": e.Handle, "roles": anyStrings(e.Roles)})
		}
		return map[string]any{"pocs": records}, nil
	}
	path := "/registry/autnum/" + url.PathEscape(p["asn"])
	if spec.Name == "asns" {
		path = "/registry/autnums/reverse_search/entity?" + url.Values{"handle": []string{p["org_handle"]}}.Encode()
	}
	body, err := c.get(ctx, c.rdapBaseURL, path, "application/rdap+json", false)
	if IsNotFound(err) && spec.Name == "asns" {
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
		var n rdapASN
		if json.Unmarshal(body, &n) != nil {
			return nil, errors.New("ARIN returned invalid ASN JSON")
		}
		record, err := n.record()
		if err != nil {
			return nil, err
		}
		requested, _ := strconv.ParseInt(p["asn"], 10, 64)
		if requested < *n.Start || requested > *n.End {
			return nil, errors.New("ARIN returned a mismatched ASN registration")
		}
		return record, nil
	}
	var response struct {
		Results *[]rdapASN `json:"autnumSearchResults"`
	}
	if json.Unmarshal(body, &response) != nil || response.Results == nil {
		return nil, errors.New("ARIN returned an unexpected ASN search response")
	}
	records := make([]any, 0, len(*response.Results))
	slices.SortFunc(*response.Results, func(a, b rdapASN) int { return strings.Compare(a.Handle, b.Handle) })
	seen := map[string]bool{}
	for _, n := range *response.Results {
		record, err := n.record()
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
		if seen[n.Handle] {
			return nil, errors.New("ARIN returned duplicate ASN handles")
		}
		seen[n.Handle] = true
		records = append(records, record)
	}
	return map[string]any{"asns": records}, nil
}
