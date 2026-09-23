package arin

import (
	"bytes"
	"crypto/x509/pkix"
	"encoding/asn1"
	"net/url"
	"strings"
)

var oidRPKISIA = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 11}
var oidRPKIRepository = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 5}
var oidRPKIManifest = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 10}

type rpkiAccessDescription struct {
	Method   asn1.ObjectIdentifier
	Location asn1.RawValue
}

// CA requests must identify both their repository directory and manifest.
// Preserve the full ordered DER value for comparison with the issued certificate.
func rpkiCASIA(extensions []pkix.Extension) ([]byte, error) {
	var value []byte
	for _, ext := range extensions {
		if ext.Id.Equal(oidRPKISIA) {
			if value != nil || ext.Critical || len(ext.Value) == 0 || len(ext.Value) > 512000 {
				return nil, errRPKIUpDown
			}
			value = ext.Value
		}
	}
	if value == nil {
		return nil, errRPKIUpDown
	}
	var descriptions []rpkiAccessDescription
	rest, err := asn1.Unmarshal(value, &descriptions)
	if err != nil || len(rest) != 0 || len(descriptions) == 0 {
		return nil, errRPKIUpDown
	}
	canonical, err := asn1.Marshal(descriptions)
	if err != nil || !bytes.Equal(value, canonical) {
		return nil, errRPKIUpDown
	}
	repository, manifest := false, false
	for _, d := range descriptions {
		if d.Location.Class != asn1.ClassContextSpecific || d.Location.Tag != 6 || d.Location.IsCompound {
			return nil, errRPKIUpDown
		}
		uri := string(d.Location.Bytes)
		if uri == "" || len(uri) > 4096 {
			return nil, errRPKIUpDown
		}
		for _, r := range uri {
			if r < 0x21 || r > 0x7e {
				return nil, errRPKIUpDown
			}
		}
		u, err := url.Parse(uri)
		if err != nil || u.Scheme == "" || u.User != nil || strings.Contains(uri, "#") {
			return nil, errRPKIUpDown
		}
		if u.Scheme == "rsync" {
			if !publicationURI(uri) {
				return nil, errRPKIUpDown
			}
			if d.Method.Equal(oidRPKIRepository) {
				if !strings.HasSuffix(uri, "/") {
					return nil, errRPKIUpDown
				}
				repository = true
			}
			if d.Method.Equal(oidRPKIManifest) {
				if strings.HasSuffix(uri, "/") {
					return nil, errRPKIUpDown
				}
				manifest = true
			}
		}
	}
	if !repository || !manifest {
		return nil, errRPKIUpDown
	}
	return bytes.Clone(value), nil
}

// EE SIA references only the signed object, never a CA repository. Return the
// ordered locations so a caller can bind them to the retrieved manifest URI.
func rpkiEESIA(extensions []pkix.Extension) ([]string, error) {
	var value []byte
	for _, ext := range extensions {
		if ext.Id.Equal(oidRPKISIA) {
			if value != nil || ext.Critical || len(ext.Value) == 0 || len(ext.Value) > 512000 {
				return nil, errRPKIUpDown
			}
			value = ext.Value
		}
	}
	var descriptions []rpkiAccessDescription
	if !rpkiCSRDER(value, &descriptions) || len(descriptions) == 0 {
		return nil, errRPKIUpDown
	}
	var locations []string
	rsync := false
	for _, d := range descriptions {
		if !d.Method.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 11}) || d.Location.Class != 2 || d.Location.Tag != 6 || d.Location.IsCompound {
			return nil, errRPKIUpDown
		}
		uri := string(d.Location.Bytes)
		if !validRPKIProfileURI(uri) {
			return nil, errRPKIUpDown
		}
		u, err := url.Parse(uri)
		if err != nil || u.User != nil || strings.Contains(uri, "#") {
			return nil, errRPKIUpDown
		}
		if u.Scheme == "rsync" {
			if !publicationURI(uri) || strings.HasSuffix(uri, "/") {
				return nil, errRPKIUpDown
			}
			rsync = true
		}
		locations = append(locations, uri)
	}
	if !rsync {
		return nil, errRPKIUpDown
	}
	return locations, nil
}
