package arin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type RPKIPublicationRecoveryReport struct {
	RequestSHA256  string            `json:"request_sha256"`
	RecoveryPeerID string            `json:"recovery_peer_id"`
	Outcome        string            `json:"outcome"`
	Committed      bool              `json:"committed"`
	Sent           time.Time         `json:"sent"`
	Received       time.Time         `json:"received"`
	Before         map[string]string `json:"before"`
	After          map[string]string `json:"after"`
}

// RecoverRPKIPublication observes when expected is empty. Otherwise expected must
// explicitly select matches_before or matches_after for durable reconciliation.
// It sends inventory reads only, never a publication mutation.
func RecoverRPKIPublication(ctx context.Context, config RPKIPublicationReadConfig, digest, expected string) (*RPKIPublicationRecoveryReport, error) {
	return recoverRPKIPublication(ctx, config, digest, expected, nil)
}
func recoverRPKIPublication(ctx context.Context, config RPKIPublicationReadConfig, digest, expected string, clock func() time.Time) (*RPKIPublicationRecoveryReport, error) {
	if !exchangeDigest.MatchString(digest) || (expected != "" && expected != "matches_before" && expected != "matches_after") {
		return nil, errRPKIExchangeState
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	exchange, err := config.exchange()
	if err != nil {
		return nil, err
	}
	exchange.Clock = clock
	var observation rpkiPublicationRecoveryObservation
	if expected == "" {
		observation, err = exchange.observePendingPublication(ctx, digest)
	} else {
		observation, err = exchange.reconcilePendingPublication(ctx, digest, expected)
	}
	if err != nil {
		probe := exchange
		probe.RecoveryOf = digest
		if peer, peerErr := probe.peerID(); peerErr == nil {
			return nil, fmt.Errorf("publication recovery failed (recovery journal peer %s): %w", peer, err)
		}
		return nil, err
	}
	return &RPKIPublicationRecoveryReport{RequestSHA256: observation.Plan.RequestSHA256, RecoveryPeerID: observation.RecoveryPeerID, Outcome: observation.Outcome, Committed: expected != "", Sent: observation.Sent, Received: observation.Received, Before: observation.Plan.Before, After: observation.Plan.After}, nil
}

// LoadRPKIPublicationConfig loads a bounded private JSON configuration using the
// Terraform BPKI attribute names. It accepts no private key bytes or object data.
func LoadRPKIPublicationConfig(path string) (RPKIPublicationReadConfig, error) {
	var config RPKIPublicationReadConfig
	fields := rpkiConfigFields(&config)
	fields["publisher_handle"] = &config.Publisher
	if err := loadPrivateRPKIConfig(path, fields); err != nil {
		return RPKIPublicationReadConfig{}, err
	}
	return config, nil
}

func rpkiConfigFields(config *RPKIPublicationReadConfig) map[string]*string {
	return map[string]*string{"endpoint": &config.Endpoint, "journal_directory": &config.JournalDirectory, "signing_key_file": &config.SigningKeyFile, "signing_certificate_pem": &config.SigningCertificatePEM, "signing_ca_pem": &config.SigningAnchorPEM, "signing_intermediates_pem": &config.SigningIntermediatesPEM, "signing_crls_pem": &config.SigningCRLsPEM, "peer_ca_pem": &config.PeerAnchorPEM, "peer_intermediates_pem": &config.PeerIntermediatesPEM}
}

func loadPrivateRPKIConfig(path string, fields map[string]*string) error {
	return loadPrivateRPKIConfigFields(path, fields, nil)
}
func loadPrivateRPKIConfigFields(path string, fields map[string]*string, lists map[string]*[]string) error {
	fail := func() error { return errRPKIIdentityConfig }
	if !filepath.IsAbs(path) {
		return fail()
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16<<20 {
		return fail()
	}
	f, err := os.Open(path)
	if err != nil {
		return fail()
	}
	actual, statErr := f.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	closeErr := f.Close()
	if statErr != nil || !os.SameFile(info, actual) || readErr != nil || closeErr != nil || len(raw) > 16<<20 {
		return fail()
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return fail()
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return fail()
		}
		name, ok := token.(string)
		dest, known := fields[name]
		list, isList := lists[name]
		if !ok || (!known && !isList) || seen[name] {
			return fail()
		}
		seen[name] = true
		token, err = d.Token()
		if err != nil {
			return fail()
		}
		if isList {
			if token != json.Delim('[') {
				return fail()
			}
			values := []string{}
			for d.More() {
				v, err := d.Token()
				s, ok := v.(string)
				if err != nil || !ok || s == "" || len(values) >= 31 {
					return fail()
				}
				values = append(values, s)
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') || len(values) == 0 {
				return fail()
			}
			*list = values
			continue
		}
		value, ok := token.(string)
		if !ok {
			return fail()
		}
		*dest = value
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return fail()
	}
	for name, value := range fields {
		if name != "signing_intermediates_pem" && name != "peer_intermediates_pem" && *value == "" {
			return fail()
		}
	}
	for name := range lists {
		if !seen[name] {
			return fail()
		}
	}
	return nil
}
