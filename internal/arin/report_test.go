package arin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func reportTicketXML(number, kind, status string) string {
	return fmt.Sprintf(`<ticket xmlns="http://www.arin.net/regrws/core/v1"><ticketNo>%s</ticketNo><webTicketType>%s</webTicketType><webTicketStatus>%s</webTicketStatus><createdDate>2026-09-23</createdDate></ticket>`, number, kind, status)
}
func TestReportRequests(t *testing.T) {
	tests := []struct {
		request    ReportRequest
		path, kind string
	}{
		{ReportRequest{Type: ReportAssociations}, "/rest/report/associations", "ASSOCIATIONS_REPORT"},
		{ReportRequest{Type: ReportReassignment, Target: "NET-192-0-2-0-1"}, "/rest/report/reassignment/NET-192-0-2-0-1", "USER_REASSIGNMENT_REPORT"},
		{ReportRequest{Type: ReportWhoWasASN, Target: "64496"}, "/rest/report/whoWas/asn/64496", "WHOWAS_REPORT"},
		{ReportRequest{Type: ReportWhoWasNet, Target: "2001:db8::1"}, "/rest/report/whoWas/net/2001:db8::1", "WHOWAS_REPORT"},
	}
	for _, tc := range tests {
		t.Run(tc.request.Type, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, _ := io.ReadAll(r.Body)
				if r.Method != "GET" || r.URL.Path != tc.path || r.URL.RawQuery != "" || len(body) != 0 || r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Content-Type") != "application/xml" {
					t.Error("incorrect report request")
				}
				w.WriteHeader(202)
				fmt.Fprint(w, reportTicketXML("20260923-X1", tc.kind, "IN_PROGRESS"))
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			ticket, err := c.RequestReport(context.Background(), tc.request)
			if err != nil || ticket.Number != "20260923-X1" || ticket.Status != "IN_PROGRESS" || calls != 1 {
				t.Fatalf("report result or request count incorrect: %v", err)
			}
		})
	}
}
func TestReportValidation(t *testing.T) {
	for _, r := range []ReportRequest{{Type: "unknown"}, {Type: ReportAssociations, Target: "unexpected"}, {Type: ReportReassignment, Target: "../ticket"}, {Type: ReportReassignment, Target: "FT-684"}, {Type: ReportWhoWasASN, Target: "0"}, {Type: ReportWhoWasASN, Target: "064496"}, {Type: ReportWhoWasASN, Target: "4294967296"}, {Type: ReportWhoWasNet, Target: "192.0.2.0/24"}, {Type: ReportWhoWasNet, Target: "fe80::1%en0"}, {Type: ReportWhoWasNet, Target: "::ffff:192.0.2.1"}} {
		if r.Validate() == nil {
			t.Fatalf("invalid report accepted: %#v", r)
		}
	}
}
func TestReportGETDoesNotReplayAfterAcceptance(t *testing.T) {
	for _, mutating := range []bool{false, true} {
		t.Run(fmt.Sprint(mutating), func(t *testing.T) {
			var mu sync.Mutex
			calls := 0
			warmConnection := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				io.Copy(io.Discard, r.Body)
				if strings.HasSuffix(r.URL.Path, "/summary") {
					warmConnection = r.RemoteAddr
					fmt.Fprint(w, reportTicketXML("20260923-X1", "ASSOCIATIONS_REPORT", "RESOLVED"))
					return
				}
				calls++
				if calls == 1 {
					if r.RemoteAddr != warmConnection {
						t.Error("test did not reuse the warm connection")
					}
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					conn.Close()
					return
				}
				fmt.Fprint(w, reportTicketXML("20260923-X2", "ASSOCIATIONS_REPORT", "IN_PROGRESS"))
			}))
			defer server.Close()
			transport := http.DefaultTransport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: &http.Client{Transport: transport}})
			ctx := context.Background()
			if _, err := c.GetTicket(ctx, "20260923-X1"); err != nil {
				t.Fatal(err)
			}
			var err error
			if mutating {
				_, err = c.RequestReport(ctx, ReportRequest{Type: ReportAssociations})
			} else {
				_, err = c.request(ctx, http.MethodGet, c.baseURL, "/rest/report/associations", "application/xml", true, nil)
			}
			mu.Lock()
			defer mu.Unlock()
			if mutating {
				if err == nil || calls != 1 {
					t.Fatalf("accepted report was replayed: calls=%d err=%v", calls, err)
				}
			} else if err != nil || calls != 2 {
				t.Fatalf("test did not exercise Go's GET replay behavior: calls=%d err=%v", calls, err)
			}
		})
	}
}
func TestReportHTTP2EmptyBody(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.ProtoMajor != 2 || len(body) != 0 {
			t.Error("invalid HTTP/2 report request")
		}
		fmt.Fprint(w, reportTicketXML("20260923-X1", "ASSOCIATIONS_REPORT", "RESOLVED"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if _, err := c.RequestReport(context.Background(), ReportRequest{Type: ReportAssociations}); err != nil {
		t.Fatal(err)
	}
}
func TestReportRetainsTicketOnIncompleteResponse(t *testing.T) {
	for _, body := range []string{reportTicketXML("20260923-X1", "ASSOCIATIONS_REPORT", ""), reportTicketXML("20260923-X1", "ORG_CREATE", "PENDING_REVIEW")} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
		ticket, err := c.RequestReport(context.Background(), ReportRequest{Type: ReportAssociations})
		server.Close()
		if err == nil || ticket == nil || ticket.Number != "20260923-X1" {
			t.Fatal("accepted report identity lost on validation error")
		}
	}
}
