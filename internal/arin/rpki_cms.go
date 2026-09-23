package arin

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"time"
)

var (
	cmsSignedDataOID  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	cmsXMLOID         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 28}
	cmsSHA256OID      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	cmsRSAOID         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	cmsSHA256RSAOID   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	cmsContentTypeOID = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	cmsDigestOID      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	cmsSigningTimeOID = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
	cmsBinaryTimeOID  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 46}
)

// untrustedRPKICMS has a checked CMS signature but no authenticated identity.
// It must never reach a protocol consumer before path, CRL, replay and message
// identity validation. No public API exposes these intermediate contents.
type untrustedRPKICMS struct {
	Content      []byte
	SigningTime  time.Time
	Signer       *x509.Certificate
	Certificates []*x509.Certificate
	CRLs         []*x509.RevocationList
}

var errRPKICMS = errors.New("invalid RPKI CMS profile or signature")

func cmsRaw(der []byte) (asn1.RawValue, error) {
	var r asn1.RawValue
	rest, err := asn1.Unmarshal(der, &r)
	if err != nil || len(rest) != 0 {
		return r, errRPKICMS
	}
	return r, nil
}
func cmsChildren(r asn1.RawValue, class, tag, max int, ordered bool) ([]asn1.RawValue, error) {
	if r.Class != class || r.Tag != tag || !r.IsCompound {
		return nil, errRPKICMS
	}
	var out []asn1.RawValue
	data := r.Bytes
	for len(data) > 0 {
		var child asn1.RawValue
		rest, err := asn1.Unmarshal(data, &child)
		if err != nil || len(rest) >= len(data) || len(out) >= max {
			return nil, errRPKICMS
		}
		if ordered && len(out) > 0 && bytes.Compare(out[len(out)-1].FullBytes, child.FullBytes) > 0 {
			return nil, errRPKICMS
		}
		out = append(out, child)
		data = rest
	}
	return out, nil
}
func cmsValue(r asn1.RawValue, out any) error {
	rest, err := asn1.Unmarshal(r.FullBytes, out)
	if err != nil || len(rest) != 0 {
		return errRPKICMS
	}
	return nil
}
func cmsOID(r asn1.RawValue, want asn1.ObjectIdentifier) bool {
	var oid asn1.ObjectIdentifier
	return cmsValue(r, &oid) == nil && oid.Equal(want)
}
func cmsVersion3(r asn1.RawValue) bool { var v int; return cmsValue(r, &v) == nil && v == 3 }
func cmsAlgorithm(r asn1.RawValue, oids ...asn1.ObjectIdentifier) bool {
	parts, err := cmsChildren(r, 0, 16, 2, false)
	if err != nil || len(parts) < 1 {
		return false
	}
	if len(parts) == 2 && !bytes.Equal(parts[1].FullBytes, []byte{5, 0}) {
		return false
	}
	for _, oid := range oids {
		if cmsOID(parts[0], oid) {
			return true
		}
	}
	return false
}

// decodeRPKICMS checks the RFC 6492 CMS envelope and SHA-256/RSA signature.
// Trust and revocation are deliberately separate from this private decoder.
func decodeRPKICMS(der []byte) (*untrustedRPKICMS, error) {
	if len(der) == 0 || len(der) > 4<<20 {
		return nil, errRPKICMS
	}
	root, err := cmsRaw(der)
	if err != nil {
		return nil, err
	}
	outer, err := cmsChildren(root, 0, 16, 2, false)
	if err != nil || len(outer) != 2 || !cmsOID(outer[0], cmsSignedDataOID) {
		return nil, errRPKICMS
	}
	explicit, err := cmsChildren(outer[1], 2, 0, 1, false)
	if err != nil || len(explicit) != 1 {
		return nil, errRPKICMS
	}
	sd, err := cmsChildren(explicit[0], 0, 16, 6, false)
	if err != nil || len(sd) != 6 || !cmsVersion3(sd[0]) {
		return nil, errRPKICMS
	}
	algorithms, err := cmsChildren(sd[1], 0, 17, 1, true)
	if err != nil || len(algorithms) != 1 || !cmsAlgorithm(algorithms[0], cmsSHA256OID) {
		return nil, errRPKICMS
	}
	eci, err := cmsChildren(sd[2], 0, 16, 2, false)
	if err != nil || len(eci) != 2 || !cmsOID(eci[0], cmsXMLOID) {
		return nil, errRPKICMS
	}
	wrapped, err := cmsChildren(eci[1], 2, 0, 1, false)
	if err != nil || len(wrapped) != 1 {
		return nil, errRPKICMS
	}
	out := &untrustedRPKICMS{}
	if cmsValue(wrapped[0], &out.Content) != nil || len(out.Content) == 0 {
		return nil, errRPKICMS
	}
	certs, err := cmsChildren(sd[3], 2, 0, 32, true)
	if err != nil || len(certs) == 0 {
		return nil, errRPKICMS
	}
	eeCount := 0
	for _, r := range certs {
		if r.Class != 0 || r.Tag != 16 {
			return nil, errRPKICMS
		}
		cert, err := x509.ParseCertificate(r.FullBytes)
		if err != nil {
			return nil, errRPKICMS
		}
		if !cert.IsCA {
			eeCount++
		}
		out.Certificates = append(out.Certificates, cert)
	}
	if eeCount != 1 {
		return nil, errRPKICMS
	}
	crls, err := cmsChildren(sd[4], 2, 1, 32, true)
	if err != nil || len(crls) == 0 {
		return nil, errRPKICMS
	}
	for _, r := range crls {
		if r.Class != 0 || r.Tag != 16 {
			return nil, errRPKICMS
		}
		crl, err := x509.ParseRevocationList(r.FullBytes)
		if err != nil {
			return nil, errRPKICMS
		}
		out.CRLs = append(out.CRLs, crl)
	}
	signers, err := cmsChildren(sd[5], 0, 17, 1, true)
	if err != nil || len(signers) != 1 {
		return nil, errRPKICMS
	}
	si, err := cmsChildren(signers[0], 0, 16, 6, false)
	if err != nil || len(si) != 6 || !cmsVersion3(si[0]) || !cmsAlgorithm(si[2], cmsSHA256OID) || !cmsAlgorithm(si[4], cmsRSAOID, cmsSHA256RSAOID) {
		return nil, errRPKICMS
	}
	sid := si[1]
	if sid.Class != 2 || sid.Tag != 0 || sid.IsCompound || len(sid.Bytes) == 0 {
		return nil, errRPKICMS
	}
	for _, cert := range out.Certificates {
		if bytes.Equal(cert.SubjectKeyId, sid.Bytes) {
			if out.Signer != nil || cert.IsCA {
				return nil, errRPKICMS
			}
			out.Signer = cert
		}
	}
	if out.Signer == nil {
		return nil, errRPKICMS
	}
	attrs, err := cmsChildren(si[3], 2, 0, 4, true)
	if err != nil || len(attrs) < 3 {
		return nil, errRPKICMS
	}
	seen := map[string]bool{}
	var signing, binary *time.Time
	digest := sha256.Sum256(out.Content)
	for _, attr := range attrs {
		parts, err := cmsChildren(attr, 0, 16, 2, false)
		if err != nil || len(parts) != 2 {
			return nil, errRPKICMS
		}
		var oid asn1.ObjectIdentifier
		if cmsValue(parts[0], &oid) != nil || seen[oid.String()] {
			return nil, errRPKICMS
		}
		seen[oid.String()] = true
		vals, err := cmsChildren(parts[1], 0, 17, 1, true)
		if err != nil || len(vals) != 1 {
			return nil, errRPKICMS
		}
		switch {
		case oid.Equal(cmsContentTypeOID):
			if !cmsOID(vals[0], cmsXMLOID) {
				return nil, errRPKICMS
			}
		case oid.Equal(cmsDigestOID):
			var got []byte
			if cmsValue(vals[0], &got) != nil || !bytes.Equal(got, digest[:]) {
				return nil, errRPKICMS
			}
		case oid.Equal(cmsSigningTimeOID):
			var t time.Time
			if cmsValue(vals[0], &t) != nil {
				return nil, errRPKICMS
			}
			params := ""
			if vals[0].Tag == asn1.TagGeneralizedTime {
				params = "generalized"
			}
			canonical, e := asn1.MarshalWithParams(t.UTC(), params)
			if e != nil || !bytes.Equal(canonical, vals[0].FullBytes) {
				return nil, errRPKICMS
			}
			signing = &t
		case oid.Equal(cmsBinaryTimeOID):
			var seconds int64
			if cmsValue(vals[0], &seconds) != nil || seconds < 0 {
				return nil, errRPKICMS
			}
			t := time.Unix(seconds, 0).UTC()
			binary = &t
		default:
			return nil, errRPKICMS
		}
	}
	if !seen[cmsContentTypeOID.String()] || !seen[cmsDigestOID.String()] || (signing == nil && binary == nil) {
		return nil, errRPKICMS
	}
	if signing != nil && binary != nil && !signing.Equal(*binary) {
		return nil, errRPKICMS
	}
	if signing != nil {
		out.SigningTime = *signing
	} else {
		out.SigningTime = *binary
	}
	var signature []byte
	if cmsValue(si[5], &signature) != nil {
		return nil, errRPKICMS
	}
	key, ok := out.Signer.PublicKey.(*rsa.PublicKey)
	if !ok || key.N.BitLen() < 2048 || key.N.BitLen() > 8192 {
		return nil, errRPKICMS
	}
	// CMS signs a DER SET OF, not the context-specific signedAttrs tag.
	signedDER := bytes.Clone(si[3].FullBytes)
	signedDER[0] = 0x31
	signedDigest := sha256.Sum256(signedDER)
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, signedDigest[:], signature) != nil {
		return nil, errRPKICMS
	}
	return out, nil
}
