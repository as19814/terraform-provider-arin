package arin

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"strings"
	"time"
)

// DiscoverPath resolves intermediate CA certificates through their child's rsync
// AIA references inside explicitly configured RRDP repositories. Notifications
// remain ordered immediate issuer to anchor. No AIA URL is fetched directly.
// The returned path is untrusted until anchored manifest/history validation.
func (c rrdpHTTPClient) DiscoverPath(ctx context.Context, directory string, leaf, anchor *x509.Certificate, notifications []string, now time.Time) (rpkiIssuePath, error) {
	fail := func() (rpkiIssuePath, error) { return rpkiIssuePath{}, errRPKIRRDP }
	if len(notifications) == 0 || len(notifications) > 31 || leaf == nil || anchor == nil || now.IsZero() {
		return fail()
	}
	for _, uri := range notifications {
		if !validRRDPURL(uri) {
			return fail()
		}
	}
	parse := func(cert *x509.Certificate) (*x509.Certificate, error) {
		if len(cert.Raw) == 0 || len(cert.Raw) > 512000 {
			return nil, errRPKIRRDP
		}
		parsed, err := x509.ParseCertificate(bytes.Clone(cert.Raw))
		if err != nil || validateRPKICAProfile(parsed) != nil {
			return nil, errRPKIRRDP
		}
		return parsed, nil
	}
	current, err := parse(leaf)
	if err != nil {
		return fail()
	}
	trusted, err := parse(anchor)
	if err != nil {
		return fail()
	}
	path := []*x509.Certificate{current}
	seen := map[string]bool{rpkiManifestDigest(current.Raw): true}
	repositories := map[string]bool{}
	var totalBytes int64
	totalObjects, totalURIBytes := 0, 0
	certificateBytes := len(current.Raw)
	for depth := 0; depth < len(notifications); depth++ {
		if err := ctx.Err(); err != nil {
			return rpkiIssuePath{}, err
		}
		issuer := trusted
		if depth < len(notifications)-1 {
			notification := notifications[depth+1]
			cache, err := c.RefreshDiskPersistent(ctx, directory, notification, now)
			if err != nil && !errors.Is(err, errRPKIRRDPPoll) {
				cache.Close()
				return rpkiIssuePath{}, err
			}
			if cache == nil || cache.Record.Session == "" {
				cache.Close()
				return fail()
			}
			if !repositories[notification] {
				repositories[notification] = true
				totalBytes += cache.Record.Size
				totalObjects += len(cache.Record.Entries)
				for uri := range cache.Record.Entries {
					totalURIBytes += len(uri)
				}
			}
			if totalBytes > 2<<30 || totalObjects > 1000000 || totalURIBytes > 128<<20 {
				cache.Close()
				return fail()
			}
			issuer = nil
			for _, uri := range current.IssuingCertificateURL {
				if !publicationURI(uri) || !strings.HasSuffix(uri, ".cer") {
					continue
				}
				if _, exists := cache.Record.Entries[uri]; !exists {
					continue
				}
				raw, readErr := cache.Objects.ReadObject(uri)
				if readErr != nil || len(raw) > 512000 {
					cache.Close()
					return fail()
				}
				candidate, parseErr := x509.ParseCertificate(raw)
				if parseErr != nil || validateRPKICAProfile(candidate) != nil {
					cache.Close()
					return fail()
				}
				if issuer != nil && !bytes.Equal(issuer.Raw, candidate.Raw) {
					cache.Close()
					return fail()
				}
				issuer = candidate
			}
			if err := cache.Close(); err != nil {
				return fail()
			}
			if issuer == nil || bytes.Equal(issuer.Raw, trusted.Raw) {
				return fail()
			}
		}
		if !bytes.Equal(current.RawIssuer, issuer.RawSubject) || len(current.AuthorityKeyId) == 0 || !bytes.Equal(current.AuthorityKeyId, issuer.SubjectKeyId) || current.CheckSignatureFrom(issuer) != nil {
			return fail()
		}
		digest := rpkiManifestDigest(issuer.Raw)
		if seen[digest] {
			return fail()
		}
		seen[digest] = true
		certificateBytes += len(issuer.Raw)
		if certificateBytes > 4<<20 {
			return fail()
		}
		path = append(path, issuer)
		current = issuer
	}
	return c.RetrievePath(ctx, directory, path, notifications, now)
}
