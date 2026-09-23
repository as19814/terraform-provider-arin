package arin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// This test verifies a full-record PUT without requesting any account changes.
// It does not create an organization or assume staff-reviewed tickets complete.
func TestOTEOrganizationClientNoChange(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, handle := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || handle == "" {
		t.Fatal("requires sandbox key and organization handle")
	}
	c, err := New(Config{APIKey: key, BaseURL: OTEURL, RDAPBaseURL: RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	before, err := c.GetRegisteredOrganization(ctx, handle)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "terraform-provider-arin")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(handle))
	snapshot := filepath.Join(dir, fmt.Sprintf("ote-org-record-%x.json", hash[:8]))
	data, err := json.Marshal(before)
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
	pending := false
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		after, err := c.GetRegisteredOrganization(cleanup, handle)
		if err != nil {
			t.Errorf("cannot verify unchanged organization; retain snapshot %s: %v", snapshot, err)
			return
		}
		before.POCs = sortedOrgPOCs(before.POCs)
		after.POCs = sortedOrgPOCs(after.POCs)
		if !reflect.DeepEqual(before, after) {
			t.Errorf("organization differs after no-change PUT; retain snapshot %s", snapshot)
			return
		}
		if pending {
			t.Errorf("pending update requires reconciliation; retain snapshot %s", snapshot)
			return
		}
		if err = os.Remove(snapshot); err != nil {
			t.Error(err)
		}
	})
	out, err := c.UpdateOrganization(ctx, *before)
	if out != nil && out.TicketNumber != "" {
		pending = true
		recovery, encodeErr := json.Marshal(struct {
			Original *RegisteredOrganization
			Result   *OrganizationWriteResult
		}{before, out})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		if writeErr := os.WriteFile(snapshot, recovery, 0600); writeErr != nil {
			t.Fatalf("cannot persist pending ticket %s: %v", out.TicketNumber, writeErr)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	if out.TicketNumber != "" {
		t.Fatalf("no-change update generated a ticket; verify it before retrying: %s", out.TicketNumber)
	}
	if out.Organization == nil {
		t.Fatal("no-change update did not return the organization")
	}
}
