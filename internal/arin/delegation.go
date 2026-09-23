package arin

import (
	"context"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const delegationTTLNamespace = "http://www.arin.net/regrws/ttl/v1"

type DelegationNameserver struct {
	Name string `xml:",chardata"`
	TTL  *int64 `xml:"http://www.arin.net/regrws/ttl/v1 ttl,attr,omitempty"`
}
type DelegationDS struct {
	Algorithm  int64  `xml:"algorithm"`
	Digest     string `xml:"digest"`
	TTL        *int64 `xml:"http://www.arin.net/regrws/ttl/v1 ttl,omitempty"`
	DigestType int64  `xml:"digestType"`
	KeyTag     int64  `xml:"keyTag"`
}
type Delegation struct {
	Name        string
	Nameservers []DelegationNameserver
	DSRecords   []DelegationDS
}
type delegationXML struct {
	XMLName     xml.Name               `xml:"http://www.arin.net/regrws/core/v1 delegation"`
	Name        string                 `xml:"name,omitempty"`
	Keys        []DelegationDS         `xml:"delegationKeys>delegationKey"`
	Nameservers []DelegationNameserver `xml:"nameservers>nameserver"`
}

var dnsLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func validDNSHost(name string) bool {
	if len(name) > 253 || !strings.Contains(name, ".") {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if !dnsLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}
func ValidateDelegationName(name string) error {
	if !strings.HasSuffix(name, ".") || !validDNSHost(strings.TrimSuffix(name, ".")) ||
		(!strings.HasSuffix(name, ".in-addr.arpa.") && !strings.HasSuffix(name, ".ip6.arpa.")) {
		return errors.New("delegation name must be a lowercase reverse DNS zone ending in .in-addr.arpa. or .ip6.arpa.")
	}
	return nil
}
func validateDelegationTTL(ttl *int64) error {
	if ttl != nil && (*ttl < 0 || *ttl > 2147483647) {
		return errors.New("DNS TTL must be between 0 and 2147483647 seconds")
	}
	return nil
}
func (n DelegationNameserver) Validate() error {
	if !validDNSHost(n.Name) {
		return errors.New("nameserver must be a lowercase fully qualified hostname without a trailing dot")
	}
	return validateDelegationTTL(n.TTL)
}
func (d Delegation) Validate() error {
	if err := ValidateDelegationName(d.Name); err != nil {
		return err
	}
	names := map[string]bool{}
	for _, n := range d.Nameservers {
		if err := n.Validate(); err != nil {
			return err
		}
		if names[n.Name] {
			return errors.New("duplicate delegation nameserver")
		}
		names[n.Name] = true
	}
	keys := map[string]bool{}
	for _, k := range d.DSRecords {
		if k.Algorithm < 1 || k.Algorithm > 255 || k.DigestType < 1 || k.DigestType > 255 || k.KeyTag < 0 || k.KeyTag > 65535 {
			return errors.New("DS algorithm/digest type must be 1..255 and key tag 0..65535")
		}
		if err := validateDelegationTTL(k.TTL); err != nil {
			return err
		}
		digest, err := hex.DecodeString(k.Digest)
		if err != nil || len(digest) == 0 || strings.ToUpper(k.Digest) != k.Digest {
			return errors.New("DS digest must be nonempty uppercase hexadecimal")
		}
		size := map[int64]int{1: 20, 2: 32, 4: 48}[k.DigestType]
		if size != 0 && len(digest) != size {
			return fmt.Errorf("incorrect DS digest length for digest type %d", k.DigestType)
		}
		key := fmt.Sprintf("%d/%d/%d/%s", k.Algorithm, k.DigestType, k.KeyTag, k.Digest)
		if keys[key] {
			return errors.New("duplicate delegation DS record")
		}
		keys[key] = true
	}
	return nil
}
func (d Delegation) marshal() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	// OT&E requires the existing name in the body; omitting it is rejected as
	// an attempt to modify immutable identity.
	return xml.Marshal(delegationXML{Name: d.Name, Keys: d.DSRecords, Nameservers: d.Nameservers})
}
func decodeDelegation(body []byte, name string) (*Delegation, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "delegation" || root.Name.Space != registrationNamespace {
		return nil, errors.New("ARIN returned an unexpected delegation payload")
	}
	allowed := map[string]bool{}
	for _, p := range []string{"name", "nameservers", "nameservers/nameserver", "delegationKeys", "delegationKeys/delegationKey", "delegationKeys/delegationKey/algorithm", "delegationKeys/delegationKey/digest", "delegationKeys/delegationKey/digestType", "delegationKeys/delegationKey/keyTag", "delegationKeys/delegationKey/ttl"} {
		allowed[p] = true
	}
	var walk func(*xmlNode, string) error
	walk = func(node *xmlNode, path string) error {
		for _, a := range node.Attrs {
			if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
				continue
			}
			if path == "nameservers/nameserver" && a.Name.Local == "ttl" && (a.Name.Space == delegationTTLNamespace || a.Name.Space == "") {
				continue
			}
			if (path == "delegationKeys/delegationKey/algorithm" || path == "delegationKeys/delegationKey/digestType") && a.Name.Space == "" && a.Name.Local == "name" {
				continue
			}
			return errors.New("ARIN returned unsupported delegation attributes")
		}
		counts := map[string]int{}
		for _, child := range node.Children {
			p := child.Name.Local
			if path != "" {
				p = path + "/" + p
			}
			if !allowed[p] || (child.Name.Space != registrationNamespace && !(p == "delegationKeys/delegationKey/ttl" && child.Name.Space == delegationTTLNamespace)) {
				return errors.New("ARIN returned unsupported delegation fields")
			}
			counts[p]++
			if counts[p] > 1 && p != "nameservers/nameserver" && p != "delegationKeys/delegationKey" {
				return errors.New("ARIN returned duplicate delegation fields")
			}
			if err := walk(child, p); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return nil, err
	}
	values, err := decodeFields(root, delegationFields)
	if err != nil {
		return nil, err
	}
	canonical := strings.ToLower(strings.TrimSuffix(netString(values, "name"), ".")) + "."
	if canonical != name {
		return nil, errors.New("ARIN returned a mismatched delegation")
	}
	d := &Delegation{Name: canonical}
	ttl := func(v any) *int64 {
		n, ok := v.(int64)
		if !ok {
			return nil
		}
		return &n
	}
	for _, item := range values["nameservers"].([]any) {
		n := item.(map[string]any)
		d.Nameservers = append(d.Nameservers, DelegationNameserver{Name: strings.ToLower(strings.TrimSuffix(netString(n, "name"), ".")), TTL: ttl(n["ttl"])})
	}
	for _, item := range values["ds_records"].([]any) {
		k := item.(map[string]any)
		number := func(field string) (int64, error) {
			n, ok := k[field].(int64)
			if !ok {
				return 0, fmt.Errorf("ARIN omitted DS %s", field)
			}
			return n, nil
		}
		algorithm, err := number("algorithm")
		if err != nil {
			return nil, err
		}
		digestType, err := number("digest_type")
		if err != nil {
			return nil, err
		}
		keyTag, err := number("key_tag")
		if err != nil {
			return nil, err
		}
		d.DSRecords = append(d.DSRecords, DelegationDS{Algorithm: algorithm, DigestType: digestType, KeyTag: keyTag, Digest: strings.ToUpper(netString(k, "digest")), TTL: ttl(k["ttl"])})
	}
	if err := d.Validate(); err != nil {
		return nil, fmt.Errorf("ARIN returned an invalid delegation: %w", err)
	}
	return d, nil
}
func (c *Client) GetDelegation(ctx context.Context, name string) (*Delegation, error) {
	if err := ValidateDelegationName(name); err != nil {
		return nil, err
	}
	body, err := c.get(ctx, c.baseURL, "/rest/delegation/"+url.PathEscape(name), "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodeDelegation(body, name)
}
func (c *Client) UpdateDelegation(ctx context.Context, d Delegation) (*Delegation, error) {
	body, err := d.marshal()
	if err != nil {
		return nil, err
	}
	// Confirm the existing zone and reject fields we cannot faithfully represent.
	if _, err := c.GetDelegation(ctx, d.Name); err != nil {
		return nil, err
	}
	return c.writeDelegation(ctx, http.MethodPut, "/rest/delegation/"+url.PathEscape(d.Name), d.Name, body)
}
func (c *Client) writeDelegation(ctx context.Context, method, path, name string, body []byte) (*Delegation, error) {
	response, err := c.request(ctx, method, c.baseURL, path, "application/xml", true, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return nil, errors.New("ARIN did not confirm completed delegation modification")
	}
	return decodeDelegation(response.Body, name)
}
func (c *Client) SetDelegationNameserver(ctx context.Context, name string, n DelegationNameserver) (*Delegation, error) {
	if err := ValidateDelegationName(name); err != nil {
		return nil, err
	}
	if err := n.Validate(); err != nil {
		return nil, err
	}
	path := "/rest/delegation/" + url.PathEscape(name) + "/nameserver/" + url.PathEscape(n.Name)
	if n.TTL != nil {
		path += "?ttl=" + strconv.FormatInt(*n.TTL, 10)
	}
	return c.writeDelegation(ctx, http.MethodPost, path, name, nil)
}
func (c *Client) DeleteDelegationNameserver(ctx context.Context, name, server string) (*Delegation, error) {
	if err := ValidateDelegationName(name); err != nil {
		return nil, err
	}
	if err := (DelegationNameserver{Name: server}).Validate(); err != nil {
		return nil, err
	}
	return c.writeDelegation(ctx, http.MethodDelete, "/rest/delegation/"+url.PathEscape(name)+"/nameserver/"+url.PathEscape(server), name, nil)
}
func (c *Client) DeleteDelegationNameservers(ctx context.Context, name string) (*Delegation, error) {
	if err := ValidateDelegationName(name); err != nil {
		return nil, err
	}
	return c.writeDelegation(ctx, http.MethodDelete, "/rest/delegation/"+url.PathEscape(name)+"/nameservers", name, nil)
}
