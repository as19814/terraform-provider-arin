package arin

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"sort"
	"strings"
	"testing"
)

func TestRPKINameProfile(t *testing.T) {
	attr := func(oid asn1.ObjectIdentifier, tag int, value string) []byte {
		return resourceTestDER(t, struct {
			Type  asn1.ObjectIdentifier
			Value asn1.RawValue
		}{oid, asn1.RawValue{Class: 0, Tag: tag, Bytes: []byte(value)}})
	}
	cnOID, snOID := asn1.ObjectIdentifier{2, 5, 4, 3}, asn1.ObjectIdentifier{2, 5, 4, 5}
	cn := attr(cnOID, 19, "resource CA")
	sn := attr(snOID, 19, "123")
	name := func(groups ...[][]byte) []byte {
		var sets []asn1.RawValue
		for _, g := range groups {
			sets = append(sets, asn1.RawValue{Class: 0, Tag: 17, IsCompound: true, Bytes: bytes.Join(g, nil)})
		}
		return resourceTestDER(t, sets)
	}
	combined := [][]byte{cn, sn}
	sort.Slice(combined, func(i, j int) bool { return bytes.Compare(combined[i], combined[j]) < 0 })
	for _, der := range [][]byte{name([][]byte{cn}), name([][]byte{cn}, [][]byte{sn}), name(combined)} {
		if !validRPKIName(der) {
			t.Fatal("valid resource name rejected")
		}
	}
	for label, der := range map[string][]byte{
		"empty": name(), "serial_only": name([][]byte{sn}), "duplicate_cn": name([][]byte{cn}, [][]byte{cn}),
		"utf8_cn":      name([][]byte{attr(cnOID, 12, "resource CA")}),
		"extra_org":    name([][]byte{cn}, [][]byte{attr(asn1.ObjectIdentifier{2, 5, 4, 10}, 19, "organization")}),
		"empty_cn":     name([][]byte{attr(cnOID, 19, "")}),
		"long_cn":      name([][]byte{attr(cnOID, 19, strings.Repeat("a", 65))}),
		"nonprintable": name([][]byte{attr(cnOID, 19, "name*value")}),
		"unsorted_set": name([][]byte{combined[1], combined[0]}),
	} {
		t.Run(label, func(t *testing.T) {
			if validRPKIName(der) {
				t.Fatal("invalid name accepted")
			}
		})
	}
}

func TestRPKITimeProfile(t *testing.T) {
	for _, tc := range []struct {
		tag   int
		text  string
		valid bool
	}{
		{23, "500101000000Z", true}, {23, "491231235959Z", true}, {23, "260923120000Z", true},
		{24, "20500101000000Z", true}, {24, "99991231235959Z", true},
		{24, "20491231235959Z", false}, {23, "2609231200Z", false}, {23, "260923120000+0000", false},
		{24, "20500101000000.0Z", false}, {23, "260230120000Z", false}, {23, "260923126000Z", false}, {23, "+60923120000Z", false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			if validRPKITime(asn1.RawValue{Class: 0, Tag: tc.tag, Bytes: []byte(tc.text)}) != tc.valid {
				t.Fatal("wrong time encoding result")
			}
		})
	}
}

func TestRPKIPathRejectsOutOfProfileName(t *testing.T) {
	f := resourceCertificateFixture(t)
	leaf := *f.certs[0]
	leaf.RawSubject = nil
	leaf.Subject = pkix.Name{CommonName: "leaf", Organization: []string{"unexpected organization"}}
	leaf.ExtraExtensions = leaf.Extensions
	der, err := x509.CreateCertificate(rand.Reader, &leaf, f.certs[1], &f.keys[0].PublicKey, f.keys[1])
	if err != nil {
		t.Fatal(err)
	}
	bad, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	if err := bad.CheckSignatureFrom(f.certs[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyRPKICertificatePath([]*x509.Certificate{bad, f.certs[1], f.certs[2]}, f.certs[2], f.crls, cmsTrustNow()); err == nil {
		t.Fatal("signed out-of-profile name accepted")
	}
}
