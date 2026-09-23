package arin

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRemoveNetAssignment(t *testing.T) {
	for _, outcome := range []string{"complete", "pending", "malformed", "lost", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			n := testRegisteredNet()
			n.POCs = []NetPOC{{Handle: "TECH-ARIN", Function: "T", Description: "Tech"}}
			original, _ := n.marshal()
			var puts atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					w.Write(original)
					return
				}
				puts.Add(1)
				if r.Method != http.MethodPut || r.URL.Path != "/rest/net/"+n.Handle+"/remove" || r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Content-Type") != "application/xml" {
					t.Error("incorrect removal request")
					w.WriteHeader(400)
					return
				}
				body, _ := io.ReadAll(r.Body)
				var got, before registeredNetXML
				if xml.Unmarshal(body, &got) != nil || xml.Unmarshal(original, &before) != nil {
					t.Error("invalid NET payload")
					w.WriteHeader(400)
					return
				}
				messages := got.Messages
				got.Messages = nil
				if !reflect.DeepEqual(got, before) {
					t.Error("removal changed existing NET fields")
				}
				if messages == nil || len(messages.Messages) != 2 {
					t.Error("messages missing")
					w.WriteHeader(400)
					return
				}
				first := messages.Messages[0]
				if first.Subject != "Removal & evidence" || first.Category != "JUSTIFICATION" || first.Text == nil || len(first.Text.Lines) != 2 || first.Text.Lines[0].Text != "First <line>" || first.Text.Lines[1].Text != "" || first.Attachments == nil {
					t.Error("message fields did not round trip")
				}
				attachment := first.Attachments.Attachments[0]
				data, err := base64.StdEncoding.DecodeString(attachment.Data)
				if err != nil || !reflect.DeepEqual(data, []byte{0, 1, 2, 255}) || attachment.Filename != "evidence.bin" || messages.Messages[1].Category != "NONE" {
					t.Error("attachment or default category changed")
				}
				switch outcome {
				case "complete":
					fmt.Fprintf(w, `<ticketedRequest xmlns="%s">%s</ticketedRequest>`, registrationNamespace, original)
				case "pending":
					w.WriteHeader(202)
					fmt.Fprintf(w, `<ticketedRequest xmlns="%s"><ticket><ticketNo>20260923-X1</ticketNo><webTicketStatus>PENDING_REVIEW</webTicketStatus></ticket></ticketedRequest>`, registrationNamespace)
				case "malformed":
					fmt.Fprintf(w, `<ticketedRequest xmlns="%s"><net/><ticket><ticketNo>20260923-X1</ticketNo><webTicketStatus>PENDING_REVIEW</webTicketStatus></ticket></ticketedRequest>`, registrationNamespace)
				case "lost":
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
				case "rejected":
					w.WriteHeader(400)
				}
			}))
			defer server.Close()
			c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
			result, err := c.RemoveNetAssignment(context.Background(), n.Handle, []RegistrationMessage{
				{Subject: "Removal & evidence", Text: []string{"First <line>", ""}, Category: "JUSTIFICATION", Attachments: []RegistrationAttachment{{Filename: "evidence.bin", Data: []byte{0, 1, 2, 255}}}},
				{Text: []string{"Second message"}},
			})
			if puts.Load() != 1 {
				t.Fatalf("removal submitted %d times", puts.Load())
			}
			switch outcome {
			case "complete":
				if err != nil || result == nil || result.Net == nil || result.Net.Handle != n.Handle {
					t.Fatalf("completed removal lost: %v", err)
				}
			case "pending":
				if err != nil || result == nil || result.TicketNumber != "20260923-X1" || result.TicketStatus != "PENDING_REVIEW" {
					t.Fatalf("pending receipt lost: %v", err)
				}
			case "malformed":
				if err == nil || result == nil || result.TicketNumber != "20260923-X1" {
					t.Fatal("malformed NET discarded recoverable ticket")
				}
			default:
				if err == nil {
					t.Fatal("unconfirmed removal accepted")
				}
			}
		})
	}
}

func TestRemoveNetGuards(t *testing.T) {
	for _, mode := range []string{"missing", "direct", "invalid-message", "oversized-message"} {
		t.Run(mode, func(t *testing.T) {
			var calls, writes atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet {
					writes.Add(1)
					w.WriteHeader(500)
					return
				}
				if mode == "missing" {
					w.WriteHeader(404)
					return
				}
				n := testRegisteredNet()
				n.Blocks[0].Type = "DA"
				body, _ := n.marshal()
				w.Write(body)
			}))
			defer server.Close()
			c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
			var messages []RegistrationMessage
			if mode == "invalid-message" {
				messages = []RegistrationMessage{{Subject: "bad\x00subject"}}
			}
			if mode == "oversized-message" {
				messages = []RegistrationMessage{{Text: []string{strings.Repeat("x", maxResponseBytes+1)}}}
			}
			result, err := c.RemoveNetAssignment(context.Background(), "NET-192-0-2-0-2", messages)
			if writes.Load() != 0 {
				t.Fatal("unsafe write issued")
			}
			if mode == "missing" {
				if err != nil || result == nil {
					t.Fatal("missing NET not idempotent")
				}
			} else if err == nil {
				t.Fatal("invalid removal accepted")
			}
			if strings.Contains(mode, "message") && calls.Load() != 0 {
				t.Fatal("invalid messages reached the API")
			}
		})
	}
}

func TestRegistrationMessageValidation(t *testing.T) {
	for _, message := range []RegistrationMessage{
		{}, {Subject: "x", Category: "OTHER"}, {Subject: "x\ny"}, {Text: []string{"x\ny"}}, {Text: []string{"\xff"}},
		{Attachments: []RegistrationAttachment{{Filename: "../file"}}}, {Attachments: []RegistrationAttachment{{Filename: ""}}},
		{Attachments: []RegistrationAttachment{{Filename: "a", Data: make([]byte, maxResponseBytes)}}},
	} {
		if _, err := registrationMessages([]RegistrationMessage{message}); err == nil {
			t.Fatal("invalid message accepted")
		}
	}
}

func TestRemoveNet404RequiresConfirmedAbsence(t *testing.T) {
	for _, absent := range []bool{false, true} {
		t.Run(fmt.Sprint(absent), func(t *testing.T) {
			var writes atomic.Int64
			n := testRegisteredNet()
			body, _ := n.marshal()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					writes.Add(1)
					w.WriteHeader(404)
					return
				}
				if absent && writes.Load() > 0 {
					w.WriteHeader(404)
					return
				}
				w.Write(body)
			}))
			defer server.Close()
			c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
			result, err := c.RemoveNetAssignment(context.Background(), n.Handle, nil)
			if writes.Load() != 1 {
				t.Fatal("removal resubmitted")
			}
			if absent {
				if err != nil || result == nil {
					t.Fatal("confirmed absence rejected")
				}
			} else if err == nil {
				t.Fatal("endpoint 404 mistaken for NET absence")
			}
		})
	}
}
