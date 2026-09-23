package arin

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func testAutnum() Autnum {
	return Autnum{Name: "AS64496", ASName: "EXAMPLE-AS", OrgHandle: "EXAMPLE-1", Description: []string{"Test & peers"}, Remarks: []string{"Remark"}, MemberOf: []string{"AS-EXAMPLE"}, ImportPolicy: []string{"from AS64497 accept ANY"}, ExportPolicy: []string{"to AS64497 announce AS64496"}, DefaultPolicy: []string{"to AS64497 networks ANY"}, MPImportPolicy: []string{"afi ipv6.unicast from AS64497 accept ANY"}, MPExportPolicy: []string{"afi ipv6.unicast to AS64497 announce AS64496"}, MPDefaultPolicy: []string{"afi ipv6.unicast to AS64497 networks ANY"}}
}
func TestAutnumPayload(t *testing.T) {
	input := testAutnum()
	b, err := input.marshal()
	if err != nil {
		t.Fatal(err)
	}
	var wire autnumXML
	if err := xml.Unmarshal(b, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Name != input.Name || wire.ASName != input.ASName || len(wire.MemberOf) != 1 || wire.MPDefault.Lines[0].Text != input.MPDefaultPolicy[0] || strings.Contains(string(b), "pocLinks") {
		t.Fatal("invalid write representation")
	}
	output, err := decodeAutnum(b, input.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*output, input) {
		t.Fatalf("round-trip mismatch: %+v", output)
	}
	empty := Autnum{Name: "AS64496", ASName: "EXAMPLE-AS", OrgHandle: "EXAMPLE-1", Description: []string{"Example"}}
	b, err = empty.marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, element := range []string{"remarks", "memberOf", "import", "export", "default", "mpImport", "mpExport", "mpDefault"} {
		if strings.Contains(string(b), "<"+element) {
			t.Fatalf("empty %s must be omitted", element)
		}
	}
}
func TestAutnumValidation(t *testing.T) {
	for _, n := range []string{"AS1", "AS64496", "AS4294967295"} {
		if err := ValidateAutnumName(n); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"AS0", "AS01", "64496", "AS4294967296", "AS-64496", "AS64496/other", "as64496"} {
		if err := ValidateAutnumName(n); err == nil {
			t.Fatalf("accepted %s", n)
		}
	}
	for _, change := range []func(*Autnum){func(r *Autnum) { r.MemberOf = []string{"RS-BAD"} }, func(r *Autnum) { r.ImportPolicy = []string{"one\ntwo"} }, func(r *Autnum) { r.ASName = "" }} {
		r := testAutnum()
		change(&r)
		if err := r.Validate(); err == nil {
			t.Fatal("accepted invalid aut-num")
		}
	}
	for _, payload := range []string{`<autnum/>`, `<autnum xmlns="http://www.arin.net/regrws/core/v1"><asNumber>AS64497</asNumber><asName>EXAMPLE</asName><orgHandle>EXAMPLE-1</orgHandle><source>ARIN</source></autnum>`, `<autnum xmlns="http://www.arin.net/regrws/core/v1"><asNumber>AS64496</asNumber><orgHandle>EXAMPLE-1</orgHandle><source>ARIN</source></autnum>`} {
		if _, err := decodeAutnum([]byte(payload), "AS64496"); err == nil {
			t.Fatal("accepted invalid response")
		}
	}
}
func TestAutnumWriteFailure(t *testing.T) {
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		for _, status := range []int{202, 307, 403, 409, 429, 500} {
			t.Run(fmt.Sprintf("%s/%d", method, status), func(t *testing.T) {
				writes := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == "GET" {
						w.WriteHeader(404)
						return
					}
					writes++
					if r.Method != method || r.URL.Path != "/rest/irr/aut-num/AS64496" || r.URL.RawQuery != "" {
						t.Errorf("unexpected mutation request")
					}
					if r.Header.Get("Authorization") != "ApiKey test-secret" {
						t.Error("incorrect authentication")
					}
					w.Header().Set("Location", "/redirect")
					w.WriteHeader(status)
					fmt.Fprint(w, `<error><message>test-secret failure</message></error>`)
				}))
				defer server.Close()
				c, _ := New(Config{APIKey: "test-secret", BaseURL: server.URL})
				var err error
				switch method {
				case "POST":
					_, err = c.CreateAutnum(context.Background(), testAutnum())
				case "PUT":
					_, err = c.UpdateAutnum(context.Background(), testAutnum())
				case "DELETE":
					err = c.DeleteAutnum(context.Background(), "AS64496")
				}
				if err == nil || strings.Contains(err.Error(), "test-secret") || writes != 1 {
					t.Fatalf("err=%v writes=%d", err, writes)
				}
			})
		}
	}
}
