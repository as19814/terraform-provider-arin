// Package arin implements the ARIN registration API independently of Terraform.
package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	ProductionURL    = "https://reg.arin.net"
	OTEURL           = "https://reg.ote.arin.net"
	DefaultTimeout   = 30 * time.Second
	maxResponseBytes = 4 << 20
)

// Config is immutable after New. HTTPClient allows callers to supply a transport.
type Config struct {
	APIKey     string
	BaseURL    string
	Timeout    time.Duration
	UserAgent  string
	HTTPClient *http.Client
}

// Client is safe for concurrent use. Credentials are never placed in request URLs.
type Client struct {
	baseURL   string
	apiKey    string
	userAgent string
	http      *http.Client
}

func ValidateBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return errors.New("base_url must be an absolute HTTPS URL")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("base_url must not include credentials, a path, a query, or a fragment")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return errors.New("base_url requires HTTPS; HTTP is allowed only for loopback test servers")
	}
	return nil
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" || strings.ContainsAny(cfg.APIKey, "\r\n") {
		return nil, errors.New("api_key must be nonempty and must not contain line breaks")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = ProductionURL
	}
	if err := ValidateBaseURL(cfg.BaseURL); err != nil {
		return nil, err
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.Timeout < 0 {
		return nil, errors.New("timeout must be positive")
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "terraform-provider-arin/dev"
	}
	hc := http.Client{}
	if cfg.HTTPClient != nil {
		hc = *cfg.HTTPClient
	}
	hc.Timeout = cfg.Timeout
	// Never forward credentials or replay mutations through redirects.
	hc.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: strings.TrimRight(cfg.BaseURL, "/"), apiKey: cfg.APIKey, userAgent: cfg.UserAgent, http: &hc}, nil
}

// APIError exposes status and sanitized ARIN error fields without retaining a raw body.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	detail := e.Code
	if e.Message != "" {
		if detail != "" {
			detail += ": "
		}
		detail += e.Message
	}
	if detail == "" {
		detail = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("ARIN returned HTTP %d: %s", e.StatusCode, detail)
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// getXML performs one request. Retry policies belong to individual API operations.
func (c *Client) getXML(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return errors.New("could not construct ARIN request")
	}
	req.Header.Set("Authorization", "ApiKey "+c.apiKey)
	req.Header.Set("Accept", "application/xml")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("ARIN request interrupted: %w", ctx.Err())
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return errors.New("ARIN request timed out")
		}
		// Do not expose transport errors that may include URLs or custom headers.
		return errors.New("ARIN request failed; check connectivity and TLS configuration")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return errors.New("could not read ARIN response")
	}
	if len(body) > maxResponseBytes {
		return errors.New("ARIN response exceeded the 4 MiB limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload struct {
			XMLName xml.Name `xml:"error"`
			Code    string   `xml:"code"`
			Message string   `xml:"message"`
		}
		apiErr := &APIError{StatusCode: resp.StatusCode}
		if xml.Unmarshal(body, &payload) == nil {
			apiErr.Code = c.redact(payload.Code)
			apiErr.Message = c.redact(payload.Message)
		}
		return apiErr
	}
	if err := xml.Unmarshal(body, out); err != nil {
		return errors.New("ARIN returned an invalid or unexpected XML payload")
	}
	return nil
}

func (c *Client) redact(s string) string {
	s = strings.ReplaceAll(s, c.apiKey, "[REDACTED]")
	s = strings.ReplaceAll(s, url.QueryEscape(c.apiKey), "[REDACTED]")
	return s
}

var handlePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// Organization contains only the fields currently exposed by the read-only data source.
// It must not be serialized as a replacement payload for an organization update.
type Organization struct {
	XMLName          xml.Name `xml:"http://www.arin.net/regrws/core/v1 org"`
	Handle           string   `xml:"handle"`
	Name             string   `xml:"orgName"`
	RegistrationDate string   `xml:"registrationDate"`
}

func (c *Client) GetOrganization(ctx context.Context, handle string) (*Organization, error) {
	if !handlePattern.MatchString(handle) {
		return nil, errors.New("organization handle must contain only letters, digits, and hyphens, starting with a letter or digit")
	}
	var org Organization
	if err := c.getXML(ctx, "/rest/org/"+url.PathEscape(handle), &org); err != nil {
		return nil, err
	}
	if org.Handle == "" || org.Name == "" || !strings.EqualFold(org.Handle, handle) {
		return nil, errors.New("ARIN returned an incomplete or mismatched organization")
	}
	return &org, nil
}
