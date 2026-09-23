package provider

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func sortedOrgPOCs(p []arin.OrgPOC) []arin.OrgPOC {
	out := append([]arin.OrgPOC{}, p...)
	sort.Slice(out, func(i, j int) bool { return out[i].Handle+"/"+out[i].Function < out[j].Handle+"/"+out[j].Function })
	return out
}
func TestOTEOrgPOCLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and org handle")
	}
	c, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	before, err := c.GetOrganizationPOCs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 5)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	p := arin.POC{ContactType: "ROLE", CompanyName: "Example Networks", CountryCode: "US", City: "Chantilly", Subdivision: "VA", PostalCode: "20151", StreetAddress: []string{"123 Example Street"}, Emails: []string{"noc@example.net"}, Phones: []arin.POCPhone{{Type: "O", Number: "+1-202-555-0100"}}}
	p.LastName = fmt.Sprintf("Terraform Org Link %x", nonce)
	created, err := c.CreatePOC(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	handle := created.Handle
	backup := ""
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		current, err := c.GetOrganizationPOCs(ctx, org)
		if err != nil {
			t.Errorf("cannot verify org links; recovery file %s: %v", backup, err)
			return
		}
		for _, link := range current {
			if link.Handle == handle {
				if _, err = c.RemoveOrganizationPOC(ctx, org, handle, link.Function); err != nil {
					t.Errorf("cannot remove disposable POC link; recovery file %s: %v", backup, err)
					return
				}
			}
		}
		current, err = c.GetOrganizationPOCs(ctx, org)
		if err != nil || !reflect.DeepEqual(sortedOrgPOCs(current), sortedOrgPOCs(before)) {
			t.Errorf("original organization links not restored; recovery file %s: %v", backup, err)
			return
		}
		if err = c.DeletePOC(ctx, handle); err != nil {
			t.Errorf("cannot delete disposable POC %s; recovery file %s: %v", handle, backup, err)
			return
		}
		if backup != "" {
			if err = os.Remove(backup); err != nil {
				t.Error(err)
			}
		}
	})
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "terraform-provider-arin")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(org))
	backup = filepath.Join(dir, fmt.Sprintf("ote-org-pocs-%x.json", hash[:8]))
	data, err := json.Marshal(struct {
		Org, POC string
		Original []arin.OrgPOC
	}{org, handle, before})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		backup = ""
		t.Fatalf("cannot create exclusive org recovery snapshot: %v", err)
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ARIN_API_KEY", key)
	t.Setenv("ARIN_BASE_URL", arin.OTEURL)
	t.Setenv("ARIN_RDAP_BASE_URL", arin.RDAPOTEURL)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: orgPOCSteps(org, handle), CheckDestroy: func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		current, err := c.GetOrganizationPOCs(ctx, org)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(sortedOrgPOCs(current), sortedOrgPOCs(before)) {
			return fmt.Errorf("original organization associations not restored after destroy")
		}
		return nil
	}})
}
