package arin

import (
	"bytes"
	"crypto/x509/pkix"
	"encoding/asn1"
	"net/netip"
)

var oidRPKIIPResources = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 7}
var oidRPKIASResources = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 8}

type rpkiASRange struct{ Min, Max uint32 }
type rpkiIPRange struct{ Min, Max netip.Addr }
type rpkiASSet struct {
	Inherit bool
	Ranges  []rpkiASRange
}
type rpkiIPSet struct {
	Inherit bool
	Ranges  []rpkiIPRange
}
type rpkiCertificateResources struct {
	ASN        *rpkiASSet
	IPv4, IPv6 *rpkiIPSet
}

// Decode the RFC 6487 subset of RFC 3779. Nil means absent, not inherited.
// This does not authenticate the certificate or resolve inheritance.
func parseRPKICertificateResources(extensions []pkix.Extension) (*rpkiCertificateResources, error) {
	out := &rpkiCertificateResources{}
	seen := make(map[string]bool)
	for _, e := range extensions {
		if !e.Id.Equal(oidRPKIASResources) && !e.Id.Equal(oidRPKIIPResources) {
			continue
		}
		if seen[e.Id.String()] || !e.Critical || len(e.Value) > 512000 {
			return nil, errRPKIUpDown
		}
		seen[e.Id.String()] = true
		nodes, err := rpkiResourceSequence(e.Value)
		if err != nil || len(nodes) == 0 {
			return nil, errRPKIUpDown
		}
		if e.Id.Equal(oidRPKIASResources) {
			// RDI is excluded from the resource-certificate profile.
			if len(nodes) != 1 || nodes[0].Class != 2 || nodes[0].Tag != 0 || !nodes[0].IsCompound {
				return nil, errRPKIUpDown
			}
			out.ASN, err = parseRPKIASChoice(nodes[0].Bytes)
			if err != nil {
				return nil, err
			}
		} else {
			previous := byte(0)
			for _, n := range nodes {
				var family struct {
					AFI    []byte
					Choice asn1.RawValue
				}
				if !rpkiCSRDER(n.FullBytes, &family) || len(family.AFI) != 2 || family.AFI[0] != 0 || family.AFI[1] < 1 || family.AFI[1] > 2 || family.AFI[1] <= previous {
					return nil, errRPKIUpDown
				}
				previous = family.AFI[1]
				bits := 32
				if previous == 2 {
					bits = 128
				}
				set, err := parseRPKIIPChoice(family.Choice.FullBytes, bits)
				if err != nil {
					return nil, err
				}
				if previous == 1 {
					out.IPv4 = set
				} else {
					out.IPv6 = set
				}
			}
		}
	}
	if len(seen) == 0 {
		return nil, errRPKIUpDown
	}
	return out, nil
}

func rpkiResourceSequence(der []byte) ([]asn1.RawValue, error) {
	var raw asn1.RawValue
	rest, err := asn1.Unmarshal(der, &raw)
	if err != nil || len(rest) != 0 || raw.Class != 0 || raw.Tag != 16 || !raw.IsCompound {
		return nil, errRPKIUpDown
	}
	var nodes []asn1.RawValue
	for remaining := raw.Bytes; len(remaining) > 0; {
		var n asn1.RawValue
		remaining, err = asn1.Unmarshal(remaining, &n)
		if err != nil {
			return nil, errRPKIUpDown
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func parseRPKIASChoice(der []byte) (*rpkiASSet, error) {
	if bytes.Equal(der, []byte{5, 0}) {
		return &rpkiASSet{Inherit: true}, nil
	}
	nodes, err := rpkiResourceSequence(der)
	if err != nil || len(nodes) == 0 {
		return nil, errRPKIUpDown
	}
	out := &rpkiASSet{}
	for _, n := range nodes {
		var low, high int64
		if n.Class == 0 && n.Tag == 2 {
			if !rpkiCSRDER(n.FullBytes, &low) {
				return nil, errRPKIUpDown
			}
			high = low
		} else {
			var r struct{ Min, Max int64 }
			if !rpkiCSRDER(n.FullBytes, &r) || r.Min >= r.Max {
				return nil, errRPKIUpDown
			}
			low, high = r.Min, r.Max
		}
		if low < 0 || high > 0xffffffff {
			return nil, errRPKIUpDown
		}
		if len(out.Ranges) > 0 && uint64(low) <= uint64(out.Ranges[len(out.Ranges)-1].Max)+1 {
			return nil, errRPKIUpDown
		}
		out.Ranges = append(out.Ranges, rpkiASRange{uint32(low), uint32(high)})
	}
	return out, nil
}

func parseRPKIIPChoice(der []byte, bits int) (*rpkiIPSet, error) {
	if bytes.Equal(der, []byte{5, 0}) {
		return &rpkiIPSet{Inherit: true}, nil
	}
	nodes, err := rpkiResourceSequence(der)
	if err != nil || len(nodes) == 0 {
		return nil, errRPKIUpDown
	}
	out := &rpkiIPSet{}
	for _, n := range nodes {
		var low, high netip.Addr
		if n.Class == 0 && n.Tag == 3 {
			var prefix asn1.BitString
			if !rpkiCSRDER(n.FullBytes, &prefix) || prefix.BitLength > bits {
				return nil, errRPKIUpDown
			}
			low, high = rpkiResourceAddress(prefix, bits, false), rpkiResourceAddress(prefix, bits, true)
		} else {
			var r struct{ Min, Max asn1.BitString }
			if !rpkiCSRDER(n.FullBytes, &r) || r.Min.BitLength > bits || r.Max.BitLength > bits {
				return nil, errRPKIUpDown
			}
			// Minimum omits trailing zero bits, maximum omits trailing one bits.
			if (r.Min.BitLength > 0 && r.Min.At(r.Min.BitLength-1) != 1) || (r.Max.BitLength > 0 && r.Max.At(r.Max.BitLength-1) != 0) {
				return nil, errRPKIUpDown
			}
			low, high = rpkiResourceAddress(r.Min, bits, false), rpkiResourceAddress(r.Max, bits, true)
			if low.Compare(high) >= 0 || rpkiRangeIsPrefix(low, high) {
				return nil, errRPKIUpDown
			}
		}
		if len(out.Ranges) > 0 {
			prior := out.Ranges[len(out.Ranges)-1].Max
			if low.Compare(prior) <= 0 || low == prior.Next() {
				return nil, errRPKIUpDown
			}
		}
		out.Ranges = append(out.Ranges, rpkiIPRange{low, high})
	}
	return out, nil
}

func rpkiResourceAddress(value asn1.BitString, bits int, fill bool) netip.Addr {
	var raw [16]byte
	for bit := 0; bit < bits; bit++ {
		if (bit < value.BitLength && value.At(bit) == 1) || (bit >= value.BitLength && fill) {
			raw[bit/8] |= 1 << uint(7-bit%8)
		}
	}
	if bits == 32 {
		return netip.AddrFrom4([4]byte(raw[:4]))
	}
	return netip.AddrFrom16(raw)
}

func rpkiRangeIsPrefix(low, high netip.Addr) bool {
	a, b := low.AsSlice(), high.AsSlice()
	differing := false
	for i := range a {
		for bit := 7; bit >= 0; bit-- {
			x, y := (a[i]>>uint(bit))&1, (b[i]>>uint(bit))&1
			if !differing && x == y {
				continue
			}
			differing = true
			if x != 0 || y != 1 {
				return false
			}
		}
	}
	return true
}
