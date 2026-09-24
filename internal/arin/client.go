// Package arin implements the ARIN registration API independently of Terraform.
package arin

import (
	"bytes"
	"context"
	"encoding/json"
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
	ProductionURL      = "https://reg.arin.net"
	OTEURL             = "https://reg.ote.arin.net"
	RDAPProductionURL  = "https://rdap.arin.net"
	RDAPOTEURL         = "https://rdap.ote.arin.net"
	WhoisProductionURL = "https://whois.arin.net"
	WhoisOTEURL        = "https://whois.ote.arin.net"
	DefaultTimeout     = 30 * time.Second
	maxResponseBytes   = 4 << 20
)

// Config is immutable after New. HTTPClient allows callers to supply a transport.
type Config struct {
	APIKey          string
	BaseURL         string
	RDAPBaseURL     string
	WhoisBaseURL    string
	DownloadBaseURL string
	Timeout         time.Duration
	UserAgent       string
	HTTPClient      *http.Client
}

// Client is safe for concurrent use. Credentials are never placed in request URLs.
type Client struct {
	baseURL         string
	rdapBaseURL     string
	whoisBaseURL    string
	downloadBaseURL string
	apiKey          string
	userAgent       string
	http            *http.Client
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
	if (cfg.APIKey != "" && strings.TrimSpace(cfg.APIKey) == "") || strings.ContainsAny(cfg.APIKey, "\r\n") {
		return nil, errors.New("api_key must be nonempty and must not contain line breaks")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = ProductionURL
	}
	if err := ValidateBaseURL(cfg.BaseURL); err != nil {
		return nil, err
	}
	if cfg.RDAPBaseURL == "" {
		switch strings.TrimRight(cfg.BaseURL, "/") {
		case ProductionURL:
			cfg.RDAPBaseURL = RDAPProductionURL
		case OTEURL:
			cfg.RDAPBaseURL = RDAPOTEURL
		}
	}
	if cfg.RDAPBaseURL != "" {
		if err := ValidateBaseURL(cfg.RDAPBaseURL); err != nil {
			return nil, fmt.Errorf("invalid rdap_base_url: %w", err)
		}
	}
	if cfg.WhoisBaseURL == "" {
		switch strings.TrimRight(cfg.BaseURL, "/") {
		case ProductionURL:
			cfg.WhoisBaseURL = WhoisProductionURL
		case OTEURL:
			cfg.WhoisBaseURL = WhoisOTEURL
		}
	}
	if cfg.WhoisBaseURL != "" {
		if err := ValidateBaseURL(cfg.WhoisBaseURL); err != nil {
			return nil, fmt.Errorf("invalid whois_base_url: %w", err)
		}
	}
	if cfg.DownloadBaseURL == "" {
		switch strings.TrimRight(cfg.BaseURL, "/") {
		case ProductionURL:
			cfg.DownloadBaseURL = DownloadProductionURL
		case OTEURL:
			cfg.DownloadBaseURL = DownloadOTEURL
		}
	}
	if cfg.DownloadBaseURL != "" {
		if err := ValidateBaseURL(cfg.DownloadBaseURL); err != nil {
			return nil, fmt.Errorf("invalid download_base_url: %w", err)
		}
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
	return &Client{baseURL: strings.TrimRight(cfg.BaseURL, "/"), rdapBaseURL: strings.TrimRight(cfg.RDAPBaseURL, "/"), whoisBaseURL: strings.TrimRight(cfg.WhoisBaseURL, "/"), downloadBaseURL: strings.TrimRight(cfg.DownloadBaseURL, "/"), apiKey: cfg.APIKey, userAgent: cfg.UserAgent, http: &hc}, nil
}

// APIError exposes status and sanitized ARIN error fields without retaining a raw body.
type APIError struct {
	// True only for a complete, structured RDAP 404 response. Search callers
	// can distinguish no matches from a proxy or wrong-origin HTTP 404.
	rdapNotFound bool
	// Native Whois 404 page explicitly identifies an empty result, not an unknown handle.
	whoisNoMatches bool
	// Some hierarchy searches express no matches as an empty array on HTTP 404.
	rdapEmptyDomains  bool
	rdapEmptyNetworks bool
	StatusCode        int
	Code              string
	Message           string
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
	body, err := c.get(ctx, c.baseURL, path, "application/xml", true)
	if err != nil {
		return err
	}
	if err := xml.Unmarshal(body, out); err != nil {
		return errors.New("ARIN returned an invalid or unexpected XML payload")
	}
	return nil
}

type readResponse struct {
	StatusCode int
	Body       []byte
	Header     http.Header
}

func (c *Client) get(ctx context.Context, origin, path, accept string, authenticated bool) ([]byte, error) {
	response, err := c.fetch(ctx, origin, path, accept, authenticated)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

func (c *Client) fetch(ctx context.Context, origin, path, accept string, authenticated bool) (*readResponse, error) {
	return c.request(ctx, http.MethodGet, origin, path, accept, authenticated, nil)
}

// request makes one application-level call. Ticket-creating report GETs use
// requestReport to also prevent transport-level replay after a lost response.
func (c *Client) request(ctx context.Context, method, origin, path, accept string, authenticated bool, payload []byte) (*readResponse, error) {
	return c.doRequest(ctx, method, origin, path, accept, authenticated, payload, true)
}

// requestReport uses a non-rewindable empty body so Go's HTTP transport does not
// replay a ticket-creating GET after a reused connection loses its response.
func (c *Client) requestReport(ctx context.Context, path string) (*readResponse, error) {
	return c.doRequest(ctx, http.MethodGet, c.baseURL, path, "application/xml", true, nil, false)
}

func (c *Client) doRequest(ctx context.Context, method, origin, path, accept string, authenticated bool, payload []byte, replayable bool) (*readResponse, error) {
	if authenticated && c.apiKey == "" {
		return nil, errors.New("api_key or ARIN_API_KEY is required for Reg-RWS operations")
	}
	req, err := http.NewRequestWithContext(ctx, method, origin+path, bytes.NewReader(payload))
	if err != nil {
		return nil, errors.New("could not construct ARIN request")
	}
	if !replayable {
		// Do not use http.NoBody or a rewindable bytes.Reader here. Both make
		// GET eligible for transport retries. Preserve mutation payloads while
		// preventing an append from being replayed after a lost response.
		req.Body = io.NopCloser(bytes.NewReader(payload))
		req.GetBody = nil
	}
	if authenticated {
		req.Header.Set("Authorization", "ApiKey "+c.apiKey)
		// ASPA reads require Content-Type even though GET has no body.
		req.Header.Set("Content-Type", "application/xml")
		if accept == rpslMediaType {
			req.Header.Set("Content-Type", rpslMediaType)
		}
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ARIN request interrupted: %w", ctx.Err())
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, errors.New("ARIN request timed out")
		}
		// Do not expose transport errors that may include URLs or custom headers.
		return nil, errors.New("ARIN request failed; check connectivity and TLS configuration")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errors.New("could not read ARIN response")
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("ARIN response exceeded the 4 MiB limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload struct {
			XMLName    xml.Name `xml:"error"`
			Code       string   `xml:"code"`
			Message    string   `xml:"message"`
			Components []struct {
				Name    string `xml:"name"`
				Message string `xml:"message"`
			} `xml:"components>component"`
			AdditionalInfo []string `xml:"additionalInfo>message"`
		}
		apiErr := &APIError{StatusCode: resp.StatusCode}
		if !authenticated && origin == c.whoisBaseURL && accept == "application/xml" && resp.StatusCode == http.StatusNotFound {
			page := string(body)
			apiErr.whoisNoMatches = strings.Contains(page, "<title>Whois-RWS</title>") && (strings.Contains(page, "Sorry, no related resources were found for the handle provided.") || strings.Contains(page, "Sorry, there were no results."))
		}
		if xml.Unmarshal(body, &payload) == nil {
			apiErr.Code = c.redact(payload.Code)
			details := []string{payload.Message}
			for _, component := range payload.Components {
				if component.Name != "" {
					details = append(details, component.Name+": "+component.Message)
				} else {
					details = append(details, component.Message)
				}
			}
			details = append(details, payload.AdditionalInfo...)
			apiErr.Message = c.redact(strings.Join(details, "; "))
		}
		if accept == "application/rdap+json" {
			var rdapError struct {
				ErrorCode   *int               `json:"errorCode"`
				Domains     *[]json.RawMessage `json:"domainSearchResults"`
				Networks    *[]json.RawMessage `json:"ipSearchResults"`
				ASNs        *[]json.RawMessage `json:"autnumSearchResults"`
				Entities    *[]json.RawMessage `json:"entitySearchResults"`
				Title       string             `json:"title"`
				Description []string           `json:"description"`
			}
			if json.Unmarshal(body, &rdapError) == nil {
				apiErr.Message = c.redact(strings.Join(append([]string{rdapError.Title}, rdapError.Description...), " "))
				notFoundCode := rdapError.ErrorCode != nil && *rdapError.ErrorCode == http.StatusNotFound
				emptyResults := true
				for _, records := range []*[]json.RawMessage{rdapError.Domains, rdapError.Networks, rdapError.ASNs, rdapError.Entities} {
					if records != nil && len(*records) > 0 {
						emptyResults = false
					}
				}
				apiErr.rdapNotFound = resp.StatusCode == http.StatusNotFound && notFoundCode && emptyResults && checkRDAPErrorCompleteness(body) == nil
				apiErr.rdapEmptyDomains = resp.StatusCode == http.StatusNotFound && emptyResults && (rdapError.ErrorCode == nil || notFoundCode) && rdapError.Domains != nil && len(*rdapError.Domains) == 0 && checkRDAPErrorCompleteness(body) == nil
				apiErr.rdapEmptyNetworks = resp.StatusCode == http.StatusNotFound && emptyResults && (rdapError.ErrorCode == nil || notFoundCode) && rdapError.Networks != nil && len(*rdapError.Networks) == 0 && checkRDAPErrorCompleteness(body) == nil
			}
		}
		return nil, apiErr
	}
	return &readResponse{Body: body, Header: resp.Header.Clone(), StatusCode: resp.StatusCode}, nil
}

func (c *Client) redact(s string) string {
	if c.apiKey == "" {
		return s
	}
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
