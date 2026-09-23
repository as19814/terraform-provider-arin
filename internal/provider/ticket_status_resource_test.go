package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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

type ticketStatusFake struct {
	mu                            sync.Mutex
	tickets                       map[string]arin.Ticket
	puts, writeStatus, readStatus int
	ignoreClose                   bool
}

func (f *ticketStatusFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "ApiKey test-key" {
		w.WriteHeader(403)
		return
	}
	number := strings.Split(strings.TrimPrefix(r.URL.Path, "/rest/ticket/"), "/")[0]
	ticket, ok := f.tickets[number]
	if !ok {
		w.WriteHeader(404)
		return
	}
	if r.Method == "PUT" {
		f.puts++
		if r.URL.Path != "/rest/ticket/"+number+"/ticketStatus/CLOSED" || r.URL.RawQuery != "msgRefs=true" {
			w.WriteHeader(400)
			return
		}
		if ticket.Status != "RESOLVED" {
			w.WriteHeader(400)
			return
		}
		if !f.ignoreClose {
			ticket.Status = "CLOSED"
			f.tickets[number] = ticket
		}
		if f.writeStatus != 0 {
			w.WriteHeader(f.writeStatus)
			return
		}
	} else if r.Method != "GET" || r.URL.Path != "/rest/ticket/"+number+"/summary" {
		w.WriteHeader(405)
		return
	} else if f.readStatus != 0 {
		w.WriteHeader(f.readStatus)
		return
	}
	fmt.Fprint(w, reportFakeXML(ticket))
}
func setupTicketStatusFake(t *testing.T) (*ticketStatusFake, *arin.Client) {
	t.Helper()
	f := &ticketStatusFake{tickets: map[string]arin.Ticket{"20260923-X1": {Number: "20260923-X1", Type: "ASSOCIATIONS_REPORT", Status: "RESOLVED", Resolution: "PROCESSED"}, "20260923-X2": {Number: "20260923-X2", Type: "ASSOCIATIONS_REPORT", Status: "CLOSED", Resolution: "PROCESSED"}}}
	server := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(server.Close)
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_BASE_URL", server.URL)
	t.Setenv("ARIN_RDAP_BASE_URL", "")
	c, err := arin.New(arin.Config{APIKey: "test-key", BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return f, c
}
func ticketStatusConfig(number, status string) string {
	return fmt.Sprintf("provider \"arin\" {}\nresource \"arin_ticket_status\" \"test\" {\nticket_number = %q\nstatus = %q\n}\n", number, status)
}
func TestAccTicketStatusLifecycle(t *testing.T) {
	f, _ := setupTicketStatusFake(t)
	first := ticketStatusConfig("20260923-X1", "CLOSED")
	second := ticketStatusConfig("20260923-X2", "CLOSED")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: first, Check: resource.TestCheckResourceAttr("arin_ticket_status.test", "status", "CLOSED")},
		{ResourceName: "arin_ticket_status.test", ImportState: true, ImportStateId: "20260923-X1", ImportStateVerify: true},
		{Config: first, PlanOnly: true},
		{Config: second},
		{Config: second, PreConfig: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			ticket := f.tickets["20260923-X2"]
			ticket.Status = "RESOLVED"
			f.tickets[ticket.Number] = ticket
		}},
		{Config: second, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.tickets, "20260923-X2") }, Check: resource.TestCheckResourceAttr("arin_ticket_status.test", "ticket_available", "false")},
		{Config: second, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.puts != 2 || len(f.tickets) != 1 || f.tickets["20260923-X1"].Status != "CLOSED" {
			return fmt.Errorf("ticket closure replayed or destroy changed ticket state")
		}
		return nil
	}})
}
func TestAccTicketStatusUncertainClosure(t *testing.T) {
	f, _ := setupTicketStatusFake(t)
	f.writeStatus = 500
	config := ticketStatusConfig("20260923-X1", "CLOSED")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: config, ExpectError: regexp.MustCompile("Could not confirm ticket closure")},
		{Config: config, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.writeStatus = 0 }},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.puts != 1 || f.tickets["20260923-X1"].Status != "CLOSED" {
			return fmt.Errorf("uncertain closure was replayed or reverted")
		}
		return nil
	}})
}
func TestAccTicketStatusRejectsInvalidTransition(t *testing.T) {
	for _, status := range []string{"PENDING_REVIEW", "RESOLVED"} {
		t.Run(status, func(t *testing.T) {
			f, _ := setupTicketStatusFake(t)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: ticketStatusConfig("20260923-X1", status), ExpectError: regexp.MustCompile("Unsupported ticket status")}}})
			if f.puts != 0 {
				t.Fatal("unsupported transition reached API")
			}
		})
	}
}
