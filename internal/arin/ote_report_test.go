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
	"slices"
	"strings"
	"testing"
	"time"
)

type reportOTETransport struct{ submissions int }

func (t *reportOTETransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.Path, "/rest/report/") {
		t.submissions++
	}
	return http.DefaultTransport.RoundTrip(req)
}

type reportOTEReceipt struct {
	Request         ReportRequest
	Ticket          *Ticket
	RejectionStatus int
	RejectionCode   string
}

// Report tickets cannot be deleted through Reg-RWS. Keep a private receipt after
// completion, and reuse the ticket on later runs rather than submit another job.
func TestOTEReportClientLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and organization")
	}
	transport := &reportOTETransport{}
	c, err := New(Config{APIKey: key, BaseURL: OTEURL, RDAPBaseURL: RDAPOTEURL, HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	networks, err := c.ListOrganizationNetworks(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(networks, func(a, b Network) int { return strings.Compare(a.Handle, b.Handle) })
	netHandle := ""
	for _, network := range networks {
		if !strings.Contains(strings.ToLower(network.Type), "allocation") {
			continue
		}
		net, err := c.GetRegisteredNet(ctx, network.Handle)
		if err != nil {
			t.Fatal(err)
		}
		if net.OrgHandle == org && len(net.Blocks) > 0 {
			netHandle = network.Handle
			break
		}
	}
	if netHandle == "" {
		t.Fatal("requires an owned allocation for report tests")
	}
	requests := []ReportRequest{{Type: ReportAssociations}, {Type: ReportReassignment, Target: netHandle}}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "terraform-provider-arin")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, request := range requests {
		t.Run(request.Type, func(t *testing.T) {
			before, want := transport.submissions, 0
			defer func() {
				if transport.submissions-before != want {
					t.Errorf("report submission count=%d, want %d", transport.submissions-before, want)
				}
			}()
			identity, _ := json.Marshal(struct {
				Org     string
				Request ReportRequest
			}{org, request})
			hash := sha256.Sum256(identity)
			receiptPath := filepath.Join(dir, fmt.Sprintf("ote-report-%x.json", hash[:8]))
			receipt := reportOTEReceipt{Request: request}
			saved, err := os.ReadFile(receiptPath)
			if errors.Is(err, os.ErrNotExist) {
				initial, _ := json.Marshal(receipt)
				f, err := os.OpenFile(receiptPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.Write(initial); err != nil {
					f.Close()
					t.Fatal(err)
				}
				if err = f.Sync(); err != nil {
					f.Close()
					t.Fatal(err)
				}
				if err = f.Close(); err != nil {
					t.Fatal(err)
				}
				want = 1
				receipt.Ticket, err = c.RequestReport(ctx, request)
				var api *APIError
				if receipt.Ticket == nil && errors.As(err, &api) && api.StatusCode >= 400 && api.StatusCode < 500 && api.StatusCode != 408 {
					receipt.RejectionStatus = api.StatusCode
					receipt.RejectionCode = api.Code
				}
				// Persist the known ticket or definite rejection even when validation fails.
				data, marshalErr := json.Marshal(receipt)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				temp, writeErr := os.CreateTemp(dir, "report-receipt-*")
				if writeErr != nil {
					t.Fatal(writeErr)
				}
				defer os.Remove(temp.Name())
				if _, writeErr = temp.Write(data); writeErr != nil {
					temp.Close()
					t.Fatal(writeErr)
				}
				if writeErr = temp.Sync(); writeErr != nil {
					temp.Close()
					t.Fatal(writeErr)
				}
				if writeErr = temp.Close(); writeErr != nil {
					t.Fatal(writeErr)
				}
				if writeErr = os.Rename(temp.Name(), receiptPath); writeErr != nil {
					t.Fatal(writeErr)
				}
				if err != nil {
					t.Logf("native report response: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if err = json.Unmarshal(saved, &receipt); err != nil {
				t.Fatal(err)
			} else {
				t.Log("reusing durable receipt without a report submission")
			}
			if receipt.Request != request {
				t.Fatal("report receipt does not match request")
			}
			if receipt.RejectionStatus != 0 {
				t.Fatalf("unexpected report rejection: HTTP %d %s; inspect %s", receipt.RejectionStatus, receipt.RejectionCode, receiptPath)
			}
			if receipt.Ticket == nil || receipt.Ticket.Number == "" {
				t.Fatalf("report outcome uncertain; inspect %s before any new submission", receiptPath)
			}
			deadline := time.Now().Add(20 * time.Second)
			for {
				ticket, err := c.GetTicket(ctx, receipt.Ticket.Number)
				if err != nil {
					t.Fatal(err)
				}
				if !request.MatchesTicket(*ticket) {
					t.Fatal("report ticket has unexpected type")
				}
				if ticket.Status == "CLOSED" {
					t.Log("report ticket is closed; durable receipt retained")
					return
				}
				if ticket.Status == "RESOLVED" {
					closed, err := c.CloseTicket(ctx, ticket.Number)
					if err != nil {
						t.Fatal(err)
					}
					if closed.Status != "CLOSED" {
						t.Fatal("report closure unconfirmed")
					}
					t.Log("report resolved and explicit closure verified; durable receipt retained")
					return
				}
				if time.Now().After(deadline) {
					t.Logf("report remains %s; durable receipt retained for a later read-only poll", ticket.Status)
					return
				}
				select {
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(time.Second):
				}
			}
		})
	}
}
