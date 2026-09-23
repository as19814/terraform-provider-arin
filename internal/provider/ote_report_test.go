package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type reportResourceReceipt struct {
	Org          string
	Request      arin.ReportRequest
	TicketNumber string
}

func saveReportResourceReceipt(path string, receipt reportResourceReceipt) error {
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "report-resource-receipt-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
func TestOTEReportResourceLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and org handle")
	}
	client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "terraform-provider-arin")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(org))
	receiptPath := filepath.Join(dir, fmt.Sprintf("ote-report-resource-%x.json", hash[:8]))
	receipt := reportResourceReceipt{Org: org, Request: arin.ReportRequest{Type: arin.ReportAssociations}}
	existing := false
	if data, err := os.ReadFile(receiptPath); err == nil {
		existing = true
		if err = json.Unmarshal(data, &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.Org != org || receipt.Request.Type != arin.ReportAssociations || receipt.Request.Target != "" || receipt.TicketNumber == "" {
			t.Fatalf("reconcile the saved report receipt before another submission: %s", receiptPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	} else {
		data, _ := json.Marshal(receipt)
		file, err := os.OpenFile(receiptPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write(data); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err = file.Sync(); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ARIN_API_KEY", key)
	t.Setenv("ARIN_BASE_URL", arin.OTEURL)
	t.Setenv("ARIN_RDAP_BASE_URL", arin.RDAPOTEURL)
	config := reportConfig(arin.ReportAssociations, "")
	first := resource.TestStep{Config: config, Check: func(s *terraform.State) error {
		receipt.TicketNumber = s.RootModule().Resources["arin_report_request.test"].Primary.ID
		return saveReportResourceReceipt(receiptPath, receipt)
	}}
	if existing {
		first = resource.TestStep{Config: config, ResourceName: "arin_report_request.test", ImportState: true, ImportStateId: "associations/" + receipt.TicketNumber, ImportStatePersist: true}
		t.Log("reusing existing report ticket without submission")
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("ote-test")())}, Steps: []resource.TestStep{
		first,
		{Config: config, ResourceName: "arin_report_request.test", ImportState: true, ImportStateIdFunc: func(_ *terraform.State) (string, error) { return "associations/" + receipt.TicketNumber, nil }, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"status", "resolution", "updated_date", "resolved_date", "closed_date"}},
		{Config: config, PlanOnly: true},
	}, CheckDestroy: func(_ *terraform.State) error {
		ticket, err := client.GetTicket(ctx, receipt.TicketNumber)
		if err != nil {
			return err
		}
		if !receipt.Request.MatchesTicket(*ticket) {
			return fmt.Errorf("report ticket changed after forgetting Terraform receipt")
		}
		t.Logf("Terraform receipt removed; server ticket remains %s", ticket.Status)
		return nil
	}})
}
