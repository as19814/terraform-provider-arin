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

func pocConfig(kind, last, extra string) string {
	first := ""
	if kind == "PERSON" {
		first = `first_name = "Example"`
	}
	return fmt.Sprintf(`provider "arin" {}
resource "arin_poc" "test" {
 contact_type = %q
 last_name = %q
 %s
 company_name = "Example Networks"
 country_code = "US"
 subdivision = "VA"
 postal_code = "20151"
 city = "Chantilly"
 street_address = ["123 Example Street"]
 %s
}
`, kind, last, first, extra)
}

const pocContactBase = `emails = ["noc@example.net"]
phones = [{type="O",number="+1-202-555-0100"}]`
const pocContactUpdated = `emails = ["abuse@example.net","noc@example.net"]
phones = [{type="F",number="+1-202-555-0101",extension="42"},{type="O",number="+1-202-555-0100",extension="123"}]
comments = ["Test operational comment"]`

func pocSteps(last string) []resource.TestStep {
	base := pocConfig("ROLE", last, pocContactBase)
	updated := pocConfig("ROLE", last, pocContactUpdated)
	person := pocConfig("PERSON", last, pocContactBase)
	return []resource.TestStep{
		{Config: base, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_poc.test", "contact_type", "ROLE"), resource.TestCheckResourceAttr("arin_poc.test", "country_code3", "USA"), resource.TestCheckResourceAttr("arin_poc.test", "country_calling_code", "1"))},
		{ResourceName: "arin_poc.test", ImportState: true, ImportStateVerify: true},
		{Config: updated, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_poc.test", "emails.#", "2"), resource.TestCheckResourceAttr("arin_poc.test", "phones.#", "2"))},
		{Config: base, Check: resource.TestCheckResourceAttr("arin_poc.test", "comments.#", "0")},
		{Config: base, PlanOnly: true},
		{Config: person, Check: resource.TestCheckResourceAttr("arin_poc.test", "contact_type", "PERSON")},
		{ResourceName: "arin_poc.test", ImportState: true, ImportStateVerify: true},
		{Config: person, PlanOnly: true},
	}
}
func TestAccPOCResourceLifecycle(t *testing.T) {
	var mu sync.Mutex
	objects := map[string]string{}
	created := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		handle := strings.TrimPrefix(r.URL.Path, "/rest/poc/")
		switch r.Method {
		case "GET":
			body, ok := objects[handle]
			if !ok {
				w.WriteHeader(404)
				return
			}
			fmt.Fprint(w, body)
		case "POST", "PUT":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			var p struct {
				Handle string `xml:"handle"`
				Date   string `xml:"registrationDate"`
			}
			if err := xml.Unmarshal(body, &p); err != nil {
				t.Error(err)
				return
			}
			if r.Method == "POST" {
				if r.URL.Path != "/rest/poc;makeLink=true" || p.Handle != "" || p.Date != "" {
					t.Error("invalid create context")
				}
				created++
				handle = fmt.Sprintf("TEST%d-ARIN", created)
				body = []byte(strings.Replace(string(body), "</poc>", "<handle>"+handle+"</handle><registrationDate>2026-01-01</registrationDate></poc>", 1))
			} else if p.Handle != handle || p.Date != "2026-01-01" {
				t.Error("invalid update identity")
			}
			body = []byte(strings.Replace(string(body), "</iso3166-1>", "<name>UNITED STATES</name><code3>USA</code3><e164>1</e164></iso3166-1>", 1))
			objects[handle] = string(body)
			fmt.Fprint(w, string(body))
		case "DELETE":
			delete(objects, handle)
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	steps := pocSteps("Terraform Test Contact")
	steps = append(steps, resource.TestStep{PreConfig: func() {
		mu.Lock()
		defer mu.Unlock()
		for k, v := range objects {
			objects[k] = strings.Replace(v, "Chantilly", "Herndon", 1)
		}
	}, Config: steps[len(steps)-1].Config})
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: steps, CheckDestroy: func(_ *terraform.State) error {
		mu.Lock()
		defer mu.Unlock()
		if len(objects) != 0 || created != 2 {
			return fmt.Errorf("unexpected POC lifecycle: remaining=%d created=%d", len(objects), created)
		}
		return nil
	}})
}
