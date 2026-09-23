package provider

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func seedMetadataNet(t *testing.T, f *fakeNetAPI) {
	t.Helper()
	body, err := os.ReadFile("../arin/testdata/net.xml")
	if err != nil {
		t.Fatal(err)
	}
	f.objects["NET-192-0-2-0-2"] = strings.Replace(string(body), "<handle>NET-192-0-2-0-1</handle>", "<handle>NET-192-0-2-0-2</handle>", 1)
}
func metadataConfig(extra string) string {
	return `provider "arin" {}
resource "arin_net_metadata" "test" {
 handle = "NET-192-0-2-0-2"
 ` + extra + "\n}"
}
func TestAccNetMetadataLifecycle(t *testing.T) {
	f, _ := setupNetFake(t)
	seedMetadataNet(t, f)
	base := metadataConfig(`name = "MANAGED-NET"
comments = ["Operational comment"]
poc_links = [{handle = "TECH-1", function = "T"}]`)
	omitted := metadataConfig(`name = "RENAMED-NET"`)
	clear := metadataConfig(`name = "RENAMED-NET"
comments = []
poc_links = []`)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.objects) != 1 || f.writes["create"] != 0 || f.writes["delete"] != 0 || f.writes["update"] != 4 {
				return fmt.Errorf("unexpected metadata lifecycle: %v objects=%d", f.writes, len(f.objects))
			}
			if !strings.Contains(f.objects["NET-192-0-2-0-2"], "<netName>RENAMED-NET</netName>") {
				return fmt.Errorf("destroy altered retained metadata")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: base, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_net_metadata.test", "poc_links.0.description", "Tech"), resource.TestCheckResourceAttr("arin_net_metadata.test", "id", "NET-192-0-2-0-2"), resource.TestCheckResourceAttr("arin_net_metadata.test", "poc_links.#", "1"))},
			{ResourceName: "arin_net_metadata.test", ImportState: true, ImportStateVerify: true},
			{PreConfig: func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.objects["NET-192-0-2-0-2"] = strings.Replace(f.objects["NET-192-0-2-0-2"], `description="Tech"`, `description="Technical role"`, 1)
			}, Config: base, Check: resource.TestCheckResourceAttr("arin_net_metadata.test", "poc_links.0.description", "Technical role")},
			{PreConfig: func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.objects["NET-192-0-2-0-2"] = strings.Replace(f.objects["NET-192-0-2-0-2"], "Operational comment", "External change", 1)
			}, Config: base},
			{Config: omitted, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_net_metadata.test", "comments.0", "Operational comment"), resource.TestCheckResourceAttr("arin_net_metadata.test", "poc_links.#", "1"))},
			{Config: clear, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_net_metadata.test", "comments.#", "0"), resource.TestCheckResourceAttr("arin_net_metadata.test", "poc_links.#", "0"))},
			{Config: clear, PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}
func TestAccNetMetadataRejectsAdmin(t *testing.T) {
	f, _ := setupNetFake(t)
	seedMetadataNet(t, f)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		Steps:                    []resource.TestStep{{Config: metadataConfig(`poc_links = [{handle = "ADMIN-1", function = "AD"}]`), ExpectError: regexp.MustCompile("Invalid NET metadata")}},
	})
	if len(f.writes) != 0 {
		t.Fatal("invalid POC caused mutation")
	}
}

func TestAccNetAndMetadataSeparateFields(t *testing.T) {
	f, _ := setupNetFake(t)
	config := func(updated bool) string {
		name, role := "EXAMPLE-NET", "T"
		if updated {
			name, role = "UPDATED-NET", "N"
		}
		return fmt.Sprintf(`provider "arin" {}
resource "arin_net" "test" {
 parent_net_handle = "NET-192-0-2-0-1"
 org_handle = "EXAMPLE-1"
 reallocate = true
 prefixes = ["192.0.2.0/29"]
 name = %q
 comments = ["Owned by arin_net"]
}
resource "arin_net_metadata" "test" {
 handle = arin_net.test.id
 poc_links = [{handle = "TECH-1", function = %q}]
}`, name, role)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
		CheckDestroy: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.objects) != 0 || f.writes["create"] != 1 || f.writes["delete"] != 1 {
				return fmt.Errorf("unexpected combined lifecycle: %v", f.writes)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config(false)},
			{Config: config(true), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_net_metadata.test", "name", "UPDATED-NET"), resource.TestCheckResourceAttr("arin_net_metadata.test", "comments.0", "Owned by arin_net"))},
			{Config: config(true), PlanOnly: true, ExpectNonEmptyPlan: false},
		},
	})
}
