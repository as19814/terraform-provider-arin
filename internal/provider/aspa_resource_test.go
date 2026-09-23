package provider

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type testASPAXML struct {
	XMLName   xml.Name `xml:"http://www.arin.net/regrws/rpki/v1 aspa"`
	Customer  int64    `xml:"customerAsId"`
	Providers []int64  `xml:"providerAsIds>providerAsId"`
}
type testASPATransactionXML struct {
	XMLName xml.Name      `xml:"http://www.arin.net/regrws/rpki/v1 rpkiTransaction"`
	Delete  []int64       `xml:"aspaDelete>customerAsId"`
	Add     []testASPAXML `xml:"aspaAdd>aspa"`
}
type aspaFake struct {
	afterWriteReadStatus                         int
	mu                                           sync.Mutex
	objects                                      map[int64][]int64
	readStatus, writeStatus, posts, replacements int
	ignoreDelete, applyBeforeError               bool
}

func (f *aspaFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Content-Type") != "application/xml" {
		w.WriteHeader(403)
		return
	}
	if r.Method == "GET" {
		if f.readStatus != 0 {
			w.WriteHeader(f.readStatus)
			return
		}
		fmt.Fprint(w, `<collection xmlns="http://www.arin.net/regrws/core/v1">`)
		keys := []int64{}
		for asn := range f.objects {
			keys = append(keys, asn)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, asn := range keys {
			b, _ := xml.Marshal(testASPAXML{Customer: asn, Providers: f.objects[asn]})
			w.Write(b)
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
	body, _ := io.ReadAll(r.Body)
	var p testASPATransactionXML
	if xml.Unmarshal(body, &p) != nil {
		w.WriteHeader(400)
		return
	}
	if len(p.Delete) > 0 && len(p.Add) > 0 {
		f.replacements++
	}
	for _, asn := range p.Delete {
		if !f.ignoreDelete {
			delete(f.objects, asn)
		}
	}
	for _, a := range p.Add {
		f.objects[a.Customer] = a.Providers
	}
	if f.writeStatus != 0 {
		w.WriteHeader(f.writeStatus)
		return
	}
	if f.afterWriteReadStatus != 0 {
		f.readStatus = f.afterWriteReadStatus
	}
	b, _ := xml.Marshal(p)
	w.Write(b)
}
func setupASPAFake(t *testing.T) (*aspaFake, *arin.Client) {
	t.Helper()
	f := &aspaFake{objects: map[int64][]int64{64500: {64501}}}
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
func aspaConfig(org string, customer int64, providers []int64) string {
	values := ""
	for _, asn := range providers {
		values += fmt.Sprintf("%d,", asn)
	}
	return fmt.Sprintf("provider \"arin\" {}\nresource \"arin_aspa\" \"test\" {\n org_handle = %q\n customer_asn = %d\n provider_asns = [%s]\n}\n", org, customer, values)
}
func TestAccASPAResourceLifecycle(t *testing.T) {
	f, _ := setupASPAFake(t)
	first := aspaConfig("EXAMPLE-1", 64496, []int64{64498, 64497})
	next := aspaConfig("EXAMPLE-1", 64496, []int64{64499})
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: first, Check: resource.TestCheckResourceAttr("arin_aspa.test", "id", "EXAMPLE-1/64496")},
		{ResourceName: "arin_aspa.test", ImportState: true, ImportStateVerify: true},
		{Config: next, Check: resource.TestCheckResourceAttr("arin_aspa.test", "provider_asns.#", "1")},
		{Config: next, PlanOnly: true},
		{Config: next, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.objects[64496] = []int64{64497} }},
		{Config: next, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.objects, 64496) }},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !reflect.DeepEqual(f.objects, map[int64][]int64{64500: {64501}}) || f.replacements != 2 {
			return fmt.Errorf("ASPA lifecycle changed siblings or missed atomic updates")
		}
		return nil
	}})
}
