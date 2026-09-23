package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type certificatePathTestProvider struct {
	frameworkprovider.Provider
	discover func(context.Context, string, arin.RPKICertificateValidation) (string, error)
}

func (p certificatePathTestProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{func() datasource.DataSource {
		return &rpkiCertificatePathDataSource{discover: p.discover}
	}}
}

// Signed TLS discovery and manifest validation are exercised in internal/arin.
// This test covers Terraform configuration, state, refresh and diagnostics.
func TestAccRPKICertificatePath(t *testing.T) {
	t.Setenv("ARIN_API_KEY", "")
	chain := "issuer-v1\nanchor"
	p := certificatePathTestProvider{Provider: New("test")(), discover: func(_ context.Context, certificate string, v arin.RPKICertificateValidation) (string, error) {
		if v.AnchorPEM != "anchor" || v.IssuerChainPEM != "" || v.HistoryDirectory != "/private/history" || (v.CacheDirectory != "/private/cache" && v.CacheDirectory != "/private/relocated") || !slices.Equal(v.Notifications, []string{"https://first.example/notification.xml", "https://root.example/notification.xml"}) {
			return "", errors.New("certificate validation configuration mapping failed")
		}
		switch certificate {
		case "rejected":
			return "", errors.New("manifest validation rejected")
		case "empty":
			return "", nil
		case "leaf":
			return chain, nil
		default:
			return "", errors.New("unexpected certificate")
		}
	}}
	factories := map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(p)}
	config := func(certificate, cache string) string {
		return fmt.Sprintf(`data "arin_rpki_certificate_path" "test" {
 certificate_pem = %q
 resource_anchor_pem = "anchor"
 rrdp_notifications = ["https://first.example/notification.xml", "https://root.example/notification.xml"]
 rrdp_cache_directory = %q
 manifest_history_directory = "/private/history"
}`, certificate, cache)
	}
	const address = "data.arin_rpki_certificate_path.test"
	var originalID string
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: config("leaf", "/private/cache"), Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(address, "issuer_chain_pem", "issuer-v1\nanchor"),
			resource.TestMatchResourceAttr(address, "id", regexp.MustCompile(`^[a-f0-9]{64}$`)),
			func(s *terraform.State) error { originalID = s.RootModule().Resources[address].Primary.ID; return nil },
		)},
		{Config: config("leaf", "/private/cache"), PlanOnly: true},
		{Config: config("leaf", "/private/relocated"), Check: func(s *terraform.State) error {
			if s.RootModule().Resources[address].Primary.ID != originalID {
				return errors.New("cache relocation changed certificate identity")
			}
			return nil
		}},
		{PreConfig: func() { chain = "issuer-v2\nanchor" }, Config: config("leaf", "/private/relocated"), Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(address, "issuer_chain_pem", "issuer-v2\nanchor"),
			func(s *terraform.State) error {
				if s.RootModule().Resources[address].Primary.ID == originalID {
					return errors.New("refresh retained old chain identity")
				}
				return nil
			},
		)},
		{Config: config("leaf", "/private/relocated"), PlanOnly: true},
		{Config: config("rejected", "/private/cache"), ExpectError: regexp.MustCompile("manifest validation rejected")},
		{Config: config("empty", "/private/cache"), ExpectError: regexp.MustCompile("empty issuer chain")},
	}})
}
