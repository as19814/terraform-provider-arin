package arin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type ticketOTETransport struct {
	number string
	writes int
}

func (t *ticketOTETransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || req.URL.Host != "reg.ote.arin.net" {
		return nil, errors.New("unexpected sandbox origin")
	}
	if req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/rest/report/") {
		return nil, errors.New("ticket status test must not submit reports")
	}
	if req.Method != "GET" {
		if req.Method != http.MethodPut || req.URL.Path != "/rest/ticket/"+t.number+"/ticketStatus/CLOSED" || req.URL.RawQuery != "msgRefs=true" {
			return nil, errors.New("unexpected sandbox mutation")
		}
		t.writes++
	}
	return http.DefaultTransport.RoundTrip(req)
}
func TestOTETicketStatusClientLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and org handle")
	}
	request := ReportRequest{Type: ReportAssociations}
	identity, _ := json.Marshal(struct {
		Org     string
		Request ReportRequest
	}{org, request})
	hash := sha256.Sum256(identity)
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cache, "terraform-provider-arin", fmt.Sprintf("ote-report-%x.json", hash[:8])))
	if err != nil {
		t.Fatal("requires an existing disposable associations report receipt from TestOTEReportClientLifecycle")
	}
	var receipt reportOTEReceipt
	if err = json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Request != request || receipt.Ticket == nil {
		t.Fatal("report receipt does not identify an accepted associations request")
	}
	trace := &ticketOTETransport{number: receipt.Ticket.Number}
	c, err := New(Config{APIKey: key, BaseURL: OTEURL, RDAPBaseURL: RDAPOTEURL, HTTPClient: &http.Client{Transport: trace}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	before, err := c.GetTicket(ctx, receipt.Ticket.Number)
	if err != nil {
		t.Fatal(err)
	}
	if !request.MatchesTicket(*before) {
		t.Fatal("receipt ticket is not an associations report")
	}
	if before.Status != "CLOSED" && before.Status != "RESOLVED" {
		t.Skipf("disposable report is still %s", before.Status)
	}
	closed, err := c.CloseTicket(ctx, before.Number)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != "CLOSED" {
		t.Fatal("closure not confirmed")
	}
	if before.Status == "CLOSED" && trace.writes != 0 {
		t.Fatal("already closed ticket caused a write")
	}
	if before.Status == "RESOLVED" && trace.writes != 1 {
		t.Fatal("resolved ticket was not closed exactly once")
	}
	t.Logf("guarded closure from %s performed %d writes", before.Status, trace.writes)
	// Probe only the disposable report's already-closed status. This establishes
	// the native endpoint's behavior without touching unrelated account tickets.
	_, err = c.request(ctx, http.MethodPut, c.baseURL, "/rest/ticket/"+before.Number+"/ticketStatus/CLOSED?msgRefs=true", "application/xml", true, nil)
	var api *APIError
	if !errors.As(err, &api) || api.StatusCode != 400 || api.Code != "E_BAD_REQUEST" || !strings.Contains(api.Message, "Modifications to closed tickets") {
		t.Fatalf("unexpected native already-closed result: %v", err)
	}
	after, readErr := c.GetTicket(ctx, before.Number)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !reflect.DeepEqual(after, closed) {
		t.Fatal("rejected closed-ticket update changed ticket metadata")
	}
	t.Log("native already-closed PUT rejected with HTTP 400; metadata unchanged")
}
