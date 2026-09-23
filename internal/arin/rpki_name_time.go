package arin

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func validRPKIName(der []byte) bool {
	rdns, err := rpkiResourceSequence(der)
	if err != nil || len(rdns) == 0 || len(rdns) > 2 {
		return false
	}
	seen := make(map[string]bool)
	for _, rdn := range rdns {
		if rdn.Class != 0 || rdn.Tag != 17 || !rdn.IsCompound || len(rdn.Bytes) == 0 {
			return false
		}
		var previous []byte
		remaining := rdn.Bytes
		for len(remaining) > 0 {
			var raw asn1.RawValue
			remaining, err = asn1.Unmarshal(remaining, &raw)
			if err != nil {
				return false
			}
			if previous != nil && bytes.Compare(previous, raw.FullBytes) > 0 {
				return false
			}
			previous = raw.FullBytes
			var attr struct {
				Type  asn1.ObjectIdentifier
				Value asn1.RawValue
			}
			if !rpkiCSRDER(raw.FullBytes, &attr) || attr.Value.Class != 0 || attr.Value.Tag != asn1.TagPrintableString || attr.Value.IsCompound {
				return false
			}
			oid := attr.Type.String()
			if (oid != "2.5.4.3" && oid != "2.5.4.5") || seen[oid] {
				return false
			}
			seen[oid] = true
			var value string
			rest, err := asn1.Unmarshal(attr.Value.FullBytes, &value)
			if err != nil || len(rest) != 0 || len(value) == 0 || len(value) > 64 {
				return false
			}
			for _, r := range value {
				if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune(" '()+,-./:=?", r)) {
					return false
				}
			}
		}
	}
	return seen["2.5.4.3"]
}

// RPKI uses the RFC 5280 UTC, whole-second form, with GeneralizedTime for
// dates from 2050 onward. Offset, fractional and minute-only forms fail.
func validRPKITime(raw asn1.RawValue) bool {
	if raw.Class != 0 || raw.IsCompound {
		return false
	}
	s := string(raw.Bytes)
	if len(s) == 0 {
		return false
	}
	for _, r := range s[:len(s)-1] {
		if r < '0' || r > '9' {
			return false
		}
	}
	switch raw.Tag {
	case asn1.TagUTCTime:
		if len(s) != 13 || s[12] != 'Z' {
			return false
		}
		y, err := strconv.Atoi(s[:2])
		if err != nil {
			return false
		}
		if y >= 50 {
			y += 1900
		} else {
			y += 2000
		}
		s = fmt.Sprintf("%04d%s", y, s[2:])
	case asn1.TagGeneralizedTime:
		if len(s) != 15 || s[14] != 'Z' {
			return false
		}
		y, err := strconv.Atoi(s[:4])
		if err != nil || y < 2050 {
			return false
		}
	default:
		return false
	}
	for _, r := range s[:len(s)-1] {
		if r < '0' || r > '9' {
			return false
		}
	}
	parsed, err := time.Parse("20060102150405Z", s)
	return err == nil && parsed.Format("20060102150405Z") == s
}

func validateRPKICertificateNamesAndTimes(cert *x509.Certificate) error {
	if cert.Version != 3 || !validRPKIName(cert.RawIssuer) || !validRPKIName(cert.RawSubject) {
		return errRPKIUpDown
	}
	tbs, err := rpkiResourceSequence(cert.RawTBSCertificate)
	// Version, serial, signature, issuer, validity, subject, SPKI, extensions.
	// Unique identifiers and extra fields are not part of this profile.
	if err != nil || len(tbs) != 8 || tbs[7].Class != 2 || tbs[7].Tag != 3 || !tbs[7].IsCompound {
		return errRPKIUpDown
	}
	validity, err := rpkiResourceSequence(tbs[4].FullBytes)
	if err != nil || len(validity) != 2 || !validRPKITime(validity[0]) || !validRPKITime(validity[1]) {
		return errRPKIUpDown
	}
	return nil
}

func validateRPKICRLNamesAndTimes(crl *x509.RevocationList, tbs []asn1.RawValue) error {
	if len(tbs) < 6 || len(tbs) > 7 || !validRPKIName(crl.RawIssuer) || !validRPKITime(tbs[3]) || !validRPKITime(tbs[4]) {
		return errRPKIUpDown
	}
	last := tbs[len(tbs)-1]
	if last.Class != 2 || last.Tag != 0 || !last.IsCompound {
		return errRPKIUpDown
	}
	if len(tbs) == 7 {
		entries, err := rpkiResourceSequence(tbs[5].FullBytes)
		if err != nil || len(entries) == 0 {
			return errRPKIUpDown
		}
		for _, entry := range entries {
			fields, err := rpkiResourceSequence(entry.FullBytes)
			if err != nil || len(fields) != 2 || !validRPKITime(fields[1]) {
				return errRPKIUpDown
			}
		}
	}
	return nil
}
