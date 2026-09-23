package provider

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func nameserverConfig(zone, name, ttl string) string {
	return fmt.Sprintf(`provider "arin" {}
resource "arin_delegation_nameserver" "test" {
 delegation = %q
 name = %q
 %s
}
resource "arin_delegation_nameserver" "sibling" {
 delegation = %q
 name = "tf-sibling.example.net"
 ttl = 7200
}
`, zone, name, ttl, zone)
}
func nameserverSteps(zone string) []resource.TestStep {
	config := nameserverConfig(zone, "tf-ns1.example.net", "ttl = 3600")
	return []resource.TestStep{
		{Config: config, Check: resource.TestCheckResourceAttr("arin_delegation_nameserver.test", "ttl", "3600")},
		{ResourceName: "arin_delegation_nameserver.test", ImportState: true, ImportStateId: zone + "/tf-ns1.example.net", ImportStateVerify: true},
		{Config: nameserverConfig(zone, "tf-ns1.example.net", "ttl = 7200"), Check: resource.TestCheckResourceAttr("arin_delegation_nameserver.test", "ttl", "7200")},
		{Config: nameserverConfig(zone, "tf-ns1.example.net", ""), Check: resource.TestCheckNoResourceAttr("arin_delegation_nameserver.test", "ttl")},
		{Config: nameserverConfig(zone, "tf-ns1.example.net", ""), PlanOnly: true},
		{Config: nameserverConfig(zone, "tf-ns2.example.net", "ttl = 1800"), Check: resource.TestCheckResourceAttr("arin_delegation_nameserver.test", "id", zone+"/tf-ns2.example.net")},
	}
}
func TestAccDelegationNameserverLifecycle(t *testing.T) {
	var mu sync.Mutex
	zone := "2.0.192.in-addr.arpa."
	original := fakeDelegation{Name: zone, NS: []arin.DelegationNameserver{{Name: "retained.example.net"}}, DS: []arin.DelegationDS{{Algorithm: 13, DigestType: 2, KeyTag: 12345, Digest: strings.Repeat("AB", 32)}}}
	current := original
	current.NS = append([]arin.DelegationNameserver{}, original.NS...)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		base := "/rest/delegation/" + zone
		if r.URL.Path != base && !strings.HasPrefix(r.URL.Path, base+"/nameserver/") {
			w.WriteHeader(404)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, base+"/nameserver/")
		switch r.Method {
		case "GET":
		case "POST", "DELETE":
			if r.URL.Path == base {
				t.Error("unexpected full-zone mutation")
				w.WriteHeader(400)
				return
			}
			next := []arin.DelegationNameserver{}
			for _, n := range current.NS {
				if n.Name != name {
					next = append(next, n)
				}
			}
			if r.Method == "POST" {
				n := arin.DelegationNameserver{Name: name}
				if v := r.URL.Query().Get("ttl"); v != "" {
					ttl, err := strconv.ParseInt(v, 10, 64)
					if err != nil {
						t.Error(err)
					}
					n.TTL = &ttl
				}
				next = append(next, n)
			}
			current.NS = next
		default:
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(405)
			return
		}
		_ = xml.NewEncoder(w).Encode(current)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	steps := nameserverSteps(zone)
	steps = append(steps, resource.TestStep{PreConfig: func() {
		mu.Lock()
		defer mu.Unlock()
		for i, n := range current.NS {
			if n.Name == "tf-ns2.example.net" {
				ttl := int64(999)
				current.NS[i].TTL = &ttl
			}
		}
	}, Config: steps[len(steps)-1].Config})
	steps = append(steps, resource.TestStep{PreConfig: func() {
		mu.Lock()
		defer mu.Unlock()
		out := []arin.DelegationNameserver{}
		for _, n := range current.NS {
			if n.Name != "tf-ns2.example.net" {
				out = append(out, n)
			}
		}
		current.NS = out
	}, Config: steps[len(steps)-1].Config})
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: steps, CheckDestroy: func(_ *terraform.State) error {
		mu.Lock()
		defer mu.Unlock()
		if !reflect.DeepEqual(current.NS, original.NS) || !reflect.DeepEqual(current.DS, original.DS) {
			return fmt.Errorf("destroy did not preserve unmanaged records")
		}
		return nil
	}})
}
func TestAccDelegationNameserverRequiresImport(t *testing.T) {
	zone := "2.0.192.in-addr.arpa."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("unexpected mutation")
			w.WriteHeader(500)
			return
		}
		_ = xml.NewEncoder(w).Encode(fakeDelegation{Name: zone, NS: []arin.DelegationNameserver{{Name: "tf-ns1.example.net"}, {Name: "tf-sibling.example.net"}}})
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: nameserverConfig(zone, "tf-ns1.example.net", ""), ExpectError: regexp.MustCompile("Nameserver already exists")}}})
}
