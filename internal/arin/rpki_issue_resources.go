package arin

import (
	"encoding/asn1"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

// RFC 6492 omitted request attributes select the whole allocation; an empty
// attribute selects none. A supplied set bounds the allocated resources.
func validateIssuedResources(class *rpkiResourceClass, got *rpkiCertificateResources) error {
	if class == nil || got == nil || len(class.Certificates) != 1 {
		return errRPKIUpDown
	}
	issued := class.Certificates[0]
	as, err := issueASRanges(class.ASN)
	if err != nil {
		return err
	}
	if issued.RequestedASN != nil {
		want, err := issueASRanges(*issued.RequestedASN)
		if err != nil {
			return err
		}
		as = intersectIssueAS(as, want)
	}
	var actualAS []rpkiASRange
	if got.ASN != nil {
		if got.ASN.Inherit {
			return errRPKIUpDown
		}
		actualAS = got.ASN.Ranges
	}
	if !slices.Equal(as, actualAS) {
		return errRPKIUpDown
	}
	for _, f := range []struct {
		allocation string
		request    *string
		actual     *rpkiIPSet
		family     int
	}{{class.IPv4, issued.RequestedIPv4, got.IPv4, 4}, {class.IPv6, issued.RequestedIPv6, got.IPv6, 6}} {
		expected, err := issueIPRanges(f.allocation, f.family)
		if err != nil {
			return err
		}
		if f.request != nil {
			want, err := issueIPRanges(*f.request, f.family)
			if err != nil {
				return err
			}
			expected = intersectIssueIP(expected, want)
		}
		var actual []rpkiIPRange
		if f.actual != nil {
			if f.actual.Inherit {
				return errRPKIUpDown
			}
			actual = f.actual.Ranges
		}
		if !slices.Equal(expected, actual) {
			return errRPKIUpDown
		}
	}
	return nil
}

func issueASRanges(s string) ([]rpkiASRange, error) {
	if !upDownResources(s, 0) {
		return nil, errRPKIUpDown
	}
	if s == "" {
		return nil, nil
	}
	var out []rpkiASRange
	for _, item := range strings.Split(s, ",") {
		p := strings.Split(item, "-")
		low, _ := strconv.ParseUint(p[0], 10, 32)
		high, _ := strconv.ParseUint(p[len(p)-1], 10, 32)
		r := rpkiASRange{uint32(low), uint32(high)}
		if len(out) > 0 && uint64(out[len(out)-1].Max)+1 == low {
			out[len(out)-1].Max = r.Max
		} else {
			out = append(out, r)
		}
	}
	return out, nil
}

func issueIPRanges(s string, family int) ([]rpkiIPRange, error) {
	if (family != 4 && family != 6) || !upDownResources(s, family) {
		return nil, errRPKIUpDown
	}
	if s == "" {
		return nil, nil
	}
	var out []rpkiIPRange
	for _, item := range strings.Split(s, ",") {
		var low, high netip.Addr
		if strings.Contains(item, "/") {
			p, _ := netip.ParsePrefix(item)
			low = p.Addr()
			raw := low.AsSlice()
			high = rpkiResourceAddress(asn1.BitString{Bytes: raw, BitLength: p.Bits()}, low.BitLen(), true)
		} else {
			p := strings.Split(item, "-")
			low, _ = netip.ParseAddr(p[0])
			high, _ = netip.ParseAddr(p[1])
		}
		r := rpkiIPRange{low, high}
		if len(out) > 0 && out[len(out)-1].Max.Next() == low {
			out[len(out)-1].Max = high
		} else {
			out = append(out, r)
		}
	}
	return out, nil
}

func intersectIssueAS(a, b []rpkiASRange) []rpkiASRange {
	var out []rpkiASRange
	for i, j := 0, 0; i < len(a) && j < len(b); {
		low, high := max(a[i].Min, b[j].Min), min(a[i].Max, b[j].Max)
		if low <= high {
			out = append(out, rpkiASRange{low, high})
		}
		if a[i].Max < b[j].Max {
			i++
		} else {
			j++
		}
	}
	return out
}

func intersectIssueIP(a, b []rpkiIPRange) []rpkiIPRange {
	var out []rpkiIPRange
	for i, j := 0, 0; i < len(a) && j < len(b); {
		low, high := a[i].Min, a[i].Max
		if b[j].Min.Compare(low) > 0 {
			low = b[j].Min
		}
		if b[j].Max.Compare(high) < 0 {
			high = b[j].Max
		}
		if low.Compare(high) <= 0 {
			out = append(out, rpkiIPRange{low, high})
		}
		if a[i].Max.Compare(b[j].Max) < 0 {
			i++
		} else {
			j++
		}
	}
	return out
}
