package provider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// This audit reuses a saved report receipt and permits only ticket GETs.
// It cannot generate another report or send correspondence.
func TestOTEReportReadLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit sandbox test opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and organization")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(org))
	raw, err := os.ReadFile(filepath.Join(cache, "terraform-provider-arin", fmt.Sprintf("ote-report-resource-%x.json", hash[:8])))
	if err != nil {
		t.Fatal("requires existing report resource receipt")
	}
	var receipt reportResourceReceipt
	if json.Unmarshal(raw, &receipt) != nil || receipt.Org != org || receipt.Request.Type != arin.ReportAssociations || arin.ValidateTicketNumber(receipt.TicketNumber) != nil {
		t.Fatal("invalid saved associations report receipt")
	}
	var requestMu sync.Mutex
	var lastRequest time.Time
	transport := graphRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != "GET" || req.URL.Scheme != "https" || req.URL.Host != "reg.ote.arin.net" || !(strings.HasPrefix(req.URL.Path, "/rest/ticket/") || strings.HasPrefix(req.URL.Path, "/rest/ticket;")) {
			return nil, errors.New("report read audit allows only sandbox ticket GETs")
		}
		// Terraform refreshes independent data sources concurrently. Keep this
		// audit below one request per second without replaying failed requests.
		requestMu.Lock()
		defer requestMu.Unlock()
		if delay := time.Until(lastRequest.Add(1100 * time.Millisecond)); delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-timer.C:
			}
		}
		lastRequest = time.Now()
		return http.DefaultTransport.RoundTrip(req)
	})
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	read := func(name string, params map[string]string) map[string]any {
		for _, spec := range arin.RegistrationReads() {
			if spec.Name == name {
				values, err := client.ReadRegistration(ctx, spec, params)
				if err != nil {
					t.Fatalf("%s read: %v", name, err)
				}
				return values
			}
		}
		t.Fatalf("unknown read %s", name)
		return nil
	}
	ticket, err := client.GetTicket(ctx, receipt.TicketNumber)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Request.MatchesTicket(*ticket) || ticket.Status != "CLOSED" {
		t.Fatal("saved report must be complete and CLOSED")
	}
	detail := read("ticket", map[string]string{"ticket_number": ticket.Number, "message_references_only": "true"})
	refs := detail["message_references"].([]any)
	if len(refs) == 0 {
		t.Fatal("completed report has no message references")
	}
	config := fmt.Sprintf(`provider "arin" {}
data "arin_ticket" "references" { ticket_number = %q }
data "arin_ticket" "full" {
 ticket_number = %q
 message_references_only = false
}
data "arin_ticket_summary" "summary" { ticket_number = %q }
data "arin_tickets" "list" {
 ticket_type = "ASSOCIATIONS_REPORT"
 ticket_status = "CLOSED"
}
data "arin_ticket_summaries" "list" {
 ticket_type = "ASSOCIATIONS_REPORT"
 ticket_status = "CLOSED"
}
`, ticket.Number, ticket.Number, ticket.Number)
	checks := []resource.TestCheckFunc{resource.TestCheckResourceAttr("data.arin_ticket.references", "ticket_number", ticket.Number), resource.TestCheckResourceAttr("data.arin_ticket.full", "ticket_number", ticket.Number), resource.TestCheckResourceAttr("data.arin_ticket_summary.summary", "ticket_number", ticket.Number), resource.TestCheckResourceAttr("data.arin_ticket.references", "message_references.#", strconv.Itoa(len(refs))), resource.TestCheckResourceAttr("data.arin_ticket.full", "messages.#", strconv.Itoa(len(refs)))}
	attachments := 0
	for i, raw := range refs {
		ref := raw.(map[string]any)
		id, ok := ref["message_id"].(int64)
		if !ok || id <= 0 {
			t.Fatal("invalid message reference ID")
		}
		message := read("ticket_message", map[string]string{"ticket_number": ticket.Number, "message_id": strconv.FormatInt(id, 10)})
		typed, err := client.GetTicketMessage(ctx, ticket.Number, strconv.FormatInt(id, 10))
		if err != nil || typed.ID != strconv.FormatInt(id, 10) {
			t.Fatalf("typed message read failed: %v", err)
		}
		if message["message_id"] != id {
			t.Fatal("message identity mismatch")
		}
		config += fmt.Sprintf("data \"arin_ticket_message\" \"m%d\" {\n ticket_number = %q\n message_id = %q\n}\n", i, ticket.Number, strconv.FormatInt(id, 10))
		checks = append(checks, resource.TestCheckResourceAttr(fmt.Sprintf("data.arin_ticket_message.m%d", i), "message_id", strconv.FormatInt(id, 10)))
		for j, raw := range message["attachment_references"].([]any) {
			ref := raw.(map[string]any)
			attachmentID, ok := ref["attachment_id"].(string)
			if !ok || attachmentID == "" {
				t.Fatal("missing attachment ID")
			}
			attachment := read("ticket_attachment", map[string]string{"ticket_number": ticket.Number, "message_id": strconv.FormatInt(id, 10), "attachment_id": attachmentID})
			data, err := base64.StdEncoding.DecodeString(attachment["content_base64"].(string))
			if err != nil || len(data) == 0 {
				t.Fatal("empty or invalid report attachment")
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(data))
			if attachment["sha256"] != digest || attachment["size_bytes"] != int64(len(data)) {
				t.Fatal("attachment metadata mismatch")
			}
			if attachment["filename"] != ref["filename"] || attachment["content_type"] != "application/octet-stream" {
				t.Fatal("attachment response headers differ from the documented reference")
			}
			messageID := strconv.FormatInt(id, 10)
			filename := ref["filename"].(string)
			checks = append(checks, func(state *terraform.State) error {
				attrs := state.RootModule().Resources["data.arin_ticket.full"].Primary.Attributes
				for key, value := range attrs {
					if !strings.HasPrefix(key, "messages.") || !strings.HasSuffix(key, ".message_id") || value != messageID {
						continue
					}
					prefix := strings.TrimSuffix(key, "message_id") + "attachments."
					for field, name := range attrs {
						if strings.HasPrefix(field, prefix) && strings.HasSuffix(field, ".filename") && name == filename {
							content := attrs[strings.TrimSuffix(field, "filename")+"content_base64"]
							decoded, err := base64.StdEncoding.DecodeString(content)
							if err != nil || fmt.Sprintf("%x", sha256.Sum256(decoded)) != digest {
								return errors.New("embedded report attachment differs from direct download")
							}
							return nil
						}
					}
				}
				return errors.New("full ticket did not include the report attachment")
			})
			label := fmt.Sprintf("a%d_%d", i, j)
			config += fmt.Sprintf("data \"arin_ticket_attachment\" %q {\n ticket_number = %q\n message_id = %q\n attachment_id = %q\n}\n", label, ticket.Number, strconv.FormatInt(id, 10), attachmentID)
			checks = append(checks, resource.TestCheckResourceAttr("data.arin_ticket_attachment."+label, "sha256", digest), resource.TestCheckResourceAttr("data.arin_ticket_attachment."+label, "size_bytes", strconv.Itoa(len(data))))
			attachments++
		}
	}
	if attachments == 0 {
		t.Fatal("completed associations report has no downloadable attachment")
	}
	checks = append(checks, func(state *terraform.State) error {
		for _, name := range []string{"data.arin_tickets.list", "data.arin_ticket_summaries.list"} {
			item := state.RootModule().Resources[name]
			if item == nil {
				return errors.New("missing ticket list state")
			}
			found := false
			for key, value := range item.Primary.Attributes {
				if strings.HasPrefix(key, "tickets.") && strings.HasSuffix(key, ".ticket_number") && value == ticket.Number {
					found = true
				}
			}
			if !found {
				return errors.New("saved report absent from filtered ticket list")
			}
		}
		return nil
	})
	p := &ticketStatusOTEProvider{ARINProvider: &ARINProvider{version: "ote-read"}, client: client}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(p)}, Steps: []resource.TestStep{{Config: config, Check: resource.ComposeAggregateTestCheckFunc(checks...)}, {Config: config, PlanOnly: true}}})
	t.Logf("read-only report integration verified %d messages, %d attachments, all six ticket data sources and a clean plan", len(refs), attachments)
}
