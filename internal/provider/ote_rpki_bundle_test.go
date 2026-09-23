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
	"strings"
	"testing"
	"time"
)

func TestOTERPKIBundleLifecycle(t *testing.T) {
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
	original := beforeASPAs[0]
	asn := original.CustomerASN
	changed := []int64{0}
	if len(original.ProviderASNs) == 1 && original.ProviderASNs[0] == 0 {
		changed = []int64{13335}
		for _, spec := range arin.PublicReads() {
			if spec.Name == "asn" {
				if _, err := c.ReadRegistration(ctx, spec, map[string]string{"asn": "13335"}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
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
	name := fmt.Sprintf("tf-ote-bundle-%x", nonce)
	for _, a := range beforeROAs {
		if a.Name == name+"-v4" || a.Name == name+"-v6" {
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
			if a.Name == name+"-v4" || a.Name == name+"-v6" {
				txn.DeleteROAs = append(txn.DeleteROAs, arin.ROADelete{Handle: a.Handle, AutoLink: true})
			}
		}

		aspas, err := c.ListASPAs(ctx, org)
		if err != nil {
			t.Errorf("cannot read ASPA restoration inventory; retain %s: %v", snapshot, err)
			return
		}
		restored, present := false, false
		for _, a := range aspas {
			if a.CustomerASN == original.CustomerASN {
				present = true
				restored = arin.ASPAEqual(a, original)
			}
		}
		if !restored {
			txn.AddASPAs = []arin.ASPA{original}
			if present {
				txn.DeleteASPAs = []int64{original.CustomerASN}
			}
		}
		if len(txn.DeleteROAs)+len(txn.AddASPAs) > 0 {
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
		aspas, err = c.ListASPAs(ctx, org)
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
	config := func(count int, providers []int64, linked bool) string {
		var roas, aspa strings.Builder
		for i := 0; i < count; i++ {
			label := []string{"v4", "v6"}[i]
			p := resources[i].Prefix
			fmt.Fprintf(&roas, "%s = { name = %q, asn = %d, prefixes = { %q = %d }, auto_link = %t, delete_linked_routes = %t }\n", label, name+"-"+label, asn, p, netip.MustParsePrefix(p).Bits(), linked, linked)
		}
		for _, provider := range providers {
			fmt.Fprintf(&aspa, "%d,", provider)
		}
		return fmt.Sprintf("provider \"arin\" {}\nresource \"arin_rpki_bundle\" \"test\" {\n org_handle = %q\n name = %q\n roas = {\n%s}\n aspas = { %q = [%s] }\n}\n", org, name, roas.String(), fmt.Sprint(asn), aspa.String())
	}
	initial := config(0, original.ProviderASNs, false)
	created := config(2, changed, false)
	linked := config(2, original.ProviderASNs, true)
	reduced := config(1, changed, true)
	manifest, _ := json.Marshal(map[string]any{"org_handle": org, "name": name, "aspas": []int64{asn}})
	var handles map[string]string
	checkDestroyed := func(_ *terraform.State) error {
		roas, err := c.ListROAs(ctx, org)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(roaSnapshotOrder(roas), roaSnapshotOrder(beforeROAs)) {
			return fmt.Errorf("bundle destroy did not preserve ROA baseline")
		}
		aspas, err := c.ListASPAs(ctx, org)
		if err != nil {
			return err
		}
		for _, a := range aspas {
			if a.CustomerASN == asn {
				return fmt.Errorf("bundle ASPA remains after destroy")
			}
		}
		for _, id := range routeIDs {
			if _, err := c.GetIRRRoute(ctx, id); !arin.IsNotFound(err) {
				return fmt.Errorf("bundle linked route remains: %v", err)
			}
		}
		return nil
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: []resource.TestStep{
		{Config: initial, ResourceName: "arin_rpki_bundle.test", ImportState: true, ImportStateId: string(manifest), ImportStatePersist: true},
		{Config: created, Check: func(s *terraform.State) error {
			a := s.RootModule().Resources["arin_rpki_bundle.test"].Primary.Attributes
			handles = map[string]string{"v4": a["roas.v4.handle"], "v6": a["roas.v6.handle"]}
			if handles["v4"] == "" || handles["v6"] == "" || handles["v4"] == handles["v6"] {
				return fmt.Errorf("bundle did not track distinct created ROAs")
			}
			return nil
		}},
		{ResourceName: "arin_rpki_bundle.test", ImportState: true, ImportStateVerify: true, ImportStateIdFunc: func(_ *terraform.State) (string, error) {
			b, err := json.Marshal(map[string]any{"org_handle": org, "name": name, "roas": handles, "aspas": []int64{asn}})
			return string(b), err
		}},
		{Config: created, PlanOnly: true},
		{Config: linked, Check: func(s *terraform.State) error {
			a := s.RootModule().Resources["arin_rpki_bundle.test"].Primary.Attributes
			for i, id := range routeIDs {
				route, err := c.GetIRRRoute(ctx, id)
				if err != nil {
					return err
				}
				label := []string{"v4", "v6"}[i]
				if route.AutoLinkedROAHandle != a["roas."+label+".handle"] {
					return fmt.Errorf("bundle IRR link does not match tracked ROA")
				}
			}
			return nil
		}},
		{Config: linked, PlanOnly: true},
		{Config: reduced, Check: func(_ *terraform.State) error {
			if _, err := c.GetIRRRoute(ctx, routeIDs[1]); !arin.IsNotFound(err) {
				return fmt.Errorf("removed member left linked route: %v", err)
			}
			return nil
		}},
		{Config: reduced, PlanOnly: true},
	}, CheckDestroy: checkDestroyed})
	// Destroy above removed the explicitly imported ASPA. Exercise actual Create
	// with both categories before cleanup restores the saved original ASPA.
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())},
		Steps:                    []resource.TestStep{{Config: created}, {Config: created, PlanOnly: true}},
		CheckDestroy:             checkDestroyed,
	})
	t.Log("native bundle create/import, combined additions/replacements, member removal, clean plans and destroy passed; cleanup restores the ASPA baseline")
}
