package arin

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testAssignment() NetAssignment {
	return NetAssignment{ParentNetHandle: "NET-192-0-2-0-1", Name: "EXAMPLE-NET", CustomerHandle: "C123", Prefixes: []string{"192.0.2.0/29"}, Comments: []string{"Operational & <comment>"}}
}
func testRegisteredNet() RegisteredNet {
	n := testAssignment().net()
	n.OriginASNs = []string{"AS64496"}
	n.Handle = "NET-192-0-2-0-2"
	n.RegistrationDate = "2026-01-01T00:00:00Z"
	return n
}
func TestNetAssignmentValidation(t *testing.T) {
	for _, change := range []func(*NetAssignment){
		func(a *NetAssignment) { a.OrgHandle = "ORG-1" },
		func(a *NetAssignment) { a.CustomerHandle = "" },
		func(a *NetAssignment) { a.Reallocate = true },
		func(a *NetAssignment) { a.ParentNetHandle = "../bad" },
		func(a *NetAssignment) { a.Prefixes = nil },
		func(a *NetAssignment) { a.Prefixes = []string{"192.0.2.1/29"} },
		func(a *NetAssignment) { a.Prefixes = []string{"2001:db8::/65"} },
		func(a *NetAssignment) { a.Prefixes = []string{"192.0.2.0/24", "192.0.2.0/29"} },
		func(a *NetAssignment) { a.Prefixes = []string{"192.0.2.0/29", "192.0.2.16/29"} },
		func(a *NetAssignment) { a.Prefixes = []string{"192.0.2.0/29", "192.0.2.8/29"} },
		func(a *NetAssignment) { a.Prefixes = []string{"192.0.2.0/29", "2001:db8::/64"} },
		func(a *NetAssignment) { a.Name = "bad/name" },
		func(a *NetAssignment) { a.OriginASNs = []string{"64496"} },
	} {
		a := testAssignment()
		change(&a)
		if a.Validate() == nil {
			t.Fatal("accepted invalid assignment")
		}
	}
	for _, reallocate := range []bool{true, false} {
		a := testAssignment()
		a.CustomerHandle = ""
		a.OrgHandle = "ORG-1"
		a.Reallocate = reallocate
		a.Prefixes = []string{"2001:db8::/63", "2001:db8:0:2::/64"}
		if err := a.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
func TestRegisteredNetRoundTrip(t *testing.T) {
	n := testRegisteredNet()
	b, _ := n.marshal()
	out, err := decodeRegisteredNet(b, n.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != n.Name || out.Blocks[0].EndAddress != "192.0.2.7" || out.Comments[0] != n.Comments[0] {
		t.Fatalf("bad round trip: %+v", out)
	}
	for _, body := range []string{
		strings.Replace(string(b), "<netName>", "<future>new</future><netName>", 1),
		strings.Replace(string(b), "<type>", "<extension>new</extension><type>", 1),
		strings.Replace(string(b), "192.0.2.7", "192.0.2.8", 1),
		strings.Replace(string(b), "<version>4</version>", "", 1),
	} {
		if _, err := decodeRegisteredNet([]byte(body), n.Handle); err == nil {
			t.Fatal("accepted incomplete network")
		}
	}
}
func TestNetTicketResponse(t *testing.T) {
	for _, status := range []string{"PENDING_REVIEW", "RESOLVED"} {
		body := fmt.Sprintf(`<ticketedRequest xmlns="%s"><ticket><ticketNo>20260922-X1</ticketNo><webTicketStatus>%s</webTicketStatus></ticket></ticketedRequest>`, registrationNamespace, status)
		out, err := decodeNetWriteResult([]byte(body))
		if err != nil || out.Net != nil || out.TicketNumber != "20260922-X1" || out.TicketStatus != status {
			t.Fatalf("ticket lost: %+v %v", out, err)
		}
	}
	if _, err := decodeNetWriteResult([]byte(`<ticketedRequest xmlns="http://www.arin.net/regrws/core/v1"/>`)); err == nil {
		t.Fatal("accepted empty result")
	}
}
func TestNetAssignmentClientLifecycle(t *testing.T) {
	ctx := context.Background()
	var stored []byte
	puts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Content-Type") != "application/xml" {
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		switch r.Method {
		case "GET":
			if stored == nil {
				w.WriteHeader(404)
			} else {
				w.Write(stored)
			}
		case "PUT":
			b, _ := io.ReadAll(r.Body)
			var n registeredNetXML
			if xml.Unmarshal(b, &n) != nil {
				t.Error("invalid XML")
				w.WriteHeader(400)
				return
			}
			puts++
			if strings.HasSuffix(r.URL.Path, "/reassign") {
				if n.Handle != "" || n.RegistrationDate != "" || len(n.POCs) != 0 || n.CustomerHandle != "C123" {
					t.Error("bad create payload")
				}
				n.Handle = "NET-192-0-2-0-2"
				n.RegistrationDate = "2026-01-01T00:00:00Z"
				stored, _ = xml.Marshal(n)
				fmt.Fprintf(w, `<ticketedRequest xmlns="%s">%s</ticketedRequest>`, registrationNamespace, stored)
			} else {
				if n.Handle != "NET-192-0-2-0-2" || n.RegistrationDate != "2026-01-01T00:00:00Z" || n.Blocks[0].Type != "S" || n.CustomerHandle != "C123" {
					t.Error("update lost immutable fields")
				}
				stored = b
				w.Write(stored)
			}
		case "DELETE":
			fmt.Fprintf(w, `<ticketedRequest xmlns="%s">%s</ticketedRequest>`, registrationNamespace, stored)
			stored = nil
		}
	}))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
	result, err := c.CreateNetAssignment(ctx, testAssignment())
	if err != nil {
		t.Fatal(err)
	}
	handle := result.Net.Handle
	updated, err := c.UpdateRegisteredNet(ctx, handle, "UPDATED", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "UPDATED" || len(updated.Comments) != 0 || len(updated.OriginASNs) != 0 {
		t.Fatal("metadata not cleared")
	}
	if _, err = c.DeleteNetAssignment(ctx, handle); err != nil {
		t.Fatal(err)
	}
	if _, err = c.GetRegisteredNet(ctx, handle); !IsNotFound(err) {
		t.Fatal("network not removed")
	}
	if _, err = c.DeleteNetAssignment(ctx, handle); err != nil {
		t.Fatal(err)
	}
	if puts != 2 {
		t.Fatal("unexpected write retries")
	}
}
func TestNetAssignmentDeletionRefusesDirectAllocation(t *testing.T) {
	n := testRegisteredNet()
	n.Blocks[0].Type = "DA"
	body, _ := n.marshal()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" {
			t.Error("direct allocation mutated")
		}
		w.Write(body)
	}))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
	if _, err := c.DeleteNetAssignment(context.Background(), n.Handle); err == nil {
		t.Fatal("allowed direct allocation deletion")
	}
	if calls != 1 {
		t.Fatal("unexpected calls")
	}
}

func TestNetTicketPreservedWithInvalidNetwork(t *testing.T) {
	body := `<ticketedRequest xmlns="http://www.arin.net/regrws/core/v1"><net/><ticket><ticketNo>20260922-X1</ticketNo><webTicketStatus>PENDING_REVIEW</webTicketStatus></ticket></ticketedRequest>`
	result, err := decodeNetWriteResult([]byte(body))
	if err == nil || result == nil || result.TicketNumber != "20260922-X1" {
		t.Fatalf("lost reconciliation ticket: %+v %v", result, err)
	}
}
func TestNetAssignmentPendingAndFailedWrites(t *testing.T) {
	for _, status := range []int{200, 202, 403, 409, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "PUT" || r.URL.Path != "/rest/net/NET-192-0-2-0-1/reallocate" {
					t.Error("incorrect reallocation request")
				}
				w.WriteHeader(status)
				fmt.Fprint(w, `<ticketedRequest xmlns="http://www.arin.net/regrws/core/v1"><ticket><ticketNo>20260922-X1</ticketNo><webTicketStatus>PENDING_REVIEW</webTicketStatus></ticket></ticketedRequest>`)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			a := testAssignment()
			a.Reallocate = true
			a.CustomerHandle = ""
			a.OrgHandle = "ORG-1"
			result, err := c.CreateNetAssignment(context.Background(), a)
			if status < 300 {
				if err != nil || result.Net != nil || result.TicketNumber != "20260922-X1" {
					t.Fatalf("pending ticket not preserved: %+v %v", result, err)
				}
			} else if err == nil {
				t.Fatal("ignored failed write")
			}
			if calls != 1 {
				t.Fatal("retried write")
			}
		})
	}
}
func TestNetUpdateCannotChangeIdentity(t *testing.T) {
	original := testRegisteredNet()
	body, _ := original.marshal()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Write(body)
			return
		}
		changed := original
		changed.CustomerHandle = "C999"
		b, _ := changed.marshal()
		w.Write(b)
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
	if _, err := c.UpdateRegisteredNet(context.Background(), original.Handle, "UPDATED", nil, nil, nil); err == nil {
		t.Fatal("accepted changed recipient")
	}
}

func TestNetUpdateReadErrorPreventsMutation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" {
			t.Error("mutated without fresh identity")
		}
		w.WriteHeader(403)
	}))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
	if _, err := c.UpdateRegisteredNet(context.Background(), "NET-192-0-2-0-2", "UPDATED", nil, nil, nil); err == nil {
		t.Fatal("ignored read error")
	}
	if calls != 1 {
		t.Fatal("unexpected calls")
	}
}
func TestNetDeletePendingTicket(t *testing.T) {
	n := testRegisteredNet()
	body, _ := n.marshal()
	deletes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Write(body)
			return
		}
		deletes++
		w.WriteHeader(202)
		fmt.Fprint(w, `<ticketedRequest xmlns="http://www.arin.net/regrws/core/v1"><ticket><ticketNo>20260922-X1</ticketNo><webTicketStatus>IN_PROGRESS</webTicketStatus></ticket></ticketedRequest>`)
	}))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
	result, err := c.DeleteNetAssignment(context.Background(), n.Handle)
	if err != nil || result.Net != nil || result.TicketNumber != "20260922-X1" {
		t.Fatalf("pending deletion not retained: %+v %v", result, err)
	}
	if deletes != 1 {
		t.Fatal("retried pending deletion")
	}
}

func TestRegisteredNetNumericOrigins(t *testing.T) {
	body, _ := testRegisteredNet().marshal()
	body = []byte(strings.ReplaceAll(string(body), "<originAS>AS64496</originAS>", "<originAS>64496</originAS>"))
	n, err := decodeRegisteredNet(body, "NET-192-0-2-0-2")
	if err != nil || len(n.OriginASNs) != 1 || n.OriginASNs[0] != "AS64496" {
		t.Fatalf("numeric origin not normalized: %+v %v", n, err)
	}
}

func TestRetiredNetOriginsRejected(t *testing.T) {
	a := testAssignment()
	a.OriginASNs = []string{"AS64496"}
	if err := a.Validate(); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatal("retired origin field accepted")
	}
}
func TestFindNetAssignmentRejectsOtherRecipient(t *testing.T) {
	n := testRegisteredNet()
	n.CustomerHandle = "C999"
	body, _ := n.marshal()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
	if _, err := c.FindNetAssignment(context.Background(), testAssignment()); err == nil {
		t.Fatal("adopted another recipient's network")
	}
}

func TestFindNetAssignmentUsesCompleteRange(t *testing.T) {
	a := testAssignment()
	a.Prefixes = []string{"192.0.2.0/30", "192.0.2.4/31"}
	n := a.net()
	n.Handle = "NET-192-0-2-0-2"
	n.RegistrationDate = "2026-01-01T00:00:00Z"
	body, _ := n.marshal()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/rest/net/mostSpecificNet/192.0.2.0/192.0.2.5" {
			t.Error("did not query complete registration range")
			w.WriteHeader(404)
			return
		}
		w.Write(body)
	}))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
	found, err := c.FindNetAssignment(context.Background(), a)
	if err != nil || found == nil || calls != 1 {
		t.Fatalf("multi-block reconciliation failed: %+v %v calls=%d", found, err, calls)
	}
}

func TestNetMetadataPatchPreservesOmittedFields(t *testing.T) {
	current := testRegisteredNet()
	current.POCs = []NetPOC{{Handle: "TECH-1", Function: "T", Description: "Tech"}}
	body, _ := current.marshal()
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Write(body)
			return
		}
		writes++
		b, _ := io.ReadAll(r.Body)
		var n registeredNetXML
		if err := xml.Unmarshal(b, &n); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if n.Name != "CHANGED" || n.CustomerHandle != current.CustomerHandle || n.RegistrationDate != current.RegistrationDate || len(n.POCs) != 1 || n.POCs[0].Handle != "TECH-1" || n.Comments == nil || n.Comments.Lines[0].Text != current.Comments[0] {
			t.Error("omitted metadata or identity lost")
		}
		w.Write(b)
	}))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
	name := "CHANGED"
	if _, err := c.UpdateNetMetadata(context.Background(), current.Handle, NetMetadataPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatal("unexpected mutations")
	}
}
func TestNetPOCValidation(t *testing.T) {
	for _, role := range []string{"AD", "R", "D", "unknown"} {
		if ValidateNetMetadata("EXAMPLE", nil, []NetPOC{{Handle: "POC-1", Function: role}}) == nil {
			t.Fatalf("accepted unsupported role %s", role)
		}
	}
	for _, role := range []string{"T", "AB", "N"} {
		if err := ValidateNetMetadata("EXAMPLE", nil, []NetPOC{{Handle: "POC-1", Function: role}}); err != nil {
			t.Fatal(err)
		}
	}
	if ValidateNetMetadata("EXAMPLE", nil, []NetPOC{{Handle: "POC-1", Function: "T"}, {Handle: "POC-1", Function: "T"}}) == nil {
		t.Fatal("accepted duplicate association")
	}
}
