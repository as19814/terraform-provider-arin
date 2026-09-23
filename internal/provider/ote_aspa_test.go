package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func aspaSnapshotOrder(values []arin.ASPA) []arin.ASPA {
	out := slices.Clone(values)
	for i := range out {
		out[i].ProviderASNs = slices.Clone(out[i].ProviderASNs)
		slices.Sort(out[i].ProviderASNs)
	}
	slices.SortFunc(out, func(a, b arin.ASPA) int {
		if a.CustomerASN < b.CustomerASN {
			return -1
		}
		if a.CustomerASN > b.CustomerASN {
			return 1
		}
		return 0
	})
	return out
}
func roaSnapshotOrder(values []arin.ROA) []arin.ROA {
	out := slices.Clone(values)
	for i := range out {
		out[i].Resources = slices.Clone(out[i].Resources)
		slices.SortFunc(out[i].Resources, func(a, b arin.ROAResource) int { return strings.Compare(a.Prefix, b.Prefix) })
	}
	slices.SortFunc(out, func(a, b arin.ROA) int { return strings.Compare(a.Handle, b.Handle) })
	return out
}
func TestOTEASPALifecycle(t *testing.T) {
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
	before, err := c.ListASPAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) == 0 {
		t.Fatal("requires existing sandbox ASPA to snapshot and restore")
	}
	beforeROAs, err := c.ListROAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	original := before[0]
	changed := slices.DeleteFunc(slices.Clone(original.ProviderASNs), func(asn int64) bool { return asn == 0 })
	for _, candidate := range []int64{13335, 15169, 3356, 1299} {
		if candidate == original.CustomerASN || slices.Contains(changed, candidate) {
			continue
		}
		verified := false
		for _, spec := range arin.PublicReads() {
			if spec.Name == "asn" {
				if _, err := c.ReadRegistration(ctx, spec, map[string]string{"asn": strconv.FormatInt(candidate, 10)}); err == nil {
					verified = true
				}
				break
			}
		}
		if verified {
			changed = append(changed, candidate)
			break
		}
	}
	if slices.Equal(changed, original.ProviderASNs) {
		t.Fatal("no registered test provider ASN available")
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
		Org         string
		ROAs        []arin.ROA
		ASPAs       []arin.ASPA
		CustomerASN int64
	}{org, beforeROAs, before, original.CustomerASN})
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
		current, err := c.ListASPAs(ctx, org)
		if err != nil {
			t.Errorf("cannot read restoration inventory; retain %s: %v", snapshot, err)
			return
		}
		present, restored := false, false
		for _, a := range current {
			if a.CustomerASN == original.CustomerASN {
				present = true
				restored = arin.ASPAEqual(a, original)
			}
		}
		if !restored {
			txn := arin.RPKITransaction{AddASPAs: []arin.ASPA{original}}
			if present {
				txn.DeleteASPAs = []int64{original.CustomerASN}
			}
			if _, err = c.ApplyRPKITransaction(ctx, org, txn); err != nil {
				t.Logf("restoration response requires verification: %v", err)
			}
		}
		current, err = c.ListASPAs(ctx, org)
		if err != nil {
			t.Error(err)
			return
		}
		roas, err := c.ListROAs(ctx, org)
		if err != nil {
			t.Error(err)
			return
		}
		if !reflect.DeepEqual(aspaSnapshotOrder(current), aspaSnapshotOrder(before)) || !reflect.DeepEqual(roaSnapshotOrder(roas), roaSnapshotOrder(beforeROAs)) {
			t.Errorf("original RPKI inventories not restored; retain %s", snapshot)
			return
		}
		if err = os.Remove(snapshot); err != nil {
			t.Error(err)
		}
	})
	t.Setenv("ARIN_API_KEY", key)
	t.Setenv("ARIN_BASE_URL", arin.OTEURL)
	t.Setenv("ARIN_RDAP_BASE_URL", arin.RDAPOTEURL)
	first := aspaConfig(org, original.CustomerASN, original.ProviderASNs)
	next := aspaConfig(org, original.CustomerASN, changed)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: []resource.TestStep{
		{Config: first, ResourceName: "arin_aspa.test", ImportState: true, ImportStateId: fmt.Sprintf("%s/%d", org, original.CustomerASN), ImportStatePersist: true},
		{Config: next},
		{ResourceName: "arin_aspa.test", ImportState: true, ImportStateVerify: true},
		{Config: next, PlanOnly: true},
		{Config: aspaConfig(org, original.CustomerASN, []int64{0})},
		{Config: first},
	}, CheckDestroy: func(_ *terraform.State) error {
		current, err := c.ListASPAs(ctx, org)
		if err != nil {
			return err
		}
		for _, a := range current {
			if a.CustomerASN == original.CustomerASN {
				return fmt.Errorf("managed ASPA remains after destroy")
			}
		}
		return nil
	}})
}
