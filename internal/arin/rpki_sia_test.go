package arin

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"
)

func rpkiSIATestExtension(t *testing.T) pkix.Extension {
	t.Helper()
	value, err := asn1.Marshal([]rpkiAccessDescription{
		{Method: oidRPKIRepository, Location: asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte("rsync://repo.example/module/child/")}},
		{Method: oidRPKIManifest, Location: asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte("rsync://repo.example/module/child/manifest.mft")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return pkix.Extension{Id: oidRPKISIA, Value: value}
}

func TestRPKICASIA(t *testing.T) {
	good := rpkiSIATestExtension(t)
	got, err := rpkiCASIA([]pkix.Extension{good})
	if err != nil || !bytes.Equal(got, good.Value) {
		t.Fatal("valid SIA rejected")
	}
	got[0] ^= 1
	if bytes.Equal(got, good.Value) {
		t.Fatal("SIA aliases input")
	}
	var descriptions []rpkiAccessDescription
	if _, err := asn1.Unmarshal(good.Value, &descriptions); err != nil {
		t.Fatal(err)
	}
	encode := func(d []rpkiAccessDescription) pkix.Extension {
		raw, err := asn1.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		return pkix.Extension{Id: oidRPKISIA, Value: raw}
	}
	noRepo := encode(descriptions[1:])
	noManifest := encode(descriptions[:1])
	badType := append([]rpkiAccessDescription(nil), descriptions...)
	badType[0].Location = asn1.RawValue{Class: 2, Tag: 2, Bytes: []byte("repo.example")}
	badDirectory := append([]rpkiAccessDescription(nil), descriptions...)
	badDirectory[0].Location = asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte("rsync://repo.example/module/child")}
	badManifest := append([]rpkiAccessDescription(nil), descriptions...)
	badManifest[1].Location = asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte("rsync://repo.example/module/child/")}
	httpOnly := append([]rpkiAccessDescription(nil), descriptions...)
	httpOnly[0].Location = asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte("https://repo.example/module/child/")}
	for name, exts := range map[string][]pkix.Extension{
		"missing": nil, "duplicate": {good, good}, "critical": {{Id: oidRPKISIA, Value: good.Value, Critical: true}},
		"trailing_der":  {{Id: oidRPKISIA, Value: append(bytes.Clone(good.Value), 0)}},
		"empty":         {{Id: oidRPKISIA, Value: []byte{0x30, 0}}},
		"no_repository": {noRepo}, "no_manifest": {noManifest}, "not_uri": {encode(badType)}, "not_directory": {encode(badDirectory)}, "directory_manifest": {encode(badManifest)}, "no_rsync_repository": {encode(httpOnly)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := rpkiCASIA(exts); err == nil {
				t.Fatal("invalid SIA accepted")
			}
		})
	}
}

func TestUpDownIssueRequiresSIA(t *testing.T) {
	identity, _ := cmsSigningFixture(t)
	for _, extensions := range [][]pkix.Extension{nil, {rpkiSIATestExtension(t)}} {
		csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{ExtraExtensions: extensions}, identity.Signer)
		if err != nil {
			t.Fatal(err)
		}
		_, err = buildUpDownIssue("child", "parent", rpkiIssueRequest{Class: "class", CSRDER: csr})
		if (len(extensions) == 0) != (err != nil) {
			t.Fatal("incorrect SIA preflight")
		}
	}
}

func TestUpDownIssuePreservesSIA(t *testing.T) {
	for _, mode := range []string{"missing", "changed", "critical"} {
		t.Run(mode, func(t *testing.T) {
			input, reply := upDownIssueFixture(t, mode)
			query, err := buildUpDownIssue("child", "parent", input)
			if err != nil {
				t.Fatal(err)
			}
			got, rejected, err := validateUpDownIssue(query, []byte(reply), "child", "parent", cmsTrustNow())
			if err == nil || got != nil || rejected != nil {
				t.Fatal("issued SIA mismatch accepted")
			}
		})
	}
}
