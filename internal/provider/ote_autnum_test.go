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

func TestOTEAutnumLifecycle(t *testing.T) {
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
	c, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := c.GetOrganization(ctx, org); err != nil {
		t.Fatal(err)
	}
	var inventory map[string]any
	for _, spec := range arin.PublicReads() {
		if spec.Name == "asns" {
			inventory, err = c.ReadRegistration(ctx, spec, map[string]string{"org_handle": org})
			break
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	name := ""
	for _, record := range inventory["asns"].([]any) {
		row := record.(map[string]any)
		candidate := fmt.Sprintf("AS%d", row["start_asn"].(int64))
		_, err := c.GetAutnum(ctx, candidate)
		if arin.IsNotFound(err) {
			name = candidate
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if name == "" {
		t.Fatal("no registered ASN without an existing OT&E IRR aut-num; no writes attempted")
	}
	t.Logf("Disposable OT&E IRR aut-num: %s (ASN registration is unchanged)", name)
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	setName := "AS-TF-OTE-" + strings.ToUpper(hex.EncodeToString(suffix))
	if _, err := c.GetASSet(ctx, setName); !arin.IsNotFound(err) {
		t.Fatalf("AS set preflight did not confirm absence: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := c.DeleteASSet(ctx, setName); err != nil {
			t.Errorf("helper set cleanup %s: %v", setName, err)
			return
		}
		if _, err := c.GetASSet(ctx, setName); !arin.IsNotFound(err) {
			t.Errorf("helper AS set still present after cleanup: %v", err)
		}
	})
	if _, err := c.CreateASSet(ctx, arin.ASSet{Name: setName, OrgHandle: org, Description: []string{"Disposable aut-num membership test"}, MembersByRef: []string{"MNT-" + org}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		r, err := c.GetAutnum(ctx, name)
		if arin.IsNotFound(err) {
			return
		}
		if err != nil {
			t.Errorf("aut-num cleanup read %s: %v", name, err)
			return
		}
		if r.OrgHandle != org {
			t.Errorf("aut-num org mismatch during cleanup")
			return
		}
		if err := c.DeleteAutnum(ctx, name); err != nil {
			t.Errorf("aut-num cleanup %s: %v", name, err)
			return
		}
		if _, err := c.GetAutnum(ctx, name); !arin.IsNotFound(err) {
			t.Errorf("aut-num remains: %v", err)
		}
	})
	t.Setenv("ARIN_API_KEY", key)
	config := func(stage int) string {
		fields := map[string]any{
			"as_number": name, "as_name": "TF-OTE-" + name, "org_handle": org, "description": []string{"Disposable Terraform OT&E aut-num"},
			"remarks": []string{"Initial remarks"}, "member_of": []string{setName},
			"import_policy": []string{"from AS64496 accept ANY"}, "export_policy": []string{"to AS64496 announce " + name}, "default_policy": []string{"to AS64496 networks ANY"},
			"mp_import_policy": []string{"afi ipv6.unicast from AS64496 accept ANY"}, "mp_export_policy": []string{"afi ipv6.unicast to AS64496 announce " + name}, "mp_default_policy": []string{"afi ipv6.unicast to AS64496 networks ANY"},
		}
		if stage > 0 {
			fields["description"] = []string{"Updated disposable Terraform OT&E aut-num"}
			fields["as_name"] = "TF-OTE-UPDATED"
			fields["import_policy"] = []string{"from AS64497 accept ANY"}
		}
		if stage > 1 {
			for _, k := range []string{"remarks", "member_of", "import_policy", "export_policy", "default_policy", "mp_import_policy", "mp_export_policy", "mp_default_policy"} {
				fields[k] = []string{}
			}
		}
		out := `provider "arin" {` + "\n base_url = \"https://reg.ote.arin.net\"\n rdap_base_url = \"https://rdap.ote.arin.net\"\n}\nresource \"arin_irr_aut_num\" \"test\" {\n"
		for k, v := range fields {
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			out += k + " = " + string(b) + "\n"
		}
		return out + "}\n"
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())},
		CheckDestroy: func(_ *terraform.State) error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if _, err := c.GetAutnum(ctx, name); !arin.IsNotFound(err) {
				return fmt.Errorf("aut-num deletion not confirmed: %v", err)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config(0), Check: resource.TestCheckResourceAttr("arin_irr_aut_num.test", "id", name)},
			{Config: config(1), Check: resource.TestCheckResourceAttr("arin_irr_aut_num.test", "import_policy.0", "from AS64497 accept ANY")},
			{Config: config(2), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_irr_aut_num.test", "member_of.#", "0"), resource.TestCheckResourceAttr("arin_irr_aut_num.test", "mp_default_policy.#", "0"))},
			{ResourceName: "arin_irr_aut_num.test", ImportState: true, ImportStateVerify: true},
			{Config: config(2), PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}
