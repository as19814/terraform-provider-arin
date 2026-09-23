package provider

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
)

func approvedRemovalMessage() arin.RegistrationMessage {
	return arin.RegistrationMessage{
		Subject:     "Terraform provider OT&E removal test",
		Text:        []string{"Removing a disposable sandbox network for provider verification."},
		Category:    "NONE",
		Attachments: []arin.RegistrationAttachment{{Filename: "evidence.txt", Data: []byte("Disposable OT&E test evidence.")}},
	}
}
func validateApprovedRemovalMessage(body []byte) error {
	var payload struct {
		Messages []struct {
			Subject     string   `xml:"subject"`
			Text        []string `xml:"text>line"`
			Category    string   `xml:"category"`
			ID          string   `xml:"messageId"`
			Created     string   `xml:"createdDate"`
			Attachments []struct {
				Filename string `xml:"filename"`
				Data     string `xml:"data"`
			} `xml:"attachments>attachment"`
		} `xml:"messages>message"`
	}
	if xml.Unmarshal(body, &payload) != nil || len(payload.Messages) != 1 {
		return errors.New("exactly one approved removal message is required")
	}
	m := payload.Messages[0]
	want := approvedRemovalMessage()
	if m.Subject != want.Subject || m.Category != want.Category || len(m.Text) != 1 || m.Text[0] != want.Text[0] || m.ID != "" || m.Created != "" || len(m.Attachments) != 1 || m.Attachments[0].Filename != want.Attachments[0].Filename || m.Attachments[0].Data != base64.StdEncoding.EncodeToString(want.Attachments[0].Data) {
		return errors.New("removal correspondence differs from the explicitly approved payload")
	}
	return nil
}

// Explicitly authorized once for each IP family. Not part of make testote.
// Keep the completed durable receipts, even after all disposable records vanish.
func TestOTENetRemoveMessageLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_MESSAGE_TESTS") != "1" {
		t.Skip("requires the explicit approved-message test opt-in")
	}
	testOTECustomerNetGraph(t, true, true, false)
}

func TestApprovedRemovalMessageGuard(t *testing.T) {
	const netBody = `<net xmlns="http://www.arin.net/regrws/core/v1"><handle>NET-192-0-2-0-2</handle><netName>TEST-MESSAGE</netName><parentNetHandle>NET-192-0-2-0-1</parentNetHandle><customerHandle>C123</customerHandle><netBlocks><netBlock><type>S</type><startAddress>192.0.2.0</startAddress><endAddress>192.0.2.0</endAddress><cidrLength>32</cidrLength></netBlock></netBlocks></net>`
	message := `<messages><message><subject>Terraform provider OT&amp;E removal test</subject><text><line number="0">Removing a disposable sandbox network for provider verification.</line></text><category>NONE</category><attachments><attachment><filename>evidence.txt</filename><data>` + base64.StdEncoding.EncodeToString(approvedRemovalMessage().Attachments[0].Data) + `</data></attachment></attachments></message></messages>`
	for _, mode := range []string{"complete", "changed", "missing", "duplicate", "lost", "already_attempted"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			g := &customerNetOTETransport{path: filepath.Join(t.TempDir(), "receipt.json"), allowEmptyRemoval: true, allowRemovalMessage: true, receipt: customerNetReceipt{Name: "TEST-MESSAGE", Parent: "NET-192-0-2-0-1", Prefix: "192.0.2.0/32", Customers: []string{"C123"}, Nets: []string{"NET-192-0-2-0-2"}}}
			if mode == "already_attempted" {
				g.receipt.MessageAttempted = true
			}
			g.transport = graphRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				raw, err := os.ReadFile(g.path)
				var saved customerNetReceipt
				if err != nil || json.Unmarshal(raw, &saved) != nil || !saved.MessageAttempted || saved.Pending == "" {
					t.Fatal("correspondence dispatched before durable receipt")
				}
				if mode == "lost" {
					return nil, errors.New("lost response")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<ticketedRequest xmlns="http://www.arin.net/regrws/core/v1">` + netBody + `</ticketedRequest>`))}, nil
			})
			body := strings.Replace(netBody, "</net>", message+"</net>", 1)
			switch mode {
			case "changed":
				body = strings.Replace(body, "evidence.txt", "other.txt", 1)
			case "missing":
				body = netBody
			case "duplicate":
				body = strings.Replace(body, "</messages>", strings.TrimSuffix(strings.TrimPrefix(message, "<messages>"), "</messages>")+"</messages>", 1)
			}
			req, _ := http.NewRequest("PUT", "https://reg.ote.arin.net/rest/net/NET-192-0-2-0-2/remove", strings.NewReader(body))
			response, err := g.RoundTrip(req)
			if response != nil {
				response.Body.Close()
			}
			if mode == "complete" {
				if err != nil || calls != 1 || !g.receipt.MessageAttempted || g.receipt.Pending != "" {
					t.Fatalf("approved removal failed: %v", err)
				}
			} else if err == nil {
				t.Fatal("unsafe or uncertain removal accepted")
			}
			if mode == "complete" || mode == "lost" {
				if _, err := g.RoundTrip(req); err == nil || calls != 1 {
					t.Fatal("message replayed")
				}
			} else if calls != 0 {
				t.Fatal("unapproved message dispatched")
			}
		})
	}
}

const terraformRemovalMessageConfig = `removal_messages = [{
 subject = "Terraform provider OT&E removal test"
 text = ["Removing a disposable sandbox network for provider verification."]
 category = "NONE"
 attachments = { "evidence.txt" = base64encode("Disposable OT&E test evidence.") }
}]`

// No new correspondence has been authorized. Enabling the environment variable
// alone must not reuse the two consumed client-test approvals.
const terraformRemovalMessagesApproved = false

func TestOTENetTerraformRemovalMessages(t *testing.T) {
	if !terraformRemovalMessagesApproved || os.Getenv("ARIN_OTE_TERRAFORM_MESSAGE_TESTS") != "1" {
		t.Skip("requires separate approval for two additional Terraform removal messages")
	}
	testOTECustomerNetGraph(t, true, true, true)
}
