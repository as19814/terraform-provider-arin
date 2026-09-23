package arin

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOTEPOCClientLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key := os.Getenv("ARIN_OTE_API_KEY")
	if key == "" {
		t.Fatal("requires ARIN_OTE_API_KEY")
	}
	c, err := New(Config{APIKey: key, BaseURL: OTEURL, RDAPBaseURL: RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"ROLE", "PERSON"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			nonce := make([]byte, 5)
			if _, err := rand.Read(nonce); err != nil {
				t.Fatal(err)
			}
			p := examplePOC()
			p.ContactType = kind
			p.LastName = "Terraform Test " + strings.Map(func(r rune) rune {
				if r >= '0' && r <= '9' {
					return 'g' + r - '0'
				}
				return r
			}, fmt.Sprintf("%x", nonce))
			if kind == "PERSON" {
				p.FirstName = "Example"
			}
			out, err := c.CreatePOC(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			handle := out.Handle
			t.Logf("created disposable %s POC %s", kind, handle)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if err := c.DeletePOC(ctx, handle); err != nil {
					t.Errorf("cleanup POC %s: %v", handle, err)
				}
			})
			out.City = "Herndon"
			out.CompanyName = "Example Networks Updated"
			out.Comments = nil
			out.Emails = append(out.Emails, "abuse@example.net")
			out.Phones[0].Extension = "123"
			out.Phones = append(out.Phones, POCPhone{Type: "F", Number: "+1-202-555-0101", Extension: "42"})
			changed, err := c.UpdatePOC(ctx, *out)
			if err != nil {
				t.Fatal(err)
			}
			if changed.City != "Herndon" || changed.CompanyName != out.CompanyName || len(changed.Comments) != 0 || len(changed.Emails) != 2 || len(changed.Phones) != 2 {
				t.Fatal("POC update did not round-trip")
			}
			changed.Emails = changed.Emails[:1]
			changed.Phones = []POCPhone{{Type: "O", Number: "+1-202-555-0100"}}
			cleared, err := c.UpdatePOC(ctx, *changed)
			if err != nil {
				t.Fatal(err)
			}
			if len(cleared.Phones) != 1 || cleared.Phones[0].Extension != "" {
				t.Fatal("phone extension was not cleared")
			}
			if err = c.DeletePOC(ctx, handle); err != nil {
				t.Fatal(err)
			}
			if _, err = c.GetPOC(ctx, handle); !IsNotFound(err) {
				t.Fatalf("expected 404 after cleanup, got %v", err)
			}
		})
	}
}
