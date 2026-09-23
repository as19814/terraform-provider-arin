package provider

import (
	"context"
	"crypto/rand"
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

func TestOTEPOCLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key := os.Getenv("ARIN_OTE_API_KEY")
	if key == "" {
		t.Fatal("requires ARIN_OTE_API_KEY")
	}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 5)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	name := "Terraform Test " + strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return 'g' + r - '0'
		}
		return r
	}, fmt.Sprintf("%x", nonce))
	t.Setenv("ARIN_API_KEY", key)
	t.Setenv("ARIN_BASE_URL", arin.OTEURL)
	t.Setenv("ARIN_RDAP_BASE_URL", arin.RDAPOTEURL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: pocSteps(name), CheckDestroy: func(state *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for _, rs := range state.RootModule().Resources {
			if rs.Type != "arin_poc" {
				continue
			}
			if _, err := client.GetPOC(ctx, rs.Primary.ID); !arin.IsNotFound(err) {
				return fmt.Errorf("POC %s still exists or cannot be verified: %v", rs.Primary.ID, err)
			}
		}
		return nil
	}})
}
