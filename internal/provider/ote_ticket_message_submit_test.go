package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// This approval has not been granted. Neither ordinary OT&E opt-ins nor the
// consumed NET-removal approvals authorize this separate ticket append.
const oteTicketAppendApproved = false
const oteTicketAppendSubject = "Terraform provider OT&E ticket-message test"
const oteTicketAppendText = "Verifying ticket-message submission for our disposable sandbox organization request. No additional registry changes are requested."
const oteTicketAppendXML = `<message xmlns="http://www.arin.net/regrws/core/v1"><subject>Terraform provider OT&amp;E ticket-message test</subject><text><line number="0">Verifying ticket-message submission for our disposable sandbox organization request. No additional registry changes are requested.</line></text><category>NONE</category><attachments><attachment><data>ZXZpZGVuY2U=</data><filename>evidence.txt</filename></attachment></attachments></message>`

// Save evidence before dispatch with exclusive creation and file/directory sync.
// Any existing intent, including one left by an uncertain request, blocks replay.
func saveTicketAppendEvidence(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(raw)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("ticket append evidence could not be persisted")
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	err = d.Sync()
	closeErr = d.Close()
	if err != nil {
		return err
	}
	return closeErr
}

type ticketAppendTransport struct {
	mu           sync.Mutex
	number, path string
	attempted    bool
	reads        *ticketMessageImportTransport
	transport    http.RoundTripper
}

func (g *ticketAppendTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if req.Method == http.MethodGet {
		return g.reads.RoundTrip(req)
	}
	if g.attempted || req.Method != http.MethodPost || req.URL.Scheme != "https" || req.URL.Host != "reg.ote.arin.net" || req.URL.User != nil || req.URL.RawQuery != "" || req.URL.Path != "/rest/ticket/"+g.number+"/message" {
		return nil, errors.New("only one exact approved sandbox ticket append is permitted")
	}
	raw, err := io.ReadAll(io.LimitReader(req.Body, int64(len(oteTicketAppendXML)+1)))
	if err != nil || string(raw) != oteTicketAppendXML {
		return nil, errors.New("ticket correspondence differs from proposed payload")
	}
	digest := sha256.Sum256(raw)
	if err := saveTicketAppendEvidence(g.path, map[string]any{"ticket_number": g.number, "request_sha256": fmt.Sprintf("%x", digest), "attempted": true}); err != nil {
		return nil, err
	}
	g.attempted = true
	req.Body = io.NopCloser(bytes.NewReader(raw))
	return g.transport.RoundTrip(req)
}

func ticketAppendConfig(ticket string) string {
	return ticketMessageImportConfig(ticket, &arin.TicketMessage{Subject: oteTicketAppendSubject, Category: "NONE", Text: []string{oteTicketAppendText}}, map[string]string{"evidence.txt": "ZXZpZGVuY2U="})
}
func testTicketAppendTerraform(t *testing.T, client *arin.Client, guard *ticketAppendTransport) {
	t.Helper()
	p := &ticketStatusOTEProvider{ARINProvider: &ARINProvider{version: "ote-ticket-append"}, client: client}
	var messageID string
	config := ticketAppendConfig(guard.number)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(p)}, Steps: []resource.TestStep{
		{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("arin_ticket_message.existing", "pending_submission", "false"),
			resource.TestCheckResourceAttr("arin_ticket_message.existing", "subject", oteTicketAppendSubject),
			func(s *terraform.State) error {
				messageID = s.RootModule().Resources["arin_ticket_message.existing"].Primary.Attributes["message_id"]
				if messageID == "" {
					return errors.New("missing message identity")
				}
				return nil
			},
		)},
		{Config: config, ResourceName: "arin_ticket_message.existing", ImportState: true, ImportStateIdFunc: func(*terraform.State) (string, error) { return guard.number + "/" + messageID, nil }, ImportStateVerify: true},
		{Config: config, PlanOnly: true},
		{RefreshState: true},
		{Config: config, PlanOnly: true},
	}, CheckDestroy: func(*terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		message, err := client.GetTicketMessage(ctx, guard.number, messageID)
		if err != nil {
			return err
		}
		if message.Subject != oteTicketAppendSubject || len(message.Text) != 1 || message.Text[0] != oteTicketAppendText || len(message.Attachments) != 1 || message.Attachments[0].Filename != "evidence.txt" {
			return errors.New("state-only destroy did not preserve correspondence")
		}
		guard.mu.Lock()
		defer guard.mu.Unlock()
		if !guard.attempted {
			return errors.New("message was not dispatched")
		}
		return saveTicketAppendEvidence(guard.path+".confirmed.json", map[string]string{"ticket_number": guard.number, "message_id": messageID})
	}})
}

func TestOTETicketMessageSubmitLifecycle(t *testing.T) {
	if !oteTicketAppendApproved || os.Getenv("ARIN_OTE_TICKET_APPEND_TESTS") != "1" || os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires separate explicit approval for one ticket message")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org != "FT-684" {
		t.Fatal("requires the approved sandbox account and key")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "terraform-provider-arin")
	hash := sha256.Sum256([]byte(org))
	raw, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("ote-org-create-%x.json", hash[:8])))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct{ Result *arin.OrganizationWriteResult }
	if json.Unmarshal(raw, &receipt) != nil || receipt.Result == nil || receipt.Result.Organization != nil || arin.ValidateTicketNumber(receipt.Result.TicketNumber) != nil {
		t.Fatal("requires the existing ticket-only disposable organization receipt")
	}
	number := receipt.Result.TicketNumber
	path := filepath.Join(dir, fmt.Sprintf("ote-ticket-append-20260923-%x.json", hash[:8]))
	for _, p := range []string{path, path + ".confirmed.json"} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatal("prior ticket append evidence blocks another send")
		}
	}
	trace := &ticketMessageImportTransport{number: number}
	guard := &ticketAppendTransport{number: number, path: path, reads: trace, transport: http.DefaultTransport}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, HTTPClient: &http.Client{Transport: guard}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ticket, err := client.GetTicket(ctx, number)
	if err != nil || ticket.Status != "PENDING_REVIEW" {
		t.Fatal("expected pending-review disposable organization ticket")
	}
	testTicketAppendTerraform(t, client, guard)
}

func TestAccProposedTicketAppend(t *testing.T) {
	fake := setupTicketMessageFake(t)
	endpoint, err := url.Parse(os.Getenv("ARIN_BASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	wire := graphRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		u := *req.URL
		u.Scheme = endpoint.Scheme
		u.Host = endpoint.Host
		clone.URL = &u
		return http.DefaultTransport.RoundTrip(clone)
	})
	number := "20260923-X1"
	trace := &ticketMessageImportTransport{number: number, transport: wire}
	guard := &ticketAppendTransport{number: number, path: filepath.Join(t.TempDir(), "intent.json"), reads: trace, transport: wire}
	client, err := arin.New(arin.Config{APIKey: "test-key", BaseURL: arin.OTEURL, HTTPClient: &http.Client{Transport: guard}})
	if err != nil {
		t.Fatal(err)
	}
	testTicketAppendTerraform(t, client, guard)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.submissions != 1 || len(fake.messages) != 1 {
		t.Fatal("Terraform replayed or removed correspondence")
	}
}

func TestTicketAppendReplayGuard(t *testing.T) {
	for _, mode := range []string{"valid", "changed", "production", "wrong_ticket", "query", "lost", "existing"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "intent.json")
			if mode == "existing" {
				if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			wire := graphRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				raw, err := os.ReadFile(path)
				if err != nil || !bytes.Contains(raw, []byte(`"attempted":true`)) {
					t.Fatal("dispatch preceded durable intent")
				}
				if mode == "lost" {
					return nil, errors.New("lost response")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
			})
			guard := &ticketAppendTransport{number: "20260923-X1", path: path, transport: wire}
			endpoint := "https://reg.ote.arin.net/rest/ticket/20260923-X1/message"
			body := oteTicketAppendXML
			if mode == "production" {
				endpoint = strings.Replace(endpoint, "reg.ote.arin.net", "reg.arin.net", 1)
			}
			if mode == "wrong_ticket" {
				endpoint = strings.Replace(endpoint, "20260923-X1", "20260923-X2", 1)
			}
			if mode == "query" {
				endpoint += "?unexpected=true"
			}
			if mode == "changed" {
				body = strings.Replace(body, "evidence.txt", "other.txt", 1)
			}
			req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
			response, err := guard.RoundTrip(req)
			if response != nil {
				response.Body.Close()
			}
			if mode == "valid" {
				if err != nil || calls != 1 {
					t.Fatal("valid guarded request failed")
				}
			} else if err == nil {
				t.Fatal("unsafe or uncertain append accepted")
			}
			if mode == "valid" || mode == "lost" {
				fresh := &ticketAppendTransport{number: guard.number, path: path, transport: wire}
				retry, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
				if _, err := fresh.RoundTrip(retry); err == nil || calls != 1 {
					t.Fatal("request replayed after reopening")
				}
			} else if calls != 0 {
				t.Fatal("unapproved dispatch")
			}
		})
	}
}
