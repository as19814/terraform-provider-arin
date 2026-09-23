package arin

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"strings"
	"time"
)

// RetrievePath assembles publications for an explicitly supplied leaf-first CA
// chain. Notification URIs correspond to each non-anchor certificate's issuer.
// The result is untrusted until the caller validates it against its configured
// anchor, including persistent manifest history and issuance-response binding.
func (c rrdpHTTPClient) RetrievePath(ctx context.Context, directory string, certificates []*x509.Certificate, notifications []string, now time.Time) (rpkiIssuePath, error) {
	fail := func() (rpkiIssuePath, error) { return rpkiIssuePath{}, errRPKIRRDP }
	if len(certificates) < 2 || len(certificates) > 32 || len(notifications) != len(certificates)-1 {
		return fail()
	}
	result := rpkiIssuePath{Certificates: make([]*x509.Certificate, len(certificates))}
	manifests := make([]string, len(notifications))
	for i, cert := range certificates {
		if cert == nil || len(cert.Raw) == 0 || len(cert.Raw) > 512000 {
			return fail()
		}
		parsed, err := x509.ParseCertificate(bytes.Clone(cert.Raw))
		if err != nil || validateRPKICAProfile(parsed) != nil {
			return fail()
		}
		result.Certificates[i] = parsed
		if i == 0 {
			continue
		}
		if !validRRDPURL(notifications[i-1]) {
			return fail()
		}
		sia, err := rpkiCASIA(parsed.Extensions)
		var descriptions []rpkiAccessDescription
		if err != nil || !rpkiCSRDER(sia, &descriptions) {
			return fail()
		}
		for _, d := range descriptions {
			uri := string(d.Location.Bytes)
			if d.Method.Equal(oidRPKIManifest) && strings.HasPrefix(uri, "rsync://") {
				if manifests[i-1] != "" && manifests[i-1] != uri {
					return fail()
				}
				manifests[i-1] = uri
			}
		}
		if manifests[i-1] == "" {
			return fail()
		}
	}
	// Keep repositories scoped to their notification URI, even when object URIs
	// collide. Shared notifications are refreshed only once per resolution.
	repositories := map[string]rrdpRepository{}
	total, repositoryTotal := 0, 0
	for i, notification := range notifications {
		if err := ctx.Err(); err != nil {
			return rpkiIssuePath{}, err
		}
		repository, ok := repositories[notification]
		if !ok {
			cache, err := c.RefreshPersistent(ctx, directory, notification, now)
			if err != nil && !errors.Is(err, errRPKIRRDPPoll) {
				return rpkiIssuePath{}, err
			}
			if cache.Repository.Session == "" {
				return fail()
			}
			repository = cache.Repository
			for _, data := range repository.Objects {
				repositoryTotal += len(data)
				if repositoryTotal > 128<<20 {
					return fail()
				}
			}
			repositories[notification] = repository
		}
		manifestURI := manifests[i]
		manifestDER := repository.Objects[manifestURI]
		manifest, err := decodeRPKIManifest(manifestDER)
		if err != nil {
			return fail()
		}
		publication := rpkiPathPublication{ManifestURI: manifestURI, Files: map[string][]byte{}}
		total += len(manifestDER)
		if total > 128<<20 {
			return fail()
		}
		publication.ManifestDER = bytes.Clone(manifestDER)
		prefix := manifestURI[:strings.LastIndexByte(manifestURI, '/')+1]
		for name := range manifest.Content.Files {
			data, ok := repository.Objects[prefix+name]
			if !ok {
				return fail()
			}
			total += len(data)
			if total > 128<<20 {
				return fail()
			}
			publication.Files[name] = bytes.Clone(data)
			if strings.HasSuffix(name, ".cer") && bytes.Equal(data, result.Certificates[i].Raw) {
				if publication.ChildURI != "" {
					return fail()
				}
				publication.ChildURI = prefix + name
			}
		}
		if publication.ChildURI == "" {
			return fail()
		}
		result.Publications = append(result.Publications, publication)
	}
	return result, nil
}
