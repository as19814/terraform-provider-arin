package provider

import (
	"context"
	"crypto/rand"
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

func TestOTEPOCContactsLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key := os.Getenv("ARIN_OTE_API_KEY")
	if key == "" {
		t.Fatal("requires ARIN_OTE_API_KEY")
	}
	c, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 5)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	original, err := c.CreatePOC(ctx, arin.POC{ContactType: "ROLE", CompanyName: "Example Networks", LastName: fmt.Sprintf("Terraform Contacts %x", nonce), CountryCode: "US", City: "Chantilly", Subdivision: "VA", PostalCode: "20151", StreetAddress: []string{"123 Example Street"}, Emails: []string{"noc@example.net"}, Phones: []arin.POCPhone{{Type: "O", Number: "+1-202-555-0100"}}})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := c.DeletePOC(ctx, original.Handle); err != nil {
			t.Errorf("cleanup POC %s: %v", original.Handle, err)
		}
	})
	t.Setenv("ARIN_API_KEY", key)
	t.Setenv("ARIN_BASE_URL", arin.OTEURL)
	t.Setenv("ARIN_RDAP_BASE_URL", arin.RDAPOTEURL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: contactSteps(original.Handle), CheckDestroy: func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		current, err := c.GetPOC(ctx, original.Handle)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(current, original) {
			return fmt.Errorf("individual contact resources changed baseline POC details")
		}
		return nil
	}})
}
