package provider

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type contactPhoneXML struct {
	Type struct {
		Code string `xml:"code"`
	} `xml:"type"`
	Number    string `xml:"number"`
	Extension string `xml:"extension,omitempty"`
}
type contactLineXML struct {
	Number int    `xml:"number,attr"`
	Text   string `xml:",chardata"`
}
type contactPOCXML struct {
	XMLName xml.Name `xml:"http://www.arin.net/regrws/core/v1 poc"`
	Handle  string   `xml:"handle"`
	Date    string   `xml:"registrationDate"`
	Type    string   `xml:"contactType"`
	Company string   `xml:"companyName"`
	Name    string   `xml:"lastName"`
	Country struct {
		Code string `xml:"code2"`
	} `xml:"iso3166-1"`
	Subdivision string            `xml:"iso3166-2"`
	Postal      string            `xml:"postalCode"`
	Street      []contactLineXML  `xml:"streetAddress>line"`
	Emails      []string          `xml:"emails>email"`
	Phones      []contactPhoneXML `xml:"phones>phone"`
}
type contactFake struct {
	mu                      sync.Mutex
	emails                  []string
	phones                  []arin.POCPhone
	writes                  int
	readStatus, writeStatus int
	ignoreDelete            bool
}

func (f *contactFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	base := "/rest/poc/TEST-ARIN"
	if r.Method == "GET" && f.readStatus != 0 {
		w.WriteHeader(f.readStatus)
		return
	}
	if r.Method != "GET" {
		f.writes++
		if f.writeStatus != 0 {
			w.WriteHeader(f.writeStatus)
			return
		}
	}
	switch {
	case r.Method == "GET" && r.URL.Path == base:
		p := contactPOCXML{Handle: "TEST-ARIN", Date: "2026-01-01", Type: "ROLE", Company: "Example", Name: "Test NOC", Subdivision: "VA", Postal: "20151", Street: []contactLineXML{{Number: 1, Text: "123 Example Street"}}, Emails: append([]string{}, f.emails...)}
		p.Country.Code = "US"
		sort.Strings(p.Emails)
		for _, ph := range f.phones {
			x := contactPhoneXML{Number: ph.Number, Extension: ph.Extension}
			x.Type.Code = ph.Type
			p.Phones = append(p.Phones, x)
		}
		_ = xml.NewEncoder(w).Encode(p)
		return
	case strings.HasPrefix(r.URL.Path, base+"/email/"):
		e := strings.TrimPrefix(r.URL.Path, base+"/email/")
		if r.Method == "POST" {
			found := false
			for _, old := range f.emails {
				if old == e {
					found = true
				}
			}
			if !found {
				f.emails = append(f.emails, e)
			}
		} else if r.Method == "DELETE" {
			if !f.ignoreDelete {
				next := []string{}
				for _, old := range f.emails {
					if old != e {
						next = append(next, old)
					}
				}
				f.emails = next
			}
		} else {
			w.WriteHeader(405)
			return
		}
	case r.Method == "PUT" && r.URL.Path == base+"/phone":
		var ph contactPhoneXML
		if xml.NewDecoder(r.Body).Decode(&ph) != nil {
			w.WriteHeader(400)
			return
		}
		found := false
		for _, old := range f.phones {
			if old.Type == ph.Type.Code && old.Number == ph.Number {
				found = true
			}
		}
		if !found {
			f.phones = append(f.phones, arin.POCPhone{Type: ph.Type.Code, Number: ph.Number, Extension: ph.Extension})
		}
	case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, base+"/phone/"):
		number, kind, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, base+"/phone/"), ";type=")
		if !ok || number == "" || kind == "" {
			w.WriteHeader(400)
			return
		}
		if !f.ignoreDelete {
			next := []arin.POCPhone{}
			for _, old := range f.phones {
				if old.Type != kind || old.Number != number {
					next = append(next, old)
				}
			}
			f.phones = next
		}
	default:
		w.WriteHeader(404)
		return
	}
	w.WriteHeader(204)
}
func setupContactFake(t *testing.T) (*contactFake, *arin.Client) {
	t.Helper()
	f := &contactFake{emails: []string{"noc@example.net"}, phones: []arin.POCPhone{{Type: "O", Number: "+1-202-555-0100"}}}
	s := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(s.Close)
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", s.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	c, err := arin.New(arin.Config{APIKey: "test-key", BaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}
func contactConfig(handle, email, extension string) string {
	return fmt.Sprintf(`provider "arin" {}
resource "arin_poc_email" "test" {
 poc_handle = %q
 email = %q
}
resource "arin_poc_email" "sibling" {
 poc_handle = %q
 email = "sibling@example.net"
}
resource "arin_poc_phone" "test" {
 poc_handle = %q
 type = "F"
 number = "+1-202-555-0101"
 %s
}
resource "arin_poc_phone" "sibling" {
 poc_handle = %q
 type = "M"
 number = "+1-202-555-0102"
}
`, handle, email, handle, handle, extension, handle)
}
func contactSteps(handle string) []resource.TestStep {
	first := contactConfig(handle, "extra+tag@example.net", `extension = "42"`)
	changed := contactConfig(handle, "replacement@example.net", `extension = "43"`)
	cleared := contactConfig(handle, "replacement@example.net", "")
	return []resource.TestStep{
		{Config: first, Check: resource.TestCheckResourceAttr("arin_poc_phone.test", "extension", "42")},
		{ResourceName: "arin_poc_email.test", ImportState: true, ImportStateId: handle + "/extra+tag@example.net", ImportStateVerify: true},
		{ResourceName: "arin_poc_phone.test", ImportState: true, ImportStateId: handle + "/F/+1-202-555-0101", ImportStateVerify: true},
		{Config: changed, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_poc_phone.test", "extension", "43"), resource.TestCheckResourceAttr("arin_poc_email.test", "email", "replacement@example.net"))},
		{Config: cleared, Check: resource.TestCheckResourceAttr("arin_poc_phone.test", "extension", "")},
		{Config: cleared, PlanOnly: true},
	}
}
func TestAccPOCContactsLifecycle(t *testing.T) {
	f, _ := setupContactFake(t)
	steps := contactSteps("TEST-ARIN")
	steps = append(steps, resource.TestStep{PreConfig: func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		for i, p := range f.phones {
			if p.Type == "F" {
				f.phones[i].Extension = "99"
			}
		}
		next := []string{}
		for _, e := range f.emails {
			if e != "replacement@example.net" {
				next = append(next, e)
			}
		}
		f.emails = next
	}, Config: steps[len(steps)-1].Config})
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: steps, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !reflect.DeepEqual(f.emails, []string{"noc@example.net"}) || !reflect.DeepEqual(f.phones, []arin.POCPhone{{Type: "O", Number: "+1-202-555-0100"}}) {
			return fmt.Errorf("unmanaged contact records changed")
		}
		return nil
	}})
}
