package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
)

func TestOTEDelegationClientLifecycle(t *testing.T) {
	for _, family := range []string{"v4", "v6"} {
		t.Run(family, func(t *testing.T) { testOTEDelegationClientLifecycle(t, family) })
	}
}
func oteDelegationSnapshot(t *testing.T, family string) (*arin.Client, string) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	networks, err := client.ListOrganizationNetworks(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	var original *arin.Delegation
	for _, net := range networks {
		if net.IPVersion != family {
			continue
		}
		detail, e := client.GetRegisteredNet(ctx, net.Handle)
		if e != nil {
			t.Fatal(e)
		}
		if detail.OrgHandle != org {
			continue
		}
		direct := false
		for _, b := range detail.Blocks {
			if b.Type == "DA" {
				direct = true
			}
		}
		if !direct {
			continue
		}
		for _, spec := range arin.RegistrationReads() {
			if spec.Name != "net_delegations" {
				continue
			}
			values, e := client.ReadRegistration(ctx, spec, map[string]string{"net_handle": net.Handle})
			if e != nil {
				t.Fatal(e)
			}
			for _, item := range values["delegations"].([]any) {
				record := item.(map[string]any)
				name, _ := record["name"].(string)
				d, e := client.GetDelegation(ctx, strings.ToLower(strings.TrimSuffix(name, "."))+".")
				if e != nil {
					t.Fatal(e)
				}
				if original == nil {
					original = d
				}
				if len(d.Nameservers) == 0 && len(d.DSRecords) == 0 {
					original = d
					break
				}
			}
		}
		if original != nil {
			break
		}
	}
	if original == nil {
		t.Fatal("no delegation found on an owned direct allocation")
	}
	zone := original.Name
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "terraform-provider-arin")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(zone))
	backup := filepath.Join(dir, fmt.Sprintf("ote-delegation-%x.json", sum[:8]))
	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatalf("cannot create recovery copy, investigate any existing backup before retrying: %s: %v", backup, err)
	}
	if _, err = file.Write(data); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Sync(); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		current, e := client.GetDelegation(ctx, zone)
		if e != nil || !reflect.DeepEqual(current, original) {
			if _, e = client.UpdateDelegation(ctx, *original); e != nil {
				t.Errorf("restoration failed; recovery copy %s: %v", backup, e)
				return
			}
			current, e = client.GetDelegation(ctx, zone)
		}
		if e != nil || !reflect.DeepEqual(current, original) {
			t.Errorf("restoration not confirmed; recovery copy %s: %v", backup, e)
			return
		}
		if e = os.Remove(backup); e != nil {
			t.Errorf("restored delegation, could not remove backup: %v", e)
		}
	})
	t.Logf("OT&E delegation: %s (original nameservers=%d DS=%d)", zone, len(original.Nameservers), len(original.DSRecords))
	return client, zone
}
func testOTEDelegationClientLifecycle(t *testing.T, family string) {
	client, zone := oteDelegationSnapshot(t, family)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	ttl := int64(3600)
	desired := arin.Delegation{Name: zone, Nameservers: []arin.DelegationNameserver{{Name: "ns1.example.net", TTL: &ttl}, {Name: "ns2.example.net"}}}
	changed, err := client.UpdateDelegation(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range changed.Nameservers {
		if n.Name == "ns1.example.net" && (n.TTL == nil || *n.TTL != ttl) {
			t.Fatal("explicit nameserver TTL did not round-trip")
		}
		if n.Name == "ns2.example.net" && n.TTL != nil {
			t.Fatal("inherited nameserver TTL changed")
		}
	}
	if len(changed.Nameservers) != 2 {
		t.Fatal("nameserver replacement failed")
	}
	ds := arin.DelegationDS{Algorithm: 13, DigestType: 2, KeyTag: 12345, Digest: strings.Repeat("AB", 32), TTL: &ttl}
	desired.DSRecords = []arin.DelegationDS{ds}
	changed, err = client.UpdateDelegation(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.DSRecords) != 1 || changed.DSRecords[0].Digest != ds.Digest || changed.DSRecords[0].TTL == nil || *changed.DSRecords[0].TTL != ttl {
		t.Fatal("DS update failed")
	}
	desired.DSRecords[0].TTL = nil
	changed, err = client.UpdateDelegation(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.DSRecords) != 1 || changed.DSRecords[0].TTL == nil {
		t.Fatal("omitted DS TTL response is incomplete")
	}
	if *changed.DSRecords[0].TTL != ttl {
		t.Fatal("omitting an existing DS TTL did not preserve it")
	}
	desired.DSRecords[0].KeyTag = 12346
	changed, err = client.UpdateDelegation(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.DSRecords) != 1 || changed.DSRecords[0].KeyTag != 12346 || changed.DSRecords[0].TTL != nil {
		t.Fatal("new DS record did not retain inherited TTL")
	}
	desired.DSRecords[0].KeyTag = 12345
	desired.DSRecords[0].TTL = &ttl
	changed, err = client.UpdateDelegation(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	updatedTTL := int64(7200)
	changed, err = client.SetDelegationNameserver(ctx, zone, arin.DelegationNameserver{Name: "ns1.example.net", TTL: &updatedTTL})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Nameservers) != 2 || len(changed.DSRecords) != 1 {
		t.Fatal("single-NS update lost other records")
	}
	for _, n := range changed.Nameservers {
		if n.Name == "ns1.example.net" && (n.TTL == nil || *n.TTL != updatedTTL) {
			t.Fatal("nameserver TTL update failed")
		}
	}
	if changed.DSRecords[0].TTL == nil || *changed.DSRecords[0].TTL != ttl {
		t.Fatal("nameserver update changed DS TTL")
	}
	changed, err = client.SetDelegationNameserver(ctx, zone, arin.DelegationNameserver{Name: "ns1.example.net"})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range changed.Nameservers {
		if n.Name == "ns1.example.net" && n.TTL != nil {
			t.Fatal("nameserver TTL inheritance not restored")
		}
	}
	changed, err = client.SetDelegationNameserver(ctx, zone, arin.DelegationNameserver{Name: "ns3.example.net", TTL: &updatedTTL})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Nameservers) != 3 {
		t.Fatal("single nameserver addition failed")
	}
	changed, err = client.DeleteDelegationNameserver(ctx, zone, "ns3.example.net")
	if err != nil {
		t.Fatal(err)
	}
	changed, err = client.DeleteDelegationNameserver(ctx, zone, "ns2.example.net")
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Nameservers) != 1 || len(changed.DSRecords) != 1 {
		t.Fatal("single-NS deletion failed")
	}
	changed, err = client.UpdateDelegation(ctx, arin.Delegation{Name: zone, Nameservers: changed.Nameservers})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.DSRecords) != 0 {
		t.Fatal("DS clearing failed")
	}
	changed, err = client.DeleteDelegationNameservers(ctx, zone)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Nameservers) != 0 || len(changed.DSRecords) != 0 {
		t.Fatal("delegation clearing failed")
	}
	changed, err = client.UpdateDelegation(ctx, arin.Delegation{Name: zone})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed.Nameservers) != 0 || len(changed.DSRecords) != 0 {
		t.Fatal("empty full update did not clear the delegation")
	}

}
