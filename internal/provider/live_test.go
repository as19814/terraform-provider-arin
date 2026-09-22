package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestLiveOrganization reads an existing organization. It never creates, updates,
// or deletes records. Live tests require a separate opt-in from fake acceptance tests.
func TestLiveOrganization(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" {
		t.Skip("set ARIN_LIVE_TESTS=1 and TF_ACC=1 to run read-only live tests")
	}
	if os.Getenv("TF_ACC") != "1" {
		t.Fatal("live tests also require TF_ACC=1")
	}
	if os.Getenv("ARIN_API_KEY") == "" {
		t.Fatal("ARIN_API_KEY is required")
	}
	handle := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if handle == "" {
		t.Fatal("ARIN_TEST_ORG_HANDLE is required")
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"arin": providerserver.NewProtocol6WithError(New("live-test")()),
		},
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf("provider \"arin\" {}\ndata \"arin_org\" \"live\" { handle = %q }", handle),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.arin_org.live", "handle", handle),
				resource.TestCheckResourceAttrSet("data.arin_org.live", "name"),
			),
		}},
	})
}

// TestLiveNetworks discovers the organization's public registrations without
// transmitting the API key. It intentionally does not pin a changing inventory count.
func TestLiveNetworks(t *testing.T) {
	if os.Getenv("ARIN_LIVE_TESTS") != "1" {
		t.Skip("set ARIN_LIVE_TESTS=1 and TF_ACC=1 to run read-only live tests")
	}
	if os.Getenv("TF_ACC") != "1" {
		t.Fatal("live tests also require TF_ACC=1")
	}
	handle := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if handle == "" {
		t.Fatal("ARIN_TEST_ORG_HANDLE is required")
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("live-test")())},
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf("provider \"arin\" {}\ndata \"arin_networks\" \"live\" { org_handle = %q }", handle),
			Check:  resource.TestCheckResourceAttrSet("data.arin_networks.live", "networks.%"),
		}},
	})
}
