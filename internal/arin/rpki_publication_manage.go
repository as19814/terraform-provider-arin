package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
)

// DecodeRPKIPublicationObjects validates a desired bundle of caller-signed
// objects. The 3 MiB aggregate decoded limit leaves room for base64; the final
// encoded batch is additionally subject to the protocol's 4 MiB limit.
func DecodeRPKIPublicationObjects(encoded map[string]string) (map[string][]byte, map[string]string, error) {
	if len(encoded) > 10000 {
		return nil, nil, errRPKIPublicationBatch
	}
	objects := make(map[string][]byte, len(encoded))
	hashes := make(map[string]string, len(encoded))
	total := 0
	for uri, value := range encoded {
		if !publicationURI(uri) || strings.HasSuffix(uri, "/") || len(value) > 4<<20 {
			return nil, nil, errRPKIPublicationBatch
		}
		data, err := base64.StdEncoding.Strict().DecodeString(value)
		if err != nil || len(data) == 0 || base64.StdEncoding.EncodeToString(data) != value {
			return nil, nil, errRPKIPublicationBatch
		}
		total += len(data)
		if total > 3<<20 {
			return nil, nil, errRPKIPublicationBatch
		}
		objects[uri] = data
		hashes[uri] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return objects, hashes, nil
}

// ApplyRPKIPublicationBundle creates, replaces and withdraws the owned objects
// in one atomic protocol request. Prior hashes must come from Terraform's last
// inventory refresh. Objects outside the union of desired and prior are untouched.
func ApplyRPKIPublicationBundle(ctx context.Context, config RPKIPublicationReadConfig, desired map[string]string, prior map[string]string) error {
	changes, err := planRPKIPublicationBundle(desired, prior)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		return nil
	}
	exchange, err := config.exchange()
	if err != nil {
		return err
	}
	return (rpkiPublicationClient{Exchange: exchange}).Apply(ctx, changes)
}
func planRPKIPublicationBundle(desired map[string]string, prior map[string]string) ([]rpkiPublicationChange, error) {
	objects, hashes, err := DecodeRPKIPublicationObjects(desired)
	if err != nil {
		return nil, err
	}
	if len(prior) > 10000 {
		return nil, errRPKIPublicationBatch
	}
	uris := make(map[string]bool, len(desired)+len(prior))
	for uri := range desired {
		uris[uri] = true
	}
	for uri, hash := range prior {
		if !publicationURI(uri) || strings.HasSuffix(uri, "/") || !exchangeDigest.MatchString(hash) {
			return nil, errRPKIPublicationBatch
		}
		uris[uri] = true
	}
	keys := make([]string, 0, len(uris))
	for uri := range uris {
		keys = append(keys, uri)
	}
	sort.Strings(keys)
	var changes []rpkiPublicationChange
	for _, uri := range keys {
		data, wanted := objects[uri]
		old, exists := prior[uri]
		if wanted && exists && hashes[uri] == old {
			continue
		}
		changes = append(changes, rpkiPublicationChange{URI: uri, OldSHA256: old, DER: bytes.Clone(data), Withdraw: !wanted})
	}
	// Validate aggregate wire limits before loading the private key or dispatching.
	if len(changes) > 0 {
		if _, err := buildRPKIPublicationBatch(changes); err != nil {
			return nil, err
		}
	}
	return changes, nil
}
