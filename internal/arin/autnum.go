package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Autnum is the complete writable representation of an XML (simple) IRR aut-num.
// Dates, source, and POC links are server-owned. POCs are read-only output.
type Autnum struct {
	Name, ASName, OrgHandle                                                                                                    string
	Description, Remarks, MemberOf, ImportPolicy, ExportPolicy, DefaultPolicy, MPImportPolicy, MPExportPolicy, MPDefaultPolicy []string
	POCs                                                                                                                       []IRRPOC
	CreationDate, LastModifiedDate                                                                                             string
}
type autnumXML struct {
	XMLName     xml.Name     `xml:"http://www.arin.net/regrws/core/v1 autnum"`
	Description []irrLine    `xml:"description>line"`
	OrgHandle   string       `xml:"orgHandle"`
	Remarks     *irrLinesXML `xml:"remarks,omitempty"`
	Source      string       `xml:"source"`
	MemberOf    []irrMember  `xml:"memberOf"`
	Name        string       `xml:"asNumber"`
	ASName      string       `xml:"asName"`
	Import      *irrLinesXML `xml:"import,omitempty"`
	Export      *irrLinesXML `xml:"export,omitempty"`
	Default     *irrLinesXML `xml:"default,omitempty"`
	MPImport    *irrLinesXML `xml:"mpImport,omitempty"`
	MPExport    *irrLinesXML `xml:"mpExport,omitempty"`
	MPDefault   *irrLinesXML `xml:"mpDefault,omitempty"`
}

func ValidateAutnumName(name string) error {
	if !strings.HasPrefix(name, "AS") {
		return errors.New("as_number must start with AS")
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(name, "AS"), 10, 32)
	if err != nil || n == 0 || "AS"+strconv.FormatUint(n, 10) != name {
		return errors.New("as_number must be canonical AS1 through AS4294967295")
	}
	return nil
}
func (s Autnum) Validate() error {
	if err := ValidateAutnumName(s.Name); err != nil {
		return err
	}
	if !handlePattern.MatchString(s.OrgHandle) || s.OrgHandle != strings.ToUpper(s.OrgHandle) {
		return errors.New("org_handle must be an uppercase ARIN organization handle")
	}
	if len(s.Description) == 0 {
		return errors.New("description must contain at least one line")
	}
	for _, lines := range [][]string{s.Description, s.Remarks, s.ImportPolicy, s.ExportPolicy, s.DefaultPolicy, s.MPImportPolicy, s.MPExportPolicy, s.MPDefaultPolicy} {
		for _, line := range lines {
			if strings.TrimSpace(line) != line || line == "" || strings.ContainsAny(line, "\r\n") {
				return errors.New("description and remarks must contain nonempty individual lines without surrounding whitespace")
			}
		}
	}
	if !handlePattern.MatchString(s.ASName) {
		return errors.New("as_name must be an alphanumeric identifier with optional hyphens")
	}
	for _, m := range s.MemberOf {
		if err := ValidateASSetName(m); err != nil {
			return fmt.Errorf("member_of: %w", err)
		}
	}

	return nil
}
func (s Autnum) marshal() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	p := autnumXML{Name: s.Name, ASName: s.ASName, OrgHandle: s.OrgHandle, Source: "ARIN"}
	for i, v := range s.Description {
		p.Description = append(p.Description, irrLine{i, v})
	}
	if len(s.Remarks) > 0 {
		p.Remarks = &irrLinesXML{}
		for i, v := range s.Remarks {
			p.Remarks.Lines = append(p.Remarks.Lines, irrLine{i, v})
		}
	}
	for _, v := range s.MemberOf {
		p.MemberOf = append(p.MemberOf, irrMember{v})
	}
	p.Import = xmlPolicy(s.ImportPolicy)
	p.Export = xmlPolicy(s.ExportPolicy)
	p.Default = xmlPolicy(s.DefaultPolicy)
	p.MPImport = xmlPolicy(s.MPImportPolicy)
	p.MPExport = xmlPolicy(s.MPExportPolicy)
	p.MPDefault = xmlPolicy(s.MPDefaultPolicy)

	return xml.Marshal(p)
}
func decodeAutnum(body []byte, name string) (*Autnum, error) {
	root, err := parseXML(body)
	if err != nil {
		return nil, err
	}
	if root.Name.Local != "autnum" || root.Name.Space != "http://www.arin.net/regrws/core/v1" {
		return nil, errors.New("ARIN returned an unexpected aut-num payload; only simple XML objects are supported")
	}
	// Refuse extensions we cannot round-trip instead of silently dropping them on PUT.
	allowed := map[string]bool{"asNumber": true, "asName": true, "import": true, "export": true, "default": true, "mpImport": true, "mpExport": true, "mpDefault": true, "memberOf": true, "orgHandle": true, "source": true, "description": true, "remarks": true, "pocLinks": true, "creationDate": true, "lastModifiedDate": true}
	for _, child := range root.Children {
		if child.Name.Space != root.Name.Space || !allowed[child.Name.Local] {
			return nil, errors.New("ARIN returned unsupported aut-num fields; refusing to manage a partial representation")
		}
	}
	values, err := decodeFields(root, autnumFields)
	if err != nil {
		return nil, err
	}
	str := func(key string) string { v, _ := values[key].(string); return v }
	if str("as_number") != name || str("as_name") == "" || str("org_handle") == "" || str("source") != "ARIN" {
		return nil, errors.New("ARIN returned an incomplete or mismatched aut-num")
	}
	list := func(key string) []string {
		out := []string{}
		for _, v := range values[key].([]any) {
			out = append(out, v.(string))
		}
		return out
	}
	s := &Autnum{Name: str("as_number"), OrgHandle: str("org_handle"), Description: list("description"), Remarks: list("remarks"), ASName: str("as_name"), MemberOf: list("member_of"), ImportPolicy: list("import_policy"), ExportPolicy: list("export_policy"), DefaultPolicy: list("default_policy"), MPImportPolicy: list("mp_import_policy"), MPExportPolicy: list("mp_export_policy"), MPDefaultPolicy: list("mp_default_policy"), CreationDate: str("creation_date"), LastModifiedDate: str("last_modified_date")}
	for _, v := range values["poc_links"].([]any) {
		p := v.(map[string]any)
		h, _ := p["handle"].(string)
		f, _ := p["function"].(string)
		if h == "" || (f != "AD" && f != "T" && f != "R") {
			return nil, errors.New("ARIN returned an unsupported IRR POC link")
		}
		s.POCs = append(s.POCs, IRRPOC{Handle: h, Function: f, Description: netString(p, "description")})
	}
	return s, nil
}
func (c *Client) GetAutnum(ctx context.Context, name string) (*Autnum, error) {
	if err := ValidateAutnumName(name); err != nil {
		return nil, err
	}
	body, err := c.get(ctx, c.baseURL, "/rest/irr/aut-num/"+url.PathEscape(name), "application/xml", true)
	if err != nil {
		return nil, err
	}
	return decodeAutnum(body, name)
}
func (c *Client) CreateAutnum(ctx context.Context, s Autnum) (*Autnum, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	// Existing objects must be imported, never silently adopted or overwritten.
	if _, err := c.GetAutnum(ctx, s.Name); err == nil {
		return nil, errors.New("aut-num already exists; import it before managing it")
	} else if !IsNotFound(err) {
		return nil, err
	}
	return c.writeAutnum(ctx, http.MethodPost, "/rest/irr/aut-num/"+url.PathEscape(s.Name), s)
}
func (c *Client) UpdateAutnum(ctx context.Context, s Autnum) (*Autnum, error) {
	return c.writeAutnum(ctx, http.MethodPut, "/rest/irr/aut-num/"+url.PathEscape(s.Name), s)
}

func (c *Client) writeAutnum(ctx context.Context, method, path string, s Autnum) (*Autnum, error) {
	payload, err := s.marshal()
	if err != nil {
		return nil, err
	}
	response, err := c.request(ctx, method, c.baseURL, path, "application/xml", true, payload)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return nil, errors.New("ARIN did not return the completed aut-num; verify its status before retrying")
	}
	result, err := decodeAutnum(response.Body, s.Name)
	if err != nil {
		return nil, fmt.Errorf("ARIN accepted the write but its result could not be verified: %w", err)
	}
	if result.OrgHandle != s.OrgHandle {
		return nil, errors.New("ARIN returned a mismatched aut-num organization after the write")
	}
	return result, nil
}
func (c *Client) DeleteAutnum(ctx context.Context, name string) error {
	if err := ValidateAutnumName(name); err != nil {
		return err
	}
	response, err := c.request(ctx, http.MethodDelete, c.baseURL, "/rest/irr/aut-num/"+url.PathEscape(name), "application/xml", true, nil)
	if IsNotFound(err) {
		return nil
	}
	if err == nil && response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		return errors.New("ARIN did not confirm completed deletion; refresh before retrying")
	}
	return err
}

func xmlPolicy(lines []string) *irrLinesXML {
	if len(lines) == 0 {
		return nil
	}
	out := &irrLinesXML{}
	for i, line := range lines {
		out.Lines = append(out.Lines, irrLine{i, line})
	}
	return out
}
