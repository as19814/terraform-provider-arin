package arin

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"net/netip"
	"testing"
)

func TestRPKIResourcePath(t *testing.T) {
	as := func(low, high int64) pkix.Extension {
		var value asn1.RawValue
		if low == high {
			value = resourceTestRaw(t, low)
		} else {
			value = resourceTestRaw(t, struct{ Min, Max int64 }{low, high})
		}
		return resourceTestAS(t, resourceTestDER(t, []asn1.RawValue{value}))
	}
	ip := func(afi byte, prefix []byte, bits int) pkix.Extension {
		return resourceTestIP(t, resourceTestFamily{[]byte{0, afi}, asn1.RawValue{FullBytes: resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, asn1.BitString{Bytes: prefix, BitLength: bits})})}})
	}
	inheritAS := resourceTestAS(t, []byte{5, 0})
	inheritV4 := resourceTestIP(t, resourceTestFamily{[]byte{0, 1}, asn1.RawValue{FullBytes: []byte{5, 0}}})
	inheritV6 := resourceTestIP(t, resourceTestFamily{[]byte{0, 2}, asn1.RawValue{FullBytes: []byte{5, 0}}})
	root := []pkix.Extension{as(0, 4294967295), ip(1, nil, 0)}
	middle := []pkix.Extension{as(64500, 64510), inheritV4}
	leaf := []pkix.Extension{inheritAS, ip(1, []byte{192, 0, 2}, 24)}
	got, err := resolveRPKIResourcePath([][]pkix.Extension{leaf, middle, root})
	if err != nil || got.ASN == nil || got.ASN.Inherit || len(got.ASN.Ranges) != 1 || got.ASN.Ranges[0] != (rpkiASRange{64500, 64510}) || got.IPv4 == nil || got.IPv4.Inherit || got.IPv4.Ranges[0].Min != netip.MustParseAddr("192.0.2.0") || got.IPv6 != nil {
		t.Fatalf("incorrect resolved path: %v", err)
	}
	// A family omitted by the child remains absent even when the issuer has it.
	got, err = resolveRPKIResourcePath([][]pkix.Extension{{as(64501, 64501)}, middle, root})
	if err != nil || got.IPv4 != nil {
		t.Fatal("omitted family gained inherited resources")
	}
	for name, path := range map[string][][]pkix.Extension{
		"empty":                          nil,
		"inherited_anchor":               {{inheritAS}},
		"missing_parent_family":          {{inheritV6}, root},
		"explicit_missing_parent_family": {{ip(2, []byte{0x20, 1, 0xd, 0xb8}, 32)}, root},
		"as_below_parent":                {{as(64499, 64501)}, middle, root},
		"as_above_parent":                {{as(64509, 64511)}, middle, root},
		"ip_outside_parent":              {{ip(1, []byte{192, 0, 3}, 24)}, {ip(1, []byte{192, 0, 2}, 24)}, root},
		"ip_superset":                    {{ip(1, []byte{192, 0, 2}, 23)}, {ip(1, []byte{192, 0, 2}, 24)}, root},
		"invalid_middle":                 {leaf, {{Id: oidRPKIASResources, Critical: true, Value: []byte{0x30, 0}}}, root},
		"dropped_then_inherited":         {{inheritV4}, {as(64500, 64510)}, root},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := resolveRPKIResourcePath(path); err == nil || got != nil {
				t.Fatal("invalid resource path accepted")
			}
		})
	}
	// A range may not span a gap between two authorized parent ranges.
	gappedAS := resourceTestAS(t, resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, struct{ Min, Max int64 }{1, 3}), resourceTestRaw(t, struct{ Min, Max int64 }{5, 7})}))
	if got, err := resolveRPKIResourcePath([][]pkix.Extension{{as(2, 6)}, {gappedAS}}); err == nil || got != nil {
		t.Fatal("AS range crossed an authorization gap")
	}
	gappedIP := resourceTestIP(t, resourceTestFamily{[]byte{0, 1}, asn1.RawValue{FullBytes: resourceTestDER(t, []asn1.RawValue{
		resourceTestRaw(t, asn1.BitString{Bytes: []byte{192, 0, 2, 0}, BitLength: 26}),
		resourceTestRaw(t, asn1.BitString{Bytes: []byte{192, 0, 2, 128}, BitLength: 26}),
	})}})
	if got, err := resolveRPKIResourcePath([][]pkix.Extension{{ip(1, []byte{192, 0, 2}, 24)}, {gappedIP}}); err == nil || got != nil {
		t.Fatal("IP range crossed an authorization gap")
	}
	// Check several child ranges against separate parent ranges in order.
	sparseChild := resourceTestAS(t, resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, int64(2)), resourceTestRaw(t, int64(6))}))
	if _, err := resolveRPKIResourcePath([][]pkix.Extension{{sparseChild}, {gappedAS}}); err != nil {
		t.Fatal(err)
	}
	v6root := []pkix.Extension{ip(2, []byte{0x20, 1, 0xd, 0xb8}, 32)}
	v6leaf := []pkix.Extension{ip(2, []byte{0x20, 1, 0xd, 0xb8, 0, 1}, 48)}
	got, err = resolveRPKIResourcePath([][]pkix.Extension{v6leaf, {inheritV6}, v6root})
	if err != nil || got.IPv6.Ranges[0].Max != netip.MustParseAddr("2001:db8:1:ffff:ffff:ffff:ffff:ffff") {
		t.Fatal("IPv6 inheritance failed")
	}
	if _, err := resolveRPKIResourcePath([][]pkix.Extension{root, root}); err != nil {
		t.Fatal("equal explicit resources rejected")
	}
	long := make([][]pkix.Extension, 33)
	for i := range long {
		long[i] = root
	}
	if _, err := resolveRPKIResourcePath(long); err == nil {
		t.Fatal("unbounded path accepted")
	}
}

func TestRPKIResourceResolutionCopies(t *testing.T) {
	parentAS := &rpkiASSet{Ranges: []rpkiASRange{{1, 5}}}
	gotAS, err := resolveRPKIASSet(&rpkiASSet{Inherit: true}, parentAS)
	if err != nil {
		t.Fatal(err)
	}
	gotAS.Ranges[0].Min = 3
	if parentAS.Ranges[0].Min != 1 {
		t.Fatal("AS inheritance aliases parent")
	}
	parentIP := &rpkiIPSet{Ranges: []rpkiIPRange{{netip.MustParseAddr("192.0.2.0"), netip.MustParseAddr("192.0.2.255")}}}
	gotIP, err := resolveRPKIIPSet(&rpkiIPSet{Inherit: true}, parentIP)
	if err != nil {
		t.Fatal(err)
	}
	gotIP.Ranges[0].Min = netip.MustParseAddr("192.0.2.1")
	if parentIP.Ranges[0].Min != netip.MustParseAddr("192.0.2.0") {
		t.Fatal("IP inheritance aliases parent")
	}
}
