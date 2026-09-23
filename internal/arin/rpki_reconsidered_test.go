package arin

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"net/netip"
	"slices"
	"testing"
)

func reconsideredTestAS(t *testing.T, low, high int64) pkix.Extension {
	t.Helper()
	e := resourceTestAS(t, resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, struct{ Min, Max int64 }{low, high})}))
	e.Id = oidRPKIASResourcesV2
	return e
}

func TestRPKIReconsideredResourcePath(t *testing.T) {
	root := reconsideredTestAS(t, 10, 20)
	middle := reconsideredTestAS(t, 1, 30)
	leaf := reconsideredTestAS(t, 15, 25)
	got, err := resolveRPKIResourcePath([][]pkix.Extension{{leaf}, {middle}, {root}})
	if err != nil || !slices.Equal(got.ASN.Ranges, []rpkiASRange{{15, 20}}) {
		t.Fatalf("overclaim intersection: %v", err)
	}
	leaf.Id = oidRPKIASResources
	if _, err := resolveRPKIResourcePath([][]pkix.Extension{{leaf}, {middle}, {root}}); err == nil {
		t.Fatal("original-profile child regained unverified resources")
	}
	leaf = reconsideredTestAS(t, 40, 50)
	got, err = resolveRPKIResourcePath([][]pkix.Extension{{middle}, {leaf}, {root}})
	if err != nil || len(got.ASN.Ranges) != 0 {
		t.Fatal("empty intersection regained resources")
	}
	inherited := resourceTestAS(t, []byte{5, 0})
	inherited.Id = oidRPKIASResourcesV2
	got, err = resolveRPKIResourcePath([][]pkix.Extension{{inherited}, {middle}, {root}})
	if err != nil || !slices.Equal(got.ASN.Ranges, []rpkiASRange{{10, 20}}) {
		t.Fatal("inheritance bypassed verified resources")
	}
	if _, err := resolveRPKIResourcePath([][]pkix.Extension{{inherited}}); err == nil {
		t.Fatal("inherited trust anchor accepted")
	}
	for _, tc := range []struct {
		afi       byte
		prefix    []byte
		bits      int
		low, high string
	}{
		{1, []byte{192, 0, 2}, 24, "192.0.2.0", "192.0.2.255"},
		{2, []byte{0x20, 1, 0xd, 0xb8}, 32, "2001:db8::", "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff"},
	} {
		parent := resourceTestIP(t, resourceTestFamily{[]byte{0, tc.afi}, asn1.RawValue{FullBytes: resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, asn1.BitString{Bytes: tc.prefix, BitLength: tc.bits})})}})
		child := resourceTestIP(t, resourceTestFamily{[]byte{0, tc.afi}, asn1.RawValue{FullBytes: resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, asn1.BitString{BitLength: 0})})}})
		child.Id = oidRPKIIPResourcesV2
		got, err := resolveRPKIResourcePath([][]pkix.Extension{{child}, {parent}})
		if err != nil {
			t.Fatal(err)
		}
		actual := got.IPv4
		if tc.afi == 2 {
			actual = got.IPv6
		}
		if !slices.Equal(actual.Ranges, []rpkiIPRange{{netip.MustParseAddr(tc.low), netip.MustParseAddr(tc.high)}}) {
			t.Fatal("IP overclaim was not clipped")
		}
		got, err = resolveRPKIResourcePath([][]pkix.Extension{{child}, {root}})
		if err != nil {
			t.Fatal(err)
		}
		actual = got.IPv4
		if tc.afi == 2 {
			actual = got.IPv6
		}
		if len(actual.Ranges) != 0 {
			t.Fatal("missing family granted resources")
		}
	}
}

func TestRPKIReconsideredSignedPath(t *testing.T) {
	for _, mode := range []string{"valid", "policy_only", "resources_only", "mixed", "noncritical", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			f := resourceCertificateFixture(t)
			c := *f.certs[0]
			c.ExtraExtensions = slices.Clone(c.Extensions)
			for i, e := range c.ExtraExtensions {
				if e.Id.Equal(oidRPKIASResources) && mode != "policy_only" {
					c.ExtraExtensions[i] = reconsideredTestAS(t, 64400, 64600)
					if mode == "noncritical" {
						c.ExtraExtensions[i].Critical = false
					}
				}
				if e.Id.String() == "2.5.29.32" && mode != "resources_only" {
					c.ExtraExtensions[i].Value = resourceTestDER(t, []struct{ ID asn1.ObjectIdentifier }{{asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 14, 3}}})
				}
			}
			if mode == "mixed" {
				c.ExtraExtensions = append(c.ExtraExtensions, resourceTestAS(t, []byte{5, 0}))
			}
			if mode == "duplicate" {
				c.ExtraExtensions = append(c.ExtraExtensions, reconsideredTestAS(t, 1, 2))
			}
			raw, err := x509.CreateCertificate(rand.Reader, &c, f.certs[1], &f.keys[0].PublicKey, f.keys[1])
			if err != nil {
				t.Fatal(err)
			}
			cert, err := x509.ParseCertificate(raw)
			if err != nil {
				if mode == "duplicate" {
					return
				}
				t.Fatal(err)
			}
			f.certs[0] = cert
			got, err := verifyRPKICertificatePath(f.certs, f.certs[2], f.crls, cmsTrustNow())
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode %s: %v", mode, err)
			}
			if err == nil && !slices.Equal(got.ASN.Ranges, []rpkiASRange{{64500, 64510}}) {
				t.Fatal("signed overclaim expanded authorization")
			}
		})
	}
}

func TestRPKIReconsideredManifestEE(t *testing.T) {
	f := resourceCertificateFixture(t)
	c := manifestEETemplate(t, f)
	for i, e := range c.ExtraExtensions {
		if e.Id.Equal(oidRPKIASResources) {
			c.ExtraExtensions[i].Id = oidRPKIASResourcesV2
		}
		if e.Id.String() == "2.5.29.32" {
			c.ExtraExtensions[i].Value = resourceTestDER(t, []struct{ ID asn1.ObjectIdentifier }{{asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 14, 3}}})
		}
	}
	if err := validateRPKIManifestEEProfile(signManifestEE(t, c, f)); err != nil {
		t.Fatal(err)
	}
	for i, e := range c.ExtraExtensions {
		if e.Id.Equal(oidRPKIASResourcesV2) {
			c.ExtraExtensions[i] = reconsideredTestAS(t, 64500, 64510)
		}
	}
	if err := validateRPKIManifestEEProfile(signManifestEE(t, c, f)); err == nil {
		t.Fatal("reconsidered manifest EE accepted explicit resources")
	}
}

func TestRPKIReconsideredResourceGaps(t *testing.T) {
	parent := resourceTestAS(t, resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, struct{ Min, Max int64 }{1, 3}), resourceTestRaw(t, struct{ Min, Max int64 }{5, 7})}))
	child := reconsideredTestAS(t, 2, 6)
	got, err := resolveRPKIResourcePath([][]pkix.Extension{{child}, {parent}})
	if err != nil || !slices.Equal(got.ASN.Ranges, []rpkiASRange{{2, 3}, {5, 6}}) {
		t.Fatal("intersection filled an authorization gap")
	}
	// An intermediate which omits AS resources cannot pass the anchor's AS set
	// to a later child, even when that child uses the alternate policy.
	ip := resourceTestIP(t, resourceTestFamily{[]byte{0, 1}, asn1.RawValue{FullBytes: resourceTestDER(t, []asn1.RawValue{resourceTestRaw(t, asn1.BitString{})})}})
	ip.Id = oidRPKIIPResourcesV2
	got, err = resolveRPKIResourcePath([][]pkix.Extension{{child}, {ip}, {parent}})
	if err != nil || len(got.ASN.Ranges) != 0 {
		t.Fatal("omitted intermediate resources reappeared")
	}
}
