package arin

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"math/big"
	"strings"
	"time"
)

var errRPKIManifest = errors.New("invalid RPKI manifest content or file set")

type rpkiManifestContent struct {
	Number                 *big.Int
	ThisUpdate, NextUpdate time.Time
	Files                  map[string][sha256.Size]byte
}

// parseRPKIManifestContent decodes RFC 9286 eContent only. It does not verify
// CMS, the manifest EE certificate, resource inheritance, or rollback history.
func parseRPKIManifestContent(der []byte) (*rpkiManifestContent, error) {
	if len(der) == 0 || len(der) > 4<<20 {
		return nil, errRPKIManifest
	}
	fields, err := rpkiResourceSequence(der)
	// DER omits the DEFAULT version zero. Other versions are unsupported.
	if err != nil || len(fields) != 5 {
		return nil, errRPKIManifest
	}
	var number *big.Int
	if !rpkiCSRDER(fields[0].FullBytes, &number) || number == nil || number.Sign() < 0 || number.BitLen() > 159 {
		return nil, errRPKIManifest
	}
	thisUpdate, ok := rpkiManifestTime(fields[1])
	if !ok {
		return nil, errRPKIManifest
	}
	nextUpdate, ok := rpkiManifestTime(fields[2])
	if !ok || !nextUpdate.After(thisUpdate) {
		return nil, errRPKIManifest
	}
	var algorithm asn1.ObjectIdentifier
	if !rpkiCSRDER(fields[3].FullBytes, &algorithm) || !algorithm.Equal(asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}) {
		return nil, errRPKIManifest
	}
	entries, err := rpkiResourceSequence(fields[4].FullBytes)
	if err != nil || len(entries) > 10000 {
		return nil, errRPKIManifest
	}
	out := &rpkiManifestContent{Number: number, ThisUpdate: thisUpdate, NextUpdate: nextUpdate, Files: make(map[string][sha256.Size]byte, len(entries))}
	for _, entry := range entries {
		pair, err := rpkiResourceSequence(entry.FullBytes)
		if err != nil || len(pair) != 2 || pair[0].Class != 0 || pair[0].Tag != asn1.TagIA5String || pair[0].IsCompound {
			return nil, errRPKIManifest
		}
		name := string(pair[0].Bytes)
		if !validRPKIManifestFilename(name) {
			return nil, errRPKIManifest
		}
		if _, exists := out.Files[name]; exists {
			return nil, errRPKIManifest
		}
		var hash asn1.BitString
		if !rpkiCSRDER(pair[1].FullBytes, &hash) || hash.BitLength != sha256.Size*8 || len(hash.Bytes) != sha256.Size {
			return nil, errRPKIManifest
		}
		out.Files[name] = [sha256.Size]byte(hash.Bytes)
	}
	return out, nil
}

func rpkiManifestTime(raw asn1.RawValue) (time.Time, bool) {
	if raw.Class != 0 || raw.Tag != asn1.TagGeneralizedTime || raw.IsCompound || len(raw.Bytes) != 15 || raw.Bytes[14] != 'Z' {
		return time.Time{}, false
	}
	for _, c := range raw.Bytes[:14] {
		if c < '0' || c > '9' {
			return time.Time{}, false
		}
	}
	t, err := time.Parse("20060102150405Z", string(raw.Bytes))
	return t, err == nil
}

func validRPKIManifestFilename(name string) bool {
	dot := len(name) - 4
	if dot < 1 || name[dot] != '.' {
		return false
	}
	for _, c := range name[:dot] {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	// IANA RPKI Repository Name Schemes, checked 2026-09-23. Recognizing a
	// filename extension does not implement validation of that object type.
	switch name[dot+1:] {
	case "asa", "ccr", "cdr", "cer", "crl", "gbr", "mft", "roa", "sig", "tak":
		return true
	default:
		return false
	}
}

// checkFiles checks freshness, completeness and hashes. The caller must first
// authenticate the manifest and enforce persistent rollback protection. Extra
// repository files are ignored and must not be used as manifest-listed objects.
func (m *rpkiManifestContent) checkFiles(now time.Time, files map[string][]byte) error {
	if m == nil || now.IsZero() || now.Before(m.ThisUpdate) || !now.Before(m.NextUpdate) {
		return errRPKIManifest
	}
	crls := 0
	for name, digest := range m.Files {
		data, exists := files[name]
		if !exists || sha256.Sum256(data) != digest {
			return errRPKIManifest
		}
		if strings.HasSuffix(name, ".crl") {
			crls++
		}
	}
	if crls != 1 {
		return errRPKIManifest
	}
	return nil
}

// untrustedRPKIManifest has a valid CMS signature and parsed content, but no
// authenticated signer. EE path, revocation, SIA binding and persistent
// rollback checks must succeed before its file set authorizes repository use.
type untrustedRPKIManifest struct {
	Content *rpkiManifestContent
	Signer  *x509.Certificate
}

func decodeRPKIManifest(der []byte) (*untrustedRPKIManifest, error) {
	cms, err := decodeRPKICMSProfile(der, true)
	if err != nil {
		return nil, err
	}
	if err := validateRPKIManifestEEProfile(cms.Signer); err != nil {
		return nil, err
	}
	content, err := parseRPKIManifestContent(cms.Content)
	if err != nil {
		return nil, err
	}
	return &untrustedRPKIManifest{Content: content, Signer: cms.Signer}, nil
}
