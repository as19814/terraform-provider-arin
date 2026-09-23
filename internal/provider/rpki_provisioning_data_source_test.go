package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

type provisioningTestProvider struct{ frameworkprovider.Provider }

func (p provisioningTestProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{func() datasource.DataSource {
		return &rpkiProvisioningDataSource{read: func(_ context.Context, c arin.RPKIProvisioningReadConfig) (*arin.RPKIProvisioningInventory, error) {
			if c.Child == "rejected" {
				return nil, errors.New("provisioning request rejected")
			}
			if c.Parent != "parent" || c.BPKI.Endpoint != "https://repo.example/provisioning" || c.BPKI.JournalDirectory != "/private/journal" || c.BPKI.SigningKeyFile != "/private/key.pem" || c.BPKI.SigningCertificatePEM != "ee" || c.BPKI.SigningAnchorPEM != "local" || c.BPKI.SigningCRLsPEM != "crl" || c.BPKI.PeerAnchorPEM != "peer" || c.BPKI.SigningIntermediatesPEM != "" || c.BPKI.PeerIntermediatesPEM != "" {
				return nil, errors.New("configuration mapping failed")
			}
			empty := ""
			classes := []arin.RPKIProvisioningClass{}
			if c.Child != "empty" {
				classes = append(classes, arin.RPKIProvisioningClass{Name: "class", ASN: "64500-64510", Certificates: []arin.RPKIProvisioningCertificate{{PEM: "certificate", SHA256: "abcd", RequestedASN: &empty}}})
			}
			return &arin.RPKIProvisioningInventory{ID: "test-identity", Classes: classes}, nil
		}}
	}}
}
func TestAccRPKIProvisioning(t *testing.T) {
	t.Setenv("ARIN_API_KEY", "")
	factories := map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(provisioningTestProvider{Provider: New("test")()})}
	config := func(child string) string {
		return fmt.Sprintf(`data "arin_rpki_provisioning" "test" {
 endpoint = "https://repo.example/provisioning"
 child_handle = %q
 parent_handle = "parent"
 journal_directory = "/private/journal"
 signing_key_file = "/private/key.pem"
 signing_certificate_pem = "ee"
 signing_ca_pem = "local"
 signing_crls_pem = "crl"
 peer_ca_pem = "peer"
}`, child)
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: config("child"), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_rpki_provisioning.test", "id", "test-identity"), resource.TestCheckResourceAttr("data.arin_rpki_provisioning.test", "classes.0.asn", "64500-64510"), resource.TestCheckResourceAttr("data.arin_rpki_provisioning.test", "signing_key_file", "/private/key.pem"), resource.TestCheckResourceAttr("data.arin_rpki_provisioning.test", "classes.0.certificates.0.requested_asn", ""), resource.TestCheckNoResourceAttr("data.arin_rpki_provisioning.test", "classes.0.certificates.0.requested_ipv4"))},
		{Config: config("child"), PlanOnly: true},
		{Config: config("empty"), Check: resource.TestCheckResourceAttr("data.arin_rpki_provisioning.test", "classes.#", "0")},
		{Config: config("rejected"), ExpectError: regexp.MustCompile("provisioning request rejected")},
	}})
}
