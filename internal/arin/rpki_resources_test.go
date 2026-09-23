package arin

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"net/netip"
	"testing"
)

func resourceTestDER(t *testing.T, value any) []byte {
	t.Helper()
	der, err := asn1.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return der
}
func resourceTestRaw(t *testing.T, value any) asn1.RawValue {
	t.Helper()
	return asn1.RawValue{FullBytes: resourceTestDER(t, value)}
}
func resourceTestAS(t *testing.T, choice []byte) pkix.Extension {
	t.Helper()
	return pkix.Extension{Id: oidRPKIASResources, Critical: true, Value: resourceTestDER(t, []asn1.RawValue{{Class: 2, Tag: 0, IsCompound: true, Bytes: choice}})}
}

type resourceTestFamily struct {
	AFI    []byte
	Choice asn1.RawValue
}

func resourceTestIP(t *testing.T, families ...resourceTestFamily) pkix.Extension {
	t.Helper()
	return pkix.Extension{Id: oidRPKIIPResources, Critical: true, Value: resourceTestDER(t, families)}
}
func TestRPKICertificateResources(t *testing.T) {
	as := resourceTestAS(t, resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, int64(19814)), resourceTestRaw(t, struct{ Min, Max int64 }{64500, 64510}), resourceTestRaw(t, int64(4294967295))}))
	v4 := resourceTestDER(t, []asn1.RawValue{
		resourceTestRaw(t, asn1.BitString{Bytes: []byte{192, 0, 2}, BitLength: 24}),
		resourceTestRaw(t, struct{ Min, Max asn1.BitString }{asn1.BitString{Bytes: []byte{192, 0, 3, 1}, BitLength: 32}, asn1.BitString{Bytes: []byte{192, 0, 3, 6}, BitLength: 32}}),
	})
	ip := resourceTestIP(t, resourceTestFamily{[]byte{0, 1}, asn1.RawValue{FullBytes: v4}}, resourceTestFamily{[]byte{0, 2}, asn1.RawValue{FullBytes: []byte{5, 0}}})
	got, err := parseRPKICertificateResources([]pkix.Extension{as, ip})
	if err != nil {
		t.Fatal(err)
	}
	if got.ASN == nil || got.ASN.Inherit || len(got.ASN.Ranges) != 3 || got.ASN.Ranges[2].Max != 4294967295 || got.IPv4 == nil || len(got.IPv4.Ranges) != 2 || got.IPv4.Ranges[0].Min != netip.MustParseAddr("192.0.2.0") || got.IPv4.Ranges[0].Max != netip.MustParseAddr("192.0.2.255") || got.IPv4.Ranges[1].Max != netip.MustParseAddr("192.0.3.6") || got.IPv6 == nil || !got.IPv6.Inherit {
		t.Fatalf("incorrect decoded resources: %+v", got)
	}
	got, err = parseRPKICertificateResources([]pkix.Extension{resourceTestAS(t, []byte{5, 0})})
	if err != nil || got.ASN == nil || !got.ASN.Inherit || got.IPv4 != nil || got.IPv6 != nil {
		t.Fatal("inheritance and absence conflated")
	}
	v6 := resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, asn1.BitString{Bytes: []byte{0x20, 1, 0x0d, 0xb8}, BitLength: 32})})
	got, err = parseRPKICertificateResources([]pkix.Extension{resourceTestIP(t, resourceTestFamily{[]byte{0, 2}, asn1.RawValue{FullBytes: v6}})})
	if err != nil || got.IPv6.Ranges[0].Min != netip.MustParseAddr("2001:db8::") || got.IPv6.Ranges[0].Max != netip.MustParseAddr("2001:db8:ffff:ffff:ffff:ffff:ffff:ffff") {
		t.Fatal("incorrect IPv6 prefix")
	}
}

func TestRPKIResourceRejections(t *testing.T) {
	asChoice := func(values ...asn1.RawValue) pkix.Extension { return resourceTestAS(t, resourceTestDER(t, values)) }
	integer := func(n int64) asn1.RawValue { return resourceTestRaw(t, n) }
	validAS := asChoice(integer(19814))
	inherit := asn1.RawValue{FullBytes: []byte{5, 0}}
	for name, exts := range map[string][]pkix.Extension{
		"absent": nil, "duplicate": {validAS, validAS}, "noncritical": {{Id: oidRPKIASResources, Value: validAS.Value}},
		"negative": {asChoice(integer(-1))}, "overflow": {asChoice(integer(4294967296))},
		"adjacent": {asChoice(integer(1), integer(2))}, "duplicate_as": {asChoice(integer(1), integer(1))}, "unordered": {asChoice(integer(3), integer(1))},
		"empty_as":         {resourceTestAS(t, []byte{0x30, 0})},
		"singleton_range":  {asChoice(resourceTestRaw(t, struct{ Min, Max int64 }{1, 1}))},
		"reversed_range":   {asChoice(resourceTestRaw(t, struct{ Min, Max int64 }{3, 1}))},
		"rdi":              {{Id: oidRPKIASResources, Critical: true, Value: resourceTestDER(t, []asn1.RawValue{{Class: 2, Tag: 1, IsCompound: true, Bytes: []byte{5, 0}}})}},
		"safi":             {resourceTestIP(t, resourceTestFamily{[]byte{0, 1, 1}, inherit})},
		"unknown_family":   {resourceTestIP(t, resourceTestFamily{[]byte{0, 3}, inherit})},
		"duplicate_family": {resourceTestIP(t, resourceTestFamily{[]byte{0, 1}, inherit}, resourceTestFamily{[]byte{0, 1}, inherit})},
		"family_order":     {resourceTestIP(t, resourceTestFamily{[]byte{0, 2}, inherit}, resourceTestFamily{[]byte{0, 1}, inherit})},
		"empty_ip":         {resourceTestIP(t)},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := parseRPKICertificateResources(exts); err == nil || got != nil {
				t.Fatal("invalid resources accepted")
			}
		})
	}
	prefix := func(b []byte, bits int) asn1.RawValue {
		return resourceTestRaw(t, asn1.BitString{Bytes: b, BitLength: bits})
	}
	rangeValue := func(lo, hi asn1.BitString) asn1.RawValue {
		return resourceTestRaw(t, struct{ Min, Max asn1.BitString }{lo, hi})
	}
	for name, values := range map[string][]asn1.RawValue{
		"empty":                   {},
		"oversized_prefix":        {prefix([]byte{0, 0, 0, 0, 0}, 33)},
		"adjacent":                {prefix([]byte{192, 0, 2}, 24), prefix([]byte{192, 0, 3}, 24)},
		"overlap":                 {prefix([]byte{192, 0, 2}, 24), prefix([]byte{192, 0, 2, 128}, 25)},
		"reversed":                {rangeValue(asn1.BitString{Bytes: []byte{192, 0, 2, 7}, BitLength: 32}, asn1.BitString{Bytes: []byte{192, 0, 2, 6}, BitLength: 32})},
		"untrimmed_min":           {rangeValue(asn1.BitString{Bytes: []byte{192, 0, 2, 2}, BitLength: 32}, asn1.BitString{Bytes: []byte{192, 0, 2, 6}, BitLength: 32})},
		"untrimmed_max":           {rangeValue(asn1.BitString{Bytes: []byte{192, 0, 2, 1}, BitLength: 32}, asn1.BitString{Bytes: []byte{192, 0, 2, 7}, BitLength: 32})},
		"range_instead_of_prefix": {rangeValue(asn1.BitString{Bytes: []byte{192, 0, 2}, BitLength: 23}, asn1.BitString{Bytes: []byte{192, 0, 2}, BitLength: 24})},
	} {
		t.Run(name, func(t *testing.T) {
			ip := resourceTestIP(t, resourceTestFamily{[]byte{0, 1}, asn1.RawValue{FullBytes: resourceTestDER(t, values)}})
			if got, err := parseRPKICertificateResources([]pkix.Extension{ip}); err == nil || got != nil {
				t.Fatal("invalid IP ranges accepted")
			}
		})
	}
}

func TestRPKIResourceFullFamilies(t *testing.T) {
	prefix := resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, asn1.BitString{})})
	ip := resourceTestIP(t, resourceTestFamily{[]byte{0, 1}, asn1.RawValue{FullBytes: prefix}}, resourceTestFamily{[]byte{0, 2}, asn1.RawValue{FullBytes: prefix}})
	got, err := parseRPKICertificateResources([]pkix.Extension{ip})
	if err != nil || got.IPv4.Ranges[0].Min != netip.MustParseAddr("0.0.0.0") || got.IPv4.Ranges[0].Max != netip.MustParseAddr("255.255.255.255") || got.IPv6.Ranges[0].Min != netip.MustParseAddr("::") || got.IPv6.Ranges[0].Max != netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff") {
		t.Fatal("incorrect full-family bounds")
	}
}
