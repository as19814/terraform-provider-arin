package provider

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type orgFake struct {
	mu                                          sync.Mutex
	body                                        string
	creates, updates, deletes, orgReads         int
	readStatus, writeStatus                     int
	pendingCreate, pendingUpdate, pendingDelete bool
	ticketStatus, ticketResolution              string
	ignoreDelete                                bool
}

func (f *orgFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml")
	if r.Header.Get("Authorization") != "ApiKey test-key" {
		w.WriteHeader(403)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/rest/ticket/") {
		fmt.Fprintf(w, `<ticket xmlns="http://www.arin.net/regrws/core/v1" xmlns:s="http://www.arin.net/regrws/shared-ticket/v1"><ticketNo>20260922-X1</ticketNo><webTicketStatus>%s</webTicketStatus><webTicketResolution>%s</webTicketResolution><s:orgHandle>UNRELATED-1</s:orgHandle></ticket>`, f.ticketStatus, f.ticketResolution)
		return
	}
	if r.Method == "GET" {
		f.orgReads++
		if f.readStatus != 0 {
			w.WriteHeader(f.readStatus)
			return
		}
		if f.body == "" {
			w.WriteHeader(404)
			return
		}
		fmt.Fprint(w, f.body)
		return
	}
	if f.writeStatus != 0 {
		w.WriteHeader(f.writeStatus)
		return
	}
	pending := false
	switch r.Method {
	case "POST":
		f.creates++
		b, _ := io.ReadAll(r.Body)
		f.body = strings.Replace(string(b), "</org>", "<handle>ORG-TEST</handle><registrationDate>2026-09-22T00:00:00Z</registrationDate></org>", 1)
		pending = f.pendingCreate
	case "PUT":
		f.updates++
		b, _ := io.ReadAll(r.Body)
		f.body = string(b)
		pending = f.pendingUpdate
	case "DELETE":
		f.deletes++
		pending = f.pendingDelete
		if !pending && !f.ignoreDelete {
			f.body = ""
		}
		if !pending {
			w.WriteHeader(204)
			return
		}
	default:
		w.WriteHeader(405)
		return
	}
	if pending {
		w.WriteHeader(202)
		fmt.Fprint(w, `<ticket xmlns="http://www.arin.net/regrws/core/v1"><ticketNo>20260922-X1</ticketNo><webTicketStatus>PENDING_REVIEW</webTicketStatus></ticket>`)
		return
	}
	fmt.Fprint(w, f.body)
}
func setupOrgFake(t *testing.T) (*orgFake, *arin.Client) {
	t.Helper()
	f := &orgFake{ticketStatus: "PENDING_REVIEW", ticketResolution: ""}
	s := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(s.Close)
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", s.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	c, err := arin.New(arin.Config{APIKey: "test-key", BaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}
func orgConfig(name, city, extras string) string {
	return fmt.Sprintf(`provider "arin" {}
resource "arin_org" "test" {
 name = %q
 country_code = "US"
 city = %q
 subdivision = "VA"
 postal_code = "20151"
 street_address = ["123 Example Street"]
 poc_links = [
  { handle = "ADMIN-ARIN", function = "AD" },
  { handle = "TECH-ARIN", function = "T" },
  { handle = "ABUSE-ARIN", function = "AB" }
 ]
 %s
}
`, name, city, extras)
}
func TestAccOrgResourceLifecycle(t *testing.T) {
	f, _ := setupOrgFake(t)
	first := orgConfig("Example Organization", "Original", `comments = ["Original comment"]
 tax_id = "test-tax-id"
 rwhois_url = "rwhois.example.net:4321"`)
	changed := orgConfig("Example Organization", "Changed", `accept_reassignments = false`)
	replaced := orgConfig("Replacement Organization", "Changed", `accept_reassignments = false`)
	allRoles := strings.Replace(changed, "ADMIN-ARIN", "SECOND-ARIN", 1)
	allRoles = strings.Replace(allRoles, `{ handle = "ABUSE-ARIN", function = "AB" }`, `{ handle = "ABUSE-ARIN", function = "AB" },
 { handle = "NOC-ARIN", function = "N" },
 { handle = "ROUTE-ARIN", function = "R" },
 { handle = "DNS-ARIN", function = "D" }`, 1)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: first, Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("arin_org.test", "id", "ORG-TEST"), resource.TestCheckResourceAttr("arin_org.test", "pending_operation", ""))},
		{ResourceName: "arin_org.test", ImportState: true, ImportStateVerify: true},
		{Config: changed, Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("arin_org.test", "city", "Changed"), resource.TestCheckResourceAttr("arin_org.test", "comments.#", "0"), resource.TestCheckResourceAttr("arin_org.test", "tax_id", ""), resource.TestCheckResourceAttr("arin_org.test", "accept_reassignments", "false"))},
		{Config: allRoles, Check: resource.TestCheckResourceAttr("arin_org.test", "poc_links.#", "6")},
		{Config: changed},
		{Config: changed, PlanOnly: true},
		{Config: changed, PreConfig: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.body = strings.Replace(f.body, "<city>Changed</city>", "<city>Drift</city>", 1)
		}},
		{Config: replaced},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.body != "" || f.creates != 2 || f.deletes != 2 {
			return fmt.Errorf("unexpected org lifecycle counts: creates=%d deletes=%d", f.creates, f.deletes)
		}
		return nil
	}})
}

func TestAccOrgPendingCreationState(t *testing.T) {
	f, _ := setupOrgFake(t)
	f.pendingCreate = true
	config := orgConfig("Example Organization", "Original", "lifecycle { create_before_destroy = true }")
	workdir := t.TempDir()
	resource.Test(t, resource.TestCase{WorkingDir: workdir, ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, ExpectError: regexp.MustCompile("Organization operation requires reconciliation")},
		{Config: config, ExpectError: regexp.MustCompile("ARIN ticket remains PENDING_REVIEW")},
		{PreConfig: func() {
			f.mu.Lock()
			f.ticketStatus = "RESOLVED"
			f.ticketResolution = "ACCEPTED"
			f.mu.Unlock()
			matches, err := filepath.Glob(filepath.Join(workdir, "work*", "terraform.tfstate"))
			if err != nil || len(matches) != 1 {
				t.Fatalf("cannot locate isolated test state: %v", err)
			}
			cmd := exec.Command("terraform", "-chdir="+filepath.Dir(matches[0]), "state", "rm", "arin_org.test")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("state recovery removal failed: %v: %s", err, output)
			}
		}, ResourceName: "arin_org.test", ImportState: true, ImportStateId: "ORG-TEST", ImportStatePersist: true},
		{Config: config, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.body != "" || f.creates != 1 || f.deletes != 1 {
			return fmt.Errorf("pending creation retried or cleanup failed: creates=%d deletes=%d", f.creates, f.deletes)
		}
		return nil
	}})
}
func TestAccOrgPendingDeletionState(t *testing.T) {
	f, _ := setupOrgFake(t)
	config := orgConfig("Example Organization", "Original", "")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config},
		{PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.pendingDelete = true }, Config: config, Destroy: true, ExpectError: regexp.MustCompile("Organization operation requires reconciliation")},
		{PreConfig: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.body = ""
			f.ticketStatus = "RESOLVED"
			f.ticketResolution = "PROCESSED"
		}, Config: config, Destroy: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.body != "" || f.creates != 1 || f.deletes != 1 {
			return fmt.Errorf("pending deletion retried or cleanup failed: creates=%d deletes=%d", f.creates, f.deletes)
		}
		return nil
	}})
}
