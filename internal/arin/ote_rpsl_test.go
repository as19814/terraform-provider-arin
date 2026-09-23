package arin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Uses only randomly named disposable sets. Failed writes are reconciled by a
// read in cleanup, never by sending the creation request again.
func TestOTERPSLClientLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and organization")
	}
	c, err := New(Config{APIKey: key, BaseURL: OTEURL, RDAPBaseURL: RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pocs, err := c.GetOrganizationPOCs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	admin, tech := "", ""
	for _, p := range pocs {
		if p.Function == "AD" {
			admin = p.Handle
		}
		if p.Function == "T" {
			tech = p.Handle
		}
	}
	if admin == "" || tech == "" {
		t.Fatal("test requires registered Admin and Tech contacts")
	}
	for _, kind := range []string{"as-set", "route-set"} {
		t.Run(kind, func(t *testing.T) {
			suffix := make([]byte, 8)
			if _, err := rand.Read(suffix); err != nil {
				t.Fatal(err)
			}
			prefix := "AS-"
			member := "AS64496"
			if kind == "route-set" {
				prefix = "RS-"
				member = "192.0.2.0/24"
			}
			name := prefix + "TF-OTE-RPSL-" + strings.ToUpper(hex.EncodeToString(suffix))
			identity := RPSLKey{Kind: kind, Name: name}
			if _, err := c.GetRPSL(ctx, identity); !IsNotFound(err) {
				t.Fatalf("absence not confirmed: %v", err)
			}
			t.Logf("Disposable advanced IRR object: %s/%s", kind, name)
			t.Cleanup(func() {
				cleanup, done := context.WithTimeout(context.Background(), time.Minute)
				defer done()
				if err := c.DeleteRPSL(cleanup, identity, org); err != nil {
					t.Errorf("cleanup of %s/%s: %v", kind, name, err)
					return
				}
				if _, err := c.GetRPSL(cleanup, identity); !IsNotFound(err) {
					t.Errorf("cleanup unconfirmed for %s/%s: %v", kind, name, err)
				}
			})
			raw := fmt.Sprintf("%s: %s\ndescr: Disposable Terraform RPSL test\nadmin-c: %s\ntech-c: %s\nmnt-by: MNT-%s\nmembers: %s\nsource: ARIN\n", kind, name, admin, tech, org, member)
			created, err := c.CreateRPSL(ctx, raw)
			if err != nil {
				t.Fatal(err)
			}
			if created.Key != identity || created.OrgHandle != org {
				t.Fatal("incorrect creation identity")
			}
			read, err := c.GetRPSL(ctx, identity)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(read.Text, "Disposable Terraform RPSL test") {
				t.Fatal("created description missing")
			}
			// Confirm the documented format boundary without mutating the object.
			path, _ := identity.path()
			_, xmlErr := c.get(ctx, c.baseURL, path, "application/xml", true)
			if xmlErr == nil {
				t.Log("sandbox also permits an XML read of this RPSL-created object")
			} else {
				t.Logf("XML read of advanced object: %v", xmlErr)
			}
			updated := strings.Replace(raw, "Disposable Terraform RPSL test", "Updated disposable RPSL test", 1) + "remarks: Test update\n"
			if _, err := c.UpdateRPSL(ctx, updated); err != nil {
				t.Fatal(err)
			}
			read, err = c.GetRPSL(ctx, identity)
			if err != nil || !strings.Contains(read.Text, "Updated disposable RPSL test") || !strings.Contains(read.Text, "Test update") {
				t.Fatalf("update not confirmed: %v", err)
			}
			if err := c.DeleteRPSL(ctx, identity, org); err != nil {
				t.Fatal(err)
			}
			if _, err := c.GetRPSL(ctx, identity); !IsNotFound(err) {
				t.Fatalf("deletion not confirmed: %v", err)
			}
		})
	}
}
