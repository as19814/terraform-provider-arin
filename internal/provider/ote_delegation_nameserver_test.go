package provider

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestOTEDelegationNameserverLifecycle(t *testing.T) {
	for _, family := range []string{"v4", "v6"} {
		t.Run(family, func(t *testing.T) {
			client, zone := oteDelegationSnapshot(t, family)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			original, err := client.GetDelegation(ctx, zone)
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"tf-ns1.example.net", "tf-ns2.example.net", "tf-sibling.example.net"} {
				if findDelegationNameserver(original, name) != nil {
					t.Fatal("synthetic test nameserver already exists")
				}
			}
			t.Setenv("ARIN_API_KEY", os.Getenv("ARIN_OTE_API_KEY"))
			t.Setenv("ARIN_BASE_URL", arin.OTEURL)
			t.Setenv("ARIN_RDAP_BASE_URL", arin.RDAPOTEURL)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: nameserverSteps(zone), CheckDestroy: func(_ *terraform.State) error {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				current, err := client.GetDelegation(ctx, zone)
				if err != nil {
					return err
				}
				if !reflect.DeepEqual(current, original) {
					return fmt.Errorf("individual nameserver lifecycle changed other DNS records")
				}
				return nil
			}})
		})
	}
}
