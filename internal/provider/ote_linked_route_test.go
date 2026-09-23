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
	"slices"
	"testing"
	"time"
)

func TestOTEIRRLinkedRouteLifecycle(t *testing.T) {
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
	beforeROAs, err := c.ListROAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	beforeASPAs, err := c.ListASPAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeASPAs) == 0 {
		t.Fatal("requires an existing ASPA to discover an owned origin ASN")
	}
	asn := beforeASPAs[0].CustomerASN
	resources := roaOTEPrefixes(t, ctx, c, org)
	routeIDs := []string{}
	for _, p := range resources {
		id := fmt.Sprintf("%s,AS%d", p.Prefix, asn)
		if _, err := c.GetIRRRoute(ctx, id); !arin.IsNotFound(err) {
			t.Fatalf("disposable IRR route must be absent: %v", err)
		}
		routeIDs = append(routeIDs, id)
	}
	nonce := make([]byte, 8)
	if _, err = rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("tf-ote-linked-%x", nonce)
	for _, a := range beforeROAs {
		if a.Name == name {
			t.Fatal("disposable name already exists")
		}
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "terraform-provider-arin")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(org))
	snapshot := filepath.Join(dir, fmt.Sprintf("ote-rpki-%x.json", hash[:8]))
	data, err := json.Marshal(struct {
		Org, Name string
		ROAs      []arin.ROA
		ASPAs     []arin.ASPA
		RouteIDs  []string
	}{org, name, beforeROAs, beforeASPAs, routeIDs})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(snapshot, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
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
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		current, err := c.ListROAs(ctx, org)
		if err != nil {
			t.Errorf("cannot read cleanup inventory; retain %s: %v", snapshot, err)
			return
		}
		txn := arin.RPKITransaction{}
		for _, a := range current {
			if a.Name == name {
				txn.DeleteROAs = append(txn.DeleteROAs, arin.ROADelete{Handle: a.Handle, AutoLink: true})
			}
		}
		if len(txn.DeleteROAs) > 0 {
			if _, err = c.ApplyRPKITransaction(ctx, org, txn); err != nil {
				t.Logf("cleanup response needs verification: %v", err)
			}
		}
		for _, id := range routeIDs {
			route, err := c.GetIRRRoute(ctx, id)
			if arin.IsNotFound(err) {
				continue
			}
			if err != nil || route.AutoLinkedROAHandle != "" {
				t.Errorf("cannot safely clean up route; retain %s: %v", snapshot, err)
				return
			}
			if err = c.DeleteIRRRoute(ctx, id); err != nil {
				t.Error(err)
				return
			}
			if _, err = c.GetIRRRoute(ctx, id); !arin.IsNotFound(err) {
				t.Errorf("route remains; retain %s: %v", snapshot, err)
				return
			}
		}
		current, err = c.ListROAs(ctx, org)
		if err != nil {
			t.Error(err)
			return
		}
		aspas, err := c.ListASPAs(ctx, org)
		if err != nil {
			t.Error(err)
			return
		}
		if !reflect.DeepEqual(roaSnapshotOrder(current), roaSnapshotOrder(beforeROAs)) || !reflect.DeepEqual(aspaSnapshotOrder(aspas), aspaSnapshotOrder(beforeASPAs)) {
			t.Errorf("RPKI baseline not restored; retain %s", snapshot)
			return
		}
		if err = os.Remove(snapshot); err != nil {
			t.Error(err)
		}
	})

	t.Setenv("ARIN_API_KEY", key)
	t.Setenv("ARIN_BASE_URL", arin.OTEURL)
	t.Setenv("ARIN_RDAP_BASE_URL", arin.RDAPOTEURL)
	request := arin.ROARequest{Name: name, ASN: asn, AutoLink: true, Resources: resources}
	created, err := c.ApplyRPKITransaction(ctx, org, arin.RPKITransaction{AddROAs: []arin.ROARequest{request}})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.ROAs) != 1 {
		t.Fatal("disposable owning ROA was not confirmed")
	}
	handle := created.ROAs[0].Handle
	for i, prefix := range resources {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			id := fmt.Sprintf("%s,AS%d", prefix.Prefix, asn)
			config := func(remark string) string {
				return fmt.Sprintf(`provider "arin" {}
resource "arin_irr_linked_route" "test" {
 prefix = %q
 origin_as = %q
 org_handle = %q
 expected_roa_handle = %q
 description = ["Disposable independently managed linked route"]
 remarks = [%q]
}
`, prefix.Prefix, fmt.Sprintf("AS%d", asn), org, handle, remark)
			}
			first, next := config("Initial linked-route remark"), config("Updated linked-route remark")
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: []resource.TestStep{
				{Config: first, ResourceName: "arin_irr_linked_route.test", ImportState: true, ImportStateId: id, ImportStatePersist: true},
				{Config: first},
				{ResourceName: "arin_irr_linked_route.test", ImportState: true, ImportStateVerify: true},
				{Config: next},
				{Config: next, PlanOnly: true},
			}, CheckDestroy: func(_ *terraform.State) error {
				if _, err := c.GetIRRRoute(ctx, id); !arin.IsNotFound(err) {
					return fmt.Errorf("independently deleted route remains: %v", err)
				}
				roas, err := c.ListROAs(ctx, org)
				if err != nil {
					return err
				}
				for _, actual := range roas {
					if actual.Handle != handle {
						continue
					}
					for _, p := range actual.Resources {
						if p.Prefix == prefix.Prefix && p.AutoLinked {
							return fmt.Errorf("deleted route remains marked linked")
						}
					}
					// Compare every authorization field independently of changed link flags.
					actual.AutoLink = nil
					actual.Resources = slices.Clone(actual.Resources)
					for i := range actual.Resources {
						actual.Resources[i].AutoLinked = false
					}
					expected := request
					expected.AutoLink = false
					if !arin.ROAMatchesRequest(actual, expected) {
						return fmt.Errorf("independent route deletion changed ROA authorization")
					}
					return nil
				}
				return fmt.Errorf("independent route deletion removed its ROA")
			}})
		})
	}
	t.Log("IPv4/IPv6 linked-route import, update and independent destroy passed; ROA authorization remained intact")
}
