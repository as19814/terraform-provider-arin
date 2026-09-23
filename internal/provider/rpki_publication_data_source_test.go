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

type publicationTestProvider struct{ frameworkprovider.Provider }

func (p publicationTestProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{func() datasource.DataSource {
		return &rpkiPublicationDataSource{read: func(_ context.Context, c arin.RPKIPublicationReadConfig) (*arin.RPKIPublicationInventory, error) {
			if c.Publisher == "rejected" {
				return nil, errors.New("publication request rejected")
			}
			if c.Endpoint != "https://repo.example/publication" || c.JournalDirectory != "/private/journal" || c.SigningKeyFile != "/private/key.pem" || c.SigningCertificatePEM != "ee" || c.SigningAnchorPEM != "local" || c.SigningCRLsPEM != "crl" || c.PeerAnchorPEM != "peer" || c.SigningIntermediatesPEM != "" || c.PeerIntermediatesPEM != "" {
				return nil, errors.New("configuration mapping failed")
			}
			objects := []arin.RPKIPublicationObject{}
			if c.Publisher != "empty" {
				objects = append(objects, arin.RPKIPublicationObject{URI: "rsync://repo.example/module/object.cer", SHA256: "abcd"})
			}
			return &arin.RPKIPublicationInventory{ID: "test-identity", Objects: objects}, nil
		}}
	}}
}
func TestAccRPKIPublication(t *testing.T) {
	t.Setenv("ARIN_API_KEY", "")
	factories := map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(publicationTestProvider{Provider: New("test")()})}
	config := func(publisher string) string {
		return fmt.Sprintf(`data "arin_rpki_publication" "test" {
 endpoint = "https://repo.example/publication"
 publisher_handle = %q
 journal_directory = "/private/journal"
 signing_key_file = "/private/key.pem"
 signing_certificate_pem = "ee"
 signing_ca_pem = "local"
 signing_crls_pem = "crl"
 peer_ca_pem = "peer"
}`, publisher)
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: config("publisher"), Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_rpki_publication.test", "id", "test-identity"), resource.TestCheckResourceAttr("data.arin_rpki_publication.test", "objects.0.uri", "rsync://repo.example/module/object.cer"), resource.TestCheckResourceAttr("data.arin_rpki_publication.test", "signing_key_file", "/private/key.pem"))},
		{Config: config("publisher"), PlanOnly: true},
		{Config: config("empty"), Check: resource.TestCheckResourceAttr("data.arin_rpki_publication.test", "objects.#", "0")},
		{Config: config("rejected"), ExpectError: regexp.MustCompile("publication request rejected")},
	}})
}
