package provider

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"sort"
	"sync"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type testROAResourceXML struct {
	Start  string `xml:"startAddress"`
	Length int    `xml:"cidrLength"`
	Max    *int64 `xml:"maxLength,omitempty"`
	Linked bool   `xml:"autoLinked"`
}
type testROAXML struct {
	XMLName   xml.Name             `xml:"http://www.arin.net/regrws/rpki/v1 roaSpec"`
	Handle    string               `xml:"roaHandle,omitempty"`
	ASN       int64                `xml:"asNumber"`
	Name      string               `xml:"name"`
	AutoLink  bool                 `xml:"autoLink"`
	Resources []testROAResourceXML `xml:"resources>roaSpecResource"`
}
type testROADeleteXML struct {
	Handle   string `xml:",chardata"`
	AutoLink bool   `xml:"autoLink,attr"`
}
type testROATransactionXML struct {
	XMLName xml.Name           `xml:"http://www.arin.net/regrws/rpki/v1 rpkiTransaction"`
	Add     []testROAXML       `xml:"roaSpecAdd>roaSpec"`
	Delete  []testROADeleteXML `xml:"roaSpecDelete>roaHandle"`
}
type roaFake struct {
	afterWriteReadStatus                                   int
	mu                                                     sync.Mutex
	objects                                                map[string]testROAXML
	posts, sequence, replacements, readStatus, writeStatus int
	applyBeforeError, ignoreDelete                         bool
}

func (f *roaFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Content-Type") != "application/xml" {
		w.WriteHeader(403)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/rest/roa/EXAMPLE-1" {
		if f.readStatus != 0 {
			w.WriteHeader(f.readStatus)
			return
		}
		fmt.Fprint(w, `<collection xmlns="http://www.arin.net/regrws/core/v1">`)
		keys := []string{}
		for k := range f.objects {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b, _ := xml.Marshal(f.objects[k])
			w.Write(b)
		}
		fmt.Fprint(w, `</collection>`)
		return
	}
	if r.Method != "POST" || r.URL.Path != "/rest/rpki/EXAMPLE-1" {
		w.WriteHeader(404)
		return
	}
	f.posts++
	if f.writeStatus != 0 && !f.applyBeforeError {
		w.WriteHeader(f.writeStatus)
		return
	}
	var txn testROATransactionXML
	if err := xml.NewDecoder(r.Body).Decode(&txn); err != nil {
		w.WriteHeader(400)
		return
	}
	if len(txn.Delete) > 0 && len(txn.Add) > 0 {
		f.replacements++
	}
	for _, d := range txn.Delete {
		if !f.ignoreDelete {
			delete(f.objects, d.Handle)
		}
	}
	for i := range txn.Add {
		f.sequence++
		a := &txn.Add[i]
		a.Handle = fmt.Sprintf("roa%d", f.sequence)
		for j := range a.Resources {
			a.Resources[j].Linked = a.AutoLink
		}
		f.objects[a.Handle] = *a
	}
	if f.writeStatus != 0 {
		w.WriteHeader(f.writeStatus)
		return
	}
	if f.afterWriteReadStatus != 0 {
		f.readStatus = f.afterWriteReadStatus
	}
	b, _ := xml.Marshal(txn)
	w.Write(b)
}
func setupROAFake(t *testing.T) (*roaFake, *arin.Client) {
	t.Helper()
	f := &roaFake{objects: map[string]testROAXML{"sibling": {Handle: "sibling", Name: "Unrelated", ASN: 64500, Resources: []testROAResourceXML{{Start: "198.51.100.0", Length: 24}}}}}
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
func roaConfig(org, name string, asn int64, prefixes map[string]int64, linked, deleteLinked bool) string {
	values := ""
	keys := []string{}
	for k := range prefixes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		values += fmt.Sprintf("%q = %d\n", k, prefixes[k])
	}
	return fmt.Sprintf("provider \"arin\" {}\nresource \"arin_roa\" \"test\" {\norg_handle = %q\nname = %q\nasn = %d\nprefixes = {\n%s}\nauto_link = %t\ndelete_linked_routes = %t\n}\n", org, name, asn, values, linked, deleteLinked)
}
func TestAccROAResourceLifecycle(t *testing.T) {
	f, _ := setupROAFake(t)
	first := roaConfig("EXAMPLE-1", "Example", 64496, map[string]int64{"192.0.2.0/24": 28, "2001:db8::/32": 48}, false, false)
	next := roaConfig("EXAMPLE-1", "Changed", 0, map[string]int64{"192.0.2.0/24": 24}, false, false)
	policy := roaConfig("EXAMPLE-1", "Changed", 0, map[string]int64{"192.0.2.0/24": 24}, false, true)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: first, Check: resource.TestCheckResourceAttr("arin_roa.test", "id", "EXAMPLE-1/roa1")},
		{ResourceName: "arin_roa.test", ImportState: true, ImportStateVerify: true},
		{Config: next, Check: resource.TestCheckResourceAttr("arin_roa.test", "handle", "roa2")},
		{Config: policy, Check: resource.TestCheckResourceAttr("arin_roa.test", "handle", "roa2")},
		{Config: policy, PlanOnly: true},
		{Config: policy, PreConfig: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			a := f.objects["roa2"]
			a.Name = "Drifted"
			f.objects["roa2"] = a
		}},
		{Config: policy, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.objects, "roa3") }},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.objects) != 1 || f.objects["sibling"].Name != "Unrelated" || f.replacements != 2 {
			return fmt.Errorf("ROA lifecycle changed siblings or missed atomic updates")
		}
		return nil
	}})
}
func testROAFromRequest(handle string, a arin.ROARequest) testROAXML {
	out := testROAXML{Handle: handle, Name: a.Name, ASN: a.ASN, AutoLink: a.AutoLink}
	for _, r := range a.Resources {
		p := netip.MustParsePrefix(r.Prefix)
		out.Resources = append(out.Resources, testROAResourceXML{Start: p.Addr().String(), Length: p.Bits(), Max: r.MaxLength, Linked: a.AutoLink})
	}
	return out
}
func TestAccROARecovery(t *testing.T) {
	f, _ := setupROAFake(t)
	f.writeStatus = 500
	first := roaConfig("EXAMPLE-1", "Example", 64496, map[string]int64{"192.0.2.0/24": 28}, false, false)
	next := roaConfig("EXAMPLE-1", "Changed", 64496, map[string]int64{"192.0.2.0/24": 28}, false, false)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: first, ExpectError: regexp.MustCompile("Could not confirm ROA transaction")},
		{Config: first, ExpectError: regexp.MustCompile("ROA transaction remains uncertain")},
		{Config: first, PreConfig: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.posts != 1 {
				t.Fatal("unresolved create retried")
			}
			length := int64(28)
			f.objects["late-roa"] = testROAFromRequest("late-roa", arin.ROARequest{Name: "Example", ASN: 64496, Resources: []arin.ROAResource{{Prefix: "192.0.2.0/24", MaxLength: &length}}})
			f.writeStatus = 0
		}},
		{Config: next, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.writeStatus = 500; f.applyBeforeError = true }, ExpectError: regexp.MustCompile("Could not confirm ROA transaction")},
		{Config: next, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.writeStatus = 0 }},
		{Config: next, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.objects) != 1 || f.posts != 5 || f.replacements != 1 {
			return fmt.Errorf("uncertain writes replayed or recovery missed cleanup: posts=%d replacements=%d remaining=%d", f.posts, f.replacements, len(f.objects))
		}
		return nil
	}})
}
func TestAccROARejectsAS0AutoLink(t *testing.T) {
	f, _ := setupROAFake(t)
	config := roaConfig("EXAMPLE-1", "AS0", 0, map[string]int64{"192.0.2.0/24": 24}, true, false)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, ExpectError: regexp.MustCompile("AS0 ROAs cannot create IRR links")},
	}})
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.posts != 0 {
		t.Fatal("invalid AS0 auto-link configuration reached the API")
	}
}
