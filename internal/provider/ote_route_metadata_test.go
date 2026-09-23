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
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestOTEIRRRouteMetadataLifecycle(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
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
	prefixes := map[string]int64{}
	routeIDs := []string{}
	for _, p := range resources {
		prefixes[p.Prefix] = int64(netip.MustParsePrefix(p.Prefix).Bits())
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
	name := fmt.Sprintf("tf-ote-metadata-%x", nonce)
	for _, a := range beforeROAs {
		if a.Name == name || a.Name == name+"-updated" {
			t.Fatal("disposable name already exists")
		}
	}
	setNames := []string{"RS-" + strings.ToUpper(name) + "-A", "RS-" + strings.ToUpper(name) + "-B"}
	for _, setName := range setNames {
		if _, err := c.GetRouteSet(ctx, setName); !arin.IsNotFound(err) {
			t.Fatalf("helper route-set absence not confirmed: %v", err)
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
		Org, Name     string
		ROAs          []arin.ROA
		ASPAs         []arin.ASPA
		RouteIDs      []string
		RouteSetNames []string
	}{org, name, beforeROAs, beforeASPAs, routeIDs, setNames})
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
			if a.Name == name || a.Name == name+"-updated" {
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
		for _, setName := range setNames {
			set, err := c.GetRouteSet(ctx, setName)
			if arin.IsNotFound(err) {
				continue
			}
			if err != nil || set.OrgHandle != org {
				t.Errorf("cannot safely clean helper route set; retain %s: %v", snapshot, err)
				return
			}
			if err := c.DeleteRouteSet(ctx, setName); err != nil {
				t.Error(err)
				return
			}
			if _, err := c.GetRouteSet(ctx, setName); !arin.IsNotFound(err) {
				t.Errorf("helper route set remains; retain %s: %v", snapshot, err)
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
	config := func(roaName string, phase int, include bool) string {
		text := roaConfig(org, roaName, asn, prefixes, true, true)
		for i, setName := range setNames {
			text += fmt.Sprintf(`
resource "arin_irr_route_set" "helper%d" {
 name = %q
 org_handle = %q
 description = ["Disposable linked route metadata test"]
 members_by_ref = [%q]
}
`, i, setName, org, "MNT-"+org)
		}
		if !include {
			return text
		}
		remarks, membership := `["Initial user remark"]`, `[arin_irr_route_set.helper0.name, arin_irr_route_set.helper1.name]`
		if phase == 1 {
			remarks = `["Updated user remark"]`
			membership = `[arin_irr_route_set.helper0.name]`
		}
		if phase == 2 {
			remarks = `[]`
			membership = `[]`
		}
		for i, p := range resources {
			text += fmt.Sprintf(`
resource "arin_irr_route_metadata" "v%d" {
 prefix = %q
 origin_as = "AS${arin_roa.test.asn}"
 org_handle = arin_roa.test.org_handle
 expected_roa_handle = arin_roa.test.handle
 description = ["Disposable route metadata"]
 remarks = %s
 member_of = %s
}
`, i, p.Prefix, remarks, membership)
		}
		return text
	}
	first := config(name, 0, true)
	changed := config(name, 1, true)
	replaced := config(name+"-updated", 1, true)
	cleared := config(name+"-updated", 2, true)
	var firstHandle string
	check := func(phase int, replacement bool) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			handle := s.RootModule().Resources["arin_roa.test"].Primary.Attributes["handle"]
			if firstHandle == "" {
				firstHandle = handle
			}
			if replacement && handle == firstHandle {
				return fmt.Errorf("owning ROA did not replace its handle")
			}
			for i, id := range routeIDs {
				route, err := c.GetIRRRoute(ctx, id)
				if err != nil {
					return err
				}
				if route.AutoLinkedROAHandle != handle {
					return fmt.Errorf("route link differs from owning Terraform ROA")
				}
				remarks, err := arin.LinkedRouteUserRemarks(*route)
				if err != nil {
					return err
				}
				wantRemarks := []string{"Initial user remark"}
				wantMembers := slices.Clone(setNames)
				if phase == 1 {
					wantRemarks = []string{"Updated user remark"}
					wantMembers = setNames[:1]
				}
				if phase == 2 {
					wantRemarks = nil
					wantMembers = nil
				}
				actualMembers := slices.Clone(route.MemberOf)
				slices.Sort(actualMembers)
				expectedMembers := slices.Clone(wantMembers)
				slices.Sort(expectedMembers)
				if !slices.Equal(remarks, wantRemarks) || !slices.Equal(actualMembers, expectedMembers) || !slices.Equal(route.Description, []string{"Disposable route metadata"}) {
					return fmt.Errorf("route metadata did not match phase %d", phase)
				}
				if state, exists := s.RootModule().Resources[fmt.Sprintf("arin_irr_route_metadata.v%d", i)]; exists {
					if state.Primary.Attributes["expected_roa_handle"] != handle || state.Primary.Attributes["auto_linked_roa_handle"] != handle {
						return fmt.Errorf("metadata state did not follow the owning ROA")
					}
				}
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: []resource.TestStep{
		{Config: first, Check: check(0, false)},
		{ResourceName: "arin_irr_route_metadata.v0", ImportState: true, ImportStateVerify: true, ImportStateId: routeIDs[0]},
		{ResourceName: "arin_irr_route_metadata.v1", ImportState: true, ImportStateVerify: true, ImportStateId: routeIDs[1]},
		{Config: first, PlanOnly: true},
		{Config: changed, Check: check(1, false)},
		{Config: replaced, Check: check(1, true)},
		{Config: replaced, PlanOnly: true},
		{Config: cleared, Check: check(2, true)},
		{Config: cleared, PlanOnly: true},
		{Config: config(name+"-updated", 2, false), Check: check(2, true)},
	}, CheckDestroy: func(_ *terraform.State) error {
		for _, id := range routeIDs {
			if _, err := c.GetIRRRoute(ctx, id); !arin.IsNotFound(err) {
				return fmt.Errorf("linked route remains after owner destroy: %v", err)
			}
		}
		for _, setName := range setNames {
			if _, err := c.GetRouteSet(ctx, setName); !arin.IsNotFound(err) {
				return fmt.Errorf("helper route set remains after destroy: %v", err)
			}
		}
		return nil
	}})
	t.Log("IPv4/IPv6 metadata graph passed: membership/import/update, owning ROA replacement, clearing, state-only removal and final owner cleanup")
}
