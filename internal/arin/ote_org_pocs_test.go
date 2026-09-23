package arin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func sortedOrgPOCs(p []OrgPOC) []OrgPOC {
	out := append([]OrgPOC{}, p...)
	sort.Slice(out, func(i, j int) bool { return out[i].Handle+"/"+out[i].Function < out[j].Handle+"/"+out[j].Function })
	return out
}
func TestOTEOrgPOCClientLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and org handle")
	}
	c, err := New(Config{APIKey: key, BaseURL: OTEURL, RDAPBaseURL: RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	before, err := c.GetOrganizationPOCs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 5)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	p := examplePOC()
	p.LastName = fmt.Sprintf("Terraform Org Link %x", nonce)
	created, err := c.CreatePOC(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	handle := created.Handle
	backup := ""
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		current, err := c.GetOrganizationPOCs(ctx, org)
		if err != nil {
			t.Errorf("cannot verify org links; recovery file %s: %v", backup, err)
			return
		}
		for _, link := range current {
			if link.Handle == handle {
				if _, err = c.RemoveOrganizationPOC(ctx, org, handle, link.Function); err != nil {
					t.Errorf("cannot remove disposable POC link; recovery file %s: %v", backup, err)
					return
				}
			}
		}
		current, err = c.GetOrganizationPOCs(ctx, org)
		if err != nil || !reflect.DeepEqual(sortedOrgPOCs(current), sortedOrgPOCs(before)) {
			t.Errorf("original organization links not restored; recovery file %s: %v", backup, err)
			return
		}
		if err = c.DeletePOC(ctx, handle); err != nil {
			t.Errorf("cannot delete disposable POC %s; recovery file %s: %v", handle, backup, err)
			return
		}
		if backup != "" {
			if err = os.Remove(backup); err != nil {
				t.Error(err)
			}
		}
	})
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "terraform-provider-arin")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(org))
	backup = filepath.Join(dir, fmt.Sprintf("ote-org-pocs-%x.json", hash[:8]))
	data, err := json.Marshal(struct {
		Org, POC string
		Original []OrgPOC
	}{org, handle, before})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		backup = ""
		t.Fatalf("cannot create exclusive org recovery snapshot: %v", err)
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
	for _, function := range []string{"T", "N", "AB", "R", "D"} {
		_, err := c.AddOrganizationPOC(ctx, org, handle, function)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("sandbox accepted %s role", function)
		if _, err = c.RemoveOrganizationPOC(ctx, org, handle, function); err != nil {
			t.Fatal(err)
		}
	}
	for _, function := range []string{"T", "N", "AB"} {
		if _, err = c.AddOrganizationPOC(ctx, org, handle, function); err != nil {
			t.Fatal(err)
		}
	}

	for _, suffix := range []string{handle, handle + ";pocFunction="} {
		_, err := c.request(ctx, http.MethodDelete, c.baseURL, "/rest/org/"+org+"/poc/"+suffix, "application/xml", true, nil)
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 || apiErr.Code != "E_BAD_REQUEST" {
			t.Fatalf("unexpected handle-only removal result: %v", err)
		}
	}
	for _, function := range []string{"T", "N", "AB"} {
		if _, err = c.RemoveOrganizationPOC(ctx, org, handle, function); err != nil {
			t.Fatal(err)
		}
	}
	role := ""
	for _, candidate := range []string{"R", "D", "N"} {
		found := false
		for _, p := range before {
			if p.Function == candidate {
				found = true
			}
		}
		if !found {
			role = candidate
			break
		}
	}
	if role != "" {
		if _, err = c.AddOrganizationPOC(ctx, org, handle, role); err != nil {
			t.Fatal(err)
		}
		for _, part := range []string{"/poc/;pocFunction=", "/poc;pocFunction="} {
			_, err := c.request(ctx, http.MethodDelete, c.baseURL, "/rest/org/"+org+part+role, "application/xml", true, nil)
			var apiErr *APIError
			if !errors.As(err, &apiErr) || (apiErr.StatusCode != 400 && apiErr.StatusCode != 404) {
				t.Fatalf("unexpected function-only removal result: %v", err)
			}
			t.Logf("function-only route %s rejected with %d", part, apiErr.StatusCode)
		}
	}
}
