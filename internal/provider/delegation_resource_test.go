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

type fakeDelegation struct {
	XMLName xml.Name                    `xml:"http://www.arin.net/regrws/core/v1 delegation"`
	Name    string                      `xml:"name"`
	NS      []arin.DelegationNameserver `xml:"nameservers>nameserver"`
	DS      []arin.DelegationDS         `xml:"delegationKeys>delegationKey"`
}

func delegationConfig(zone, ns, ds string) string {
	return fmt.Sprintf("provider \"arin\" {}\nresource \"arin_delegation\" \"test\" {\n name = %q\n nameservers = %s\n %s\n}\n", zone, ns, ds)
}
func delegationSteps(zone string) []resource.TestStep {
	ds := func(ttl string) string {
		return fmt.Sprintf(`ds_records = [{algorithm=13,digest_type=2,key_tag=12345,digest=%q%s}]`, strings.Repeat("AB", 32), ttl)
	}
	first := delegationConfig(zone, `[{name="ns1.example.net",ttl=3600},{name="ns2.example.net"}]`, ds(",ttl=3600"))
	second := delegationConfig(zone, `[{name="ns2.example.net",ttl=7200},{name="ns1.example.net"}]`, ds(""))
	inheritedDS := strings.NewReplacer("12345", "12346", "algorithm=13", "algorithm=14", "digest_type=2", "digest_type=4", strings.Repeat("AB", 32), strings.Repeat("CD", 48)).Replace(ds(""))
	inherited := delegationConfig(zone, `[{name="ns2.example.net"}]`, inheritedDS)
	return []resource.TestStep{
		{Config: first, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_delegation.test", "id", zone), resource.TestCheckResourceAttrSet("arin_delegation.test", "ds_records.0.algorithm_name"), resource.TestCheckResourceAttrSet("arin_delegation.test", "ds_records.0.digest_type_name"))},
		{ResourceName: "arin_delegation.test", ImportState: true, ImportStateId: zone, ImportStateVerify: true},
		{Config: second, Check: resource.TestCheckTypeSetElemNestedAttrs("arin_delegation.test", "ds_records.*", map[string]string{"key_tag": "12345", "ttl": "3600"})},
		{Config: second, PlanOnly: true},
		{Config: inherited, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_delegation.test", "ds_records.0.algorithm", "14"), resource.TestCheckResourceAttr("arin_delegation.test", "ds_records.0.digest_type", "4"), resource.TestMatchResourceAttr("arin_delegation.test", "ds_records.0.algorithm_name", regexp.MustCompile("384")), resource.TestMatchResourceAttr("arin_delegation.test", "ds_records.0.digest_type_name", regexp.MustCompile("384")))},
		{Config: inherited, PlanOnly: true},
		{Config: delegationConfig(zone, "[]", "")},
		{Config: first},
	}
}
func TestAccDelegationResourceLifecycle(t *testing.T) {
	var mu sync.Mutex
	zone := "2.0.192.in-addr.arpa."
	current := fakeDelegation{Name: zone}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path != "/rest/delegation/"+zone {
			w.WriteHeader(404)
			return
		}
		switch r.Method {
		case "GET":
		case "PUT":
			var next fakeDelegation
			if xml.NewDecoder(r.Body).Decode(&next) != nil || next.Name != zone {
				w.WriteHeader(400)
				return
			}
			for i, n := range next.NS {
				if n.TTL == nil {
					for _, old := range current.NS {
						if old.Name == n.Name {
							next.NS[i].TTL = old.TTL
						}
					}
				}
			}
			for i, k := range next.DS {
				if k.TTL == nil {
					for _, old := range current.DS {
						if old.Algorithm == k.Algorithm && old.DigestType == k.DigestType && old.KeyTag == k.KeyTag && old.Digest == k.Digest {
							next.DS[i].TTL = old.TTL
						}
					}
				}
			}
			current = next
			writes++
		default:
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		body, err := xml.Marshal(current)
		if err != nil {
			t.Error(err)
			return
		}
		body = []byte(strings.NewReplacer("<algorithm>13</algorithm>", `<algorithm name="ECDSAP256SHA256">13</algorithm>`, "<algorithm>14</algorithm>", `<algorithm name="ECDSAP384SHA384">14</algorithm>`).Replace(string(body)))
		body = []byte(strings.NewReplacer("<digestType>2</digestType>", `<digestType name="SHA-256">2</digestType>`, "<digestType>4</digestType>", `<digestType name="SHA-384">4</digestType>`).Replace(string(body)))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	steps := delegationSteps(zone)
	steps = append(steps, resource.TestStep{PreConfig: func() {
		mu.Lock()
		defer mu.Unlock()
		current.NS = []arin.DelegationNameserver{{Name: "drift.example.net"}}
	}, Config: steps[0].Config})
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps:                    steps,
		CheckDestroy: func(_ *terraform.State) error {
			mu.Lock()
			defer mu.Unlock()
			if len(current.NS) != 0 || len(current.DS) != 0 || writes < 7 {
				return fmt.Errorf("unexpected delegation state after destroy: writes=%d", writes)
			}
			return nil
		},
	})
}
func TestAccDelegationRequiresImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("unexpected mutation")
			w.WriteHeader(500)
			return
		}
		_ = xml.NewEncoder(w).Encode(fakeDelegation{Name: "2.0.192.in-addr.arpa.", NS: []arin.DelegationNameserver{{Name: "ns.example.net"}}})
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "acceptance-test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: delegationConfig("2.0.192.in-addr.arpa.", "[]", ""), ExpectError: regexp.MustCompile("Delegation already has DNS records")}}})
}
