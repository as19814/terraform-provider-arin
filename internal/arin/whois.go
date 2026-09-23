package arin

import (
	"context"
	"encoding/hex"
	"errors"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

const whoisNamespace = "https://www.arin.net/whoisrws/core/v1"
const whoisRDNSNamespace = "https://www.arin.net/whoisrws/rdns/v1"

var whoisCommonFields = []Field{
	text("registration_date", "registrationDate"), text("update_date", "updateDate"), lines("comments", "comment"),
	text("ref", "ref"), text("rdap_ref", "rdapRef"), {Name: "whois_xml", Kind: StringKind, Description: "Complete Whois-RWS XML, preserving references, metadata and extensions. No links are followed."},
}
var whoisAddressFields = []Field{
	lines("street_address", "streetAddress"), text("city", "city"), text("state", "iso3166-2"), text("postal_code", "postalCode"), text("country_code", "iso3166-1/code2"), text("country_name", "iso3166-1/name"), text("country_code3", "iso3166-1/code3"), text("country_calling_code", "iso3166-1/e164"),
}

func WhoisReads() []ReadSpec {
	return append(append(append(WhoisRecordReads(), WhoisRelationshipReads()...), WhoisSearchReads()...), WhoisNetworkReads()...)
}

func WhoisRecordReads() []ReadSpec {
	identity := []Field{required(text("handle", "handle")), text("name", "name")}
	return []ReadSpec{
		whoisSpec("org", "org", joinFields(identity, whoisAddressFields, []Field{{Name: "can_allocate", Path: "canAllocate", Kind: BoolKind}})),
		whoisSpec("customer", "customer", joinFields(identity, whoisAddressFields, []Field{text("parent_org_handle", "parentOrgRef/@handle"), boolean("can_allocate", "canAllocate")})),
		whoisSpec("poc", "poc", joinFields([]Field{required(text("handle", "handle")), text("first_name", "firstName"), text("middle_name", "middleName"), text("last_name", "lastName"), text("company_name", "companyName"), text("poc_type", "pocType/type"), text("poc_type_description", "pocType/description"), text("status", "status/code"), text("status_description", "status/description"), {Name: "is_role_account", Path: "isRoleAccount", Kind: BoolKind}, stringsField("emails", "emails/email"), objects("phones", "phones/phone", text("number", "number"), text("type", "type/code"), text("description", "type/description"))}, whoisAddressFields)),
		whoisSpec("asn", "asn", joinFields(identity, []Field{required(integer("start_asn", "startAsNumber")), required(integer("end_asn", "endAsNumber")), text("org_handle", "orgRef/@handle")})),
		whoisSpec("net", "net", joinFields(identity, []Field{required(text("start_address", "startAddress")), required(text("end_address", "endAddress")), required(text("ip_version", "version")), text("org_handle", "orgRef/@handle"), text("customer_handle", "customerRef/@handle"), text("parent_handle", "parentNetRef/@handle"), objects("net_blocks", "netBlocks/netBlock", required(text("start_address", "startAddress")), required(text("end_address", "endAddress")), required(integer("cidr_length", "cidrLength")), text("type", "type"), text("description", "description"))})),
		whoisSpec("delegation", "delegation", []Field{required(text("name", "name")), stringsField("nameservers", "nameservers/nameserver"), objects("ds_records", "delegationKeys/delegationKey", required(integer("key_tag", "keyTag")), required(integer("algorithm", "algorithm")), text("algorithm_name", "algorithm/@name"), required(integer("digest_type", "digestType")), text("digest_type_name", "digestType/@name"), required(text("digest", "digest")))}),
	}
}
func whoisSpec(name, root string, fields []Field) ReadSpec {
	in := input("handle", "handle", "EXAMPLE-1", "ARIN registration handle.")
	switch name {
	case "net":
		in.Example = "NET-192-0-2-0-1"
	case "asn":
		in.Example = "AS64496"
	case "poc":
		in.Example = "EXAMPLE-ARIN"
	case "customer":
		in.Example = "C00000001"
	case "delegation":
		in = input("name", "rdap_domain", "2.0.192.in-addr.arpa.", "Reverse DNS delegation name. Case and an optional trailing dot are normalized.")
	}
	spec := ReadSpec{Name: "whois_" + name, Root: root, Public: true, Description: "Read a public " + name + " registration through Whois-RWS, including typed fields and complete XML. No API key is sent. Missing records, partial responses and referrals remain errors. Public data may omit private registration fields.", Inputs: []Input{in, optional(input("show_details", "bool", "false", "Ask ARIN to expand related information inline. Complete XML preserves extra records; any nested truncation is rejected."), "false")}, Fields: joinFields(fields, whoisCommonFields)}
	if name == "org" {
		spec.Inputs = append(spec.Inputs, optional(input("show_pocs", "bool", "false", "Include organization POC references inline without expanding its network and ASN inventories. Complete XML retains them. show_details also expands the other relationships."), "false"))
	}
	return spec
}

// Normalize only recognized Whois namespaces in a private tree for the existing
// typed-field decoder. Foreign namespaces are marked so they cannot supply typed fields.
func normalizeWhoisTree(node *xmlNode) error {
	recognized := node.Name.Space == whoisNamespace || node.Name.Space == "http://www.arin.net/whoisrws/core/v1" || node.Name.Space == whoisRDNSNamespace || node.Name.Space == "http://www.arin.net/whoisrws/rdns/v1"
	if recognized {
		if node.Name.Local == "canAllocate" || node.Name.Local == "isRoleAccount" {
			switch strings.TrimSpace(node.Text) {
			case "Y":
				node.Text = "true"
			case "N":
				node.Text = "false"
			}
		}
		if node.Name.Local == "limitExceeded" {
			value, err := strconv.ParseBool(strings.TrimSpace(node.Text))
			if err != nil || value {
				return errors.New("ARIN returned truncated or invalid Whois results")
			}
		}
		node.Name.Space = "http://www.arin.net/regrws/core/v1"
	}
	if !recognized {
		node.Name.Space = "urn:arin-provider:foreign-xml"
	}
	for i := range node.Attrs {
		ns := node.Attrs[i].Name.Space
		if ns == "" {
			continue
		}
		if ns == whoisNamespace || ns == whoisRDNSNamespace || ns == strings.Replace(whoisNamespace, "https:", "http:", 1) || ns == strings.Replace(whoisRDNSNamespace, "https:", "http:", 1) {
			node.Attrs[i].Name.Space = "http://www.arin.net/regrws/core/v1"
		} else {
			node.Attrs[i].Name.Space = "urn:arin-provider:foreign-xml"
		}
	}
	for _, child := range node.Children {
		if err := normalizeWhoisTree(child); err != nil {
			return err
		}
	}
	return nil
}
func (c *Client) readWhois(ctx context.Context, spec ReadSpec, p map[string]string) (map[string]any, error) {
	if c.whoisBaseURL == "" {
		return nil, errors.New("whois_base_url is required for Whois reads with a custom base_url")
	}
	if spec.Name == "whois_ip" || spec.Name == "whois_cidr" || spec.Name == "whois_cidr_networks" {
		return c.readWhoisNetwork(ctx, spec, p)
	}
	if info, ok := whoisSearch(spec.Name); ok {
		return c.readWhoisSearch(ctx, spec, info, p)
	}
	if info, ok := whoisRelationship(spec.Name); ok {
		return c.readWhoisRelated(ctx, spec, info, p)
	}
	endpoint := strings.TrimPrefix(spec.Name, "whois_")
	if !slices.Contains([]string{"org", "customer", "poc", "asn", "net", "delegation"}, endpoint) {
		return nil, errors.New("unsupported Whois lookup")
	}
	identity := p["handle"]
	if endpoint == "delegation" {
		endpoint = "rdns"
		var err error
		identity, err = rdapDomainName(p["name"])
		if err != nil {
			return nil, err
		}
		identity = strings.TrimSuffix(identity, ".")
	}
	path := "/rest/" + endpoint + "/" + url.PathEscape(identity)
	query := url.Values{}
	if p["show_details"] == "true" {
		query.Set("showDetails", "true")
	}
	if endpoint == "org" && p["show_pocs"] == "true" {
		query.Set("showPocs", "true")
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	body, err := c.get(ctx, c.whoisBaseURL, path, "application/xml", false)
	if err != nil {
		return nil, err
	}
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	expectedNamespace := whoisNamespace
	if endpoint == "rdns" {
		expectedNamespace = whoisRDNSNamespace
	}
	if root.Name.Local != spec.Root || (root.Name.Space != expectedNamespace && root.Name.Space != strings.Replace(expectedNamespace, "https:", "http:", 1)) {
		return nil, errors.New("ARIN returned an unexpected Whois object or namespace")
	}
	if err := normalizeWhoisTree(root); err != nil {
		return nil, err
	}
	record, err := decodeWhoisRecord(root, spec)
	if err != nil {
		return nil, err
	}
	field := "handle"
	if endpoint == "rdns" {
		field = "name"
	}
	if !strings.EqualFold(strings.TrimSuffix(record[field].(string), "."), identity) {
		return nil, errors.New("ARIN returned a mismatched Whois registration")
	}
	record["whois_xml"] = string(body)
	return record, nil
}

// decodeWhoisRecord consumes a namespace-checked and normalized XML tree.
func decodeWhoisRecord(root *xmlNode, spec ReadSpec) (map[string]any, error) {
	record, err := decodeFields(root, spec.Fields)
	if err != nil {
		return nil, err
	}
	endpoint := spec.Root
	if endpoint == "asn" {
		first, last := record["start_asn"].(int64), record["end_asn"].(int64)
		if first < 0 || last > 4294967295 || first > last {
			return nil, errors.New("ARIN returned an invalid Whois ASN range")
		}
	}
	if endpoint == "net" {
		start, _ := netip.ParseAddr(record["start_address"].(string))
		end, _ := netip.ParseAddr(record["end_address"].(string))
		version := record["ip_version"].(string)
		if version == "4" || version == "6" {
			version = "v" + version
			record["ip_version"] = version
		}
		if !start.IsValid() || !end.IsValid() || start.BitLen() != end.BitLen() || start.Compare(end) > 0 || (version != "v4" && version != "v6") || (version == "v4") != start.Is4() {
			return nil, errors.New("ARIN returned an invalid Whois network range")
		}
		for _, raw := range record["net_blocks"].([]any) {
			block := raw.(map[string]any)
			first, _ := netip.ParseAddr(block["start_address"].(string))
			last, _ := netip.ParseAddr(block["end_address"].(string))
			bits := block["cidr_length"].(int64)
			if bits < 0 || bits > int64(start.BitLen()) || first.BitLen() != start.BitLen() || last.BitLen() != start.BitLen() || first.Compare(start) < 0 || last.Compare(end) > 0 || first.Compare(last) > 0 {
				return nil, errors.New("ARIN returned an invalid Whois network block")
			}
			prefix := netip.PrefixFrom(first, int(bits))
			if prefix != prefix.Masked() || prefixLastAddress(prefix) != last {
				return nil, errors.New("ARIN returned a Whois block inconsistent with its CIDR length")
			}
		}
	}
	if endpoint == "delegation" {
		for _, raw := range record["ds_records"].([]any) {
			ds := raw.(map[string]any)
			if ds["key_tag"].(int64) < 0 || ds["key_tag"].(int64) > 65535 || ds["algorithm"].(int64) < 0 || ds["algorithm"].(int64) > 255 || ds["digest_type"].(int64) < 0 || ds["digest_type"].(int64) > 255 {
				return nil, errors.New("ARIN returned invalid Whois DS numeric fields")
			}
			if _, err := hex.DecodeString(ds["digest"].(string)); err != nil {
				return nil, errors.New("ARIN returned an invalid Whois DS digest")
			}
		}
	}
	return record, nil
}
