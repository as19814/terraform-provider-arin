package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestOTECustomerLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires ARIN_OTE_WRITE_TESTS=1 and TF_ACC=1")
	}
	key := os.Getenv("ARIN_OTE_API_KEY")
	if key == "" {
		key = os.Getenv("ARIN_API_KEY")
	}
	org := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("set ARIN_OTE_API_KEY and ARIN_TEST_ORG_HANDLE")
	}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := client.GetOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	networks, err := client.ListOrganizationNetworks(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	parent := ""
	for _, n := range networks {
		if strings.Contains(strings.ToLower(n.Type), "allocation") {
			parent = n.Handle
			break
		}
	}
	if parent == "" {
		t.Fatal("no OT&E allocation available for customer creation")
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "Terraform OTE customer " + hex.EncodeToString(suffix)
	handle := ""
	t.Cleanup(func() {
		if handle == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := client.DeleteCustomer(ctx, handle); err != nil {
			t.Errorf("customer cleanup %s: %v", handle, err)
			return
		}
		if _, err := client.GetCustomer(ctx, handle); !arin.IsNotFound(err) {
			t.Errorf("customer cleanup unconfirmed %s: %v", handle, err)
		}
	})
	t.Setenv("ARIN_API_KEY", key)
	config := func(updated bool) string {
		customerName := name
		private := true
		comments := `["Disposable sandbox customer"]`
		street := `["123 Test Street"]`
		if updated {
			customerName += " updated"
			private = false
			comments = `[]`
			street = `["456 Test Street", "Suite 2"]`
		}
		return fmt.Sprintf(`provider "arin" {
 base_url = "https://reg.ote.arin.net"
 rdap_base_url = "https://rdap.ote.arin.net"
}
resource "arin_customer" "test" {
 parent_net_handle = %q
 name = %q
 country_code = "US"
 city = "Chantilly"
 subdivision = "VA"
 postal_code = "20151"
 street_address = %s
 comments = %s
 private_customer = %t
}`, parent, customerName, street, comments, private)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())},
		CheckDestroy: func(_ *terraform.State) error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, err := client.GetCustomer(ctx, handle); !arin.IsNotFound(err) {
				return fmt.Errorf("customer deletion not confirmed: %v", err)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config(false), Check: func(s *terraform.State) error {
				r := s.RootModule().Resources["arin_customer.test"]
				if r == nil {
					return fmt.Errorf("missing customer state")
				}
				handle = r.Primary.ID
				t.Logf("Disposable OT&E customer: %s", handle)
				return nil
			}},
			{Config: config(true), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_customer.test", "private_customer", "false"), resource.TestCheckResourceAttr("arin_customer.test", "comments.#", "0"), resource.TestCheckResourceAttr("arin_customer.test", "street_address.#", "2"))},
			{ResourceName: "arin_customer.test", ImportState: true, ImportStateIdFunc: func(_ *terraform.State) (string, error) { return parent + "/" + handle, nil }, ImportStateVerify: true},
			{Config: config(true), PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}
