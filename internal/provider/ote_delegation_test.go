package provider

import (
	"context"
	"fmt"
	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"os"
	"testing"
	"time"
)

func TestOTEDelegationLifecycle(t *testing.T) {
	for _, family := range []string{"v4", "v6"} {
		t.Run(family, func(t *testing.T) {
			client, zone := oteDelegationSnapshot(t, family)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			_, err := client.UpdateDelegation(ctx, arin.Delegation{Name: zone})
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("ARIN_API_KEY", os.Getenv("ARIN_OTE_API_KEY"))
			t.Setenv("ARIN_BASE_URL", arin.OTEURL)
			t.Setenv("ARIN_RDAP_BASE_URL", arin.RDAPOTEURL)
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())},
				Steps:                    delegationSteps(zone),
				CheckDestroy: func(_ *terraform.State) error {
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
					defer cancel()
					d, err := client.GetDelegation(ctx, zone)
					if err != nil {
						return err
					}
					if len(d.Nameservers) != 0 || len(d.DSRecords) != 0 {
						return fmt.Errorf("destroy did not clear delegation")
					}
					return nil
				},
			})
		})
	}
}
