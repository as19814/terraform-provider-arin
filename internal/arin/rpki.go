package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

const rpkiNamespace = "http://www.arin.net/regrws/rpki/v1"

type ROAResource struct {
	Prefix     string
	MaxLength  *int64
	AutoLinked bool
}
type ROA struct {
	Handle, Name, NotValidBefore, NotValidAfter string
	ASN                                         int64
	AutoRenewed                                 bool
	Resources                                   []ROAResource
	AutoLink                                    *bool
}
type ROARequest struct {
	Name      string
	ASN       int64
	AutoLink  bool
	Resources []ROAResource
}
type ROADelete struct {
	Handle   string
	AutoLink bool
}
type ASPA struct {
	CustomerASN  int64
	ProviderASNs []int64
}
type RPKITransaction struct {
	AddROAs     []ROARequest
	DeleteROAs  []ROADelete
	AddASPAs    []ASPA
	DeleteASPAs []int64
}
type RPKITransactionResult struct {
	ROAs         []ROA
	ASPAs        []ASPA
	DeletedROAs  []string
	DeletedASPAs []int64
}

func validateRPKIASN(asn int64, zero bool) error {
	if asn < 0 || asn > 4294967295 || (!zero && asn == 0) {
		return errors.New("invalid RPKI AS number")
	}
	return nil
}
func (r ROARequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" || strings.ContainsAny(r.Name, "\r\n") {
		return errors.New("ROA name must be a nonempty single line")
	}
	if err := validateRPKIASN(r.ASN, true); err != nil {
		return err
	}
	if r.ASN == 0 && r.AutoLink {
		return errors.New("AS0 ROAs cannot create IRR links; set auto_link to false")
	}
	if len(r.Resources) == 0 {
		return errors.New("ROA requires at least one resource prefix")
	}
	seen := map[string]bool{}
	for _, resource := range r.Resources {
		prefix, err := netip.ParsePrefix(resource.Prefix)
		if err != nil || prefix.Addr().Is4In6() || prefix != prefix.Masked() || prefix.String() != resource.Prefix {
			return errors.New("ROA resource must be a canonical IPv4 or IPv6 CIDR")
		}
		if resource.MaxLength != nil && (*resource.MaxLength < int64(prefix.Bits()) || *resource.MaxLength > int64(prefix.Addr().BitLen())) {
			return errors.New("ROA max_length must be between prefix length and address family width")
		}
		if seen[resource.Prefix] {
			return errors.New("duplicate ROA resource prefix")
		}
		seen[resource.Prefix] = true
	}
	return nil
}
func (a ASPA) Validate() error {
	if err := validateRPKIASN(a.CustomerASN, false); err != nil {
		return err
	}
	if len(a.ProviderASNs) == 0 {
		return errors.New("ASPA requires at least one provider AS")
	}
	seen := map[int64]bool{}
	for _, asn := range a.ProviderASNs {
		if err := validateRPKIASN(asn, true); err != nil {
			return err
		}
		if asn == 0 && len(a.ProviderASNs) != 1 {
			return errors.New("AS0 must be the sole ASPA provider")
		}
		if asn == a.CustomerASN || seen[asn] {
			return errors.New("ASPA providers must be unique and differ from the customer AS")
		}
		seen[asn] = true
	}
	return nil
}
func (t RPKITransaction) Validate() error {
	if len(t.AddROAs)+len(t.DeleteROAs)+len(t.AddASPAs)+len(t.DeleteASPAs) == 0 {
		return errors.New("RPKI transaction must contain at least one change")
	}
	for _, r := range t.AddROAs {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	handles := map[string]bool{}
	for _, r := range t.DeleteROAs {
		if !handlePattern.MatchString(r.Handle) || handles[r.Handle] {
			return errors.New("ROA deletion handles must be valid and unique")
		}
		handles[r.Handle] = true
	}
	customers := map[int64]bool{}
	for _, a := range t.AddASPAs {
		if err := a.Validate(); err != nil {
			return err
		}
		if customers[a.CustomerASN] {
			return errors.New("duplicate ASPA customer addition")
		}
		customers[a.CustomerASN] = true
	}
	customers = map[int64]bool{}
	for _, asn := range t.DeleteASPAs {
		if err := validateRPKIASN(asn, false); err != nil {
			return err
		}
		if customers[asn] {
			return errors.New("duplicate ASPA customer deletion")
		}
		customers[asn] = true
	}
	return nil
}

type roaResourceXML struct {
	Start string `xml:"startAddress"`
	CIDR  int    `xml:"cidrLength"`
	Max   *int64 `xml:"maxLength,omitempty"`
}
type roaRequestXML struct {
	AutoLink  bool             `xml:"autoLink"`
	ASN       int64            `xml:"asNumber"`
	Name      string           `xml:"name"`
	Resources []roaResourceXML `xml:"resources>roaSpecResource"`
}
type roaDeleteXML struct {
	AutoLink bool   `xml:"autoLink,attr"`
	Handle   string `xml:",chardata"`
}
type aspaXML struct {
	Customer  int64   `xml:"customerAsId"`
	Providers []int64 `xml:"providerAsIds>providerAsId"`
}
type rpkiTransactionXML struct {
	XMLName     xml.Name           `xml:"http://www.arin.net/regrws/rpki/v1 rpkiTransaction"`
	DeleteROAs  *roaDeleteListXML  `xml:"roaSpecDelete,omitempty"`
	AddROAs     *roaAddListXML     `xml:"roaSpecAdd,omitempty"`
	DeleteASPAs *aspaDeleteListXML `xml:"aspaDelete,omitempty"`
	AddASPAs    *aspaAddListXML    `xml:"aspaAdd,omitempty"`
}
type roaDeleteListXML struct {
	Items []roaDeleteXML `xml:"roaHandle"`
}
type roaAddListXML struct {
	Items []roaRequestXML `xml:"roaSpec"`
}
type aspaDeleteListXML struct {
	Items []int64 `xml:"customerAsId"`
}
type aspaAddListXML struct {
	Items []aspaXML `xml:"aspa"`
}

func (t RPKITransaction) marshal() ([]byte, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	p := rpkiTransactionXML{}
	if len(t.DeleteROAs) > 0 {
		p.DeleteROAs = &roaDeleteListXML{}
		for _, r := range t.DeleteROAs {
			p.DeleteROAs.Items = append(p.DeleteROAs.Items, roaDeleteXML{AutoLink: r.AutoLink, Handle: r.Handle})
		}
	}
	if len(t.AddROAs) > 0 {
		p.AddROAs = &roaAddListXML{}
		for _, r := range t.AddROAs {
			x := roaRequestXML{AutoLink: r.AutoLink, ASN: r.ASN, Name: r.Name}
			for _, b := range r.Resources {
				prefix := netip.MustParsePrefix(b.Prefix)
				x.Resources = append(x.Resources, roaResourceXML{Start: prefix.Addr().String(), CIDR: prefix.Bits(), Max: b.MaxLength})
			}
			p.AddROAs.Items = append(p.AddROAs.Items, x)
		}
	}
	if len(t.DeleteASPAs) > 0 {
		p.DeleteASPAs = &aspaDeleteListXML{Items: t.DeleteASPAs}
	}
	if len(t.AddASPAs) > 0 {
		p.AddASPAs = &aspaAddListXML{}
		for _, a := range t.AddASPAs {
			p.AddASPAs.Items = append(p.AddASPAs.Items, aspaXML{Customer: a.CustomerASN, Providers: a.ProviderASNs})
		}
	}
	return xml.Marshal(p)
}

// RPKI reads use core-namespace collection entries and RPKI-namespace fields.
// Accept those documented shapes without accepting arbitrary extension data.
func validateRPKIFields(root *xmlNode, allowed, repeated map[string]bool) error {
	var walk func(*xmlNode, string) error
	walk = func(n *xmlNode, path string) error {
		for _, a := range n.Attrs {
			if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
				continue
			}
			return errors.New("ARIN returned unsupported RPKI attributes")
		}
		seen := map[string]bool{}
		for _, child := range n.Children {
			p := child.Name.Local
			if path != "" {
				p = path + "/" + p
			}
			if child.Name.Space != rpkiNamespace || !allowed[p] {
				return errors.New("ARIN returned unsupported RPKI fields")
			}
			if seen[p] && !repeated[p] {
				return errors.New("ARIN returned duplicate RPKI fields")
			}
			seen[p] = true
			if err := walk(child, p); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root, "")
}
func decodeROA(n *xmlNode) (*ROA, error) {
	if n.Name.Local != "roaSpec" || (n.Name.Space != rpkiNamespace && n.Name.Space != registrationNamespace) {
		return nil, errors.New("ARIN returned an unexpected ROA")
	}
	wrapped := false
	containers := 0
	for _, resource := range n.Children {
		if resource.Name.Local == "resources" {
			containers++
			for _, child := range resource.Children {
				if child.Name.Local == "roaSpecResource" {
					wrapped = true
				}
			}
		}
	}
	if wrapped && containers != 1 {
		return nil, errors.New("ARIN returned multiple wrapped ROA resource collections")
	}
	allowed := map[string]bool{}
	for _, p := range []string{"roaHandle", "asNumber", "name", "notValidBefore", "notValidAfter", "autoRenewed", "autoLink", "resources"} {
		allowed[p] = true
	}
	resourcePath := "resources"
	repeated := map[string]bool{"resources": true}
	if wrapped {
		resourcePath = "resources/roaSpecResource"
		allowed[resourcePath] = true
		repeated[resourcePath] = true
	}
	for _, p := range []string{"startAddress", "endAddress", "cidrLength", "maxLength", "ipVersion", "autoLinked"} {
		allowed[resourcePath+"/"+p] = true
	}
	if err := validateRPKIFields(n, allowed, repeated); err != nil {
		return nil, err
	}
	fields := append([]Field{}, roaFields...)
	fields = append(fields, boolean("auto_link", "autoLink"))
	for i, f := range fields {
		if f.Name == "asn" || f.Name == "name" {
			fields[i] = required(f)
		}
		if f.Name == "resources" {
			fields[i].Path = resourcePath
		}
	}
	v, err := decodeFields(n, fields)
	if err != nil {
		return nil, err
	}
	r := &ROA{Handle: netString(v, "handle"), Name: netString(v, "name"), ASN: v["asn"].(int64), NotValidBefore: netString(v, "not_valid_before"), NotValidAfter: netString(v, "not_valid_after")}
	r.AutoRenewed, _ = v["auto_renewed"].(bool)
	if link, ok := v["auto_link"].(bool); ok {
		r.AutoLink = &link
	}
	if !handlePattern.MatchString(r.Handle) {
		return nil, errors.New("ARIN returned an invalid ROA handle")
	}
	for _, item := range v["resources"].([]any) {
		b := item.(map[string]any)
		ip, err := netip.ParseAddr(netString(b, "start_address"))
		bits, ok := b["cidr_length"].(int64)
		if err != nil || ip.Is4In6() || !ok || bits < 0 || bits > int64(ip.BitLen()) {
			return nil, errors.New("ARIN returned an invalid ROA resource")
		}
		prefix := netip.PrefixFrom(ip, int(bits))
		if prefix != prefix.Masked() {
			return nil, errors.New("ARIN returned a noncanonical ROA resource")
		}
		if end := netString(b, "end_address"); end != "" {
			address, err := netip.ParseAddr(end)
			if err != nil || address != prefixEnd(prefix) {
				return nil, errors.New("ARIN returned a mismatched ROA end address")
			}
		}
		if version, ok := b["ip_version"].(int64); ok {
			want := int64(6)
			if ip.Is4() {
				want = 4
			}
			if version != want {
				return nil, errors.New("ARIN returned a mismatched ROA address family")
			}
		}
		resource := ROAResource{Prefix: prefix.String()}
		resource.AutoLinked, _ = b["auto_linked"].(bool)
		if max, ok := b["max_length"].(int64); ok {
			resource.MaxLength = &max
		}
		r.Resources = append(r.Resources, resource)
	}
	if err := (ROARequest{Name: r.Name, ASN: r.ASN, Resources: r.Resources}).Validate(); err != nil {
		return nil, fmt.Errorf("ARIN returned an invalid ROA: %w", err)
	}
	return r, nil
}
func decodeASPA(n *xmlNode) (*ASPA, error) {
	if n.Name.Local != "aspa" || (n.Name.Space != rpkiNamespace && n.Name.Space != registrationNamespace) {
		return nil, errors.New("ARIN returned an unexpected ASPA")
	}
	if err := validateRPKIFields(n, map[string]bool{"customerAsId": true, "providerAsIds": true, "providerAsIds/providerAsId": true}, map[string]bool{"providerAsIds/providerAsId": true}); err != nil {
		return nil, err
	}
	v, err := decodeFields(n, aspaFields)
	if err != nil {
		return nil, err
	}
	a := &ASPA{CustomerASN: v["customer_asn"].(int64)}
	for _, x := range v["provider_asns"].([]any) {
		a.ProviderASNs = append(a.ProviderASNs, x.(int64))
	}
	if err := a.Validate(); err != nil {
		return nil, fmt.Errorf("ARIN returned an invalid ASPA: %w", err)
	}
	slices.Sort(a.ProviderASNs)
	return a, nil
}
func (c *Client) rpkiCollection(ctx context.Context, org, kind string) ([]*xmlNode, error) {
	if !handlePattern.MatchString(org) {
		return nil, errors.New("invalid organization handle")
	}
	body, err := c.get(ctx, c.baseURL, "/rest/"+kind+"/"+url.PathEscape(org), "application/xml", true)
	if err != nil {
		return nil, err
	}
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "collection" || root.Name.Space != registrationNamespace {
		return nil, errors.New("ARIN returned an unexpected RPKI collection")
	}
	for _, a := range root.Attrs {
		if a.Name.Space != "xmlns" && !(a.Name.Space == "" && a.Name.Local == "xmlns") {
			return nil, errors.New("ARIN returned unsupported RPKI collection attributes; refusing a partial inventory")
		}
	}
	return root.Children, nil
}
func (c *Client) ListROAs(ctx context.Context, org string) ([]ROA, error) {
	nodes, err := c.rpkiCollection(ctx, org, "roa")
	if err != nil {
		return nil, err
	}
	out := []ROA{}
	seen := map[string]bool{}
	for _, n := range nodes {
		r, err := decodeROA(n)
		if err != nil {
			return nil, err
		}
		if seen[r.Handle] {
			return nil, errors.New("ARIN returned duplicate ROA handles")
		}
		seen[r.Handle] = true
		out = append(out, *r)
	}
	return out, nil
}
func (c *Client) ListASPAs(ctx context.Context, org string) ([]ASPA, error) {
	nodes, err := c.rpkiCollection(ctx, org, "aspa")
	if err != nil {
		return nil, err
	}
	out := []ASPA{}
	seen := map[int64]bool{}
	for _, n := range nodes {
		a, err := decodeASPA(n)
		if err != nil {
			return nil, err
		}
		if seen[a.CustomerASN] {
			return nil, errors.New("ARIN returned duplicate ASPA customers")
		}
		seen[a.CustomerASN] = true
		out = append(out, *a)
	}
	return out, nil
}
func decodeRPKITransaction(body []byte) (*RPKITransactionResult, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "rpkiTransaction" || root.Name.Space != rpkiNamespace {
		return nil, errors.New("ARIN returned an unexpected RPKI transaction")
	}
	result := &RPKITransactionResult{}
	var failures []error
	seen := map[string]bool{}
	for _, list := range root.Children {
		if list.Name.Space != rpkiNamespace || seen[list.Name.Local] {
			failures = append(failures, errors.New("ARIN returned invalid RPKI transaction fields"))
			continue
		}
		seen[list.Name.Local] = true
		for _, n := range list.Children {
			switch list.Name.Local {
			case "roaSpecAdd":
				r, err := decodeROA(n)
				if err != nil {
					failures = append(failures, err)
				} else {
					result.ROAs = append(result.ROAs, *r)
				}
			case "aspaAdd":
				a, err := decodeASPA(n)
				if err != nil {
					failures = append(failures, err)
				} else {
					result.ASPAs = append(result.ASPAs, *a)
				}
			case "roaSpecDelete":
				if n.Name.Local != "roaHandle" || n.Name.Space != rpkiNamespace || len(n.Children) != 0 || !handlePattern.MatchString(strings.TrimSpace(n.Text)) {
					failures = append(failures, errors.New("ARIN returned invalid ROA deletion identity"))
				} else {
					result.DeletedROAs = append(result.DeletedROAs, strings.TrimSpace(n.Text))
				}
			case "aspaDelete":
				if n.Name.Local != "customerAsId" || n.Name.Space != rpkiNamespace || len(n.Children) != 0 {
					failures = append(failures, errors.New("ARIN returned invalid ASPA deletion identity"))
					continue
				}
				asn, err := strconv.ParseInt(strings.TrimSpace(n.Text), 10, 64)
				if err != nil || validateRPKIASN(asn, false) != nil {
					failures = append(failures, errors.New("ARIN returned invalid ASPA deletion identity"))
				} else {
					result.DeletedASPAs = append(result.DeletedASPAs, asn)
				}
			default:
				failures = append(failures, errors.New("ARIN returned unsupported RPKI transaction content"))
			}
		}
		if list.Name.Local != "roaSpecAdd" && list.Name.Local != "aspaAdd" && list.Name.Local != "roaSpecDelete" && list.Name.Local != "aspaDelete" {
			failures = append(failures, errors.New("ARIN returned unsupported RPKI transaction fields"))
		}
	}
	return result, errors.Join(failures...)
}

// ApplyRPKITransaction submits once. A failed or lost response may follow an
// accepted write; callers must reconcile the inventories before retrying.
func (c *Client) ApplyRPKITransaction(ctx context.Context, org string, t RPKITransaction) (*RPKITransactionResult, error) {
	if !handlePattern.MatchString(org) {
		return nil, errors.New("invalid organization handle")
	}
	body, err := t.marshal()
	if err != nil {
		return nil, err
	}
	unlock, err := lockOrganization(ctx, c.baseURL, org)
	if err != nil {
		return nil, err
	}
	defer unlock()
	response, err := c.request(ctx, http.MethodPost, c.baseURL, "/rest/rpki/"+url.PathEscape(org), "application/xml", true, body)
	if err != nil {
		return nil, err
	}
	result, err := decodeRPKITransaction(response.Body)
	if err != nil {
		return result, fmt.Errorf("RPKI transaction response could not be verified: %w", err)
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return result, errors.New("ARIN did not confirm completed RPKI transaction")
	}
	if len(result.ROAs) != len(t.AddROAs) || len(result.ASPAs) != len(t.AddASPAs) {
		return result, errors.New("ARIN did not return every RPKI addition; reconcile before retrying")
	}
	if err := c.verifyRPKITransaction(ctx, org, t, result); err != nil {
		return result, err
	}
	return result, nil
}

// ROAMatchesRequest compares the authorization semantics, including native
// defaults for omitted maxLength, rather than generated dates or resource order.
func ROAMatchesRequest(roa ROA, want ROARequest) bool {
	if roa.AutoLink != nil && *roa.AutoLink != want.AutoLink {
		return false
	}
	if roa.Name != want.Name || roa.ASN != want.ASN || len(roa.Resources) != len(want.Resources) {
		return false
	}
	for _, requested := range want.Resources {
		found := false
		for _, actual := range roa.Resources {
			if actual.Prefix != requested.Prefix || actual.AutoLinked != want.AutoLink {
				continue
			}
			prefix, err := netip.ParsePrefix(requested.Prefix)
			if err != nil {
				return false
			}
			wantedMax, actualMax := int64(prefix.Bits()), int64(prefix.Bits())
			if requested.MaxLength != nil {
				wantedMax = *requested.MaxLength
			}
			if actual.MaxLength != nil {
				actualMax = *actual.MaxLength
			}
			if wantedMax == actualMax {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func ASPAEqual(a, b ASPA) bool {
	if a.CustomerASN != b.CustomerASN {
		return false
	}
	first, second := slices.Clone(a.ProviderASNs), slices.Clone(b.ProviderASNs)
	slices.Sort(first)
	slices.Sort(second)
	return slices.Equal(first, second)
}
func (c *Client) verifyRPKITransaction(ctx context.Context, org string, t RPKITransaction, result *RPKITransactionResult) error {
	if len(t.AddROAs)+len(t.DeleteROAs) > 0 {
		current, err := c.ListROAs(ctx, org)
		if err != nil {
			return err
		}
		added := map[string]bool{}
		matched := make([]bool, len(t.AddROAs))
		for index, r := range result.ROAs {
			if added[r.Handle] {
				return errors.New("ARIN returned duplicate added ROA handles")
			}
			added[r.Handle] = true
			match := -1
			for i, want := range t.AddROAs {
				if !matched[i] && ROAMatchesRequest(r, want) {
					match = i
					break
				}
			}
			if match < 0 {
				return errors.New("ARIN returned an unexpected ROA addition")
			}
			matched[match] = true
			found := false
			for _, actual := range current {
				if actual.Handle == r.Handle && ROAMatchesRequest(actual, t.AddROAs[match]) {
					result.ROAs[index] = actual
					found = true
					break
				}
			}
			if !found {
				return errors.New("ARIN inventory does not confirm the added ROA")
			}
		}
		for _, deleted := range t.DeleteROAs {
			if added[deleted.Handle] {
				continue
			}
			for _, actual := range current {
				if actual.Handle == deleted.Handle {
					return errors.New("ARIN inventory still contains a deleted ROA")
				}
			}
		}
	}
	if len(t.AddASPAs)+len(t.DeleteASPAs) > 0 {
		current, err := c.ListASPAs(ctx, org)
		if err != nil {
			return err
		}
		added := map[int64]bool{}
		for _, a := range result.ASPAs {
			if added[a.CustomerASN] {
				return errors.New("ARIN returned duplicate added ASPA customers")
			}
			added[a.CustomerASN] = true
			found := false
			for _, want := range t.AddASPAs {
				if ASPAEqual(a, want) {
					for _, actual := range current {
						if ASPAEqual(actual, want) {
							found = true
							break
						}
					}
					break
				}
			}
			if !found {
				return errors.New("ARIN inventory does not confirm the requested ASPA addition")
			}
		}
		for _, deleted := range t.DeleteASPAs {
			if added[deleted] {
				continue
			}
			for _, actual := range current {
				if actual.CustomerASN == deleted {
					return errors.New("ARIN inventory still contains a deleted ASPA")
				}
			}
		}
	}
	return nil
}
