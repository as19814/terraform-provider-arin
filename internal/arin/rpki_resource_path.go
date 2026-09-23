package arin

import (
	"crypto/x509/pkix"
	"slices"
)

// Resolve a leaf-first sequence ending at an explicit resource trust anchor.
// The caller must separately authenticate this exact certificate path. These
// extension sets alone prove neither signatures nor trust-anchor identity.
func resolveRPKIResourcePath(path [][]pkix.Extension) (*rpkiCertificateResources, error) {
	if len(path) == 0 || len(path) > 32 {
		return nil, errRPKIUpDown
	}
	var parent *rpkiCertificateResources
	for i := len(path) - 1; i >= 0; i-- {
		current, err := parseRPKICertificateResources(path[i])
		if err != nil {
			return nil, err
		}
		if parent == nil {
			if (current.ASN != nil && current.ASN.Inherit) || (current.IPv4 != nil && current.IPv4.Inherit) || (current.IPv6 != nil && current.IPv6.Inherit) {
				return nil, errRPKIUpDown
			}
			parent = current
			continue
		}
		resolved := &rpkiCertificateResources{}
		resolved.ASN, err = resolveRPKIASSet(current.ASN, parent.ASN)
		if err != nil {
			return nil, err
		}
		resolved.IPv4, err = resolveRPKIIPSet(current.IPv4, parent.IPv4)
		if err != nil {
			return nil, err
		}
		resolved.IPv6, err = resolveRPKIIPSet(current.IPv6, parent.IPv6)
		if err != nil {
			return nil, err
		}
		parent = resolved
	}
	return parent, nil
}

func resolveRPKIASSet(child, parent *rpkiASSet) (*rpkiASSet, error) {
	if child == nil {
		return nil, nil
	}
	if parent == nil || parent.Inherit {
		return nil, errRPKIUpDown
	}
	if child.Inherit {
		return &rpkiASSet{Ranges: slices.Clone(parent.Ranges)}, nil
	}
	j := 0
	for _, r := range child.Ranges {
		for j < len(parent.Ranges) && parent.Ranges[j].Max < r.Min {
			j++
		}
		if j == len(parent.Ranges) || r.Min < parent.Ranges[j].Min || r.Max > parent.Ranges[j].Max {
			return nil, errRPKIUpDown
		}
	}
	return &rpkiASSet{Ranges: slices.Clone(child.Ranges)}, nil
}

func resolveRPKIIPSet(child, parent *rpkiIPSet) (*rpkiIPSet, error) {
	if child == nil {
		return nil, nil
	}
	if parent == nil || parent.Inherit {
		return nil, errRPKIUpDown
	}
	if child.Inherit {
		return &rpkiIPSet{Ranges: slices.Clone(parent.Ranges)}, nil
	}
	j := 0
	for _, r := range child.Ranges {
		for j < len(parent.Ranges) && parent.Ranges[j].Max.Compare(r.Min) < 0 {
			j++
		}
		if j == len(parent.Ranges) || r.Min.Compare(parent.Ranges[j].Min) < 0 || r.Max.Compare(parent.Ranges[j].Max) > 0 {
			return nil, errRPKIUpDown
		}
	}
	return &rpkiIPSet{Ranges: slices.Clone(child.Ranges)}, nil
}
