package provider

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccCustomerResourceLifecycle(t *testing.T) {
	var mu sync.Mutex
	objects := map[string]string{}
	writes := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "ApiKey acceptance-test-key" {
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		handle := strings.TrimPrefix(r.URL.Path, "/rest/customer/")
		switch r.Method {
		case "GET":
			if b, ok := objects[handle]; ok {
				fmt.Fprint(w, b)
			} else {
				w.WriteHeader(404)
			}
		case "POST", "PUT":
			b, _ := io.ReadAll(r.Body)
			var p struct {
				Handle string `xml:"handle"`
				Date   string `xml:"registrationDate"`
				Org    string `xml:"parentOrgHandle"`
			}
			if xml.Unmarshal(b, &p) != nil {
				w.WriteHeader(400)
				return
			}
			if r.Method == "POST" {
				if !strings.HasPrefix(r.URL.Path, "/rest/net/NET-") || !strings.HasSuffix(r.URL.Path, "/customer") || p.Handle != "" || p.Date != "" || p.Org != "" {
					http.Error(w, "invalid create identity", 400)
					return
				}
				handle = fmt.Sprintf("C%d", writes["POST"]+1)
				b = []byte(strings.Replace(string(b), "</customer>", "<handle>"+handle+"</handle><registrationDate>2026-01-01T00:00:00Z</registrationDate><parentOrgHandle>EXAMPLE-1</parentOrgHandle></customer>", 1))
			} else if p.Handle != handle || p.Date != "2026-01-01T00:00:00Z" || p.Org != "EXAMPLE-1" {
				http.Error(w, "update lost server identity", 400)
				return
			}
			metadata := "<name>UNITED STATES</name><code3>USA</code3><e164>1</e164>"
			if strings.Contains(string(b), "<code2>GB</code2>") {
				metadata = "<name>UNITED KINGDOM</name><code3>GBR</code3><e164>44</e164>"
			}
			b = []byte(strings.Replace(string(b), "</iso3166-1>", metadata+"</iso3166-1>", 1))
			writes[r.Method]++
			objects[handle] = string(b)
			fmt.Fprint(w, string(b))
		case "DELETE":
			writes["DELETE"]++
			delete(objects, handle)
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	config := func(parent, extra string) string {
		return fmt.Sprintf(`provider "arin" {}
resource "arin_customer" "test" {
 parent_net_handle = %q
 name = "Example & customer"
 country_code = "US"
 subdivision = "VA"
 postal_code = "20151"
 street_address = ["123 Test Street","Suite 2"]
 %s
}`, parent, extra)
	}
	base := config("NET-192-0-2-0-1", `comments = ["Operational comment"]
private_customer = true`)
	empty := config("NET-192-0-2-0-1", "")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(_ *terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if len(objects) != 0 || writes["POST"] != 3 || writes["PUT"] != 3 || writes["DELETE"] != 2 {
				return fmt.Errorf("unexpected lifecycle: objects=%d writes=%v", len(objects), writes)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: base, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_customer.test", "id", "C1"), resource.TestCheckResourceAttr("arin_customer.test", "country_code3", "USA"), resource.TestCheckResourceAttr("arin_customer.test", "country_calling_code", "1"))},
			{ResourceName: "arin_customer.test", ImportState: true, ImportStateId: "NET-192-0-2-0-1/C1", ImportStateVerify: true},
			{PreConfig: func() {
				mu.Lock()
				defer mu.Unlock()
				objects["C1"] = strings.Replace(objects["C1"], "Suite 2", "Suite 3", 1)
			}, Config: base},
			{Config: empty, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_customer.test", "comments.#", "0"), resource.TestCheckResourceAttr("arin_customer.test", "private_customer", "false"))},
			{Config: strings.Replace(empty, `country_code = "US"`, `country_code = "GB"`, 1), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_customer.test", "country_code3", "GBR"), resource.TestCheckResourceAttr("arin_customer.test", "country_calling_code", "44"))},
			{Config: strings.Replace(empty, `country_code = "US"`, `country_code = "GB"`, 1), PlanOnly: true, ExpectNonEmptyPlan: false},
			{PreConfig: func() { mu.Lock(); defer mu.Unlock(); delete(objects, "C1") }, Config: empty, Check: resource.TestCheckResourceAttr("arin_customer.test", "id", "C2")},
			{Config: config("NET-198-51-100-0-1", ""), Check: resource.TestCheckResourceAttr("arin_customer.test", "id", "C3")},
		},
	})
}
