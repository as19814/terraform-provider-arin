package provider

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type bundleTransactionXML struct {
	XMLName     xml.Name           `xml:"http://www.arin.net/regrws/rpki/v1 rpkiTransaction"`
	ROAs        []testROAXML       `xml:"roaSpecAdd>roaSpec"`
	DeleteROAs  []testROADeleteXML `xml:"roaSpecDelete>roaHandle"`
	ASPAs       []testASPAXML      `xml:"aspaAdd>aspa"`
	DeleteASPAs []int64            `xml:"aspaDelete>customerAsId"`
}
type bundleFake struct {
	afterWriteReadStatus             int
	malformed, partial, ignoreDelete bool
	mu                               sync.Mutex
	roas                             map[string]testROAXML
	aspas                            map[int64]testASPAXML
	posts, sequence, combined        int
	writeStatus, readStatus          int
	applyBeforeError                 bool
}

func (f *bundleFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "ApiKey test-key" {
		w.WriteHeader(403)
		return
	}
	if r.Method == "GET" {
		if f.readStatus != 0 {
			w.WriteHeader(f.readStatus)
			return
		}
		if r.URL.Path != "/rest/roa/EXAMPLE-1" && r.URL.Path != "/rest/aspa/EXAMPLE-1" {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, `<collection xmlns="http://www.arin.net/regrws/core/v1">`)
		if r.URL.Path == "/rest/roa/EXAMPLE-1" {
			for _, a := range f.roas {
				b, _ := xml.Marshal(a)
				w.Write(b)
			}
		} else {
			for _, a := range f.aspas {
				b, _ := xml.Marshal(a)
				w.Write(b)
			}
		}
		fmt.Fprint(w, `</collection>`)
		return
	}
	if r.Method != "POST" || r.URL.Path != "/rest/rpki/EXAMPLE-1" {
		w.WriteHeader(405)
		return
	}
	f.posts++
	if f.writeStatus != 0 && !f.applyBeforeError {
		w.WriteHeader(f.writeStatus)
		return
	}
	var tx bundleTransactionXML
	if xml.NewDecoder(r.Body).Decode(&tx) != nil {
		w.WriteHeader(400)
		return
	}
	if len(tx.ROAs)+len(tx.DeleteROAs) > 0 && len(tx.ASPAs)+len(tx.DeleteASPAs) > 0 {
		f.combined++
	}
	for _, d := range tx.DeleteROAs {
		if !f.ignoreDelete {
			delete(f.roas, d.Handle)
		}
	}
	for _, customer := range tx.DeleteASPAs {
		if !f.ignoreDelete {
			delete(f.aspas, customer)
		}
	}
	for i := range tx.ROAs {
		f.sequence++
		a := &tx.ROAs[i]
		a.Handle = fmt.Sprintf("roa%d", f.sequence)
		for j := range a.Resources {
			a.Resources[j].Linked = a.AutoLink
		}
		f.roas[a.Handle] = *a
	}
	for _, a := range tx.ASPAs {
		f.aspas[a.Customer] = a
	}
	if f.writeStatus != 0 {
		w.WriteHeader(f.writeStatus)
		return
	}
	if f.afterWriteReadStatus != 0 {
		f.readStatus = f.afterWriteReadStatus
	}
	if f.malformed {
		fmt.Fprint(w, "<invalid>")
		return
	}
	if f.partial {
		tx.ASPAs = nil
	}
	b, _ := xml.Marshal(tx)
	w.Write(b)
}
func setupBundleFake(t *testing.T) (*bundleFake, *arin.Client) {
	t.Helper()
	f := &bundleFake{roas: map[string]testROAXML{"sibling": {Handle: "sibling", Name: "Unrelated", ASN: 64500, Resources: []testROAResourceXML{{Start: "198.51.100.0", Length: 24}}}}, aspas: map[int64]testASPAXML{64500: {Customer: 64500, Providers: []int64{64501}}}}
	server := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(server.Close)
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	c, err := arin.New(arin.Config{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}
func bundleConfig(name string, provider int, policy, second bool) string {
	extraROA, extraASPA := "", ""
	if second {
		extraROA = `v6 = { name = "Second", asn = 64497, prefixes = { "2001:db8::/48" = 64 } }`
		extraASPA = `"64497" = [64511]`
	}
	return fmt.Sprintf(`provider "arin" {}
resource "arin_rpki_bundle" "test" {
 org_handle = "EXAMPLE-1"
 name = "test"
 roas = {
  v4 = { name = %q, asn = 64496, prefixes = { "192.0.2.0/24" = 24 }, delete_linked_routes = %t }
  %s
 }
 aspas = { "64496" = [%d], %s }
}
`, name, policy, extraROA, provider, extraASPA)
}
func TestAccRPKIBundleLifecycle(t *testing.T) {
	f, _ := setupBundleFake(t)
	first := bundleConfig("First", 64510, false, true)
	changed := bundleConfig("Changed", 64512, false, true)
	policy := bundleConfig("Changed", 64512, true, true)
	reduced := bundleConfig("Changed", 64512, true, false)
	posts := func(want int) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.posts != want {
				return fmt.Errorf("got %d POSTs, want %d", f.posts, want)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: first, Check: resource.ComposeTestCheckFunc(posts(1), resource.TestCheckResourceAttr("arin_rpki_bundle.test", "roas.v4.handle", "roa1"), resource.TestCheckResourceAttr("arin_rpki_bundle.test", "roas.v6.handle", "roa2"))},
		{ResourceName: "arin_rpki_bundle.test", ImportState: true, ImportStateVerify: true, ImportStateId: `{"org_handle":"EXAMPLE-1","name":"test","roas":{"v4":"roa1","v6":"roa2"},"aspas":[64496,64497]}`},
		{Config: changed, Check: resource.ComposeTestCheckFunc(posts(2), resource.TestCheckResourceAttr("arin_rpki_bundle.test", "roas.v4.handle", "roa3"), resource.TestCheckResourceAttr("arin_rpki_bundle.test", "roas.v6.handle", "roa2"))},
		{Config: policy, Check: posts(2)},
		{Config: policy, PlanOnly: true},
		{Config: reduced, Check: posts(3)},
		{Config: reduced, PreConfig: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			a := f.roas["roa3"]
			a.Name = "Drifted"
			f.roas["roa3"] = a
			f.aspas[64496] = testASPAXML{Customer: 64496, Providers: []int64{64513}}
		}, Check: posts(4)},
		{Config: reduced, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.roas, "roa4"); delete(f.aspas, 64496) }, Check: posts(5)},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.roas) != 1 || f.roas["sibling"].Name != "Unrelated" || len(f.aspas) != 1 || f.aspas[64500].Customer != 64500 || f.posts != 6 || f.combined != 6 {
			return fmt.Errorf("bundle lifecycle changed siblings or missed atomic writes: posts=%d combined=%d", f.posts, f.combined)
		}
		return nil
	}})
}

func TestAccRPKIBundleSingleCategories(t *testing.T) {
	f, _ := setupBundleFake(t)
	roaOnly := `provider "arin" {}
resource "arin_rpki_bundle" "test" {
 org_handle = "EXAMPLE-1"
 name = "single"
 roas = { v4 = { name = "AS0", asn = 0, prefixes = { "192.0.2.0/24" = 28 } } }
 aspas = {}
}`
	aspaOnly := `provider "arin" {}
resource "arin_rpki_bundle" "test" {
 org_handle = "EXAMPLE-1"
 name = "single"
 roas = {}
 aspas = { "64496" = [0] }
}`
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: roaOnly}, {Config: roaOnly, PlanOnly: true}, {Config: aspaOnly}, {Config: aspaOnly, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.roas) != 1 || len(f.aspas) != 1 || f.posts != 3 || f.combined != 1 {
			return fmt.Errorf("single-category lifecycle changed siblings or missed transaction")
		}
		return nil
	}})
}

func TestAccRPKIBundleRecovery(t *testing.T) {
	f, _ := setupBundleFake(t)
	f.writeStatus = 500
	first := bundleConfig("First", 64510, false, false)
	next := bundleConfig("Changed", 64511, false, false)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: first, ExpectError: regexp.MustCompile("Could not confirm bundle transaction")},
		{Config: first, PreConfig: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			max := int64(24)
			f.roas["late"] = testROAFromRequest("late", arin.ROARequest{Name: "First", ASN: 64496, Resources: []arin.ROAResource{{Prefix: "192.0.2.0/24", MaxLength: &max}}})
		}, ExpectError: regexp.MustCompile("Bundle recovery remains pending")},
		{Config: first, PreConfig: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.posts != 1 {
				t.Fatal("partial visibility replayed create")
			}
			f.aspas[64496] = testASPAXML{Customer: 64496, Providers: []int64{64510}}
			f.writeStatus = 0
		}},
		{Config: next, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.writeStatus = 500; f.applyBeforeError = true }, ExpectError: regexp.MustCompile("Could not confirm bundle transaction")},
		{Config: next, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.writeStatus = 0 }},
		{Config: next, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.roas) != 1 || len(f.aspas) != 1 || f.posts != 5 {
			return fmt.Errorf("pending transaction replayed or leaked members: posts=%d", f.posts)
		}
		return nil
	}})
}
func TestAccRPKIBundleValidation(t *testing.T) {
	cases := map[string]string{
		"empty": `provider "arin" {}
resource "arin_rpki_bundle" "test" {
 org_handle = "EXAMPLE-1"
 name = "test"
 roas = {}
 aspas = {}
}`,
		"as0-link":     strings.Replace(strings.Replace(bundleConfig("First", 64510, false, false), "asn = 64496", "asn = 0, auto_link = true", 1), "[64510]", "[0]", 1),
		"customer-key": strings.Replace(bundleConfig("First", 64510, false, false), `"64496" =`, `"064496" =`, 1),
		"provider":     bundleConfig("First", 64496, false, false),
	}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			f, _ := setupBundleFake(t)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, ExpectError: regexp.MustCompile("Empty RPKI bundle|Invalid RPKI bundle|Invalid ASPA customer key")}}})
			if f.posts != 0 {
				t.Fatal("invalid configuration sent a write")
			}
		})
	}
}
