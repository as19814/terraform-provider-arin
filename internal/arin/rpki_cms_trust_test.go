package arin

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"sort"
	"testing"
	"time"
)

func cmsTrustNow() time.Time { return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) }
func cmsTestCRL(t *testing.T, ca *x509.Certificate, key *rsa.PrivateKey, modify func(*x509.RevocationList)) []byte {
	t.Helper()
	now := cmsTrustNow()
	template := &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: now.Add(-time.Minute), NextUpdate: now.Add(time.Hour)}
	if modify != nil {
		modify(template)
	}
	der, err := x509.CreateRevocationList(rand.Reader, template, ca, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}
func cmsTestSet(tag int, items ...[]byte) asn1.RawValue {
	sort.Slice(items, func(i, j int) bool { return bytes.Compare(items[i], items[j]) < 0 })
	return asn1.RawValue{Class: 2, Tag: tag, IsCompound: true, Bytes: bytes.Join(items, nil)}
}
func TestRPKICMSTrust(t *testing.T) {
	sd, key, ca := cmsTrustFixture(t)
	trust := rpkiCMSTrust{Anchor: ca, Now: cmsTrustNow(), LastSigningTime: cmsTrustNow()}
	got, err := verifyRPKICMS(cmsTestEncode(t, sd, key), trust)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Content, sd.Encap.Content) || !got.SigningTime.Equal(trust.Now) {
		t.Fatal("authenticated content changed")
	}
	// A caller-owned timestamp is not mutated by verification.
	if !trust.LastSigningTime.Equal(cmsTrustNow()) {
		t.Fatal("watermark changed before protocol validation")
	}
}
func TestRPKICMSTrustRejectsInvalid(t *testing.T) {
	for _, mode := range []string{"missing_anchor", "missing_clock", "wrong_anchor", "expired_certificate", "older_timestamp", "future_timestamp", "revoked", "expired_crl", "future_crl", "bad_crl_signature", "critical_crl", "delta_crl", "scoped_crl", "indirect_entry", "critical_entry", "remove_from_crl", "newer_expired_crl", "conflicting_crls", "number_time_conflict", "wrong_key_usage", "expired_ee", "issuer_without_crl_usage", "resource_anchor"} {
		t.Run(mode, func(t *testing.T) {
			sd, key, ca := cmsTrustFixture(t)
			trust := rpkiCMSTrust{Anchor: ca, Now: cmsTrustNow()}
			modify := func(*x509.RevocationList) {}
			switch mode {
			case "missing_anchor":
				trust.Anchor = nil
			case "missing_clock":
				trust.Now = time.Time{}
			case "wrong_anchor":
				_, _, other := cmsTrustFixture(t)
				trust.Anchor = other
			case "expired_certificate":
				trust.Now = trust.Now.Add(2 * time.Hour)
			case "older_timestamp":
				trust.LastSigningTime = trust.Now.Add(time.Second)
			case "future_timestamp":
				trust.Now = trust.Now.Add(-6 * time.Minute)
				modify = func(c *x509.RevocationList) { c.ThisUpdate = cmsTrustNow().Add(-time.Hour) }
			case "revoked":
				modify = func(c *x509.RevocationList) {
					c.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: big.NewInt(2), RevocationTime: cmsTrustNow().Add(-time.Minute)}}
				}
			case "expired_crl":
				modify = func(c *x509.RevocationList) {
					c.ThisUpdate = cmsTrustNow().Add(-time.Hour)
					c.NextUpdate = cmsTrustNow().Add(-time.Second)
				}
			case "future_crl":
				modify = func(c *x509.RevocationList) { c.ThisUpdate = cmsTrustNow().Add(time.Second) }
			case "critical_crl":
				modify = func(c *x509.RevocationList) {
					c.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 2, 3, 4}, Critical: true, Value: []byte{5, 0}}}
				}
			case "delta_crl":
				modify = func(c *x509.RevocationList) {
					c.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 27}, Value: []byte{2, 1, 1}}}
				}
			case "scoped_crl":
				modify = func(c *x509.RevocationList) {
					c.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 28}, Value: []byte{0x30, 0}}}
				}
			case "indirect_entry", "critical_entry", "remove_from_crl":
				modify = func(c *x509.RevocationList) {
					e := x509.RevocationListEntry{SerialNumber: big.NewInt(99), RevocationTime: cmsTrustNow().Add(-time.Minute)}
					if mode == "remove_from_crl" {
						e.ReasonCode = 8
					} else {
						oid := asn1.ObjectIdentifier{2, 5, 29, 29}
						critical := false
						if mode == "critical_entry" {
							oid = asn1.ObjectIdentifier{1, 2, 3, 4}
							critical = true
						}
						e.ExtraExtensions = []pkix.Extension{{Id: oid, Critical: critical, Value: []byte{0x30, 0}}}
					}
					c.RevokedCertificateEntries = []x509.RevocationListEntry{e}
				}
			case "wrong_key_usage", "expired_ee":
				ee, err := x509.ParseCertificate(sd.Certs.Bytes)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "wrong_key_usage" {
					ee.KeyUsage = x509.KeyUsageKeyEncipherment
				} else {
					ee.NotAfter = cmsTrustNow().Add(-time.Second)
				}
				raw, err := x509.CreateCertificate(rand.Reader, ee, ca, &key.PublicKey, key)
				if err != nil {
					t.Fatal(err)
				}
				sd.Certs = cmsTestSet(0, raw)
			case "issuer_without_crl_usage", "resource_anchor":
				root := *ca
				if mode == "issuer_without_crl_usage" {
					root.KeyUsage = x509.KeyUsageCertSign
				} else {
					root.ExtraExtensions = []pkix.Extension{{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 7}, Value: []byte{0x30, 0}}}
				}
				raw, err := x509.CreateCertificate(rand.Reader, &root, &root, &key.PublicKey, key)
				if err != nil {
					t.Fatal(err)
				}
				trust.Anchor, err = x509.ParseCertificate(raw)
				if err != nil {
					t.Fatal(err)
				}
			}
			crl := cmsTestCRL(t, ca, key, modify)
			if mode == "bad_crl_signature" {
				crl[len(crl)-1] ^= 1
			}
			sd.CRLs = cmsTestSet(1, crl)
			switch mode {
			case "newer_expired_crl", "conflicting_crls", "number_time_conflict":
				extra := cmsTestCRL(t, ca, key, func(c *x509.RevocationList) {
					c.Number = big.NewInt(2)
					if mode == "newer_expired_crl" {
						c.ThisUpdate = cmsTrustNow().Add(-30 * time.Second)
						c.NextUpdate = cmsTrustNow().Add(-time.Second)
					}
					if mode == "conflicting_crls" {
						c.Number = big.NewInt(1)
						c.NextUpdate = cmsTrustNow().Add(2 * time.Hour)
					}
					if mode == "number_time_conflict" {
						c.ThisUpdate = cmsTrustNow().Add(-2 * time.Minute)
					}
				})
				sd.CRLs = cmsTestSet(1, crl, extra)
			}
			if _, err := verifyRPKICMS(cmsTestEncode(t, sd, key), trust); err == nil {
				t.Fatal("invalid trust accepted")
			}
		})
	}
}
func TestRPKICMSIntermediateTrust(t *testing.T) {
	sd, key, ca := cmsTrustFixture(t)
	intKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "Intermediate"}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, SubjectKeyId: []byte{7, 8, 9}}
	raw, err := x509.CreateCertificate(rand.Reader, template, ca, &intKey.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	intermediate, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	ee, err := x509.ParseCertificate(sd.Certs.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	ee.AuthorityKeyId = nil
	eeDER, err := x509.CreateCertificate(rand.Reader, ee, intermediate, &key.PublicKey, intKey)
	if err != nil {
		t.Fatal(err)
	}
	rootCRL := cmsTestCRL(t, ca, key, nil)
	eeCRL := cmsTestCRL(t, intermediate, intKey, nil)
	sd.Certs = cmsTestSet(0, eeDER, intermediate.Raw)
	sd.CRLs = cmsTestSet(1, rootCRL, eeCRL)
	trust := rpkiCMSTrust{Anchor: ca, Now: cmsTrustNow()}
	if _, err := verifyRPKICMS(cmsTestEncode(t, sd, key), trust); err != nil {
		t.Fatal("embedded intermediate failed", err)
	}
	sd.Certs = cmsTestSet(0, eeDER)
	if _, err := verifyRPKICMS(cmsTestEncode(t, sd, key), trust); err == nil {
		t.Fatal("missing intermediate accepted")
	}
	trust.Intermediates = []*x509.Certificate{intermediate}
	if _, err := verifyRPKICMS(cmsTestEncode(t, sd, key), trust); err != nil {
		t.Fatal("configured intermediate failed", err)
	}
	sd.CRLs = cmsTestSet(1, eeCRL)
	if _, err := verifyRPKICMS(cmsTestEncode(t, sd, key), trust); err == nil {
		t.Fatal("missing intermediate CRL accepted")
	}
	revoked := cmsTestCRL(t, ca, key, func(c *x509.RevocationList) {
		c.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: intermediate.SerialNumber, RevocationTime: cmsTrustNow().Add(-time.Minute)}}
	})
	sd.CRLs = cmsTestSet(1, revoked, eeCRL)
	if _, err := verifyRPKICMS(cmsTestEncode(t, sd, key), trust); err == nil {
		t.Fatal("revoked intermediate accepted")
	}
}

func TestRPKICMSTrustCurrentValidityAndNewestCRL(t *testing.T) {
	sd, key, ca := cmsTrustFixture(t)
	now := cmsTrustNow()
	// RFC 6492 validates the EE at the current time, not at signing time.
	for i := range sd.Signers[0].Attrs {
		if sd.Signers[0].Attrs[i].OID.Equal(cmsSigningTimeOID) {
			sd.Signers[0].Attrs[i].Values = []asn1.RawValue{cmsTestValue(t, now.Add(-2*time.Hour))}
		}
	}
	old := cmsTestCRL(t, ca, key, func(c *x509.RevocationList) {
		c.ThisUpdate = now.Add(-2 * time.Minute)
		c.NextUpdate = now.Add(-time.Minute)
	})
	current := cmsTestCRL(t, ca, key, func(c *x509.RevocationList) { c.Number = big.NewInt(2) })
	sd.CRLs = cmsTestSet(1, old, current)
	trust := rpkiCMSTrust{Anchor: ca, Now: now}
	if _, err := verifyRPKICMS(cmsTestEncode(t, sd, key), trust); err != nil {
		t.Fatal("current chain/CRL rejected historical signing time", err)
	}
	root := *ca
	root.NotAfter = now.Add(-time.Second)
	raw, err := x509.CreateCertificate(rand.Reader, &root, &root, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	trust.Anchor, err = x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyRPKICMS(cmsTestEncode(t, sd, key), trust); err == nil {
		t.Fatal("expired anchor accepted while EE remains valid")
	}
}
