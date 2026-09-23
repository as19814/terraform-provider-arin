package arin

import (
	"slices"
	"testing"
)

func TestRPKIIssuedResourceMatching(t *testing.T) {
	str := func(s string) *string { return &s }
	for _, tt := range []struct {
		name, allocation, actual string
		request                  *string
		valid                    bool
	}{
		{"all", "10-20", "10-20", nil, true},
		{"adjacent", "10-12,13,14-20", "10-20", nil, true},
		{"subset", "10-20", "12-15", str("12-15"), true},
		{"bounded", "10-20", "10-15", str("0-15"), true},
		{"gaps", "10-12,15-20", "11-12,15-16", str("11-16"), true},
		{"omit_family", "10-20", "", str(""), true},
		{"disjoint", "10-20", "", str("30-40"), true},
		{"too_much", "10-20", "10-21", nil, false},
		{"too_little", "10-20", "10-19", nil, false},
		{"outside_request", "10-20", "10-20", str("12-15"), false},
		{"empty_request_ignored", "10-20", "10-20", str(""), false},
		{"uint32_end", "4294967294,4294967295", "4294967294-4294967295", nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			class := rpkiResourceClass{ASN: tt.allocation, Certificates: []rpkiResourceCertificate{{RequestedASN: tt.request}}}
			ranges, err := issueASRanges(tt.actual)
			if err != nil {
				t.Fatal(err)
			}
			got := &rpkiCertificateResources{}
			if len(ranges) > 0 {
				got.ASN = &rpkiASSet{Ranges: ranges}
			}
			if err := validateIssuedResources(&class, got); (err == nil) != tt.valid {
				t.Fatalf("valid=%v err=%v", tt.valid, err)
			}
		})
	}
	for _, tt := range []struct {
		name, allocation, request, actual string
		family                            int
	}{
		{"ipv4", "192.0.2.0/25,192.0.2.128/25", "192.0.2.64-192.0.3.255", "192.0.2.64-192.0.2.255", 4},
		{"ipv6", "2001:db8::/32", "2001:db8:1000::/36", "2001:db8:1000::-2001:db8:1fff:ffff:ffff:ffff:ffff:ffff", 6},
		{"ipv4_full", "0.0.0.0/0", "255.255.255.255/32", "255.255.255.255/32", 4},
		{"ipv6_full", "::/0", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff/128", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff/128", 6},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := issueIPRanges(tt.actual, tt.family)
			if err != nil {
				t.Fatal(err)
			}
			class := rpkiResourceClass{Certificates: []rpkiResourceCertificate{{}}}
			got := &rpkiCertificateResources{}
			if tt.family == 4 {
				class.IPv4 = tt.allocation
				class.Certificates[0].RequestedIPv4 = str(tt.request)
				got.IPv4 = &rpkiIPSet{Ranges: actual}
			} else {
				class.IPv6 = tt.allocation
				class.Certificates[0].RequestedIPv6 = str(tt.request)
				got.IPv6 = &rpkiIPSet{Ranges: actual}
			}
			if err := validateIssuedResources(&class, got); err != nil {
				t.Fatal(err)
			}
			got.IPv4 = nil
			got.IPv6 = nil
			if err := validateIssuedResources(&class, got); err == nil {
				t.Fatal("missing requested addresses accepted")
			}
		})
	}
}

func TestRPKIIssueResourceRanges(t *testing.T) {
	for _, s := range []string{"01", "20-10", "1,1", "4294967296", "1,,2"} {
		if _, err := issueASRanges(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	for _, s := range []string{"192.0.2.1/24", "192.0.2.0/24,192.0.2.0/25", "::/0", "192.0.2.1"} {
		if _, err := issueIPRanges(s, 4); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	a, _ := issueIPRanges("192.0.2.0/26,192.0.2.128/26", 4)
	b, _ := issueIPRanges("192.0.2.32-192.0.2.159", 4)
	want, _ := issueIPRanges("192.0.2.32-192.0.2.63,192.0.2.128-192.0.2.159", 4)
	if !slices.Equal(intersectIssueIP(a, b), want) {
		t.Fatal("intersection bridged allocation gap")
	}
	class := rpkiResourceClass{ASN: "1", Certificates: []rpkiResourceCertificate{{}}}
	if validateIssuedResources(&class, &rpkiCertificateResources{ASN: &rpkiASSet{Inherit: true}}) == nil {
		t.Fatal("unresolved inheritance accepted")
	}
}
