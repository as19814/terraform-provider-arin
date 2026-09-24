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

type ticketMessageFake struct {
	reject                 bool
	mu                     sync.Mutex
	messages               map[string]string
	submissions            int
	lost, partial, expired bool
}

func setupTicketMessageFake(t *testing.T) *ticketMessageFake {
	f := &ticketMessageFake{messages: map[string]string{}}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "ApiKey test-key" {
			w.WriteHeader(403)
			return
		}
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/summary") {
			fmt.Fprint(w, reportFakeXML(arin.Ticket{Number: "20260923-X1", Type: "QUESTION", Status: "IN_PROGRESS"}))
			return
		}
		base := "/rest/ticket/20260923-X1/message"
		if r.Method == "PUT" && r.URL.Path == base {
			if f.reject {
				w.WriteHeader(403)
				return
			}
			f.submissions++
			raw, _ := io.ReadAll(r.Body)
			body := string(raw)
			id := fmt.Sprint(f.submissions)
			if start := strings.Index(body, "<attachments>"); start >= 0 {
				end := strings.Index(body, "</attachments>") + len("</attachments>")
				body = body[:start] + `<attachmentReferences><attachmentReference><attachmentId>A1</attachmentId><attachmentFilename>evidence.txt</attachmentFilename></attachmentReference></attachmentReferences>` + body[end:]
			}
			body = strings.Replace(body, "</message>", `<m:messageId xmlns:m="http://www.arin.net/regrws/messages/v1">`+id+`</m:messageId></message>`, 1)
			f.messages[id] = body
			if f.lost {
				w.WriteHeader(500)
				return
			}
			if f.partial {
				body = strings.Replace(body, "<category>NONE</category>", "<category>INVALID</category>", 1)
			}
			fmt.Fprint(w, body)
			return
		}
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, base+"/") {
			if f.expired {
				w.WriteHeader(404)
				return
			}
			id := strings.TrimPrefix(r.URL.Path, base+"/")
			if strings.HasSuffix(id, "/attachment/A1") {
				fmt.Fprint(w, "evidence")
				return
			}
			if body, ok := f.messages[id]; ok {
				fmt.Fprint(w, body)
				return
			}
			w.WriteHeader(404)
			return
		}
		t.Errorf("unexpected message mutation %s %s", r.Method, r.URL.Path)
		w.WriteHeader(500)
	}))
	t.Cleanup(s.Close)
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", s.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	return f
}
func messageConfig(subject string) string {
	return fmt.Sprintf(`provider "arin" {}
resource "arin_ticket_message" "test" {
 ticket_number = "20260923-X1"
 subject = %q
 text = ["One", "Two"]
 attachments = { "evidence.txt" = base64encode("evidence") }
}
`, subject)
}
func TestAccTicketMessageLifecycle(t *testing.T) {
	f := setupTicketMessageFake(t)
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: messageConfig("First"), Check: resource.TestCheckResourceAttr("arin_ticket_message.test", "message_id", "1")},
		{ResourceName: "arin_ticket_message.test", ImportState: true, ImportStateId: "20260923-X1/1", ImportStateVerify: true},
		{Config: messageConfig("First"), PlanOnly: true},
		{Config: messageConfig("Second"), Check: resource.TestCheckResourceAttr("arin_ticket_message.test", "message_id", "2")},
		{ResourceName: "arin_ticket_message.test", ImportState: true, ImportStateId: "20260923-X1/2", ImportStateVerify: true},
		{Config: messageConfig("Second"), PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.expired = true }, Check: resource.TestCheckResourceAttr("arin_ticket_message.test", "message_available", "false")},
		{Config: messageConfig("Second"), PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.submissions != 2 || len(f.messages) != 2 {
			return fmt.Errorf("refresh or destroy changed messages")
		}
		return nil
	}})
}
func TestAccTicketMessageRecovery(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			f := setupTicketMessageFake(t)
			f.lost = lost
			f.partial = !lost
			config := strings.Replace(messageConfig("Recovery"), " subject =", " lifecycle { create_before_destroy = true }\n subject =", 1)
			workdir := t.TempDir()
			resource.Test(t, resource.TestCase{WorkingDir: workdir, ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
				{Config: config, ExpectError: regexp.MustCompile("Message submission requires reconciliation")},
				{Config: config, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.lost = false; f.partial = false }, ExpectError: regexp.MustCompile("Message submission requires import recovery")},
				{PreConfig: func() {
					f.mu.Lock()
					n := f.submissions
					f.mu.Unlock()
					if n != 1 {
						t.Fatal("message replayed")
					}
					matches, err := filepath.Glob(filepath.Join(workdir, "work*", "terraform.tfstate"))
					if err != nil || len(matches) != 1 {
						t.Fatalf("state lookup: %v", err)
					}
					out, err := exec.Command("terraform", "-chdir="+filepath.Dir(matches[0]), "state", "rm", "arin_ticket_message.test").CombinedOutput()
					if err != nil {
						t.Fatalf("state recovery: %v %s", err, out)
					}
				}, ResourceName: "arin_ticket_message.test", ImportState: true, ImportStateId: "20260923-X1/1", ImportStatePersist: true},
				{Config: config, PlanOnly: true},
			}, CheckDestroy: func(_ *terraform.State) error {
				f.mu.Lock()
				defer f.mu.Unlock()
				if f.submissions != 1 || len(f.messages) != 1 {
					return fmt.Errorf("recovery changed messages")
				}
				return nil
			}})
		})
	}
}

func TestAccTicketMessageSubjectOnlyImport(t *testing.T) {
	f := setupTicketMessageFake(t)
	config := `provider "arin" {}
resource "arin_ticket_message" "test" {
 ticket_number = "20260923-X1"
 subject = "Subject only"
}
`
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config},
		{ResourceName: "arin_ticket_message.test", ImportState: true, ImportStateId: "20260923-X1/1", ImportStateVerify: true},
		{Config: config, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.submissions != 1 {
			return fmt.Errorf("subject-only message resubmitted")
		}
		return nil
	}})
}
