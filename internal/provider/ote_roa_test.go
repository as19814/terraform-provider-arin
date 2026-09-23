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
	"math/big"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func roaOTEPrefixes(t *testing.T, ctx context.Context, c *arin.Client, org string) []arin.ROAResource {
	t.Helper()
	networks, err := c.ListOrganizationNetworks(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	out := []arin.ROAResource{}
	for _, family := range []string{"v4", "v6"} {
		found := ""
		for _, network := range networks {
			if network.IPVersion != family || !strings.Contains(strings.ToLower(network.Type), "allocation") {
				continue
			}
			n, err := c.GetRegisteredNet(ctx, network.Handle)
			if err != nil {
				t.Fatal(err)
			}
			if n.OrgHandle != org {
				continue
			}
			for _, block := range n.Blocks {
				if block.Type != "DA" && block.Type != "A" {
					continue
				}
				start, end := netip.MustParseAddr(block.StartAddress), netip.MustParseAddr(block.EndAddress)
				low, high := new(big.Int).SetBytes(start.AsSlice()), new(big.Int).SetBytes(end.AsSlice())
				width := new(big.Int).Add(new(big.Int).Sub(high, low), big.NewInt(1))
				for attempt := 0; attempt < 10; attempt++ {
					offset, err := rand.Int(rand.Reader, width)
					if err != nil {
						t.Fatal(err)
					}
					address, _ := netip.AddrFromSlice(new(big.Int).Add(low, offset).FillBytes(make([]byte, start.BitLen()/8)))
					for _, spec := range arin.RegistrationReads() {
						if spec.Name != "parent_net" {
							continue
						}
						parent, err := c.ReadRegistration(ctx, spec, map[string]string{"start_address": address.String(), "end_address": address.String()})
						if err != nil {
							t.Fatal(err)
						}
						if parent["handle"] == network.Handle && parent["org_handle"] == org {
							found = netip.PrefixFrom(address, address.BitLen()).String()
						}
						break
					}
					if found != "" {
						break
					}
				}
				if found != "" {
					break
				}
			}
			if found != "" {
				break
			}
		}
		if found == "" {
			t.Fatalf("no owned free %s host prefix for OT&E ROA", family)
		}
		out = append(out, arin.ROAResource{Prefix: found})
	}
	return out
}
func TestOTEROALifecycle(t *testing.T) {
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
	name := fmt.Sprintf("tf-ote-roa-%x", nonce)
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
	first := roaConfig(org, name, asn, prefixes, false, false)
	as0 := roaConfig(org, name, 0, prefixes, false, false)
	linked := roaConfig(org, name, asn, prefixes, true, true)
	// A rename would complicate exact cleanup identification. Change max length
	// coverage in the client test; exercise origin and linking changes here.
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: []resource.TestStep{
		{Config: first},
		{ResourceName: "arin_roa.test", ImportState: true, ImportStateVerify: true},
		{Config: first, PlanOnly: true},
		{Config: as0, Check: resource.TestCheckResourceAttr("arin_roa.test", "asn", "0")},
		{Config: linked, Check: func(s *terraform.State) error {
			h := s.RootModule().Resources["arin_roa.test"].Primary.Attributes["handle"]
			for _, id := range routeIDs {
				route, err := c.GetIRRRoute(ctx, id)
				if err != nil {
					return err
				}
				if route.AutoLinkedROAHandle != h {
					return fmt.Errorf("IRR link does not match Terraform ROA handle")
				}
			}
			return nil
		}},
		{ResourceName: "arin_roa.test", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"delete_linked_routes"}},
		{Config: first, Check: func(_ *terraform.State) error {
			for _, id := range routeIDs {
				route, err := c.GetIRRRoute(ctx, id)
				if err != nil {
					return err
				}
				if route.AutoLinkedROAHandle != "" {
					return fmt.Errorf("old route remains linked after disabling auto_link")
				}
			}
			return nil
		}},
		{Config: linked},
		{Config: linked, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		for _, id := range routeIDs {
			if _, err := c.GetIRRRoute(ctx, id); !arin.IsNotFound(err) {
				return fmt.Errorf("linked route remains after Terraform destroy: %v", err)
			}
		}
		return nil
	}})
}
