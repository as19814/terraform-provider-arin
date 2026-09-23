package arin

import (
	"strings"
	"testing"
)

func TestCountryResponseMetadata(t *testing.T) {
	metadata := `<name>UNITED STATES</name><code3>USA</code3><e164>1</e164>`
	customer := testCustomer()
	customer.CountryCode3 = "STALE"
	customer.CountryCallingCode = "99"
	org := exampleOrganization()
	org.CountryCode3 = "STALE"
	org.CountryCallingCode = "99"
	poc := examplePOC()
	poc.Handle = "EXAMPLE-ARIN"
	poc.RegistrationDate = "2026-01-01T00:00:00Z"
	poc.CountryCode3 = "STALE"
	poc.CountryCallingCode = "99"
	for _, tc := range []struct {
		name    string
		marshal func() ([]byte, error)
		decode  func([]byte) (string, string, error)
	}{
		{"customer", customer.marshal, func(b []byte) (string, string, error) {
			c, e := decodeCustomer(b, customer.Handle)
			if e != nil {
				return "", "", e
			}
			return c.CountryCode3, c.CountryCallingCode, nil
		}},
		{"org", org.marshal, func(b []byte) (string, string, error) {
			c, e := decodeRegisteredOrganization(b, org.Handle)
			if e != nil {
				return "", "", e
			}
			return c.CountryCode3, c.CountryCallingCode, nil
		}},
		{"poc", poc.marshal, func(b []byte) (string, string, error) {
			c, e := decodePOC(b, poc.Handle)
			if e != nil {
				return "", "", e
			}
			return c.CountryCode3, c.CountryCallingCode, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, e := tc.marshal()
			if e != nil {
				t.Fatal(e)
			}
			if strings.Contains(string(b), "code3") || strings.Contains(string(b), "e164") {
				t.Fatal("response metadata was sent as writable country configuration")
			}
			b = []byte(strings.Replace(string(b), "</iso3166-1>", metadata+"</iso3166-1>", 1))
			code, calling, e := tc.decode(b)
			if e != nil {
				t.Fatal(e)
			}
			if code != "USA" || calling != "1" {
				t.Fatal("lost computed country metadata")
			}
		})
	}
	for _, kind := range []string{"org", "poc", "customer"} {
		spec := whoisRecordSpec(kind)
		root, e := parseXML([]byte(whoisFixture(kind, strings.Replace(whoisFixtures()[kind], "</iso3166-1>", metadata+"</iso3166-1>", 1))))
		if e != nil {
			t.Fatal(e)
		}
		if e = normalizeWhoisTree(root); e != nil {
			t.Fatal(e)
		}
		record, e := decodeWhoisRecord(root, spec)
		if e != nil {
			t.Fatal(e)
		}
		// Some base fixtures omit the entire country; add it to exercise projection.
		if record["country_code3"] == nil {
			root, e = parseXML([]byte(whoisFixture(kind, whoisFixtures()[kind]+"<iso3166-1>"+metadata+"</iso3166-1>")))
			if e != nil {
				t.Fatal(e)
			}
			if e = normalizeWhoisTree(root); e != nil {
				t.Fatal(e)
			}
			record, e = decodeWhoisRecord(root, spec)
			if e != nil {
				t.Fatal(e)
			}
		}
		if record["country_code3"] != "USA" || record["country_calling_code"] != "1" {
			t.Fatal("lost Whois country metadata")
		}
	}
}
