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

type whoisRelation struct{ name, owner, relation, target, root, output string }

var whoisRelations = []whoisRelation{
	{"poc_orgs", "poc", "orgs", "org", "orgs", "orgs"},
	{"poc_asns", "poc", "asns", "asn", "asns", "asns"},
	{"poc_nets", "poc", "nets", "net", "nets", "networks"},
	{"org_pocs", "org", "pocs", "poc", "pocs", "pocs"},
	{"org_asns", "org", "asns", "asn", "asns", "asns"},
	{"org_nets", "org", "nets", "net", "nets", "networks"},
	{"asn_pocs", "asn", "pocs", "poc", "pocs", "pocs"},
	{"net_pocs", "net", "pocs", "poc", "pocs", "pocs"},
	{"net_parent", "net", "parent", "net", "net", "networks"},
	{"net_children", "net", "children", "net", "nets", "networks"},
	{"net_delegations", "net", "rdns", "delegation", "delegations", "delegations"},
	{"delegation_nets", "delegation", "nets", "net", "nets", "networks"},
	{"customer_nets", "customer", "nets", "net", "nets", "networks"},
}

func whoisRelationship(name string) (whoisRelation, bool) {
	for _, r := range whoisRelations {
		if name == "whois_"+r.name {
			return r, true
		}
	}
	return whoisRelation{}, false
}
func whoisRecordSpec(kind string) ReadSpec {
	for _, s := range WhoisRecordReads() {
		if s.Name == "whois_"+kind {
			return s
		}
	}
	return ReadSpec{}
}
func WhoisRelationshipReads() []ReadSpec {
	specs := make([]ReadSpec, 0, len(whoisRelations))
	for _, r := range whoisRelations {
		owner, target := whoisRecordSpec(r.owner), whoisRecordSpec(r.target)
		fields := []Field{}
		for _, field := range target.Fields {
			if field.Name != "whois_xml" {
				fields = append(fields, field)
			}
		}
		fields = append(fields, stringsField("poc_functions", ""), text("related_poc_handle", ""), text("related_poc_name", ""), text("reference_name", ""))
		fields[len(fields)-4].Description = "Sorted POC association function codes. Reference links retain their function; expanded POCs collect functions from inline links to the requested owner."
		specs = append(specs, ReadSpec{Name: "whois_" + r.name, Public: true, Collection: true, Root: r.root, Output: r.output, Inputs: owner.Inputs[:2], Fields: fields, ResponseFields: []Field{{Name: "whois_xml", Kind: StringKind, Description: "Complete relationship response XML, including references, expanded records and extensions. Null for a confirmed empty HTTP 404 response. Links are not followed."}}, Description: "List Whois-RWS " + r.relation + " related to a " + r.owner + " registration. References are returned by default; show_details requests expanded records. Parent-network lookups always return a full record in a list. POC role links remain distinct. No API key is sent. Partial responses and referrals are rejected; recognized no-results responses require an existing owner before returning an empty list."})
	}
	return specs
}
func whoisOwnerPath(info whoisRelation, p map[string]string) (string, string, error) {
	kind, identity := info.owner, p["handle"]
	if kind == "delegation" {
		kind = "rdns"
		normalized, err := rdapDomainName(p["name"])
		if err != nil {
			return "", "", err
		}
		identity = strings.TrimSuffix(normalized, ".")
	}
	return "/rest/" + kind + "/" + url.PathEscape(identity), identity, nil
}
func (c *Client) readWhoisRelated(ctx context.Context, spec ReadSpec, info whoisRelation, p map[string]string) (map[string]any, error) {
	ownerPath, identity, err := whoisOwnerPath(info, p)
	if err != nil {
		return nil, err
	}
	path := ownerPath + "/" + info.relation
	if p["show_details"] == "true" {
		path += "?showDetails=true"
	}
	body, err := c.get(ctx, c.whoisBaseURL, path, "application/xml", false)
	if err != nil {
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !apiErr.whoisNoMatches {
			return nil, err
		}
		ownerSpec := whoisRecordSpec(info.owner)
		if _, err := c.ReadRegistration(ctx, ownerSpec, map[string]string{ownerSpec.Inputs[0].Name: p[ownerSpec.Inputs[0].Name], "show_details": "false"}); err != nil {
			return nil, err
		}
		return map[string]any{spec.Output: []any{}, "whois_xml": nil}, nil
	}
	return decodeWhoisCollection(body, spec, info, identity, p["show_details"] == "true")
}

func decodeWhoisCollection(body []byte, spec ReadSpec, info whoisRelation, identity string, details bool) (map[string]any, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	namespace := whoisNamespace
	if info.target == "delegation" {
		namespace = whoisRDNSNamespace
	}
	if root.Name.Local != info.root || (root.Name.Space != namespace && root.Name.Space != strings.Replace(namespace, "https:", "http:", 1)) {
		return nil, errors.New("ARIN returned an unexpected Whois relationship response")
	}
	if err := normalizeWhoisTree(root); err != nil {
		return nil, err
	}
	nodes := root.Children
	if info.relation == "parent" {
		nodes = []*xmlNode{root}
	}
	targetSpec := whoisRecordSpec(info.target)
	records := []any{}
	for _, node := range nodes {
		tag := node.Name.Local
		full := tag == info.target
		reference := tag == info.target+"Ref" || tag == info.target+"PocLinkRef" || (info.target == "poc" && tag == "pocLinkRef")
		if node.Name.Space != "http://www.arin.net/regrws/core/v1" {
			if full || reference {
				return nil, errors.New("ARIN returned a related record in a foreign namespace")
			}
			continue
		}
		if tag == "limitExceeded" || tag == "ref" || tag == "rdapRef" {
			continue
		}
		if !full && !reference {
			return nil, errors.New("ARIN returned an unexpected related record type")
		}
		if details && !full {
			return nil, errors.New("ARIN returned a reference instead of requested Whois details")
		}
		var record map[string]any
		if full {
			record, err = decodeWhoisRecord(node, targetSpec)
			delete(record, "whois_xml")
		} else {
			record, err = decodeWhoisReference(node, targetSpec)
		}
		if err != nil {
			return nil, err
		}
		metadata, err := decodeFields(node, []Field{text("function", "@function"), text("poc_function", "@pocFunction"), text("related_function", "@relPocFunction"), text("poc_handle", "@pocHandle"), text("related_poc_handle", "@relPocHandle"), text("related_poc_name", "@relPocName"), text("reference_name", "@name")})
		if err != nil {
			return nil, err
		}
		record["related_poc_handle"], record["related_poc_name"], record["reference_name"] = metadata["related_poc_handle"], metadata["related_poc_name"], metadata["reference_name"]
		if record["related_poc_handle"] == nil {
			record["related_poc_handle"] = metadata["poc_handle"]
		}
		if info.owner == "poc" && record["related_poc_handle"] != nil && !strings.EqualFold(record["related_poc_handle"].(string), identity) {
			return nil, errors.New("ARIN returned a relationship for a different POC")
		}
		functions := []string{}
		for _, key := range []string{"function", "poc_function", "related_function"} {
			if value, ok := metadata[key].(string); ok && value != "" {
				functions = append(functions, value)
			}
		}
		if info.owner != "" && info.target == "poc" && full {
			for _, link := range nodesAt(node, info.owner+"s/"+info.owner+"PocLinkRef") {
				attrs, err := decodeFields(link, []Field{required(text("handle", "@handle")), text("function", "@relPocFunction")})
				if err != nil {
					return nil, err
				}
				if strings.EqualFold(attrs["handle"].(string), identity) {
					if function, ok := attrs["function"].(string); ok && function != "" {
						functions = append(functions, function)
					}
				}
			}
		}
		record["poc_functions"] = anyStrings(functions)
		records = append(records, record)
	}
	slices.SortStableFunc(records, func(a, b any) int {
		aa, bb := a.(map[string]any), b.(map[string]any)
		key := "handle"
		if info.target == "delegation" {
			key = "name"
		}
		if n := strings.Compare(strings.ToUpper(aa[key].(string)), strings.ToUpper(bb[key].(string))); n != 0 {
			return n
		}
		aj, _ := json.Marshal(a)
		bj, _ := json.Marshal(b)
		return strings.Compare(string(aj), string(bj))
	})
	return map[string]any{spec.Output: records, "whois_xml": string(body)}, nil
}
func decodeWhoisReference(node *xmlNode, spec ReadSpec) (map[string]any, error) {
	record := map[string]any{}
	for _, field := range spec.Fields {
		if field.Name == "whois_xml" {
			continue
		}
		switch field.Kind {
		case StringsKind, IntsKind, ObjectsKind:
			record[field.Name] = []any{}
		default:
			record[field.Name] = nil
		}
	}
	fields := []Field{required(text("handle", "@handle"))}
	if spec.Root == "delegation" {
		fields = []Field{required(text("name", "@name"))}
	} else if spec.Root != "poc" {
		fields = append(fields, text("name", "@name"))
	}
	if spec.Root == "net" {
		fields = append(fields, text("start_address", "@startAddress"), text("end_address", "@endAddress"))
	}
	values, err := decodeFields(node, fields)
	if err != nil {
		return nil, err
	}
	for key, value := range values {
		record[key] = value
	}
	if spec.Root == "delegation" {
		name, err := rdapDomainName(record["name"].(string))
		if err != nil {
			return nil, err
		}
		record["name"] = name
	}
	if spec.Root == "net" && (record["start_address"] != nil || record["end_address"] != nil) {
		startText, ok1 := record["start_address"].(string)
		endText, ok2 := record["end_address"].(string)
		start, e1 := netip.ParseAddr(startText)
		end, e2 := netip.ParseAddr(endText)
		if !ok1 || !ok2 || e1 != nil || e2 != nil || start.BitLen() != end.BitLen() || start.Compare(end) > 0 {
			return nil, errors.New("ARIN returned an invalid related network range")
		}
		record["ip_version"] = "v6"
		if start.Is4() {
			record["ip_version"] = "v4"
		}
	}
	return record, nil
}
