package arin

import (
	"crypto/sha256"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

type manifestTestFile struct {
	Name string `asn1:"ia5"`
	Hash asn1.BitString
}
type manifestTestContent struct {
	Number     *big.Int
	ThisUpdate time.Time `asn1:"generalized"`
	NextUpdate time.Time `asn1:"generalized"`
	Algorithm  asn1.ObjectIdentifier
	Files      []manifestTestFile
}

func manifestContentFixture() manifestTestContent {
	hash := sha256.Sum256([]byte("fixture CRL"))
	return manifestTestContent{big.NewInt(0), cmsTrustNow(), cmsTrustNow().Add(time.Hour), asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}, []manifestTestFile{{"Issuer_1.crl", asn1.BitString{Bytes: hash[:], BitLength: 256}}}}
}

func TestRPKIManifestContent(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*manifestTestContent)
		valid  bool
	}{
		{"valid", func(*manifestTestContent) {}, true},
		{"largest_number", func(m *manifestTestContent) {
			m.Number = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 159), big.NewInt(1))
		}, true},
		{"negative_number", func(m *manifestTestContent) { m.Number = big.NewInt(-1) }, false},
		{"oversized_number", func(m *manifestTestContent) { m.Number = new(big.Int).Lsh(big.NewInt(1), 159) }, false},
		{"equal_times", func(m *manifestTestContent) { m.NextUpdate = m.ThisUpdate }, false},
		{"reversed_times", func(m *manifestTestContent) { m.NextUpdate = m.ThisUpdate.Add(-time.Second) }, false},
		{"wrong_algorithm", func(m *manifestTestContent) { m.Algorithm = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26} }, false},
		{"short_hash", func(m *manifestTestContent) { m.Files[0].Hash = asn1.BitString{Bytes: []byte{0}, BitLength: 8} }, false},
		{"unaligned_hash", func(m *manifestTestContent) {
			m.Files[0].Hash = asn1.BitString{Bytes: make([]byte, 32), BitLength: 255}
		}, false},
		{"duplicate", func(m *manifestTestContent) { m.Files = append(m.Files, m.Files[0]) }, false},
		{"case_preserved", func(m *manifestTestContent) { f := m.Files[0]; f.Name = "issuer_1.crl"; m.Files = append(m.Files, f) }, true},
		{"empty_list_syntax", func(m *manifestTestContent) { m.Files = nil }, true},
		{"path", func(m *manifestTestContent) { m.Files[0].Name = "../Issuer_1.crl" }, false},
		{"too_many_files", func(m *manifestTestContent) { m.Files = make([]manifestTestFile, 10001) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := manifestContentFixture()
			tt.mutate(&input)
			der, err := asn1.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := parseRPKIManifestContent(der)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v, error=%v", tt.valid, err)
			}
			if err == nil && (got.Number.Cmp(input.Number) != 0 || !got.ThisUpdate.Equal(input.ThisUpdate) || len(got.Files) != len(input.Files)) {
				t.Fatal("content changed")
			}
		})
	}
}

func TestRPKIManifestEncoding(t *testing.T) {
	base, err := asn1.Marshal(manifestContentFixture())
	if err != nil {
		t.Fatal(err)
	}
	fields, err := rpkiResourceSequence(base)
	if err != nil {
		t.Fatal(err)
	}
	encode := func(fields []asn1.RawValue) []byte {
		t.Helper()
		var body []byte
		for _, f := range fields {
			body = append(body, f.FullBytes...)
		}
		der, err := asn1.Marshal(asn1.RawValue{Tag: 16, IsCompound: true, Bytes: body})
		if err != nil {
			t.Fatal(err)
		}
		return der
	}
	for _, version := range []byte{0, 1} {
		v := asn1.RawValue{FullBytes: []byte{0xa0, 3, 2, 1, version}}
		if _, err := parseRPKIManifestContent(encode(append([]asn1.RawValue{v}, fields...))); err == nil {
			t.Fatal("accepted explicit default or unknown version")
		}
	}
	for _, s := range []string{"260923120000Z", "20260923120000+0000", "20260923120000.1Z", "202609231200Z", "20261323120000Z", "+0260923120000Z"} {
		der, err := asn1.Marshal(asn1.RawValue{Tag: 24, Bytes: []byte(s)})
		if err != nil {
			t.Fatal(err)
		}
		changed := append([]asn1.RawValue(nil), fields...)
		changed[1] = asn1.RawValue{FullBytes: der}
		if _, err := parseRPKIManifestContent(encode(changed)); err == nil {
			t.Fatalf("accepted invalid time %q", s)
		}
	}
	for _, der := range [][]byte{nil, base[:len(base)-1], append(append([]byte(nil), base...), 0), make([]byte, (4<<20)+1)} {
		if _, err := parseRPKIManifestContent(der); err == nil {
			t.Fatal("accepted malformed DER")
		}
	}
}

func TestRPKIManifestFiles(t *testing.T) {
	input := manifestContentFixture()
	der, _ := asn1.Marshal(input)
	m, err := parseRPKIManifestContent(der)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"Issuer_1.crl": []byte("fixture CRL"), "unlisted.cer": []byte("ignored")}
	for _, now := range []time.Time{m.ThisUpdate, m.NextUpdate.Add(-time.Second)} {
		if err := m.checkFiles(now, files); err != nil {
			t.Fatal(err)
		}
	}
	for _, now := range []time.Time{{}, m.ThisUpdate.Add(-time.Second), m.NextUpdate, m.NextUpdate.Add(time.Second)} {
		if err := m.checkFiles(now, files); err == nil {
			t.Fatal("accepted noncurrent manifest")
		}
	}
	for _, bad := range []map[string][]byte{nil, {}, {"issuer_1.crl": []byte("fixture CRL")}, {"Issuer_1.crl": []byte("modified")}} {
		if err := m.checkFiles(m.ThisUpdate, bad); err == nil {
			t.Fatal("accepted missing or modified file")
		}
	}
	m.Files["second.crl"] = m.Files["Issuer_1.crl"]
	files["second.crl"] = []byte("fixture CRL")
	if err := m.checkFiles(m.ThisUpdate, files); err == nil {
		t.Fatal("accepted multiple CRLs")
	}
	delete(m.Files, "second.crl")
	delete(m.Files, "Issuer_1.crl")
	if err := m.checkFiles(m.ThisUpdate, files); err == nil {
		t.Fatal("accepted missing CRL")
	}
}

func TestRPKIManifestFilename(t *testing.T) {
	for _, extension := range []string{"asa", "ccr", "cdr", "cer", "crl", "gbr", "mft", "roa", "sig", "tak"} {
		if !validRPKIManifestFilename("aB_1-." + extension) {
			t.Fatal(extension)
		}
	}
	for _, name := range []string{"", ".crl", "a.CRL", "a.txt", "a.b.crl", "a/b.crl", "a\\b.crl", "a%20.crl", "a .crl", "é.crl", "a.crl\x00"} {
		if validRPKIManifestFilename(name) {
			t.Fatalf("accepted %q", name)
		}
	}
}
