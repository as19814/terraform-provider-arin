package arin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const rpslMediaType = "application/rpsl"

// RPSLKey identifies one advanced object. Routes also require an origin ASN.
type RPSLKey struct{ Kind, Name, OriginAS string }

func (k RPSLKey) Validate() error {
	switch k.Kind {
	case "route", "route6":
		if err := ValidateIRRRouteID(k.Name + "," + k.OriginAS); err != nil {
			return err
		}
		prefix, _ := netip.ParsePrefix(k.Name)
		if (k.Kind == "route") != prefix.Addr().Is4() {
			return errors.New("RPSL object type does not match the prefix address family")
		}
		return nil
	case "as-set":
		if err := ValidateASSetName(k.Name); err != nil {
			return err
		}
	case "route-set":
		if err := ValidateRouteSetName(k.Name); err != nil {
			return err
		}
	case "aut-num":
		if err := ValidateAutnumName(k.Name); err != nil {
			return err
		}
	default:
		return errors.New("RPSL object type must be route, route6, as-set, route-set or aut-num")
	}
	if k.OriginAS != "" {
		return errors.New("origin ASN is only valid for RPSL routes")
	}
	return nil
}
func (k RPSLKey) path() (string, error) {
	if err := k.Validate(); err != nil {
		return "", err
	}
	if k.Kind == "route" || k.Kind == "route6" {
		return routePath(k.Name + "," + k.OriginAS)
	}
	return "/rest/irr/" + k.Kind + "/" + url.PathEscape(k.Name), nil
}

// RPSLObject preserves attributes unknown to the provider. ARIN validates the
// complete routing policy grammar; the client validates object boundaries,
// identity and ownership before submitting it.
type RPSLObject struct {
	Key       RPSLKey
	OrgHandle string
	Text      string
}

var rpslAttribute = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func ParseRPSL(raw string) (*RPSLObject, error) {
	if !utf8.ValidString(raw) || len(raw) > maxResponseBytes {
		return nil, errors.New("RPSL must be valid UTF-8 and no larger than 4 MiB")
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	fields := map[string][]string{}
	current := ""
	ended := false
	first := ""
	for _, line := range strings.Split(raw, "\n") {
		for _, r := range line {
			if (r < 32 && r != '\t') || r == 127 {
				return nil, errors.New("RPSL contains unsupported control characters")
			}
		}
		if strings.TrimSpace(line) == "" {
			if current != "" {
				ended = true
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if ended {
			return nil, errors.New("RPSL must contain exactly one object without internal blank lines")
		}
		if line[0] == ' ' || line[0] == '\t' || line[0] == '+' {
			if current == "" {
				return nil, errors.New("RPSL continuation has no preceding attribute")
			}
			last := len(fields[current]) - 1
			fields[current][last] += " " + strings.TrimSpace(strings.TrimPrefix(line, "+"))
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		name = strings.ToLower(name)
		if !ok || !rpslAttribute.MatchString(name) {
			return nil, errors.New("RPSL contains an invalid attribute line")
		}
		if first == "" {
			first = name
		}
		current = name
		fields[name] = append(fields[name], strings.TrimSpace(value))
	}
	one := func(name string) (string, error) {
		values := fields[name]
		if len(values) != 1 {
			return "", fmt.Errorf("RPSL requires exactly one %s attribute", name)
		}
		value, _, _ := strings.Cut(values[0], "#")
		value = strings.TrimSpace(value)
		if value == "" {
			return "", fmt.Errorf("RPSL %s cannot be empty", name)
		}
		return value, nil
	}
	name, err := one(first)
	if err != nil {
		return nil, err
	}
	key := RPSLKey{Kind: first, Name: name}
	for _, kind := range []string{"route", "route6", "as-set", "route-set", "aut-num"} {
		if kind != first && len(fields[kind]) != 0 {
			return nil, errors.New("RPSL contains multiple object identities")
		}
	}
	if key.Kind == "route" || key.Kind == "route6" {
		key.OriginAS, err = one("origin")
		if err != nil {
			return nil, err
		}
	} else if len(fields["origin"]) != 0 {
		return nil, errors.New("origin attribute is only valid for RPSL routes")
	}
	if err := key.Validate(); err != nil {
		return nil, err
	}
	source, err := one("source")
	if err != nil || source != "ARIN" {
		return nil, errors.New("RPSL requires exactly one source: ARIN attribute")
	}
	maintainer, err := one("mnt-by")
	if err != nil {
		return nil, err
	}
	org := strings.TrimPrefix(maintainer, "MNT-")
	if org == maintainer || !handlePattern.MatchString(org) || org != strings.ToUpper(org) {
		return nil, errors.New("RPSL mnt-by must identify one uppercase MNT- organization handle")
	}
	return &RPSLObject{Key: key, OrgHandle: org, Text: strings.TrimSpace(raw) + "\n"}, nil
}

func (c *Client) GetRPSL(ctx context.Context, key RPSLKey) (*RPSLObject, error) {
	path, err := key.path()
	if err != nil {
		return nil, err
	}
	body, err := c.get(ctx, c.baseURL, path, rpslMediaType, true)
	if err != nil {
		return nil, err
	}
	result, err := ParseRPSL(string(body))
	if err != nil {
		return nil, fmt.Errorf("ARIN returned invalid RPSL: %w", err)
	}
	if result.Key != key {
		return nil, errors.New("ARIN returned a mismatched RPSL object")
	}
	return result, nil
}
func (c *Client) CreateRPSL(ctx context.Context, raw string) (*RPSLObject, error) {
	input, err := ParseRPSL(raw)
	if err != nil {
		return nil, err
	}
	if _, err := c.GetRPSL(ctx, input.Key); err == nil {
		return nil, errors.New("IRR object already exists; import it before managing it")
	} else if !IsNotFound(err) {
		return nil, err
	}
	path, _ := input.Key.path()
	if input.Key.Kind == "as-set" || input.Key.Kind == "route-set" {
		path = "/rest/irr/" + input.Key.Kind + "?orgHandle=" + url.QueryEscape(input.OrgHandle)
	}
	return c.writeRPSL(ctx, http.MethodPost, path, input)
}
func (c *Client) UpdateRPSL(ctx context.Context, raw string) (*RPSLObject, error) {
	input, err := ParseRPSL(raw)
	if err != nil {
		return nil, err
	}
	current, err := c.GetRPSL(ctx, input.Key)
	if err != nil {
		return nil, err
	}
	if current.OrgHandle != input.OrgHandle {
		return nil, errors.New("RPSL organization differs from configuration; refusing update")
	}
	path, _ := input.Key.path()
	return c.writeRPSL(ctx, http.MethodPut, path, input)
}
func (c *Client) writeRPSL(ctx context.Context, method, path string, input *RPSLObject) (*RPSLObject, error) {
	response, err := c.request(ctx, method, c.baseURL, path, rpslMediaType, true, []byte(input.Text))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return nil, errors.New("ARIN did not confirm a completed RPSL write; refresh before retrying")
	}
	out, err := ParseRPSL(string(response.Body))
	if err != nil {
		return nil, fmt.Errorf("ARIN accepted RPSL but its result could not be verified: %w", err)
	}
	if out.Key != input.Key || out.OrgHandle != input.OrgHandle {
		return nil, errors.New("ARIN returned a mismatched RPSL identity or organization after the write")
	}
	return out, nil
}
func (c *Client) DeleteRPSL(ctx context.Context, key RPSLKey, org string) error {
	if !handlePattern.MatchString(org) || org != strings.ToUpper(org) {
		return errors.New("invalid expected RPSL organization")
	}
	current, err := c.GetRPSL(ctx, key)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.OrgHandle != org {
		return errors.New("RPSL organization differs from state; refusing deletion")
	}
	path, _ := key.path()
	response, err := c.request(ctx, http.MethodDelete, c.baseURL, path, rpslMediaType, true, nil)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if response.StatusCode != 200 && response.StatusCode != 204 {
		return errors.New("ARIN did not confirm completed RPSL deletion; refresh before retrying")
	}
	return nil
}
