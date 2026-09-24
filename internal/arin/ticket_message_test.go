package arin

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type ticketMessageWireFunc func(*http.Request) (*http.Response, error)

func (f ticketMessageWireFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func ticketMessageXML(id string) string {
	return `<message xmlns="` + registrationNamespace + `" xmlns:m="` + messageNamespace + `"><m:messageId>` + id + `</m:messageId><m:createdDate>2026-09-23T00:00:00Z</m:createdDate><subject>Test &amp; evidence</subject><text><line number="1">First &lt;line&gt;</line><line number="2">Second line</line></text><category>JUSTIFICATION</category><attachmentReferences><attachmentReference><attachmentId>A1</attachmentId><attachmentFilename>evidence.txt</attachmentFilename></attachmentReference></attachmentReferences></message>`
}

func TestAddTicketMessage(t *testing.T) {
	for _, mode := range []string{"success", "closed", "preflight_forbidden", "append_forbidden", "lost_response", "invalid_identity", "partial_response", "verification_forbidden", "wrong_read_identity", "changed_content", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			var writes, reads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "ApiKey test-key" || r.URL.RawQuery != "" {
					t.Error("unsafe authentication or query")
				}
				switch {
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/summary"):
					if mode == "preflight_forbidden" {
						w.WriteHeader(403)
						return
					}
					status := "IN_PROGRESS"
					if mode == "closed" {
						status = "CLOSED"
					}
					fmt.Fprint(w, reportTicketXML("20260923-X1", "QUESTION", status))
				case r.Method == "PUT" && r.URL.Path == "/rest/ticket/20260923-X1/message":
					writes.Add(1)
					raw, err := io.ReadAll(r.Body)
					if err != nil {
						t.Fatal(err)
					}
					var payload struct {
						XMLName  xml.Name
						Subject  string `xml:"subject"`
						Category string `xml:"category"`
						Text     struct {
							Lines []struct {
								Number int    `xml:"number,attr"`
								Text   string `xml:",chardata"`
							} `xml:"line"`
						} `xml:"text"`
						Attachments []struct {
							Data     string `xml:"data"`
							Filename string `xml:"filename"`
						} `xml:"attachments>attachment"`
					}
					if xml.Unmarshal(raw, &payload) != nil || payload.XMLName.Local != "message" || payload.XMLName.Space != registrationNamespace || payload.Subject != "Test & evidence" || payload.Category != "JUSTIFICATION" || len(payload.Text.Lines) != 2 || payload.Text.Lines[0].Number != 0 || payload.Text.Lines[0].Text != "First <line>" || len(payload.Attachments) != 1 || payload.Attachments[0].Filename != "evidence.txt" || payload.Attachments[0].Data != base64.StdEncoding.EncodeToString([]byte("evidence")) {
						t.Errorf("message wire payload mismatch: %s", raw)
					}
					if strings.Contains(string(raw), "messageId") || strings.Contains(string(raw), "createdDate") {
						t.Error("generated fields submitted")
					}
					if mode == "append_forbidden" {
						w.WriteHeader(403)
						return
					}
					if mode == "lost_response" {
						conn, _, _ := w.(http.Hijacker).Hijack()
						conn.Close()
						return
					}
					if mode == "redirect" {
						w.Header().Set("Location", "/must-not-follow")
						w.WriteHeader(307)
						return
					}
					body := ticketMessageXML("7")
					if mode == "invalid_identity" {
						body = ticketMessageXML("../bad")
					}
					if mode == "partial_response" {
						body = strings.Replace(body, "JUSTIFICATION", "INVALID", 1)
					}
					fmt.Fprint(w, body)
				case r.Method == "GET" && r.URL.Path == "/rest/ticket/20260923-X1/message/7":
					reads.Add(1)
					if mode == "verification_forbidden" {
						w.WriteHeader(403)
						return
					}
					id := "7"
					if mode == "wrong_read_identity" {
						id = "8"
					}
					body := ticketMessageXML(id)
					if mode == "changed_content" {
						body = strings.Replace(body, "Second line", "Changed line", 1)
					}
					fmt.Fprint(w, body)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL, HTTPClient: &http.Client{Transport: ticketMessageWireFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodPut && (req.GetBody != nil || req.Body == nil || req.Body == http.NoBody) {
					t.Error("ticket append must have a non-rewindable payload")
				}
				return http.DefaultTransport.RoundTrip(req)
			})}})
			receipt, err := c.AddTicketMessage(context.Background(), "20260923-X1", RegistrationMessage{Subject: "Test & evidence", Text: []string{"First <line>", "Second line"}, Category: "JUSTIFICATION", Attachments: []RegistrationAttachment{{Filename: "evidence.txt", Data: []byte("evidence")}}})
			if mode == "closed" || mode == "preflight_forbidden" {
				if err == nil || writes.Load() != 0 || receipt != nil {
					t.Fatal("preflight failure submitted correspondence")
				}
				return
			}
			if writes.Load() != 1 || receipt == nil || receipt.TicketNumber != "20260923-X1" {
				t.Fatal("submission replayed or receipt lost")
			}
			if mode == "success" {
				if err != nil || !receipt.Confirmed || reads.Load() != 1 || receipt.Message.ID != "7" || len(receipt.Message.Text) != 2 || len(receipt.Message.Attachments) != 1 || receipt.Message.Attachments[0].ID != "A1" {
					t.Fatalf("message not confirmed: %+v %v", receipt, err)
				}
			} else {
				if err == nil || receipt.Confirmed {
					t.Fatal("unconfirmed submission accepted")
				}
				if (mode == "partial_response" || mode == "verification_forbidden" || mode == "wrong_read_identity") && (receipt.Message == nil || receipt.Message.ID != "7") {
					t.Fatal("trustworthy returned ID lost")
				}
			}
		})
	}
}

func TestTicketMessageValidation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	for _, message := range []RegistrationMessage{{}, {Subject: "bad\nsubject"}, {Subject: "test", Category: "INVALID"}, {Attachments: []RegistrationAttachment{{Filename: "../file"}}}, {Subject: strings.Repeat("<", maxResponseBytes/2)}} {
		if receipt, err := c.AddTicketMessage(context.Background(), "20260923-X1", message); err == nil || receipt != nil {
			t.Fatal("invalid message accepted")
		}
	}
	for _, id := range []string{"0", "-1", "01", "../7", "9223372036854775808"} {
		if _, err := c.GetTicketMessage(context.Background(), "20260923-X1", id); err == nil {
			t.Fatal("invalid ID accepted")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("invalid input made requests")
	}
	good := ticketMessageXML("7")
	for _, bad := range []string{strings.Replace(good, "</message>", "<m:messageId>8</m:messageId></message>", 1), strings.Replace(good, messageNamespace, "urn:wrong", 1), strings.Replace(good, registrationNamespace, "urn:wrong", 1)} {
		if message, err := decodeTicketMessage([]byte(bad), ""); err == nil || message != nil {
			t.Fatal("ambiguous identity accepted")
		}
	}
}
