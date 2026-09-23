package arin

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"reflect"
	"sort"
	"time"
)

type rpkiCMSSigningIdentity struct {
	Signer        crypto.Signer
	Certificate   *x509.Certificate
	Anchor        *x509.Certificate
	Intermediates []*x509.Certificate
	CRLs          []*x509.RevocationList
}
type rpkiCMSAttribute struct {
	OID    asn1.ObjectIdentifier
	Values []asn1.RawValue `asn1:"set"`
}
type rpkiCMSSignerInfo struct {
	Version    int
	SID        asn1.RawValue
	Digest     pkix.AlgorithmIdentifier
	Attributes []rpkiCMSAttribute `asn1:"tag:0,set"`
	Algorithm  pkix.AlgorithmIdentifier
	Signature  []byte
}
type rpkiCMSEncap struct {
	Type    asn1.ObjectIdentifier
	Content []byte `asn1:"explicit,tag:0"`
}
type rpkiCMSSignedData struct {
	Version      int
	Digests      []pkix.AlgorithmIdentifier `asn1:"set"`
	Encap        rpkiCMSEncap
	Certificates asn1.RawValue
	CRLs         asn1.RawValue
	Signers      []rpkiCMSSignerInfo `asn1:"set"`
}
type rpkiCMSOuter struct {
	Type    asn1.ObjectIdentifier
	Content asn1.RawValue
}

var errRPKICMSSigning = errors.New("invalid RPKI CMS signing identity or request")

// signRPKICMS signs XML with an existing EE identity and caller-supplied time.
// No key is generated, serialized, or persisted. The caller must serialize peer
// exchanges and durably track outgoing time before dispatching the result.
func signRPKICMS(content []byte, identity rpkiCMSSigningIdentity, now, lastSigningTime time.Time) ([]byte, error) {
	if identity.Signer == nil || identity.Certificate == nil || now.IsZero() || len(content) == 0 || len(content) > 4<<20 || len(identity.Intermediates) > 31 || len(identity.CRLs) == 0 || len(identity.CRLs) > 32 {
		return nil, errRPKICMSSigning
	}
	signerValue := reflect.ValueOf(identity.Signer)
	if signerValue.Kind() == reflect.Ptr && signerValue.IsNil() {
		return nil, errRPKICMSSigning
	}
	if len(identity.Certificate.Raw) > 4<<20 {
		return nil, errRPKICMSSigning
	}
	if _, err := parseXML(content); err != nil {
		return nil, errRPKICMSSigning
	}
	cert, err := x509.ParseCertificate(identity.Certificate.Raw)
	if err != nil || cert.IsCA || len(cert.SubjectKeyId) == 0 {
		return nil, errRPKICMSSigning
	}
	key, ok := identity.Signer.Public().(*rsa.PublicKey)
	if !ok || key == nil || key.N == nil || key.N.BitLen() < 2048 || key.N.BitLen() > 8192 {
		return nil, errRPKICMSSigning
	}
	certificateKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || key.E != certificateKey.E || key.N.Cmp(certificateKey.N) != 0 {
		return nil, errRPKICMSSigning
	}
	signingTime := now.UTC().Truncate(time.Second)
	envelope := &untrustedRPKICMS{Content: content, SigningTime: signingTime, Signer: cert, Certificates: []*x509.Certificate{cert}}
	certDER := [][]byte{cert.Raw}
	crlDER := [][]byte{}
	total := len(content) + len(cert.Raw)
	for _, c := range identity.Intermediates {
		if c == nil || len(c.Raw) > (4<<20)-total {
			return nil, errRPKICMSSigning
		}
		parsed, err := x509.ParseCertificate(c.Raw)
		if err != nil || !parsed.IsCA {
			return nil, errRPKICMSSigning
		}
		envelope.Certificates = append(envelope.Certificates, parsed)
		certDER = append(certDER, parsed.Raw)
		total += len(parsed.Raw)
	}
	for _, c := range identity.CRLs {
		if c == nil || len(c.Raw) > (4<<20)-total {
			return nil, errRPKICMSSigning
		}
		parsed, err := x509.ParseRevocationList(c.Raw)
		if err != nil {
			return nil, errRPKICMSSigning
		}
		envelope.CRLs = append(envelope.CRLs, parsed)
		crlDER = append(crlDER, parsed.Raw)
		total += len(parsed.Raw)
	}
	if total > 4<<20 {
		return nil, errRPKICMSSigning
	}
	trust := rpkiCMSTrust{Anchor: identity.Anchor, Now: now, LastSigningTime: lastSigningTime}
	if err := validateRPKICMSTrust(envelope, trust); err != nil {
		return nil, errRPKICMSSigning
	}
	digest := sha256.Sum256(content)
	attrValue := func(v any) (asn1.RawValue, error) {
		der, err := asn1.Marshal(v)
		return asn1.RawValue{FullBytes: der}, err
	}
	contentType, err := attrValue(cmsXMLOID)
	if err != nil {
		return nil, errRPKICMSSigning
	}
	digestValue, err := attrValue(digest[:])
	if err != nil {
		return nil, errRPKICMSSigning
	}
	timeValue, err := attrValue(signingTime)
	if err != nil {
		return nil, errRPKICMSSigning
	}
	attrs := []rpkiCMSAttribute{{cmsContentTypeOID, []asn1.RawValue{contentType}}, {cmsDigestOID, []asn1.RawValue{digestValue}}, {cmsSigningTimeOID, []asn1.RawValue{timeValue}}}
	signed, err := asn1.MarshalWithParams(attrs, "set")
	if err != nil {
		return nil, errRPKICMSSigning
	}
	hash := sha256.Sum256(signed)
	signature, err := identity.Signer.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		return nil, errors.New("RPKI CMS signer failed")
	}
	// Do not let an incorrect or faulty external crypto.Signer produce a request.
	if rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature) != nil {
		return nil, errors.New("RPKI CMS signer returned an invalid signature")
	}
	set := func(tag int, items [][]byte) asn1.RawValue {
		sort.Slice(items, func(i, j int) bool { return bytes.Compare(items[i], items[j]) < 0 })
		return asn1.RawValue{Class: 2, Tag: tag, IsCompound: true, Bytes: bytes.Join(items, nil)}
	}
	sd := rpkiCMSSignedData{Version: 3, Digests: []pkix.AlgorithmIdentifier{{Algorithm: cmsSHA256OID}}, Encap: rpkiCMSEncap{cmsXMLOID, content}, Certificates: set(0, certDER), CRLs: set(1, crlDER), Signers: []rpkiCMSSignerInfo{{Version: 3, SID: asn1.RawValue{Class: 2, Tag: 0, Bytes: cert.SubjectKeyId}, Digest: pkix.AlgorithmIdentifier{Algorithm: cmsSHA256OID}, Attributes: attrs, Algorithm: pkix.AlgorithmIdentifier{Algorithm: cmsRSAOID, Parameters: asn1.NullRawValue}, Signature: signature}}}
	inner, err := asn1.Marshal(sd)
	if err != nil {
		return nil, errRPKICMSSigning
	}
	der, err := asn1.Marshal(rpkiCMSOuter{cmsSignedDataOID, asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: inner}})
	if err != nil || len(der) > 4<<20 {
		return nil, errRPKICMSSigning
	}
	if _, err := verifyRPKICMS(der, trust); err != nil {
		return nil, errRPKICMSSigning
	}
	return der, nil
}
