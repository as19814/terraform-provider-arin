package arin

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const DownloadProductionURL = "https://accountws.arin.net"
const DownloadOTEURL = "https://accountws.ote.arin.net"

// DownloadRequest selects documented bulk Whois or invalid-POC artifacts.
// Empty objects with zip format selects the complete bulk archive endpoint.
type DownloadRequest struct {
	Kind, Format string
	Objects      []string
}
type DownloadMetadata struct {
	Filename, ContentType, SHA256 string
	SizeBytes                     int64
}

func (d DownloadRequest) path() (string, error) {
	if d.Kind == "invalid_pocs" {
		if len(d.Objects) != 0 || d.Format != "zip" {
			return "", errors.New("invalid_pocs requires zip format and no object selection")
		}
		return "/public/rest/downloads/nvpr", nil
	}
	if d.Kind != "bulk_whois" {
		return "", errors.New("download kind must be bulk_whois or invalid_pocs")
	}
	if !slices.Contains([]string{"zip", "xml", "txt"}, d.Format) {
		return "", errors.New("bulk Whois format must be zip, xml or txt")
	}
	objects := slices.Clone(d.Objects)
	slices.Sort(objects)
	for i, object := range objects {
		if !slices.Contains([]string{"asns", "nets", "orgs", "pocs"}, object) || i > 0 && object == objects[i-1] {
			return "", errors.New("objects must be unique selections from asns, nets, orgs and pocs")
		}
	}
	if len(objects) == 0 {
		if d.Format == "zip" {
			return "/public/rest/downloads/bulkwhois", nil
		}
		objects = []string{"asns", "nets", "orgs", "pocs"}
	}
	return "/public/rest/downloads/bulkwhois/" + strings.Join(objects, "+") + "." + d.Format, nil
}
func (d DownloadRequest) Validate() error { _, err := d.path(); return err }

// DownloadTo streams an approved artifact to a caller-owned destination. On any
// error the destination may contain partial bytes and must be discarded. Callers
// writing files should use a temporary file and rename only after success. There
// is no report creation, redirect, archive extraction or response-link following.
// ARIN documents query-key authentication for this separate account service.
func (c *Client) DownloadTo(ctx context.Context, d DownloadRequest, dst io.Writer, maxBytes int64) (*DownloadMetadata, error) {
	path, err := d.path()
	if err != nil {
		return nil, err
	}
	if c.downloadBaseURL == "" {
		return nil, errors.New("download_base_url is required with a custom registration origin")
	}
	if c.apiKey == "" {
		return nil, errors.New("API key is required for authenticated downloads")
	}
	if dst == nil || maxBytes <= 0 {
		return nil, errors.New("downloads require a destination and positive byte limit")
	}
	query := url.Values{"apikey": {c.apiKey}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.downloadBaseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return nil, errors.New("could not construct authenticated download")
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Accept", "application/octet-stream, application/zip, application/xml, text/plain")
	req.Header.Set("User-Agent", c.userAgent)
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ARIN download interrupted: %w", ctx.Err())
		}
		return nil, errors.New("ARIN download failed; check connectivity and TLS configuration")
	}
	defer response.Body.Close()
	// Never include accountws bodies, Location headers or authenticated URLs in
	// errors. Those can echo the query API key, including transformed encodings.
	if response.StatusCode != http.StatusOK {
		return nil, &APIError{StatusCode: response.StatusCode}
	}
	if response.ContentLength > maxBytes {
		return nil, errors.New("ARIN download exceeds configured byte limit")
	}
	if enc := response.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
		return nil, errors.New("unexpected download content encoding")
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil {
		return nil, errors.New("invalid download content type")
	}
	allowed := media == "application/octet-stream" || d.Format == "zip" && slices.Contains([]string{"application/zip", "application/x-zip-compressed"}, media) || d.Format == "xml" && slices.Contains([]string{"application/xml", "text/xml"}, media) || d.Format == "txt" && media == "text/plain"
	if !allowed {
		return nil, errors.New("unexpected download content type")
	}
	filename := ""
	if disposition := response.Header.Get("Content-Disposition"); disposition != "" {
		_, params, err := mime.ParseMediaType(disposition)
		if err != nil {
			return nil, errors.New("invalid download content disposition")
		}
		filename = params["filename"]
		if strings.ContainsAny(filename, "/\\\r\n") || filename == "." || filename == ".." {
			return nil, errors.New("unsafe download filename")
		}
		filename = c.redact(filename)
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(dst, hash), io.LimitReader(response.Body, maxBytes))
	if err != nil {
		return nil, errors.New("could not stream ARIN download; discard partial output")
	}
	var extra [1]byte
	n, err := io.ReadFull(response.Body, extra[:])
	if n != 0 {
		return nil, errors.New("ARIN download exceeds configured byte limit; discard partial output")
	}
	if err != io.EOF {
		return nil, errors.New("could not finish ARIN download; discard partial output")
	}
	if written == 0 || response.ContentLength >= 0 && written != response.ContentLength {
		return nil, errors.New("empty or incomplete ARIN download; discard partial output")
	}
	return &DownloadMetadata{Filename: filename, ContentType: media, SHA256: fmt.Sprintf("%x", hash.Sum(nil)), SizeBytes: written}, nil
}
