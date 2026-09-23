package arin

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func bundleROA(name, prefix string) RPKIBundleROA {
	return RPKIBundleROA{Request: ROARequest{Name: name, ASN: 64496, Resources: []ROAResource{{Prefix: prefix}}}}
}
func bundleActual(handle string, want RPKIBundleROA) ROA {
	return ROA{Handle: handle, Name: want.Request.Name, ASN: want.Request.ASN, Resources: bundleRequestedResources(want.Request)}
}
func bundleScenario() (RPKIBundleOwnership, RPKIBundleDesired, RPKIBundleInventory) {
	prior := RPKIBundleOwnership{ROAs: map[string]RPKIBundleOwnedROA{
		"keep": {Handle: "keep", DeleteLinkedRoutes: false}, "change": {Handle: "old-change", DeleteLinkedRoutes: true}, "remove": {Handle: "remove", DeleteLinkedRoutes: true}, "missing": {Handle: "missing"},
	}, ASPAs: []int64{64496, 64497, 64501}}
	desired := RPKIBundleDesired{ROAs: map[string]RPKIBundleROA{
		"keep": bundleROA("Keep", "192.0.2.0/26"), "change": bundleROA("Changed", "192.0.2.64/26"), "missing": bundleROA("Missing", "192.0.2.128/26"), "add": bundleROA("Added", "2001:db8::/48"),
	}, ASPAs: map[int64]ASPA{64496: {CustomerASN: 64496, ProviderASNs: []int64{64510}}, 64501: {CustomerASN: 64501, ProviderASNs: []int64{64510}}, 64502: {CustomerASN: 64502, ProviderASNs: []int64{64510}}}}
	keep := desired.ROAs["keep"]
	keep.DeleteLinkedRoutes = true
	desired.ROAs["keep"] = keep
	inventory := RPKIBundleInventory{ROAs: []ROA{
		bundleActual("keep", desired.ROAs["keep"]), bundleActual("old-change", bundleROA("Before change", "192.0.2.64/26")), bundleActual("remove", bundleROA("Removed", "192.0.2.192/26")), bundleActual("unrelated", bundleROA("Unrelated", "198.51.100.0/24")),
	}, ASPAs: []ASPA{{CustomerASN: 64496, ProviderASNs: []int64{64511}}, {CustomerASN: 64497, ProviderASNs: []int64{64511}}, {CustomerASN: 64500, ProviderASNs: []int64{64512}}}}
	return prior, desired, inventory
}
func completedBundle(plan *RPKIBundlePlan, before RPKIBundleInventory) RPKIBundleInventory {
	out := RPKIBundleInventory{}
	for _, r := range before.ROAs {
		if !slices.ContainsFunc(plan.Transaction.DeleteROAs, func(d ROADelete) bool { return d.Handle == r.Handle }) {
			out.ROAs = append(out.ROAs, r)
		}
	}
	for _, label := range plan.AddedROALabels {
		out.ROAs = append(out.ROAs, bundleActual("new-"+label, plan.Desired.ROAs[label]))
	}
	for _, a := range before.ASPAs {
		if !slices.Contains(plan.Transaction.DeleteASPAs, a.CustomerASN) {
			out.ASPAs = append(out.ASPAs, a)
		}
	}
	out.ASPAs = append(out.ASPAs, plan.Transaction.AddASPAs...)
	return out
}

func TestRPKIBundlePlanningAndRecovery(t *testing.T) {
	prior, desired, before := bundleScenario()
	plan, err := PlanRPKIBundle(prior, desired, before)
	if err != nil {
		t.Fatal(err)
	}
	tx := plan.Transaction
	if len(tx.AddROAs) != 3 || len(tx.DeleteROAs) != 2 || len(tx.AddASPAs) != 3 || len(tx.DeleteASPAs) != 2 {
		t.Fatalf("wrong combined changes: %+v", tx)
	}
	if plan.RetainedROAs["keep"] != "keep" {
		t.Fatal("policy-only change replaced a ROA")
	}
	for _, d := range tx.DeleteROAs {
		if d.Handle == "old-change" && d.AutoLink {
			t.Fatal("replacement deletes linked routes")
		}
		if d.Handle == "remove" && !d.AutoLink {
			t.Fatal("removal ignored saved route deletion policy")
		}
		if d.Handle == "unrelated" {
			t.Fatal("unrelated ROA scheduled for deletion")
		}
	}
	if slices.Contains(tx.DeleteASPAs, int64(64500)) {
		t.Fatal("unrelated ASPA scheduled for deletion")
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var recovered RPKIBundlePlan
	if err := json.Unmarshal(encoded, &recovered); err != nil {
		t.Fatal(err)
	}
	result, err := ReconcileRPKIBundle(&recovered, completedBundle(plan, before))
	if err != nil || len(result.ROAs) != 4 || len(result.ASPAs) != 3 || result.ROAs["change"].Handle != "new-change" {
		t.Fatalf("complete bundle did not recover: %v", err)
	}
	if _, ok := result.ROAs["unrelated"]; ok {
		t.Fatal("unrelated ROA adopted")
	}
	if _, ok := result.ASPAs[64500]; ok {
		t.Fatal("unrelated ASPA adopted")
	}
	// Deterministic request bytes must not depend on inventory iteration order.
	slices.Reverse(before.ROAs)
	slices.Reverse(before.ASPAs)
	slices.Reverse(prior.ASPAs)
	repeated, err := PlanRPKIBundle(prior, desired, before)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := plan.Transaction.marshal()
	second, _ := repeated.Transaction.marshal()
	if string(first) != string(second) {
		t.Fatal("transaction ordering is unstable")
	}
}

func TestRPKIBundleOwnershipGuards(t *testing.T) {
	for _, mode := range []string{"roa", "aspa", "duplicate-owner", "duplicate-roa", "duplicate-aspa", "duplicate-prefix", "wrong-customer-key"} {
		t.Run(mode, func(t *testing.T) {
			prior, desired, before := bundleScenario()
			switch mode {
			case "roa":
				delete(prior.ROAs, "change")
			case "aspa":
				prior.ASPAs = []int64{64497, 64501}
			case "duplicate-owner":
				prior.ROAs["alias"] = prior.ROAs["keep"]
			case "duplicate-roa":
				before.ROAs = append(before.ROAs, before.ROAs[0])
			case "duplicate-aspa":
				before.ASPAs = append(before.ASPAs, before.ASPAs[0])
			case "duplicate-prefix":
				desired.ROAs["alias"] = desired.ROAs["keep"]
			case "wrong-customer-key":
				desired.ASPAs[123] = desired.ASPAs[64496]
			}
			if _, err := PlanRPKIBundle(prior, desired, before); err == nil {
				t.Fatal("invalid ownership or inventory accepted")
			}
		})
	}
}

func TestRPKIBundleRecoveryIncomplete(t *testing.T) {
	for _, mode := range []string{"missing-addition", "surviving-old-roa", "wrong-aspa", "ambiguous", "baseline-outsider", "changed-retained", "unowned-delete", "changed-policy", "unknown-label", "unowned-aspa-delete", "changed-aspa-request", "missing-aspa-replacement"} {
		t.Run(mode, func(t *testing.T) {
			prior, desired, before := bundleScenario()
			plan, err := PlanRPKIBundle(prior, desired, before)
			if err != nil {
				t.Fatal(err)
			}
			after := completedBundle(plan, before)
			switch mode {
			case "missing-addition":
				after.ROAs = slices.DeleteFunc(after.ROAs, func(r ROA) bool { return r.Handle == "new-add" })
			case "surviving-old-roa":
				after.ROAs = append(after.ROAs, before.ROAs[1])
			case "wrong-aspa":
				for i := range after.ASPAs {
					if after.ASPAs[i].CustomerASN == 64496 {
						after.ASPAs[i].ProviderASNs = []int64{64511}
					}
				}
			case "ambiguous":
				after.ROAs = append(after.ROAs, bundleActual("other-new", desired.ROAs["add"]))
			case "baseline-outsider":
				after.ROAs = slices.DeleteFunc(after.ROAs, func(r ROA) bool { return r.Handle == "new-add" || r.Handle == "unrelated" })
				after.ROAs = append(after.ROAs, bundleActual("unrelated", desired.ROAs["add"]))
			case "changed-retained":
				after.ROAs[0].Name = "Changed elsewhere"
			case "unowned-delete":
				plan.Transaction.DeleteROAs = append(plan.Transaction.DeleteROAs, ROADelete{Handle: "unrelated"})
			case "changed-policy":
				plan.Transaction.DeleteROAs[0].AutoLink = true
			case "unknown-label":
				plan.AddedROALabels[0] = "not-configured"
			case "unowned-aspa-delete":
				plan.Transaction.DeleteASPAs = append(plan.Transaction.DeleteASPAs, 64500)
			case "missing-aspa-replacement":
				plan.Transaction.AddASPAs = slices.DeleteFunc(plan.Transaction.AddASPAs, func(a ASPA) bool { return a.CustomerASN == 64496 })
			case "changed-aspa-request":
				plan.Transaction.AddASPAs[0].ProviderASNs = []int64{64520}
			}
			if _, err := ReconcileRPKIBundle(plan, after); err == nil {
				t.Fatal("incomplete or invalid recovery accepted")
			}
		})
	}
}

func TestRPKIBundleNoOpDestroyAndIsolation(t *testing.T) {
	want := bundleROA("Owned", "192.0.2.0/24")
	prior := RPKIBundleOwnership{ROAs: map[string]RPKIBundleOwnedROA{"owned": {Handle: "owned", DeleteLinkedRoutes: true}}, ASPAs: []int64{64496}}
	desired := RPKIBundleDesired{ROAs: map[string]RPKIBundleROA{"owned": want}, ASPAs: map[int64]ASPA{64496: {CustomerASN: 64496, ProviderASNs: []int64{64501}}}}
	before := RPKIBundleInventory{ROAs: []ROA{bundleActual("owned", want)}, ASPAs: []ASPA{desired.ASPAs[64496]}}
	plan, err := PlanRPKIBundle(prior, desired, before)
	if err != nil || !plan.Empty() {
		t.Fatalf("unchanged bundle creates a transaction: %v", err)
	}
	if _, err := ReconcileRPKIBundle(plan, before); err != nil {
		t.Fatal(err)
	}
	destroy, err := PlanRPKIBundle(prior, RPKIBundleDesired{}, before)
	if err != nil || len(destroy.Transaction.DeleteROAs) != 1 || !destroy.Transaction.DeleteROAs[0].AutoLink || len(destroy.Transaction.DeleteASPAs) != 1 {
		t.Fatalf("wrong destroy transaction: %v", err)
	}
	if _, err := ReconcileRPKIBundle(destroy, before); err == nil {
		t.Fatal("destroy accepted before absence")
	}
	if _, err := ReconcileRPKIBundle(destroy, RPKIBundleInventory{}); err != nil {
		t.Fatal(err)
	}
	// The caller's mutable inputs must not change the captured transaction.
	max := int64(24)
	want.Request.Resources[0].MaxLength = &max
	desired.ROAs["owned"] = want
	planned, err := PlanRPKIBundle(RPKIBundleOwnership{}, desired, RPKIBundleInventory{})
	if err != nil {
		t.Fatal(err)
	}
	max = 25
	desired.ASPAs[64496].ProviderASNs[0] = 64502
	if *planned.Transaction.AddROAs[0].Resources[0].MaxLength != 24 || !reflect.DeepEqual(planned.Transaction.AddASPAs[0].ProviderASNs, []int64{64501}) {
		t.Fatal("caller changed captured transaction")
	}
}

func TestRPKIBundleReplacementReusesOwnedHandle(t *testing.T) {
	prior, desired, before := bundleScenario()
	plan, err := PlanRPKIBundle(prior, desired, before)
	if err != nil {
		t.Fatal(err)
	}
	after := completedBundle(plan, before)
	for i := range after.ROAs {
		if after.ROAs[i].Handle == "new-change" {
			after.ROAs[i].Handle = "old-change"
		}
	}
	state, err := ReconcileRPKIBundle(plan, after)
	if err != nil {
		t.Fatal(err)
	}
	if state.ROAs["change"].Handle != "old-change" {
		t.Fatal("replacement lost its reused owned handle")
	}
}
