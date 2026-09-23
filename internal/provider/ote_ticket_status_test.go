package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type ticketStatusOTETransport struct {
	number string
	writes atomic.Int64
}

func (t *ticketStatusOTETransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || req.URL.Host != "reg.ote.arin.net" {
		return nil, errors.New("unexpected sandbox origin")
	}
	if req.Method == "GET" && strings.HasPrefix(req.URL.Path, "/rest/report/") {
		return nil, errors.New("ticket status test must not submit reports")
	}
	if req.Method != "GET" {
		if req.Method != "PUT" || req.URL.Path != "/rest/ticket/"+t.number+"/ticketStatus/CLOSED" || req.URL.RawQuery != "msgRefs=true" {
			return nil, errors.New("unexpected sandbox mutation")
		}
		t.writes.Add(1)
	}
	return http.DefaultTransport.RoundTrip(req)
}

type ticketStatusOTEProvider struct {
	*ARINProvider
	client *arin.Client
}

func (p *ticketStatusOTEProvider) Configure(_ context.Context, _ frameworkprovider.ConfigureRequest, resp *frameworkprovider.ConfigureResponse) {
	resp.ResourceData = p.client
	resp.DataSourceData = p.client
}
func TestOTETicketStatusLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and org handle")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(org))
	data, err := os.ReadFile(filepath.Join(cache, "terraform-provider-arin", fmt.Sprintf("ote-report-resource-%x.json", hash[:8])))
	if err != nil {
		t.Fatal("requires an existing disposable report receipt from TestOTEReportResourceLifecycle")
	}
	var receipt reportResourceReceipt
	if err = json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Org != org || receipt.Request.Type != arin.ReportAssociations || receipt.TicketNumber == "" {
		t.Fatal("receipt does not identify a disposable associations report")
	}
	trace := &ticketStatusOTETransport{number: receipt.TicketNumber}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL, HTTPClient: &http.Client{Transport: trace}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	before, err := client.GetTicket(ctx, receipt.TicketNumber)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Request.MatchesTicket(*before) {
		t.Fatal("report ticket category changed")
	}
	if before.Status != "CLOSED" && before.Status != "RESOLVED" {
		t.Skipf("disposable report is still %s", before.Status)
	}
	config := ticketStatusConfig(receipt.TicketNumber, "CLOSED")
	p := &ticketStatusOTEProvider{ARINProvider: &ARINProvider{version: "ote-test"}, client: client}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(p)}, Steps: []resource.TestStep{
		{Config: config, Check: resource.TestCheckResourceAttr("arin_ticket_status.test", "status", "CLOSED")},
		{ResourceName: "arin_ticket_status.test", ImportState: true, ImportStateId: receipt.TicketNumber, ImportStateVerify: true},
		{Config: config, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		after, err := client.GetTicket(ctx, receipt.TicketNumber)
		if err != nil {
			return err
		}
		if after.Status != "CLOSED" {
			return fmt.Errorf("ticket was changed by destroy")
		}
		writes := trace.writes.Load()
		if writes > 1 || (before.Status == "CLOSED" && writes != 0) {
			return fmt.Errorf("ticket status lifecycle performed unexpected writes: %d", writes)
		}
		t.Logf("Terraform ticket status lifecycle from %s performed %d writes", before.Status, writes)
		return nil
	}})
}
