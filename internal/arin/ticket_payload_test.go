package arin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func fullTicketXML(status string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><ticket xmlns="` + registrationNamespace + `" xmlns:m="` + messageNamespace + `" xmlns:s="http://www.arin.net/regrws/shared-ticket/v1"><ticketNo>20260923-X1</ticketNo><createdDate>2026-09-23T00:00:00Z</createdDate><resolvedDate>2026-09-23T01:00:00Z</resolvedDate><closedDate/><updatedDate>2026-09-23T01:00:00Z</updatedDate><webTicketType>QUESTION</webTicketType><webTicketResolution>ANSWERED</webTicketResolution><webTicketStatus>` + status + `</webTicketStatus><s:shared>true</s:shared><s:orgHandle>EXAMPLE-1</s:orgHandle><messageReferences><messageReference><m:messageId>7</m:messageId><subject>A &amp; B</subject></messageReference></messageReferences><!-- preserve this --></ticket>`
}
func TestCloseTicketWithPayload(t *testing.T) {
	for _, mode := range []string{"success", "closed", "unresolved", "lost", "ignored", "wrong_identity", "verification_forbidden"} {
		t.Run(mode, func(t *testing.T) {
			var writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "ApiKey test-key" {
					t.Error("missing authentication")
				}
				if r.URL.Path == "/rest/ticket/20260923-X1/summary" && r.Method == "GET" {
					if mode == "verification_forbidden" {
						w.WriteHeader(403)
						return
					}
					status := "CLOSED"
					if mode == "ignored" {
						status = "RESOLVED"
					}
					fmt.Fprint(w, fullTicketXML(status))
					return
				}
				if r.URL.Path != "/rest/ticket/20260923-X1" || r.URL.RawQuery != "msgRefs=true" {
					t.Error("wrong detail endpoint")
					w.WriteHeader(500)
					return
				}
				if r.Method == "GET" {
					status := "RESOLVED"
					if mode == "closed" {
						status = "CLOSED"
					}
					if mode == "unresolved" {
						status = "IN_PROGRESS"
					}
					fmt.Fprint(w, fullTicketXML(status))
					return
				}
				if r.Method != "PUT" {
					t.Error("unexpected method")
					w.WriteHeader(500)
					return
				}
				writes.Add(1)
				body, _ := io.ReadAll(r.Body)
				if string(body) != fullTicketXML("CLOSED") {
					t.Error("full PUT changed server-owned data")
				}
				if mode == "lost" {
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				}
				body = []byte(fullTicketXML("CLOSED"))
				if mode == "wrong_identity" {
					body = []byte(strings.Replace(string(body), "20260923-X1", "20260923-X2", 1))
				}
				w.Write(body)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			result, err := c.CloseTicketWithPayload(context.Background(), "20260923-X1")
			if mode == "success" || mode == "closed" {
				if err != nil || result.Status != "CLOSED" {
					t.Fatalf("closure: %v", err)
				}
			} else if err == nil {
				t.Fatal("unconfirmed closure accepted")
			}
			want := int32(1)
			if mode == "closed" || mode == "unresolved" {
				want = 0
			}
			if writes.Load() != want {
				t.Fatal("unexpected or repeated mutation")
			}
		})
	}
}
func TestFullTicketPayloadPreservationAndRejection(t *testing.T) {
	good := fullTicketXML("RESOLVED")
	for _, body := range []string{good, strings.Replace(good, "<webTicketStatus>RESOLVED</webTicketStatus>", `<c:webTicketStatus xmlns:c="`+registrationNamespace+`">RESOLVED</c:webTicketStatus>`, 1)} {
		out, _, err := closedTicketPayload([]byte(body), "20260923-X1")
		if err != nil || string(out) != strings.Replace(body, "RESOLVED", "CLOSED", 1) {
			t.Fatalf("byte preservation failed: %v", err)
		}
	}
	for _, body := range []string{
		strings.Replace(good, "</ticket>", "<webTicketStatus>RESOLVED</webTicketStatus></ticket>", 1),
		strings.Replace(good, "<webTicketStatus>", `<webTicketStatus xmlns="urn:foreign">`, 1),
		strings.Replace(good, "RESOLVED", "<nested>RESOLVED</nested>", 1),
		strings.Replace(good, "</ticket>", "<messages><message/></messages></ticket>", 1),
		strings.Replace(good, "20260923-X1", "20260923-X2", 1),
	} {
		if _, _, err := closedTicketPayload([]byte(body), "20260923-X1"); err == nil {
			t.Fatal("unsafe full payload accepted")
		}
	}
}
