package arin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func ticketMessageXML(id string) string {
	return `<message xmlns="` + registrationNamespace + `" xmlns:m="` + messageNamespace + `"><m:messageId>` + id + `</m:messageId><m:createdDate>2026-09-23T00:00:00Z</m:createdDate><subject>Test &amp; evidence</subject><text><line number="1">First &lt;line&gt;</line><line number="2">Second line</line></text><category>JUSTIFICATION</category><attachmentReferences><attachmentReference><attachmentId>A1</attachmentId><attachmentFilename>evidence.txt</attachmentFilename></attachmentReference></attachmentReferences></message>`
}

func TestTicketMessageValidation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
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
