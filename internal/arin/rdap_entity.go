package arin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
)

var rdapEntityFields = []Field{
	required(text("handle", "")), {Name: "names", Kind: StringsKind, Description: "Formatted names from jCard fn properties, sorted and deduplicated."}, text("kind", ""), stringsField("emails", ""), {Name: "phones", Kind: StringsKind, Description: "Telephone values as returned, including tel: URIs when used by ARIN. Parameters remain in vcard_json."}, stringsField("address_labels", ""), stringsField("roles", ""), stringsField("status", ""),
	objects("entities", "", required(text("handle", "")), stringsField("roles", "")),
	objects("events", "", text("action", ""), text("date", "")),
	{Name: "vcard_json", Kind: StringKind, Description: "Complete jCard JSON, preserving structured addresses, parameters, repeated properties and extensions. Null when omitted."},
	{Name: "rdap_json", Kind: StringKind, Description: "Complete RDAP entity JSON, including nested entities, links, notices, remarks and extensions. Links are not followed."},
}

type rdapEvent struct {
	Action string `json:"eventAction"`
	Date   string `json:"eventDate"`
}

// validateEntityTree checks completeness throughout the returned entity tree.
// Embedded contacts may omit fields that are available only through direct lookup.
func validateEntityTree(body json.RawMessage, depth int) error {
	if depth > 64 {
		return errors.New("ARIN entity nesting exceeded the limit")
	}
	if len(strings.TrimSpace(string(body))) == 0 || strings.TrimSpace(string(body))[0] != '{' {
		return errors.New("ARIN returned a non-object entity")
	}
	if err := checkRDAPCompleteness(body); err != nil {
		return err
	}
	var node struct {
		Entities []json.RawMessage `json:"entities"`
	}
	if json.Unmarshal(body, &node) != nil {
		return errors.New("ARIN returned invalid nested entities")
	}
	for _, child := range node.Entities {
		if err := validateEntityTree(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) readRDAPEntity(ctx context.Context, handle string) (map[string]any, error) {
	body, err := c.get(ctx, c.rdapBaseURL, "/registry/entity/"+url.PathEscape(handle), "application/rdap+json", false)
	if err != nil {
		return nil, err
	}
	result, err := decodeRDAPEntity(body)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(result["handle"].(string), handle) {
		return nil, errors.New("ARIN returned an unexpected RDAP entity")
	}
	return result, nil
}

func decodeRDAPEntity(body []byte) (map[string]any, error) {
	if err := validateEntityTree(body, 0); err != nil {
		return nil, err
	}
	var entity struct {
		Handle   string          `json:"handle"`
		Class    string          `json:"objectClassName"`
		VCard    json.RawMessage `json:"vcardArray"`
		Roles    []string        `json:"roles"`
		Status   []string        `json:"status"`
		Entities []rdapEntityRef `json:"entities"`
		Events   []rdapEvent     `json:"events"`
	}
	if json.Unmarshal(body, &entity) != nil || entity.Class != "entity" || entity.Handle == "" {
		return nil, errors.New("ARIN returned an unexpected RDAP entity")
	}
	result, err := rdapContact(entity.VCard)
	if err != nil {
		return nil, err
	}
	result["handle"], result["roles"], result["status"] = entity.Handle, anyStrings(entity.Roles), anyStrings(entity.Status)
	refs := []any{}
	slices.SortFunc(entity.Entities, func(a, b rdapEntityRef) int { return strings.Compare(a.Handle, b.Handle) })
	seen := map[string]bool{}
	for _, ref := range entity.Entities {
		key := strings.ToUpper(ref.Handle)
		if key == "" || seen[key] {
			return nil, errors.New("ARIN returned missing or duplicate entity handles")
		}
		seen[key] = true
		refs = append(refs, map[string]any{"handle": ref.Handle, "roles": anyStrings(ref.Roles)})
	}
	result["entities"] = refs
	slices.SortFunc(entity.Events, func(a, b rdapEvent) int { return strings.Compare(a.Action+"/"+a.Date, b.Action+"/"+b.Date) })
	events := []any{}
	for _, event := range entity.Events {
		events = append(events, map[string]any{"action": event.Action, "date": event.Date})
	}
	result["events"] = events
	// Marshal RawMessage to compact JSON without converting numbers to float64.
	compact, err := json.Marshal(json.RawMessage(body))
	if err != nil {
		return nil, err
	}
	result["rdap_json"] = string(compact)
	return result, nil
}

func rdapContact(raw json.RawMessage) (map[string]any, error) {
	result := map[string]any{"names": []any{}, "kind": nil, "vcard_json": nil, "emails": []any{}, "phones": []any{}, "address_labels": []any{}}
	if len(raw) == 0 || string(raw) == "null" {
		return result, nil
	}
	invalid := errors.New("ARIN returned malformed entity jCard")
	var card []json.RawMessage
	var tag string
	var properties [][]json.RawMessage
	if json.Unmarshal(raw, &card) != nil || len(card) != 2 || json.Unmarshal(card[0], &tag) != nil || tag != "vcard" || json.Unmarshal(card[1], &properties) != nil || properties == nil {
		return nil, invalid
	}
	values := map[string][]string{}
	for _, property := range properties {
		var name, kind string
		var params map[string]json.RawMessage
		if len(property) < 4 || json.Unmarshal(property[0], &name) != nil || name == "" || json.Unmarshal(property[1], &params) != nil || params == nil || json.Unmarshal(property[2], &kind) != nil || kind == "" {
			return nil, invalid
		}
		switch name {
		case "fn", "kind", "email", "tel":
			if len(property) != 4 {
				return nil, invalid
			}
			var value string
			if json.Unmarshal(property[3], &value) != nil || string(property[3]) == "null" {
				return nil, invalid
			}
			switch name {
			case "fn":
				values["names"] = append(values["names"], value)
			case "kind":
				field := "kind"
				if result[field] != nil {
					return nil, invalid
				}
				result[field] = value
			case "email":
				values["emails"] = append(values["emails"], value)
			case "tel":
				values["phones"] = append(values["phones"], value)
			}
		case "adr":
			if label, ok := params["label"]; ok {
				var value string
				if json.Unmarshal(label, &value) != nil || string(label) == "null" {
					return nil, invalid
				}
				values["address_labels"] = append(values["address_labels"], value)
			}
		}
	}
	for field, list := range values {
		result[field] = anyStrings(list)
	}
	compact, err := json.Marshal(raw)
	if err != nil {
		return nil, invalid
	}
	result["vcard_json"] = string(compact)
	return result, nil
}
