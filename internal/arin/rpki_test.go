package arin

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testROA = `<roaSpec xmlns="http://www.arin.net/regrws/rpki/v1"><roaHandle>roa1</roaHandle><asNumber>64496</asNumber><name>Example ROA</name><notValidBefore>2026-01-01</notValidBefore><notValidAfter>2027-01-01</notValidAfter><autoRenewed>true</autoRenewed><resources><startAddress>192.0.2.0</startAddress><endAddress>192.0.2.255</endAddress><cidrLength>24</cidrLength><ipVersion>4</ipVersion><autoLinked>false</autoLinked></resources></roaSpec>`
const testASPA = `<aspa xmlns="http://www.arin.net/regrws/rpki/v1"><customerAsId>64496</customerAsId><providerAsIds><providerAsId>64498</providerAsId><providerAsId>64497</providerAsId></providerAsIds></aspa>`

func testRPKITransaction() RPKITransaction {
	return RPKITransaction{AddROAs: []ROARequest{{Name: "Example ROA", ASN: 64496, Resources: []ROAResource{{Prefix: "192.0.2.0/24"}}}}, DeleteROAs: []ROADelete{{Handle: "oldroa", AutoLink: true}}, AddASPAs: []ASPA{{CustomerASN: 64496, ProviderASNs: []int64{64497, 64498}}}, DeleteASPAs: []int64{64496}}
}
func TestRPKITransactionLifecycle(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Content-Type") != "application/xml" {
			t.Error("missing request headers")
		}
		if r.Method == "POST" {
			posts++
			body, _ := io.ReadAll(r.Body)
			var payload rpkiTransactionXML
			if err := xml.Unmarshal(body, &payload); err != nil {
				t.Error(err)
			}
			if payload.XMLName.Space != rpkiNamespace || len(payload.AddROAs.Items) != 1 || len(payload.AddASPAs.Items) != 1 || len(payload.DeleteROAs.Items) != 1 || len(payload.DeleteASPAs.Items) != 1 {
				t.Error("incomplete transaction payload")
			}
			if !payload.DeleteROAs.Items[0].AutoLink || payload.AddROAs.Items[0].AutoLink || payload.AddROAs.Items[0].Resources[0].Max != nil {
				t.Error("autolink or omitted max length changed")
			}
			fmt.Fprint(w, `<rpkiTransaction xmlns="http://www.arin.net/regrws/rpki/v1"><roaSpecDelete><roaHandle autoLink="true">oldroa</roaHandle></roaSpecDelete><roaSpecAdd>`+testROA+`</roaSpecAdd><aspaDelete><customerAsId>64496</customerAsId></aspaDelete><aspaAdd>`+testASPA+`</aspaAdd></rpkiTransaction>`)
			return
		}
		switch r.URL.Path {
		case "/rest/roa/EXAMPLE-1":
			fmt.Fprint(w, `<collection xmlns="http://www.arin.net/regrws/core/v1">`+testROA+`</collection>`)
		case "/rest/aspa/EXAMPLE-1":
			fmt.Fprint(w, `<collection xmlns="http://www.arin.net/regrws/core/v1">`+testASPA+`</collection>`)
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	out, err := c.ApplyRPKITransaction(context.Background(), "EXAMPLE-1", testRPKITransaction())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ROAs) != 1 || len(out.ASPAs) != 1 || out.ROAs[0].Resources[0].MaxLength != nil || len(out.DeletedASPAs) != 1 || posts != 1 {
		t.Fatal("transaction result lost fields")
	}
}
func TestRPKIValidation(t *testing.T) {
	low := int64(23)
	high := int64(33)
	for _, r := range []ROARequest{{Name: "Example", ASN: -1, Resources: []ROAResource{{Prefix: "192.0.2.0/24"}}}, {Name: "Example", ASN: 1, Resources: []ROAResource{{Prefix: "192.0.2.1/24"}}}, {Name: "Example", ASN: 1, Resources: []ROAResource{{Prefix: "192.0.2.0/24", MaxLength: &low}}}, {Name: "Example", ASN: 1, Resources: []ROAResource{{Prefix: "192.0.2.0/24", MaxLength: &high}}}} {
		if r.Validate() == nil {
			t.Fatal("invalid ROA accepted")
		}
	}
	for _, a := range []ASPA{{CustomerASN: 0, ProviderASNs: []int64{1}}, {CustomerASN: 1}, {CustomerASN: 1, ProviderASNs: []int64{1}}, {CustomerASN: 1, ProviderASNs: []int64{2, 2}}} {
		if a.Validate() == nil {
			t.Fatal("invalid ASPA accepted")
		}
	}
	if (ROARequest{Name: "AS0", ASN: 0, AutoLink: true, Resources: []ROAResource{{Prefix: "2001:db8::/32"}}}).Validate() == nil {
		t.Fatal("AS0 auto-link request accepted despite native silent normalization")
	}
	if (RPKITransaction{}).Validate() == nil {
		t.Fatal("empty transaction accepted")
	}
	if (ROARequest{Name: "AS0", ASN: 0, Resources: []ROAResource{{Prefix: "2001:db8::/32"}}}).Validate() != nil {
		t.Fatal("valid AS0 ROA rejected")
	}
}
func TestRPKIReadStrictness(t *testing.T) {
	root, _ := parseXML([]byte(testROA))
	roa, err := decodeROA(root)
	if err != nil || roa.Resources[0].MaxLength != nil {
		t.Fatal("ROA read failed")
	}
	for _, bad := range []string{strings.Replace(testROA, "</roaSpec>", "<unknown/></roaSpec>", 1), strings.Replace(testROA, "192.0.2.255", "192.0.3.255", 1), strings.Replace(testROA, "<ipVersion>4", "<ipVersion>6", 1), strings.Replace(testROA, "</roaSpec>", "<asNumber>2</asNumber></roaSpec>", 1)} {
		n, _ := parseXML([]byte(bad))
		if _, err := decodeROA(n); err == nil {
			t.Fatal("invalid ROA accepted")
		}
	}
	malformed := strings.Replace(testASPA, "64498", "64497", 1)
	n, _ := parseXML([]byte(malformed))
	if _, err := decodeASPA(n); err == nil {
		t.Fatal("duplicate ASPA provider accepted")
	}
}
func TestRPKIUncertainWritesAndInventoryVerification(t *testing.T) {
	for _, status := range []int{202, 403, 409, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			posts := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts++
				w.WriteHeader(status)
				fmt.Fprint(w, `<rpkiTransaction xmlns="http://www.arin.net/regrws/rpki/v1"/>`)
			}))
			defer s.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
			if _, err := c.ApplyRPKITransaction(context.Background(), "EXAMPLE-1", testRPKITransaction()); err == nil || posts != 1 {
				t.Fatal("uncertain mutation accepted or repeated")
			}
		})
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			fmt.Fprint(w, `<rpkiTransaction xmlns="http://www.arin.net/regrws/rpki/v1"><roaSpecAdd>`+testROA+`</roaSpecAdd></rpkiTransaction>`)
		} else {
			fmt.Fprint(w, `<collection xmlns="http://www.arin.net/regrws/core/v1"/>`)
		}
	}))
	defer s.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
	out, err := c.ApplyRPKITransaction(context.Background(), "EXAMPLE-1", RPKITransaction{AddROAs: testRPKITransaction().AddROAs})
	if err == nil || out == nil || len(out.ROAs) != 1 {
		t.Fatal("missing addition accepted or recovery identity lost")
	}
}

func TestRPKIWrappedROAAndASZero(t *testing.T) {
	wrapped := strings.ReplaceAll(strings.ReplaceAll(testROA, "<resources>", "<resources><roaSpecResource>"), "</resources>", "</roaSpecResource></resources>")
	wrapped = strings.Replace(wrapped, "<asNumber>", "<autoLink>false</autoLink><asNumber>", 1)
	root, _ := parseXML([]byte(wrapped))
	roa, err := decodeROA(root)
	if err != nil || roa.AutoLink == nil || *roa.AutoLink || !ROAMatchesRequest(*roa, testRPKITransaction().AddROAs[0]) {
		t.Fatalf("wrapped transaction ROA rejected: %v", err)
	}
	if (ASPA{CustomerASN: 19814, ProviderASNs: []int64{0}}).Validate() != nil {
		t.Fatal("AS0 sole provider rejected")
	}
	if (ASPA{CustomerASN: 19814, ProviderASNs: []int64{0, 13335}}).Validate() == nil {
		t.Fatal("AS0 mixed with other providers accepted")
	}
}
func TestRPKICollectionsRejectPartialInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<collection xmlns="http://www.arin.net/regrws/core/v1" truncated="true"/>`)
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	if _, err := c.ListROAs(context.Background(), "EXAMPLE-1"); err == nil {
		t.Fatal("partial ROA inventory accepted")
	}
	if _, err := c.ListASPAs(context.Background(), "EXAMPLE-1"); err == nil {
		t.Fatal("partial ASPA inventory accepted")
	}
}

func TestRPKIAdditionReturnsVerifiedInventoryMetadata(t *testing.T) {
	responseROA := strings.Replace(testROA, "<notValidBefore>2026-01-01</notValidBefore><notValidAfter>2027-01-01</notValidAfter>", "", 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			fmt.Fprint(w, `<rpkiTransaction xmlns="http://www.arin.net/regrws/rpki/v1"><roaSpecAdd>`+responseROA+`</roaSpecAdd></rpkiTransaction>`)
			return
		}
		fmt.Fprint(w, `<collection xmlns="http://www.arin.net/regrws/core/v1">`+testROA+`</collection>`)
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	result, err := c.ApplyRPKITransaction(context.Background(), "EXAMPLE-1", RPKITransaction{AddROAs: testRPKITransaction().AddROAs})
	if err != nil || result.ROAs[0].NotValidBefore != "2026-01-01" {
		t.Fatalf("generated metadata not refreshed: %v", err)
	}
}

func TestRPKIDeletionReceipts(t *testing.T) {
	for _, tc := range []struct {
		name, roas, aspas string
		reject            bool
	}{
		{"matching", `<roaHandle>oldroa</roaHandle>`, `<customerAsId>64496</customerAsId>`, false},
		{"omitted", "", "", false},
		{"foreign_roa", `<roaHandle>unrelated</roaHandle>`, `<customerAsId>64496</customerAsId>`, true},
		{"duplicate_roa", `<roaHandle>oldroa</roaHandle><roaHandle>oldroa</roaHandle>`, `<customerAsId>64496</customerAsId>`, true},
		{"foreign_aspa", `<roaHandle>oldroa</roaHandle>`, `<customerAsId>64497</customerAsId>`, true},
		{"duplicate_aspa", `<roaHandle>oldroa</roaHandle>`, `<customerAsId>64496</customerAsId><customerAsId>64496</customerAsId>`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts, gets := 0, 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					fmt.Fprintf(w, `<rpkiTransaction xmlns="%s"><roaSpecDelete>%s</roaSpecDelete><aspaDelete>%s</aspaDelete></rpkiTransaction>`, rpkiNamespace, tc.roas, tc.aspas)
					return
				}
				gets++
				fmt.Fprintf(w, `<collection xmlns="%s"/>`, registrationNamespace)
			}))
			defer s.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
			result, err := c.ApplyRPKITransaction(context.Background(), "EXAMPLE-1", RPKITransaction{DeleteROAs: []ROADelete{{Handle: "oldroa"}}, DeleteASPAs: []int64{64496}})
			if (err != nil) != tc.reject || result == nil || posts != 1 {
				t.Fatalf("unexpected deletion receipt handling: %v", err)
			}
			if !tc.reject && gets != 2 {
				t.Fatal("deletions were accepted without verifying both inventories")
			}
			if tc.reject && gets != 0 {
				t.Fatal("invalid receipt was treated as inventory-confirmable")
			}
		})
	}
}

func TestRPKIUnexpectedDeletionKeepsAdditionReceipt(t *testing.T) {
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("invalid receipt reached inventory reconciliation")
			w.WriteHeader(500)
			return
		}
		posts++
		fmt.Fprint(w, `<rpkiTransaction xmlns="http://www.arin.net/regrws/rpki/v1"><roaSpecAdd>`+testROA+`</roaSpecAdd><aspaDelete><customerAsId>64496</customerAsId></aspaDelete></rpkiTransaction>`)
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	result, err := c.ApplyRPKITransaction(context.Background(), "EXAMPLE-1", RPKITransaction{AddROAs: testRPKITransaction().AddROAs})
	if err == nil || result == nil || len(result.ROAs) != 1 || result.ROAs[0].Handle != "roa1" || len(result.DeletedASPAs) != 1 || posts != 1 {
		t.Fatal("unexpected deletion accepted or useful recovery identities lost")
	}
}
