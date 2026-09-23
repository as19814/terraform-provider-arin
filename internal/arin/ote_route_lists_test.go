package arin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Run inside the disposable organization-recipient NET fixture, before that NET
// is deleted. Persist route identity before POST so interrupted probes are visible.
func auditOTEDownstreamRoutes(t *testing.T, ctx context.Context, c *Client, org, parent, child, prefix, name string) {
	t.Helper()
	route := IRRRoute{Prefix: prefix, OriginAS: "AS64496", OrgHandle: org, Description: []string{name}}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(org))
	path := filepath.Join(cache, "terraform-provider-arin", fmt.Sprintf("ote-route-lists-%x.json", hash[:8]))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("route-list receipt must be reconciled before retry: %v", err)
	}
	receipt := struct {
		Route         IRRRoute
		Parent, Child string
	}{route, parent, child}
	err = json.NewEncoder(f).Encode(receipt)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("persist route-list receipt: %v %v", err, closeErr)
	}
	// Establish absence before taking ownership. A preexisting route is never deleted.
	if _, err := c.GetIRRRoute(ctx, route.ID()); !IsNotFound(err) {
		if err == nil {
			_ = os.Remove(path)
		}
		t.Fatalf("route-list probe requires absent route: %v", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		actual, err := c.GetIRRRoute(cleanup, route.ID())
		if !IsNotFound(err) {
			if err != nil || actual.OrgHandle != org || actual.NetHandle != child || actual.AutoLinkedROAHandle != "" || !slices.Equal(actual.Description, route.Description) {
				t.Errorf("route-list cleanup identity unconfirmed; receipt retained: %v", err)
				return
			}
			if err := c.DeleteIRRRoute(cleanup, route.ID()); err != nil {
				t.Errorf("route-list cleanup: %v", err)
				return
			}
			if _, err := c.GetIRRRoute(cleanup, route.ID()); !IsNotFound(err) {
				t.Errorf("route-list deletion unconfirmed: %v", err)
				return
			}
		}
		if err := os.Remove(path); err != nil {
			t.Error(err)
		}
	}()
	created, err := c.CreateIRRRoute(ctx, route)
	if err != nil {
		t.Fatal(err)
	}
	if created.NetHandle != child {
		t.Fatalf("route attached to %s instead of disposable child %s", created.NetHandle, child)
	}
	for _, tc := range []struct {
		name, spec, handle, include string
		want                        bool
	}{
		{"child-direct", "net_routes", child, "false", true},
		{"parent-direct", "net_routes", parent, "false", false},
		{"parent-downstream", "net_routes", parent, "true", true},
		{"organization", "irr_routes", org, "", true},
	} {
		var spec ReadSpec
		for _, candidate := range RegistrationReads() {
			if candidate.Name == tc.spec {
				spec = candidate
				break
			}
		}
		if spec.Name == "" {
			t.Fatalf("missing read spec %s", tc.spec)
		}
		values, err := c.ReadRegistration(ctx, spec, map[string]string{"net_handle": tc.handle, "org_handle": tc.handle, "include_reassignments": tc.include})
		if err != nil {
			t.Fatalf("%s list: %v", tc.name, err)
		}
		found := false
		for _, raw := range values["routes"].([]any) {
			row := raw.(map[string]any)
			if row["prefix"] == prefix && row["origin_as"] == route.OriginAS {
				if row["org_handle"] != org || row["entry_type"] != "SIMPLE" {
					t.Fatalf("%s route reference metadata mismatch", tc.name)
				}
				found = true
			}
		}
		if found != tc.want {
			t.Fatalf("%s contains disposable child route=%t, want %t", tc.name, found, tc.want)
		}
		t.Logf("%s contains disposable child route=%t", tc.name, found)
	}
}
