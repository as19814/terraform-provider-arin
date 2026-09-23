package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ASSet is the complete writable representation of an XML (simple) IRR AS set.
// Dates, source, and POC links are server-owned. POCs are read-only output.
type ASSet struct {
	Name, OrgHandle                             string
	Description, Remarks, Members, MembersByRef []string
	POCs                                        []IRRPOC
	CreationDate, LastModifiedDate              string
}
type IRRPOC struct{ Handle, Function string }
type irrLine struct {
	Number int    `xml:"number,attr"`
	Text   string `xml:",chardata"`
}
type irrMember struct {
	Name string `xml:"name,attr"`
}
type irrLinesXML struct {
	Lines []irrLine `xml:"line"`
}
type asSetXML struct {
	XMLName      xml.Name     `xml:"http://www.arin.net/regrws/core/v1 asSet"`
	Description  []irrLine    `xml:"description>line"`
	OrgHandle    string       `xml:"orgHandle"`
	Remarks      *irrLinesXML `xml:"remarks,omitempty"`
	Source       string       `xml:"source"`
	Members      []irrMember  `xml:"members>member"`
	MembersByRef []irrMember  `xml:"membersByRef>memberByRef"`
	Name         string       `xml:"name"`
}

var asSetNamePattern = regexp.MustCompile(`^(?:AS[0-9]+:|AS-[A-Z0-9][A-Z0-9_-]*:)*AS-[A-Z0-9][A-Z0-9_-]*$`)
var asMemberPattern = regexp.MustCompile(`^AS[0-9]+$`)

func ValidateASSetName(name string) error {
	if !asSetNamePattern.MatchString(name) {
		return errors.New("AS set name must be uppercase, start with AS-, or be a colon-separated hierarchy ending in AS- followed by a name")
	}
	return nil
}
func (s ASSet) Validate() error {
	if err := ValidateASSetName(s.Name); err != nil {
		return err
	}
	if !handlePattern.MatchString(s.OrgHandle) || s.OrgHandle != strings.ToUpper(s.OrgHandle) {
		return errors.New("org_handle must be an uppercase ARIN organization handle")
	}
	if len(s.Description) == 0 {
		return errors.New("description must contain at least one line")
	}
	for _, lines := range [][]string{s.Description, s.Remarks} {
		for _, line := range lines {
			if strings.TrimSpace(line) != line || line == "" || strings.ContainsAny(line, "\r\n") {
				return errors.New("description and remarks must contain nonempty individual lines without surrounding whitespace")
			}
		}
	}
	for _, m := range s.Members {
		if asMemberPattern.MatchString(m) {
			n, err := strconv.ParseUint(strings.TrimPrefix(m, "AS"), 10, 32)
			if err != nil || n == 0 || "AS"+strconv.FormatUint(n, 10) != m {
				return errors.New("AS members must use canonical AS numbers from AS1 through AS4294967295")
			}
		}
		if !asMemberPattern.MatchString(m) && ValidateASSetName(m) != nil {
			return errors.New("members must contain uppercase AS numbers or AS set names")
		}
	}
	for _, m := range s.MembersByRef {
		if m != "ANY" && (!strings.HasPrefix(m, "MNT-") || !handlePattern.MatchString(strings.TrimPrefix(m, "MNT-")) || m != strings.ToUpper(m)) {
			return errors.New("members_by_ref must contain ANY or uppercase MNT- organization handles")
		}
	}

	return nil
}
func (s ASSet) marshal() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	p := asSetXML{Name: s.Name, OrgHandle: s.OrgHandle, Source: "ARIN"}
	for i, v := range s.Description {
		p.Description = append(p.Description, irrLine{i, v})
	}
	if len(s.Remarks) > 0 {
		p.Remarks = &irrLinesXML{}
		for i, v := range s.Remarks {
			p.Remarks.Lines = append(p.Remarks.Lines, irrLine{i, v})
		}
	}
	for _, v := range s.Members {
		p.Members = append(p.Members, irrMember{v})
	}
	for _, v := range s.MembersByRef {
		p.MembersByRef = append(p.MembersByRef, irrMember{v})
	}

	return xml.Marshal(p)
}
func decodeASSet(body []byte, name string) (*ASSet, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "asSet" || root.Name.Space != "http://www.arin.net/regrws/core/v1" {
		return nil, errors.New("ARIN returned an unexpected AS set payload; only simple XML objects are supported")
	}
	// Refuse extensions we cannot round-trip instead of silently dropping them on PUT.
	allowed := map[string]bool{"name": true, "orgHandle": true, "source": true, "description": true, "remarks": true, "pocLinks": true, "members": true, "membersByRef": true, "creationDate": true, "lastModifiedDate": true}
	for _, child := range root.Children {
		if child.Name.Space != root.Name.Space || !allowed[child.Name.Local] {
			return nil, errors.New("ARIN returned unsupported AS set fields; refusing to manage a partial representation")
		}
	}
	values, err := decodeFields(root, setFields)
	if err != nil {
		return nil, err
	}
	str := func(key string) string { v, _ := values[key].(string); return v }
	if str("name") != name || str("org_handle") == "" || str("source") != "ARIN" {
		return nil, errors.New("ARIN returned an incomplete or mismatched AS set")
	}
	list := func(key string) []string {
		out := []string{}
		for _, v := range values[key].([]any) {
			out = append(out, v.(string))
		}
		return out
	}
	s := &ASSet{Name: str("name"), OrgHandle: str("org_handle"), Description: list("description"), Remarks: list("remarks"), Members: list("members"), MembersByRef: list("members_by_ref"), CreationDate: str("creation_date"), LastModifiedDate: str("last_modified_date")}
	for _, v := range values["poc_links"].([]any) {
		p := v.(map[string]any)
		h, _ := p["handle"].(string)
		f, _ := p["function"].(string)
		if h == "" || (f != "AD" && f != "T" && f != "R") {
			return nil, errors.New("ARIN returned an unsupported IRR POC link")
		}
		s.POCs = append(s.POCs, IRRPOC{h, f})
	}
	return s, nil
}
func (c *Client) GetASSet(ctx context.Context, name string) (*ASSet, error) {
	if err := ValidateASSetName(name); err != nil {
		return nil, err
	}
	body, err := c.get(ctx, c.baseURL, "/rest/irr/as-set/"+url.PathEscape(name), "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodeASSet(body, name)
}
func (c *Client) CreateASSet(ctx context.Context, s ASSet) (*ASSet, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	// Existing objects must be imported, never silently adopted or overwritten.
	if _, err := c.GetASSet(ctx, s.Name); err == nil {
		return nil, errors.New("AS set already exists; import it before managing it")
	} else if !IsNotFound(err) {
		return nil, err
	}
	return c.writeASSet(ctx, http.MethodPost, "/rest/irr/as-set?orgHandle="+url.QueryEscape(s.OrgHandle), s)
}
func (c *Client) UpdateASSet(ctx context.Context, s ASSet) (*ASSet, error) {
	return c.writeASSet(ctx, http.MethodPut, "/rest/irr/as-set/"+url.PathEscape(s.Name), s)
}

func (c *Client) writeASSet(ctx context.Context, method, path string, s ASSet) (*ASSet, error) {
	payload, err := s.marshal()
	if err != nil {
		return nil, err
	}
	response, err := c.request(ctx, method, c.baseURL, path, "application/xml", true, payload)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return nil, errors.New("ARIN did not return the completed AS set; verify its status before retrying")
	}
	result, err := decodeASSet(response.Body, s.Name)
	if err != nil {
		return nil, fmt.Errorf("ARIN accepted the write but its result could not be verified: %w", err)
	}
	if result.OrgHandle != s.OrgHandle {
		return nil, errors.New("ARIN returned a mismatched AS set organization after the write")
	}
	return result, nil
}
func (c *Client) DeleteASSet(ctx context.Context, name string) error {
	if err := ValidateASSetName(name); err != nil {
		return err
	}
	response, err := c.request(ctx, http.MethodDelete, c.baseURL, "/rest/irr/as-set/"+url.PathEscape(name), "application/xml", true, nil)
	if IsNotFound(err) {
		return nil
	}
	if err == nil && response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		return errors.New("ARIN did not confirm completed deletion; refresh before retrying")
	}
	return err
}
