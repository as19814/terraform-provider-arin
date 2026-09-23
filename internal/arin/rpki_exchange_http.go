package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type rpkiHTTPExchange struct {
	Endpoint, MediaType, Directory, PeerScope string
	Identity                                  rpkiCMSSigningIdentity
	PeerAnchor                                *x509.Certificate
	PeerIntermediates                         []*x509.Certificate
	Transport                                 http.RoundTripper
	Timeout                                   time.Duration
	Clock                                     func() time.Time
	RecoveryOf                                string
	MinimumSent, MinimumReceived              time.Time
}

var errRPKIHTTPExchange = errors.New("RPKI exchange failed; inspect the pending journal before retrying")

func (c rpkiHTTPExchange) peerID() (string, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || strings.TrimSpace(c.PeerScope) == "" || len(c.PeerScope) > 4096 {
		return "", errors.New("invalid RPKI exchange configuration")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return "", errors.New("RPKI endpoint requires HTTPS except for loopback tests")
	}
	if c.MediaType != "application/rpki-updown" && c.MediaType != "application/rpki-publication" {
		return "", errors.New("unsupported RPKI protocol media type")
	}
	if c.Identity.Anchor == nil || c.PeerAnchor == nil || len(c.Identity.Anchor.Raw) == 0 || len(c.PeerAnchor.Raw) == 0 {
		return "", errors.New("RPKI exchange requires local and peer trust anchors")
	}
	if c.RecoveryOf != "" && ((c.MediaType != "application/rpki-publication" && c.MediaType != "application/rpki-updown") || !exchangeDigest.MatchString(c.RecoveryOf)) {
		return "", errors.New("invalid RPKI recovery scope")
	}
	// Bind history to the protocol, endpoint, configured peer handles and stable
	// CA identities. EE certificate renewal does not create a new journal.
	key, _ := json.Marshal(struct {
		Endpoint, Media, Scope string
		Local, Peer            [32]byte
		RecoveryOf             string `json:",omitempty"`
	}{c.Endpoint, c.MediaType, c.PeerScope, sha256.Sum256(c.Identity.Anchor.Raw), sha256.Sum256(c.PeerAnchor.Raw), c.RecoveryOf})
	return fmt.Sprintf("%x", sha256.Sum256(key)), nil
}

// exchange sends exactly one POST. validate must authenticate protocol-level
// sender/recipient identities, response type and request correlation. The CMS
// layer authenticates the peer first; neither validation failure clears pending.
func (c rpkiHTTPExchange) exchange(ctx context.Context, operation string, requestXML []byte, validate func([]byte, []byte) error) (responseXML []byte, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if validate == nil {
		return nil, errors.New("RPKI response validator is required")
	}
	peerID, err := c.peerID()
	if err != nil {
		return nil, err
	}
	if !validExchangeTime(c.MinimumSent) || !validExchangeTime(c.MinimumReceived) {
		return nil, errRPKIExchangeState
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if timeout <= 0 || timeout > 5*time.Minute {
		return nil, errors.New("invalid RPKI exchange timeout")
	}
	clock := c.Clock
	if clock == nil {
		clock = time.Now
	}
	lease, err := openRPKIExchange(c.Directory, peerID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := lease.Close(); closeErr != nil {
			responseXML = nil
			err = closeErr
		}
	}()
	state, err := lease.State()
	if err != nil {
		return nil, err
	}
	if state.Pending != nil {
		return nil, errRPKIExchangeState
	}
	// Snapshot input so validation uses exactly what was signed and dispatched.
	if len(requestXML) > 4<<20 {
		return nil, errRPKICMSSigning
	}
	requestXML = bytes.Clone(requestXML)
	now := clock()
	lastSent := state.LastSent
	if c.MinimumSent.After(lastSent) {
		lastSent = c.MinimumSent
	}
	signed, err := signRPKICMS(requestXML, c.Identity, now, lastSent)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(signed))
	if err != nil {
		return nil, errors.New("invalid RPKI HTTP request")
	}
	req.GetBody = nil // Disable transport replay of the signed POST body.
	req.Header.Set("Content-Type", c.MediaType)
	req.Header.Set("Accept", c.MediaType)
	req.Header.Set("Accept-Encoding", "identity")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(signed))
	if err := lease.BeginSigned(digest, operation, now.UTC().Truncate(time.Second), signed); err != nil {
		return nil, err
	}
	client := &http.Client{Transport: c.Transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return nil, errRPKIHTTPExchange
	}
	defer response.Body.Close()
	media, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK || mediaErr != nil || media != c.MediaType || response.ContentLength > 4<<20 || (response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity") {
		return nil, errRPKIHTTPExchange
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		return nil, errRPKIHTTPExchange
	}
	lastReceived := state.LastReceived
	if c.MinimumReceived.After(lastReceived) {
		lastReceived = c.MinimumReceived
	}
	verified, err := verifyRPKICMS(body, rpkiCMSTrust{Anchor: c.PeerAnchor, Intermediates: c.PeerIntermediates, Now: clock(), LastSigningTime: lastReceived})
	if err != nil {
		return nil, errRPKIHTTPExchange
	}
	if err := validate(bytes.Clone(requestXML), bytes.Clone(verified.Content)); err != nil {
		return nil, errRPKIHTTPExchange
	}
	if err := lease.CompleteSigned(digest, verified.SigningTime, body); err != nil {
		return nil, err
	}
	return bytes.Clone(verified.Content), nil
}
