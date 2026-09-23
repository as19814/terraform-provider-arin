package provider

import (
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRPKISetup(t *testing.T) {
	raw, err := os.ReadFile("../arin/testdata/setup-ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	cert := base64.StdEncoding.EncodeToString(block.Bytes)
	config := "provider \"arin\" {}\n"
	checks := []resource.TestCheckFunc{}
	for _, tc := range []struct{ kind, attrs, ta, extra string }{
		{"child_request", `child_handle="" tag=""`, "child", ""},
		{"parent_response", `child_handle="child" parent_handle="parent" service_uri="https://example.net/updown"`, "parent", `<offer/><referral referrer="parent" contact_uri="mailto:rpki@example.net">YWJj</referral>`},
		{"publisher_request", `publisher_handle="publisher"`, "publisher", ""},
		{"repository_response", `publisher_handle="publisher" service_uri="https://example.net/publication" sia_base="rsync://example.net/repo/"`, "repository", ""},
	} {
		xml := `<` + tc.kind + ` xmlns="` + arin.RPKISetupNamespace + `" version="1" ` + tc.attrs + `><` + tc.ta + `_bpki_ta>` + cert + `</` + tc.ta + `_bpki_ta>` + tc.extra + `</` + tc.kind + `>`
		config += fmt.Sprintf("data \"arin_rpki_setup\" %q { xml = %q }\n", tc.kind, xml)
		address := "data.arin_rpki_setup." + tc.kind
		checks = append(checks, resource.TestCheckResourceAttr(address, "message_type", tc.kind), resource.TestCheckResourceAttrSet(address, "bpki_ta_pem"), resource.TestCheckResourceAttrSet(address, "id"))
	}
	checks = append(checks, resource.TestCheckResourceAttr("data.arin_rpki_setup.child_request", "tag", ""), resource.TestCheckResourceAttr("data.arin_rpki_setup.child_request", "child_handle", ""), resource.TestCheckResourceAttr("data.arin_rpki_setup.parent_response", "publication_offered", "true"), resource.TestCheckResourceAttr("data.arin_rpki_setup.parent_response", "referrals.0.authorization_base64", "YWJj"), resource.TestCheckNoResourceAttr("data.arin_rpki_setup.parent_response", "tag"), resource.TestCheckResourceAttr("data.arin_rpki_setup.publisher_request", "referrals.#", "0"))
	t.Setenv("ARIN_API_KEY", "")
	factories := map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)},
		{Config: config, PlanOnly: true},
		{Config: strings.ReplaceAll(config, "parent_handle=\\\"parent\\\"", "parent_handle=\\\"changed\\\""), Check: resource.TestCheckResourceAttr("data.arin_rpki_setup.parent_response", "parent_handle", "changed")},
	}})
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{{Config: `data "arin_rpki_setup" "bad" { xml = "<invalid/>" }`, ExpectError: regexp.MustCompile("Invalid RPKI setup document")}}})
}
