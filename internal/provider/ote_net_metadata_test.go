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

func TestOTENetMetadataLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires ARIN_OTE_API_KEY and ARIN_TEST_ORG_HANDLE")
	}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	poc := ""
	for _, spec := range arin.RegistrationReads() {
		if spec.Name != "org" {
			continue
		}
		record, e := client.ReadRegistration(ctx, spec, map[string]string{"handle": org})
		if e != nil {
			cancel()
			t.Fatal(e)
		}
		for _, value := range record["poc_links"].([]any) {
			link := value.(map[string]any)
			if link["function"] == "T" {
				poc, _ = link["handle"].(string)
				break
			}
		}
	}
	cancel()
	if poc == "" {
		t.Fatal("requires an existing Tech POC on the test organization")
	}
	t.Setenv("ARIN_API_KEY", key)
	factories := map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}
	provider := `provider "arin" {
 base_url = "https://reg.ote.arin.net"
 rdap_base_url = "https://rdap.ote.arin.net"
}
`
	directParent := ""
	for _, family := range []string{"v4", "v6"} {
		t.Run(family, func(t *testing.T) {
			parent, prefix := oteNetCandidate(t, client, org, family)
			if directParent == "" {
				directParent = parent
			}
			var random [8]byte
			if _, err := rand.Read(random[:]); err != nil {
				t.Fatal(err)
			}
			name := fmt.Sprintf("TERRAFORM-OTE-META-%X", random[:])
			a := arin.NetAssignment{ParentNetHandle: parent, Name: name, OrgHandle: org, Reallocate: true, Prefixes: []string{prefix}}
			handle := ""
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if handle == "" {
					if n, e := client.FindNetAssignment(ctx, a); e == nil && n != nil {
						handle = n.Handle
					} else if e != nil {
						t.Errorf("NET cleanup discovery: %v", e)
						return
					}
				}
				if handle == "" {
					return
				}
				if _, e := client.DeleteNetAssignment(ctx, handle); e != nil {
					t.Errorf("NET cleanup %s: %v", handle, e)
					return
				}
				if _, e := client.GetRegisteredNet(ctx, handle); !arin.IsNotFound(e) {
					t.Errorf("NET cleanup unconfirmed %s: %v", handle, e)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			result, err := client.CreateNetAssignment(ctx, a)
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			if result.Net == nil {
				t.Fatalf("NET creation pending: %s", result.TicketNumber)
			}
			handle = result.Net.Handle
			t.Logf("Disposable metadata NET: %s", handle)
			config := func(extra string) string {
				return provider + fmt.Sprintf(`resource "arin_net_metadata" "test" {
 handle = %q
 name = %q
 %s
}`, handle, name+"-UPDATED", extra)
			}
			first := config(fmt.Sprintf(`comments = ["Disposable metadata test"]
poc_links = [{handle = %q, function = "T"}]`, poc))
			links := "["
			for _, role := range []string{"T", "AB", "N"} {
				links += fmt.Sprintf("{handle = %q, function = %q},", poc, role)
			}
			links += "]"
			all := config("poc_links = " + links)
			clear := config("comments = []\npoc_links = []")
			wantCleared := false
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				CheckDestroy: func(_ *terraform.State) error {
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
					defer cancel()
					n, e := client.GetRegisteredNet(ctx, handle)
					if e != nil {
						return fmt.Errorf("metadata destroy removed NET: %v", e)
					}
					if wantCleared && (n.Name != name+"-UPDATED" || len(n.Comments) != 0 || len(n.POCs) != 0) {
						return fmt.Errorf("metadata destroy changed final values")
					}
					return nil
				},
				Steps: []resource.TestStep{
					{Config: first, Check: resource.TestCheckResourceAttr("arin_net_metadata.test", "poc_links.#", "1")},
					{ResourceName: "arin_net_metadata.test", ImportState: true, ImportStateVerify: true},
					{Config: all, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_net_metadata.test", "poc_links.#", "3"), resource.TestCheckResourceAttr("arin_net_metadata.test", "comments.0", "Disposable metadata test"))},
					{Config: config(""), Check: resource.TestCheckResourceAttr("arin_net_metadata.test", "poc_links.#", "3")},
					{Config: clear, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_net_metadata.test", "poc_links.#", "0"), resource.TestCheckResourceAttr("arin_net_metadata.test", "comments.#", "0"), func(_ *terraform.State) error { wantCleared = true; return nil })},
					{ResourceName: "arin_net_metadata.test", ImportState: true, ImportStateVerify: true},
					{Config: clear, PlanOnly: true, ExpectNonEmptyPlan: false},
				},
			})
		})
	}
	// Verify direct-allocation API behavior using an unchanged metadata payload.
	// Actual metadata edits above only touch disposable child registrations.
	t.Run("direct-allocation-unchanged", func(t *testing.T) {
		if directParent == "" {
			t.Fatal("no owned direct allocation discovered")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		before, e := client.GetRegisteredNet(ctx, directParent)
		cancel()
		if e != nil {
			t.Fatal(e)
		}
		if before.OrgHandle != org {
			t.Fatal("parent no longer belongs to test organization")
		}
		for _, b := range before.Blocks {
			if b.Type != "DA" {
				t.Fatal("requires a direct allocation for this test")
			}
		}
		config := provider + fmt.Sprintf("resource \"arin_net_metadata\" \"test\" { handle = %q }", directParent)
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: factories,
			CheckDestroy: func(_ *terraform.State) error {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				after, e := client.GetRegisteredNet(ctx, directParent)
				if e != nil {
					return e
				}
				if !reflect.DeepEqual(before, after) {
					return fmt.Errorf("direct-allocation metadata changed during preservation test")
				}
				return nil
			},
			Steps: []resource.TestStep{
				{Config: config},
				{ResourceName: "arin_net_metadata.test", ImportState: true, ImportStateVerify: true},
				{Config: config, PlanOnly: true, ExpectNonEmptyPlan: false},
			},
		})
	})
}
