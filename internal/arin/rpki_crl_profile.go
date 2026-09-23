package arin

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
)

// Profile checks supplement, rather than replace, signature, currentness and
// revocation checks. Call only with a CRL freshly parsed from its DER.
func validateRPKICRLProfile(crl *x509.RevocationList) error {
	if crl == nil || crl.SignatureAlgorithm != x509.SHA256WithRSA || crl.Number == nil || crl.Number.Sign() < 0 || crl.Number.BitLen() > 159 || len(crl.AuthorityKeyId) != 20 {
		return errRPKIUpDown
	}
	outer, err := rpkiResourceSequence(crl.Raw)
	if err != nil || len(outer) != 3 {
		return errRPKIUpDown
	}
	tbs, err := rpkiResourceSequence(crl.RawTBSRevocationList)
	if err != nil || len(tbs) < 5 {
		return errRPKIUpDown
	}
	var version int
	if !rpkiCSRDER(tbs[0].FullBytes, &version) || version != 1 {
		return errRPKIUpDown
	}
	var algorithm pkix.AlgorithmIdentifier
	if !rpkiCSRDER(outer[1].FullBytes, &algorithm) || !bytes.Equal(algorithm.Parameters.FullBytes, []byte{5, 0}) || !bytes.Equal(outer[1].FullBytes, tbs[1].FullBytes) {
		return errRPKIUpDown
	}
	seen := make(map[string]bool)
	for _, e := range crl.Extensions {
		oid := e.Id.String()
		if seen[oid] || e.Critical {
			return errRPKIUpDown
		}
		seen[oid] = true
		switch oid {
		case "2.5.29.35":
			var aki struct {
				KeyID []byte `asn1:"tag:0"`
			}
			if !rpkiCSRDER(e.Value, &aki) || !bytes.Equal(aki.KeyID, crl.AuthorityKeyId) {
				return errRPKIUpDown
			}
		case "2.5.29.20":
			var number asn1.RawValue
			if !rpkiCSRDER(e.Value, &number) || number.Class != 0 || number.Tag != 2 || number.IsCompound || len(number.Bytes) > 20 {
				return errRPKIUpDown
			}
		default:
			return errRPKIUpDown
		}
	}
	if len(seen) != 2 || !seen["2.5.29.35"] || !seen["2.5.29.20"] {
		return errRPKIUpDown
	}
	serials := make(map[string]bool)
	for _, entry := range crl.RevokedCertificateEntries {
		if entry.SerialNumber == nil || entry.SerialNumber.Sign() <= 0 || entry.SerialNumber.BitLen() > 159 || len(entry.Extensions) != 0 || entry.RevocationTime.IsZero() || entry.RevocationTime.After(crl.ThisUpdate) {
			return errRPKIUpDown
		}
		serial := entry.SerialNumber.String()
		if serials[serial] {
			return errRPKIUpDown
		}
		serials[serial] = true
	}
	return nil
}
