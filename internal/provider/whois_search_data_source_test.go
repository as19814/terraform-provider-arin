package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccWhoisSearches(t *testing.T) {
	fixtures := map[string]string{}
	config := "provider \"arin\" {}\n"
	checks := []resource.TestCheckFunc{}
	for _, spec := range arin.WhoisSearchReads() {
		kind := strings.TrimSuffix(strings.TrimPrefix(spec.Name, "whois_"), "s")
		body, err := os.ReadFile("../arin/testdata/whois_" + kind + ".xml")
		if err != nil {
			t.Fatal(err)
		}
		handle := ""
		for _, record := range arin.WhoisRecordReads() {
			if record.Name == "whois_"+kind {
				handle = record.Inputs[0].Example
			}
		}
		for _, key := range []string{"handle", "q"} {
			for _, details := range []bool{false, true} {
				content := fmt.Sprintf(`<%sRef handle=%q name="Example"/>`, kind, handle)
				path := "/rest/" + spec.Root + ";" + key + "=" + handle
				if details {
					content = string(body)
					path += "?showDetails=true"
				}
				fixtures[path] = fmt.Sprintf(`<%s xmlns="https://www.arin.net/whoisrws/core/v1"><limitExceeded>false</limitExceeded>%s</%s>`, spec.Root, content, spec.Root)
				label := fmt.Sprintf("%s_%t", key, details)
				encoded, _ := json.Marshal(map[string]string{key: handle})
				config += fmt.Sprintf("data %q %q {\n filters=%s\n show_details=%t\n}\n", "arin_"+spec.Name, label, encoded, details)
				address := "data.arin_" + spec.Name + "." + label
				if kind == "poc" && details {
					checks = append(checks, whoisPOCMetadataChecks(address, "pocs.0.")...)
				}
				checks = append(checks, resource.TestCheckResourceAttr(address, spec.Output+".#", "1"), resource.TestCheckResourceAttr(address, spec.Output+".0.handle", handle), resource.TestCheckResourceAttrSet(address, "whois_xml"))
			}
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected authenticated request")
		}
		body, ok := fixtures[r.URL.RequestURI()]
		if !ok {
			t.Errorf("unexpected search path %s", r.URL.RequestURI())
			w.WriteHeader(400)
			return
		}
		fmt.Fprint(w, body)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "must-not-send")
	t.Setenv("ARIN_BASE_URL", "")
	t.Setenv("ARIN_WHOIS_BASE_URL", server.URL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
}

func TestAccWhoisSearchErrors(t *testing.T) {
	for _, tc := range []struct {
		name, filters, body, pattern string
		status                       int
		request                      bool
	}{
		{"empty", `{handle="NO-MATCH"}`, `<html><title>Whois-RWS</title>Sorry, there were no results.</html>`, "", 404, true},
		{"unknown_filter", `{invalid="value"}`, "", "unsupported Whois filter", 200, false},
		{"empty_map", `{}`, "", "nonempty map", 200, false},
		{"null_value", `{handle=null}`, "", "known, non-null string", 200, false},
		{"q_invalid_wildcard", `{q="*A"}`, "", "invalid Whois filter", 200, false},
		{"q_null", `{q=null}`, "", "known, non-null string", 200, false},
		{"q_partial", `{q="A*"}`, `<orgs xmlns="https://www.arin.net/whoisrws/core/v1"><limitExceeded>true</limitExceeded></orgs>`, "truncated", 200, true},
		{"invalid_wildcard", `{handle="*A"}`, "", "invalid Whois filter", 200, false},
		{"partial", `{handle="A*"}`, `<orgs xmlns="https://www.arin.net/whoisrws/core/v1"><limitExceeded>true</limitExceeded></orgs>`, "truncated", 200, true},
		{"plain_404", `{handle="NO-MATCH"}`, `Not found`, "404", 404, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !tc.request {
					t.Error("invalid input reached ARIN")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "")
			t.Setenv("ARIN_BASE_URL", "")
			t.Setenv("ARIN_WHOIS_BASE_URL", server.URL)
			step := resource.TestStep{Config: fmt.Sprintf("provider \"arin\" {}\ndata \"arin_whois_orgs\" \"test\" {filters=%s}", tc.filters)}
			if tc.pattern != "" {
				step.ExpectError = regexp.MustCompile(tc.pattern)
			} else {
				step.Check = resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_whois_orgs.test", "orgs.#", "0"), resource.TestCheckNoResourceAttr("data.arin_whois_orgs.test", "whois_xml"))
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{step}})
		})
	}
}
