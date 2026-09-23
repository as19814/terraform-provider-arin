package arin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloseTicket(t *testing.T) {
	for _, initial := range []string{"RESOLVED", "CLOSED", "PENDING_REVIEW"} {
		t.Run(initial, func(t *testing.T) {
			status := initial
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "PUT" {
					writes++
					if r.URL.Path != "/rest/ticket/20260923-X1/ticketStatus/CLOSED" || r.URL.RawQuery != "msgRefs=true" {
						t.Error("incorrect closure endpoint")
					}
					status = "CLOSED"
				} else if r.URL.Path != "/rest/ticket/20260923-X1/summary" {
					t.Error("unexpected ticket read")
				}
				fmt.Fprint(w, reportTicketXML("20260923-X1", "ASSOCIATIONS_REPORT", status))
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			result, err := c.CloseTicket(context.Background(), "20260923-X1")
			if initial == "PENDING_REVIEW" {
				if err == nil || writes != 0 {
					t.Fatal("unresolved ticket closure permitted")
				}
				return
			}
			wantWrites := 0
			if initial == "RESOLVED" {
				wantWrites = 1
			}
			if err != nil || result.Status != "CLOSED" || writes != wantWrites {
				t.Fatalf("closure not confirmed: %v", err)
			}
		})
	}
}
func TestCloseTicketVerificationAndIdentity(t *testing.T) {
	for _, mode := range []string{"wrong_identity", "unchanged", "lost_response", "read_forbidden"} {
		t.Run(mode, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				number, status := "20260923-X1", "RESOLVED"
				if r.Method == "PUT" {
					writes++
					status = "CLOSED"
					if mode == "wrong_identity" {
						number = "20260923-X2"
					}
					if mode == "lost_response" {
						w.WriteHeader(500)
						return
					}
				}
				if writes > 0 && r.Method == "GET" && mode == "read_forbidden" {
					w.WriteHeader(403)
					return
				}
				fmt.Fprint(w, reportTicketXML(number, "ASSOCIATIONS_REPORT", status))
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			_, err := c.CloseTicket(context.Background(), "20260923-X1")
			if err == nil || writes != 1 {
				t.Fatal("unconfirmed closure accepted or replayed")
			}
		})
	}
}
func TestTicketDecodeRejectsAmbiguousIdentity(t *testing.T) {
	good := reportTicketXML("20260923-X1", "ASSOCIATIONS_REPORT", "IN_PROGRESS")
	for _, bad := range []string{strings.Replace(good, "</ticket>", "<ticketNo>20260923-X2</ticketNo></ticket>", 1), strings.Replace(good, "20260923-X1", "../ticket", 1), strings.Replace(good, registrationNamespace, "https://wrong.example", 1)} {
		if ticket, err := decodeTicket([]byte(bad), ""); err == nil || ticket != nil {
			t.Fatal("ambiguous identity retained")
		}
	}
}
