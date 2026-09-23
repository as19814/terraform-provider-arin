package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccWhoisLookups(t *testing.T) {
	fixtures := map[string]string{}
	for _, kind := range []string{"org", "customer", "poc", "asn", "net", "delegation"} {
		body, err := os.ReadFile("../arin/testdata/whois_" + kind + ".xml")
		if err != nil {
			t.Fatal(err)
		}
		fixtures[kind] = string(body)
	}
	var changed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected authenticated Whois request")
		}
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) != 4 || parts[1] != "rest" {
			t.Error("unexpected lookup path")
			w.WriteHeader(400)
			return
		}
		kind := parts[2]
		if kind == "rdns" {
			kind = "delegation"
		}
		body, ok := fixtures[kind]
		if !ok {
			t.Error("unexpected record type")
			w.WriteHeader(400)
			return
		}
		if changed.Load() {
			body = strings.Replace(body, "Example</name>", "Updated Example</name>", 1)
		}
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_WHOIS_BASE_URL", server.URL)
	config := "provider \"arin\" {}\n"
	checks := []resource.TestCheckFunc{}
	for _, spec := range arin.WhoisRecordReads() {
		in := spec.Inputs[0]
		config += fmt.Sprintf("data %q \"test\" {\n %s=%q\n show_details=true\n}\n", "arin_"+spec.Name, in.Name, in.Example)
		checks = append(checks, resource.TestCheckResourceAttrSet("data.arin_"+spec.Name+".test", "whois_xml"))
	}
	checks = append(checks, whoisPOCMetadataChecks("data.arin_whois_poc.test", "")...)
	checks = append(checks, resource.TestCheckResourceAttr("data.arin_whois_org.test", "street_address.0", "First"), resource.TestCheckResourceAttr("data.arin_whois_net.test", "ip_version", "v4"), resource.TestCheckResourceAttr("data.arin_whois_net.test", "net_blocks.0.cidr_length", "24"), resource.TestCheckResourceAttr("data.arin_whois_poc.test", "phones.0.type", "O"), resource.TestCheckResourceAttr("data.arin_whois_asn.test", "start_asn", "64496"), resource.TestCheckResourceAttr("data.arin_whois_delegation.test", "ds_records.0.key_tag", "12345"))
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PreConfig: func() { changed.Store(true) }, Check: resource.TestCheckResourceAttr("data.arin_whois_org.test", "name", "Updated Example")}, {Config: config, PlanOnly: true}}})
}
func TestAccWhoisErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, pattern string
		status              int
	}{
		{"missing", `<error/>`, "HTTP 404", 404},
		{"mismatch", `<org xmlns="https://www.arin.net/whoisrws/core/v1"><handle>WRONG</handle></org>`, "mismatched Whois", 200},
		{"partial", `<org xmlns="https://www.arin.net/whoisrws/core/v1"><handle>EXAMPLE-1</handle><resources><limitExceeded>true</limitExceeded></resources></org>`, "truncated", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "")
			t.Setenv("ARIN_BASE_URL", "")
			t.Setenv("ARIN_WHOIS_BASE_URL", "")
			config := fmt.Sprintf("provider \"arin\" { whois_base_url=%q }\ndata \"arin_whois_org\" \"test\" { handle=\"EXAMPLE-1\" }", server.URL)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, ExpectError: regexp.MustCompile(tc.pattern)}}})
		})
	}
}

func whoisPOCMetadataChecks(address, prefix string) []resource.TestCheckFunc {
	return []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(address, prefix+"poc_type_description", "Person"),
		resource.TestCheckResourceAttr(address, prefix+"status_description", "Validated"),
		resource.TestCheckResourceAttr(address, prefix+"phones.0.description", "Office"),
	}
}
