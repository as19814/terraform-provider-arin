package arin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

var errRPKIRRDPHTTP = errors.New("RPKI repository retrieval failed")

type rrdpHTTPClient struct {
	Transport http.RoundTripper
	Timeout   time.Duration
}
type rrdpNotificationResult struct {
	Notification *rrdpNotification
	LastModified time.Time
	NotModified  bool
}

// FetchNotification sends If-Modified-Since only when the caller has cached
// metadata. A 304 never produces an empty replacement notification.
func (c rrdpHTTPClient) FetchNotification(ctx context.Context, uri string, lastModified time.Time) (rrdpNotificationResult, error) {
	body, modified, unchanged, err := c.get(ctx, uri, 4<<20, lastModified)
	if err != nil {
		return rrdpNotificationResult{}, err
	}
	if unchanged {
		return rrdpNotificationResult{LastModified: lastModified, NotModified: true}, nil
	}
	n, err := parseRRDPNotification(body)
	if err != nil {
		return rrdpNotificationResult{}, err
	}
	return rrdpNotificationResult{Notification: n, LastModified: modified}, nil
}

// FetchSnapshot checks the referenced hash and notification identity after a
// bounded HTTPS download. Its output is not yet authenticated RPKI data.
func (c rrdpHTTPClient) FetchSnapshot(ctx context.Context, n *rrdpNotification) (map[string][]byte, error) {
	if n == nil || !rrdpSession.MatchString(n.Session) || !rrdpSerial.MatchString(n.Serial) {
		return nil, errRPKIRRDP
	}
	body, _, _, err := c.get(ctx, n.Snapshot.URI, 128<<20, time.Time{})
	if err != nil {
		return nil, err
	}
	return parseRRDPSnapshot(body, n.Session, n.Serial, n.Snapshot.Hash)
}

func (c rrdpHTTPClient) get(ctx context.Context, uri string, limit int64, lastModified time.Time) ([]byte, time.Time, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, false, err
	}
	if !validRRDPURL(uri) || limit <= 0 || limit > 128<<20 {
		return nil, time.Time{}, false, errRPKIRRDPHTTP
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	if timeout < 0 || timeout > 5*time.Minute {
		return nil, time.Time{}, false, errRPKIRRDPHTTP
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, time.Time{}, false, errRPKIRRDPHTTP
	}
	request.Header.Set("Accept", "application/xml")
	if !lastModified.IsZero() {
		request.Header.Set("If-Modified-Since", lastModified.UTC().Format(http.TimeFormat))
	}
	client := &http.Client{Transport: c.Transport, Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !validRRDPURL(req.URL.String()) {
			return errRPKIRRDPHTTP
		}
		return nil
	}}
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, time.Time{}, false, ctx.Err()
		}
		return nil, time.Time{}, false, errRPKIRRDPHTTP
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		if lastModified.IsZero() {
			return nil, time.Time{}, false, errRPKIRRDPHTTP
		}
		return nil, lastModified, true, nil
	}
	if response.StatusCode != http.StatusOK || response.ContentLength > limit {
		return nil, time.Time{}, false, errRPKIRRDPHTTP
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, time.Time{}, false, errRPKIRRDPHTTP
	}
	var modified time.Time
	if value := response.Header.Get("Last-Modified"); value != "" {
		modified, err = http.ParseTime(value)
		if err != nil {
			return nil, time.Time{}, false, errRPKIRRDPHTTP
		}
	}
	return body, modified, false, nil
}
