package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"reflect"
	"sort"
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

// This transport permits reads of one saved ticket only, including its messages
// and attachments. Neither report generation nor correspondence can pass it.
type ticketMessageImportTransport struct {
	mu        sync.Mutex
	number    string
	last      time.Time
	rejected  int
	transport http.RoundTripper
}

func (t *ticketMessageImportTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	base := "/rest/ticket/" + t.number
	path := req.URL.Path
	query := req.URL.Query()
	allowedQuery := req.URL.RawQuery == "" || (len(query) == 1 && len(query["msgRefs"]) == 1 && query.Get("msgRefs") == "true")
	if req.Method != http.MethodGet || req.URL.Scheme != "https" || req.URL.Host != "reg.ote.arin.net" || req.URL.User != nil || pathpkg.Clean(path) != path || !allowedQuery || !(path == base || path == base+"/summary" || strings.HasPrefix(path, base+"/message/")) {
		t.rejected++
		return nil, errors.New("message import audit permits only saved-ticket sandbox GETs")
	}
	if delay := time.Until(t.last.Add(1100 * time.Millisecond)); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
	t.last = time.Now()
	transport := t.transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(req)
}

func ticketMessageImportConfig(ticket string, message *arin.TicketMessage, attachments map[string]string) string {
	quote := func(s string) string {
		// Escape Terraform template markers before encoding a quoted HCL string.
		s = strings.NewReplacer("${", "$${", "%{", "%%{").Replace(s)
		raw, _ := json.Marshal(s)
		return string(raw)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "provider \"arin\" {}\nresource \"arin_ticket_message\" \"existing\" {\n ticket_number = %s\n subject = %s\n category = %s\n text = [", quote(ticket), quote(message.Subject), quote(message.Category))
	for _, line := range message.Text {
		fmt.Fprintf(&b, "%s,", quote(line))
	}
	b.WriteString("]\n attachments = {\n")
	names := make([]string, 0, len(attachments))
	for name := range attachments {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(&b, " %s = %s\n", quote(name), quote(attachments[name]))
	}
	b.WriteString("}\n}\n")
	return b.String()
}

func TestOTETicketMessageImportLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit sandbox test opt-in; this test permits reads only")
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
		t.Fatal("requires existing associations report receipt")
	}
	var receipt reportResourceReceipt
	if json.Unmarshal(raw, &receipt) != nil || receipt.Org != org || receipt.Request.Type != arin.ReportAssociations || arin.ValidateTicketNumber(receipt.TicketNumber) != nil {
		t.Fatal("invalid saved associations report receipt")
	}
	trace := &ticketMessageImportTransport{number: receipt.TicketNumber}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, HTTPClient: &http.Client{Transport: trace}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	read := func(name string, params map[string]string) map[string]any {
		t.Helper()
		for _, spec := range arin.RegistrationReads() {
			if spec.Name == name {
				result, err := client.ReadRegistration(ctx, spec, params)
				if err != nil {
					t.Fatalf("%s read: %v", name, err)
				}
				return result
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
	var message *arin.TicketMessage
	for _, raw := range detail["message_references"].([]any) {
		id, ok := raw.(map[string]any)["message_id"].(int64)
		if !ok || id < 1 {
			t.Fatal("invalid saved ticket message reference")
		}
		candidate, err := client.GetTicketMessage(ctx, ticket.Number, strconv.FormatInt(id, 10))
		if err != nil {
			t.Fatal(err)
		}
		if len(candidate.Attachments) > 0 {
			message = candidate
			break
		}
	}
	if message == nil {
		t.Fatal("completed report has no message with attachments")
	}
	attachments := map[string]string{}
	for _, ref := range message.Attachments {
		if _, ok := attachments[ref.Filename]; ok {
			t.Fatal("report message has duplicate attachment filenames")
		}
		value := read("ticket_attachment", map[string]string{"ticket_number": ticket.Number, "message_id": message.ID, "attachment_id": ref.ID})
		attachments[ref.Filename] = value["content_base64"].(string)
	}
	config := ticketMessageImportConfig(ticket.Number, message, attachments)
	check := func(state *terraform.State) error {
		item := state.RootModule().Resources["arin_ticket_message.existing"]
		if item == nil || item.Primary == nil {
			return errors.New("missing imported message state")
		}
		attrs := item.Primary.Attributes
		if attrs["id"] != ticket.Number+"/"+message.ID || attrs["message_id"] != message.ID || attrs["ticket_number"] != ticket.Number || attrs["subject"] != message.Subject || attrs["category"] != message.Category || attrs["created_date"] != message.CreatedDate || attrs["pending_submission"] != "false" || attrs["message_available"] != "true" || attrs["text.#"] != strconv.Itoa(len(message.Text)) || attrs["attachments.%"] != strconv.Itoa(len(attachments)) {
			return errors.New("imported message receipt or content differs")
		}
		for i, line := range message.Text {
			if attrs[fmt.Sprintf("text.%d", i)] != line {
				return errors.New("imported text differs")
			}
		}
		for name, data := range attachments {
			if attrs["attachments."+name] != data {
				return errors.New("imported attachment bytes differ")
			}
		}
		return nil
	}
	p := &ticketStatusOTEProvider{ARINProvider: &ARINProvider{version: "ote-message-import"}, client: client}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(p)}, Steps: []resource.TestStep{
		{Config: config, ResourceName: "arin_ticket_message.existing", ImportState: true, ImportStateId: ticket.Number + "/" + message.ID, ImportStatePersist: true},
		{Config: config, PlanOnly: true, Check: check},
		{RefreshState: true, Check: check},
		{Config: config, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		after, err := client.GetTicketMessage(ctx, ticket.Number, message.ID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(after, message) {
			return errors.New("state-only destroy changed server message")
		}
		final, err := client.GetTicket(ctx, ticket.Number)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(final, ticket) {
			return errors.New("message import changed ticket metadata")
		}
		trace.mu.Lock()
		defer trace.mu.Unlock()
		if trace.rejected != 0 {
			return errors.New("message import attempted a forbidden request")
		}
		return nil
	}})
	t.Logf("read-only Terraform message import, refresh, clean plan and destroy verified with %d attachments", len(attachments))
}

func TestTicketMessageImportTransportGuard(t *testing.T) {
	for _, request := range []struct{ method, url string }{
		{"POST", "https://reg.ote.arin.net/rest/ticket/20260923-X1/message"},
		{"GET", "https://reg.ote.arin.net/rest/report/associations"},
		{"GET", "https://reg.arin.net/rest/ticket/20260923-X1/summary"},
		{"GET", "https://reg.ote.arin.net/rest/ticket/20260923-X2/summary"},
		{"GET", "https://reg.ote.arin.net/rest/ticket/20260923-X1?apikey=forbidden"},
		{"GET", "https://reg.ote.arin.net/rest/ticket/20260923-X1/message/../../20260923-X2/summary"},
	} {
		guard := &ticketMessageImportTransport{number: "20260923-X1", transport: graphRoundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("forbidden request dispatched"); return nil, nil })}
		req, _ := http.NewRequest(request.method, request.url, nil)
		if _, err := guard.RoundTrip(req); err == nil || guard.rejected != 1 {
			t.Fatal("forbidden request accepted")
		}
	}
}

func TestAccTicketMessageImportExisting(t *testing.T) {
	fake := setupTicketMessageFake(t)
	message := &arin.TicketMessage{ID: "7", Subject: "Literal ${not.a.reference} %{if false} café", Category: "NONE", Text: []string{"Literal ${var.example}", "%{if true}"}}
	fake.mu.Lock()
	fake.messages[message.ID] = `<message xmlns="http://www.arin.net/regrws/core/v1"><subject>` + message.Subject + `</subject><text><line number="0">` + message.Text[0] + `</line><line number="1">` + message.Text[1] + `</line></text><category>NONE</category><m:messageId xmlns:m="http://www.arin.net/regrws/messages/v1">7</m:messageId><attachmentReferences><attachmentReference><attachmentId>A1</attachmentId><attachmentFilename>evidence.txt</attachmentFilename></attachmentReference></attachmentReferences></message>`
	fake.mu.Unlock()
	config := ticketMessageImportConfig("20260923-X1", message, map[string]string{"evidence.txt": "ZXZpZGVuY2U="})
	checks := resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_ticket_message.existing", "subject", message.Subject), resource.TestCheckResourceAttr("arin_ticket_message.existing", "text.0", message.Text[0]), resource.TestCheckResourceAttr("arin_ticket_message.existing", "text.1", message.Text[1]), resource.TestCheckResourceAttr("arin_ticket_message.existing", "attachments.evidence.txt", "ZXZpZGVuY2U="))
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, ResourceName: "arin_ticket_message.existing", ImportState: true, ImportStateId: "20260923-X1/7", ImportStatePersist: true},
		{Config: config, PlanOnly: true},
		{RefreshState: true, Check: checks},
		{Config: config, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		if fake.submissions != 0 || len(fake.messages) != 1 {
			return errors.New("import or destroy changed correspondence")
		}
		return nil
	}})
}
