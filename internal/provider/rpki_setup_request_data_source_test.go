package provider

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRPKISetupRequest(t *testing.T) {
	certificate, err := os.ReadFile("../arin/testdata/setup-ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	config := func(handle string) string {
		return fmt.Sprintf(`
 provider "arin" {}
 data "arin_rpki_setup_request" "child" {
  message_type = "child_request"
  handle = %q
  bpki_ta_pem = %q
 }
 data "arin_rpki_setup" "child" { xml = data.arin_rpki_setup_request.child.xml }
 data "arin_rpki_setup_request" "publisher" {
  message_type = "publisher_request"
  handle = "publisher/1"
  bpki_ta_pem = %q
  tag = ""
  referrals = [{referrer = "parent/1", authorization_base64 = "YWJj"}]
 }
 data "arin_rpki_setup" "publisher" { xml = data.arin_rpki_setup_request.publisher.xml }
 `, handle, string(certificate), string(certificate))
	}
	t.Setenv("ARIN_API_KEY", "")
	factories := map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: config("child/1"), Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("data.arin_rpki_setup.child", "child_handle", "child/1"),
			resource.TestCheckNoResourceAttr("data.arin_rpki_setup.child", "tag"),
			resource.TestCheckResourceAttr("data.arin_rpki_setup.publisher", "tag", ""),
			resource.TestCheckResourceAttr("data.arin_rpki_setup.publisher", "publisher_handle", "publisher/1"),
			resource.TestCheckResourceAttr("data.arin_rpki_setup.publisher", "referrals.0.authorization_base64", "YWJj"),
			resource.TestCheckResourceAttrPair("data.arin_rpki_setup_request.child", "id", "data.arin_rpki_setup.child", "id"),
		)},
		{Config: config("child/1"), PlanOnly: true},
		{Config: config("child/2"), Check: resource.TestCheckResourceAttr("data.arin_rpki_setup.child", "child_handle", "child/2")},
		{Config: config("child/2"), PlanOnly: true},
	}})
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{{Config: fmt.Sprintf(`data "arin_rpki_setup_request" "bad" {
 message_type="child_request"
 handle="child"
 bpki_ta_pem=%q
 referrals=[{referrer="p",authorization_base64="YWJj"}]
 }`, string(certificate)), ExpectError: regexp.MustCompile("child setup requests cannot contain publication referrals")}}})
}
