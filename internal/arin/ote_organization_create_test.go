package arin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Creation can require staff review, so this probe has a separate opt-in and an
// exclusive recovery journal. It must never silently submit a second request.
func TestOTEOrganizationCreateProbe(t *testing.T) {
	if os.Getenv("ARIN_OTE_ORG_CREATE_PROBE") != "1" || os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit organization creation probe opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and organization handle")
	}
	c, err := New(Config{APIKey: key, BaseURL: OTEURL, RDAPBaseURL: RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	before, err := c.GetRegisteredOrganization(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 5)
	if _, err = rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	input := exampleOrganization()
	input.Handle = ""
	input.RegistrationDate = ""
	input.TaxID = ""
	input.Name = fmt.Sprintf("Terraform OTE Organization %x", nonce)
	input.POCs = before.POCs
	record := struct {
		Input             RegisteredOrganization
		Result            *OrganizationWriteResult
		Deletion          *OrganizationWriteResult
		DeletionAttempted bool
	}{Input: input}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "terraform-provider-arin")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(org))
	journal := filepath.Join(dir, fmt.Sprintf("ote-org-create-%x.json", hash[:8]))
	initial, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(journal, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("creation recovery journal already exists or cannot be created: %v", err)
	}
	if _, err = f.Write(initial); err != nil {
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
	t.Logf("creation recovery journal: %s", journal)
	out, writeErr := c.CreateOrganization(ctx, input)
	record.Result = out
	persist := func() error {
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		temp, err := os.CreateTemp(dir, "ote-org-create-result-*.json")
		if err != nil {
			return err
		}
		defer os.Remove(temp.Name())
		if _, err = temp.Write(data); err != nil {
			temp.Close()
			return err
		}
		if err = temp.Sync(); err != nil {
			temp.Close()
			return err
		}
		if err = temp.Close(); err != nil {
			return err
		}
		return os.Rename(temp.Name(), journal)
	}
	if err = persist(); err != nil {
		t.Fatal(err)
	}
	if out != nil && out.Organization != nil && out.Organization.Handle != "" && out.Organization.Handle != org && out.Organization.Name == input.Name {
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if out.TicketNumber != "" {
				ticket, err := c.GetRegistrationTicket(cleanup, out.TicketNumber)
				if err != nil || !ticket.Terminal() {
					t.Errorf("creation ticket remains unresolved; retain journal %s", journal)
					return
				}
			}
			record.DeletionAttempted = true
			if err := persist(); err != nil {
				t.Error(err)
				return
			}
			deleted, err := c.DeleteOrganization(cleanup, out.Organization.Handle)
			record.Deletion = deleted
			if saveErr := persist(); saveErr != nil {
				t.Errorf("cannot persist deletion result; retain journal %s: %v", journal, saveErr)
				return
			}
			if err != nil || (deleted != nil && deleted.TicketNumber != "") {
				t.Errorf("organization cleanup requires recovery from %s: %v", journal, err)
				return
			}
			if err = os.Remove(journal); err != nil {
				t.Error(err)
			}
		})
	}
	if writeErr != nil {
		t.Fatalf("creation failed; inspect journal before retrying: %v", writeErr)
	}
	if out.TicketNumber != "" {
		t.Logf("creation returned ticket status %s; retained recovery journal", out.TicketStatus)
		return
	}
	if out.Organization == nil || out.Organization.Handle == org || out.Organization.Name != input.Name {
		t.Fatal("creation did not return the expected disposable organization; retain journal")
	}
	t.Log("creation returned a complete organization; cleanup will delete it")
}
