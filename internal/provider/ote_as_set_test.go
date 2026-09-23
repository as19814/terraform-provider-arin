package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

// TestOTEASSetLifecycle is explicitly opt-in and pins every request to OT&E.
// It never imports, changes, or deletes an existing account object.
func TestOTEASSetLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires ARIN_OTE_WRITE_TESTS=1 and TF_ACC=1")
	}
	key := os.Getenv("ARIN_OTE_API_KEY")
	if key == "" {
		key = os.Getenv("ARIN_API_KEY")
	}
	org := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("set ARIN_OTE_API_KEY (or ARIN_API_KEY) and ARIN_TEST_ORG_HANDLE")
	}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var values map[string]any
	for _, spec := range arin.RegistrationReads() {
		if spec.Name == "org" {
			values, err = client.ReadRegistration(ctx, spec, map[string]string{"handle": org})
			break
		}
	}
	if err != nil {
		t.Fatalf("OT&E organization preflight failed (no writes attempted): %v", err)
	}
	if values["handle"] != org {
		t.Fatal("OT&E returned a mismatched organization")
	}
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	name := "AS-TF-OTE-" + strings.ToUpper(hex.EncodeToString(suffix))
	if _, err := client.GetASSet(ctx, name); !arin.IsNotFound(err) {
		t.Fatalf("test name preflight did not confirm absence: %v", err)
	}
	t.Logf("Disposable OT&E AS set: %s", name)
	// Cleanup also covers failed applies that created an object without saving state.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		remote, err := client.GetASSet(cleanupCtx, name)
		if arin.IsNotFound(err) {
			return
		}
		if err != nil {
			t.Errorf("could not verify OT&E cleanup for %s: %v", name, err)
			return
		}
		if remote.OrgHandle != org {
			t.Errorf("refusing cleanup of %s with mismatched organization", name)
			return
		}
		if err := client.DeleteASSet(cleanupCtx, name); err != nil {
			t.Errorf("OT&E cleanup failed for %s: %v", name, err)
			return
		}
		if _, err := client.GetASSet(cleanupCtx, name); !arin.IsNotFound(err) {
			t.Errorf("OT&E cleanup could not confirm %s is absent: %v", name, err)
		}
	})
	t.Setenv("ARIN_API_KEY", key)
	config := func(updated bool) string {
		description := []string{"Disposable Terraform OT&E lifecycle test"}
		remarks := []string{"Created by an opt-in provider test"}
		members := []string{"AS64496"}
		if updated {
			description = append(description, "Updated description")
			remarks = []string{"Updated test remark"}
			members = []string{"AS64497"}
		}
		encode := func(v any) string {
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
		return fmt.Sprintf(`provider "arin" {
 base_url = "https://reg.ote.arin.net"
 rdap_base_url = "https://rdap.ote.arin.net"
}
resource "arin_irr_as_set" "test" {
 name = %q
 org_handle = %q
 description = %s
 remarks = %s
 members = %s
}`, name, org, encode(description), encode(remarks), encode(members))
	}
	cleared := strings.Replace(config(true), `["Updated test remark"]`, `[]`, 1)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())},
		CheckDestroy: func(_ *terraform.State) error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, err := client.GetASSet(ctx, name); !arin.IsNotFound(err) {
				return fmt.Errorf("OT&E deletion was not confirmed for %s: %v", name, err)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config(false), Check: resource.TestCheckResourceAttr("arin_irr_as_set.test", "id", name)},
			{Config: config(true), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckTypeSetElemAttr("arin_irr_as_set.test", "members.*", "AS64497"),
				resource.TestCheckResourceAttr("arin_irr_as_set.test", "remarks.#", "1"),
				resource.TestCheckResourceAttrSet("arin_irr_as_set.test", "poc_links.#"),
			)},
			{Config: cleared, Check: resource.TestCheckResourceAttr("arin_irr_as_set.test", "remarks.#", "0")},
			// The acceptance harness imports into a separate state and compares every field.
			{ResourceName: "arin_irr_as_set.test", ImportState: true, ImportStateVerify: true},
			{Config: cleared, PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}
