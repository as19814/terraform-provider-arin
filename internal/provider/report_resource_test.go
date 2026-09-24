package provider

import (
	"fmt"
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

type reportFake struct {
	mu                                   sync.Mutex
	tickets                              map[string]arin.Ticket
	submissions, writeStatus, readStatus int
	applyBeforeError, partialResponse    bool
}

func reportFakeXML(t arin.Ticket) string {
	return fmt.Sprintf(`<ticket xmlns="http://www.arin.net/regrws/core/v1"><ticketNo>%s</ticketNo><webTicketType>%s</webTicketType><webTicketStatus>%s</webTicketStatus><webTicketResolution>%s</webTicketResolution></ticket>`, t.Number, t.Type, t.Status, t.Resolution)
}
func (f *reportFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Method != "GET" || r.Header.Get("Authorization") != "ApiKey test-key" {
		w.WriteHeader(403)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/rest/report/") {
		f.submissions++
		if f.writeStatus != 0 && !f.applyBeforeError {
			w.WriteHeader(f.writeStatus)
			return
		}
		kind := ""
		switch {
		case r.URL.Path == "/rest/report/associations":
			kind = "ASSOCIATIONS_REPORT"
		case strings.HasPrefix(r.URL.Path, "/rest/report/reassignment/"):
			kind = "REASSIGNMENT_REPORT"
		default:
			w.WriteHeader(404)
			return
		}
		number := fmt.Sprintf("20260923-X%d", f.submissions)
		ticket := arin.Ticket{Number: number, Type: kind, Status: "CLOSED", Resolution: "PROCESSED"}
		f.tickets[number] = ticket
		if f.writeStatus != 0 {
			w.WriteHeader(f.writeStatus)
			return
		}
		if f.partialResponse {
			ticket.Status = ""
		}
		fmt.Fprint(w, reportFakeXML(ticket))
		return
	}
	if f.readStatus != 0 {
		w.WriteHeader(f.readStatus)
		return
	}
	number := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/ticket/"), "/summary")
	ticket, ok := f.tickets[number]
	if !ok {
		w.WriteHeader(404)
		return
	}
	fmt.Fprint(w, reportFakeXML(ticket))
}
func setupReportFake(t *testing.T) (*reportFake, *arin.Client) {
	t.Helper()
	f := &reportFake{tickets: map[string]arin.Ticket{}}
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
func reportConfig(kind, target string) string {
	return fmt.Sprintf("provider \"arin\" {}\nresource \"arin_report_request\" \"test\" {\nreport_type = %q\ntarget = %q\n}\n", kind, target)
}
func TestAccReportRequestLifecycle(t *testing.T) {
	f, _ := setupReportFake(t)
	first := reportConfig("associations", "")
	next := reportConfig("reassignment", "NET-192-0-2-0-1")
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
		{Config: first, Check: resource.TestCheckResourceAttr("arin_report_request.test", "ticket_number", "20260923-X1")},
		{ResourceName: "arin_report_request.test", ImportState: true, ImportStateId: "associations/20260923-X1", ImportStateVerify: true},
		{Config: first, PlanOnly: true},
		{Config: next, Check: resource.TestCheckResourceAttr("arin_report_request.test", "ticket_number", "20260923-X2")},
		{ResourceName: "arin_report_request.test", ImportState: true, ImportStateId: "reassignment/NET-192-0-2-0-1/20260923-X2", ImportStateVerify: true},
		{Config: next, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.tickets, "20260923-X2") }, Check: resource.TestCheckResourceAttr("arin_report_request.test", "ticket_available", "false")},
		{Config: next, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.submissions != 2 || len(f.tickets) != 1 || f.tickets["20260923-X1"].Status != "CLOSED" {
			return fmt.Errorf("report resubmitted on refresh/expiry or destroy mutated a ticket")
		}
		return nil
	}})
}
func TestAccReportPendingSubmissionRecovery(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprintf("lost_response_%t", lost), func(t *testing.T) {
			f, _ := setupReportFake(t)
			f.partialResponse = true
			f.readStatus = 500
			pendingError := "Report submission confirmed; import required"
			if lost {
				f.writeStatus = 500
				f.applyBeforeError = true
				pendingError = "Report submission remains uncertain"
			}
			config := strings.Replace(reportConfig("associations", ""), "\n}\n", "\n lifecycle { create_before_destroy = true }\n}\n", 1)
			workdir := t.TempDir()
			resource.Test(t, resource.TestCase{WorkingDir: workdir, ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{
				{Config: config, ExpectError: regexp.MustCompile("Report submission requires reconciliation")},
				{Config: config, PreConfig: func() { f.mu.Lock(); defer f.mu.Unlock(); f.readStatus = 0; f.writeStatus = 0 }, ExpectError: regexp.MustCompile(pendingError)},
				{PreConfig: func() {
					f.mu.Lock()
					if f.submissions != 1 {
						f.mu.Unlock()
						t.Fatal("uncertain submission replayed")
					}
					f.mu.Unlock()
					matches, err := filepath.Glob(filepath.Join(workdir, "work*", "terraform.tfstate"))
					if err != nil || len(matches) != 1 {
						t.Fatalf("cannot locate isolated state: %v", err)
					}
					cmd := exec.Command("terraform", "-chdir="+filepath.Dir(matches[0]), "state", "rm", "arin_report_request.test")
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("cannot remove pending test receipt: %v: %s", err, out)
					}
				}, ResourceName: "arin_report_request.test", ImportState: true, ImportStateId: "associations/20260923-X1", ImportStatePersist: true},
				{Config: config, PlanOnly: true},
			}, CheckDestroy: func(_ *terraform.State) error {
				f.mu.Lock()
				defer f.mu.Unlock()
				if f.submissions != 1 || len(f.tickets) != 1 {
					return fmt.Errorf("report recovery replayed submission or changed ticket")
				}
				return nil
			}})
		})
	}
}

func TestAccWhoWasReportsExcluded(t *testing.T) {
	for _, kind := range []string{"who_was_asn", "who_was_net"} {
		t.Run(kind, func(t *testing.T) {
			f, _ := setupReportFake(t)
			target := "64496"
			if kind == "who_was_net" {
				target = "192.0.2.1"
			}
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())},
				Steps:                    []resource.TestStep{{Config: reportConfig(kind, target), PlanOnly: true, ExpectError: regexp.MustCompile("report type must be associations or reassignment")}},
			})
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.submissions != 0 {
				t.Fatal("excluded WhoWas type submitted a report")
			}
		})
	}
}
