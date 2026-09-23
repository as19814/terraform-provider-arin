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

func customerNetGraphConfig(parent, prefix, name string, generation int, updated bool) string {
	street, comments := `["123 Test Street"]`, `["Disposable sandbox assignment"]`
	if updated {
		street = `["456 Test Street", "Suite 2"]`
		comments = `[]`
	}
	return fmt.Sprintf(`provider "arin" {}
resource "terraform_data" "generation" { input = %d }
resource "arin_customer" "recipient" {
 parent_net_handle = %q
 name = %q
 country_code = "US"
 city = "Chantilly"
 subdivision = "VA"
 postal_code = "20151"
 street_address = %s
 private_customer = true
 lifecycle { replace_triggered_by = [terraform_data.generation] }
}
resource "arin_net" "assignment" {
 parent_net_handle = arin_customer.recipient.parent_net_handle
 customer_handle = arin_customer.recipient.id
 name = %q
 prefixes = [%q]
 comments = %s
}
`, generation, parent, name, street, name, prefix, comments)
}

func TestAccCustomerNetGraph(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reuse_handle_%t", reuse), func(t *testing.T) { testAccCustomerNetGraph(t, reuse) })
	}
}
func testAccCustomerNetGraph(t *testing.T, reuse bool) {
	var mu sync.Mutex
	customers := map[string]string{}
	f := &fakeNetAPI{reuseHandle: reuse, objects: map[string]string{}, writes: map[string]int{}}
	created, deleted, updated := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "ApiKey acceptance-test-key" {
			w.WriteHeader(403)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/rest/customer/") || strings.HasSuffix(r.URL.Path, "/customer") {
			handle := strings.TrimPrefix(r.URL.Path, "/rest/customer/")
			switch r.Method {
			case "POST", "PUT":
				body, _ := io.ReadAll(r.Body)
				if r.Method == "POST" {
					created++
					handle = fmt.Sprintf("C%d", created)
					body = []byte(strings.Replace(string(body), "</customer>", "<handle>"+handle+"</handle><parentOrgHandle>EXAMPLE-1</parentOrgHandle><registrationDate>2026-01-01T00:00:00Z</registrationDate></customer>", 1))
				} else {
					updated++
				}
				customers[handle] = string(body)
				fmt.Fprint(w, string(body))
			case "GET":
				if body, ok := customers[handle]; ok {
					fmt.Fprint(w, body)
				} else {
					w.WriteHeader(404)
				}
			case "DELETE":
				f.mu.Lock()
				inUse := false
				for _, body := range f.objects {
					if strings.Contains(body, "<customerHandle>"+handle+"</customerHandle>") {
						inUse = true
					}
				}
				f.mu.Unlock()
				if inUse {
					t.Error("Terraform deleted customer before dependent NET")
					http.Error(w, "customer in use", 409)
					return
				}
				delete(customers, handle)
				deleted++
				w.WriteHeader(204)
			default:
				w.WriteHeader(405)
			}
			return
		}
		if r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/reassign") {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			var n struct {
				Customer string `xml:"customerHandle"`
			}
			if xml.Unmarshal(body, &n) != nil || customers[n.Customer] == "" {
				t.Error("NET created without its customer")
				http.Error(w, "missing customer", 409)
				return
			}
		}
		f.serve(w, r)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	config := func(generation int, updated bool) string {
		return customerNetGraphConfig("NET-192-0-2-0-1", "192.0.2.0/29", "EXAMPLE-GRAPH", generation, updated)
	}
	check := func(customer, net string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_customer.recipient", "id", customer), resource.TestCheckResourceAttr("arin_net.assignment", "id", net), resource.TestCheckResourceAttrPair("arin_net.assignment", "customer_handle", "arin_customer.recipient", "id"))
	}
	replacementHandle := "NET-192-0-2-0-3"
	if reuse {
		replacementHandle = "NET-192-0-2-0-2"
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, CheckDestroy: func(_ *terraform.State) error {
		mu.Lock()
		defer mu.Unlock()
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(customers) != 0 || len(f.objects) != 0 || created != 2 || deleted != 2 || updated != 1 || f.writes["create"] != 2 || f.writes["delete"] != 2 {
			return fmt.Errorf("incomplete graph lifecycle: customers=%d nets=%d customer writes=%d/%d/%d net writes=%v", len(customers), len(f.objects), created, updated, deleted, f.writes)
		}
		return nil
	}, Steps: []resource.TestStep{
		{Config: config(1, false), Check: check("C1", "NET-192-0-2-0-2")},
		{Config: config(1, true), Check: resource.ComposeAggregateTestCheckFunc(check("C1", "NET-192-0-2-0-2"), resource.TestCheckResourceAttr("arin_customer.recipient", "street_address.#", "2"), resource.TestCheckResourceAttr("arin_net.assignment", "comments.#", "0"))},
		{ResourceName: "arin_customer.recipient", ImportState: true, ImportStateId: "NET-192-0-2-0-1/C1", ImportStateVerify: true},
		{ResourceName: "arin_net.assignment", ImportState: true, ImportStateVerify: true},
		{Config: config(1, true), PlanOnly: true},
		{Config: config(2, true), Check: check("C2", replacementHandle)},
		{Config: config(2, true), PlanOnly: true},
	}})
}
