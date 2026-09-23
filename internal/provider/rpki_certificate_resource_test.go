package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type certificateTestProvider struct {
	frameworkprovider.Provider
	resource *rpkiCertificateResource
}

func (p certificateTestProvider) Resources(context.Context) []func() frameworkresource.Resource {
	return []func() frameworkresource.Resource{func() frameworkresource.Resource { return p.resource }}
}

func TestAccRPKICertificate(t *testing.T) {
	var mu sync.Mutex
	objects := map[string]*arin.RPKIIssuedCertificate{}
	issued, revoked := 0, 0
	failRead := false
	check := func(c arin.RPKIProvisioningReadConfig, input arin.RPKICertificateRequest, v arin.RPKICertificateValidation) error {
		if c.Child != "child" || c.Parent != "parent" || c.BPKI.SigningKeyFile != "/private/signing.pem" || input.Class != "class" || v.AnchorPEM != "anchor" || v.IssuerChainPEM != "chain" || len(v.Notifications) != 1 || v.Notifications[0] != "https://repo.example/notification.xml" || input.RequestedIPv4 == nil || *input.RequestedIPv4 != "" || input.RequestedIPv6 != nil {
			return errors.New("configuration mapping failed")
		}
		return nil
	}
	r := &rpkiCertificateResource{
		issue: func(_ context.Context, c arin.RPKIProvisioningReadConfig, input arin.RPKICertificateRequest, v arin.RPKICertificateValidation) (*arin.RPKIIssuedCertificate, error) {
			mu.Lock()
			defer mu.Unlock()
			if err := check(c, input, v); err != nil {
				return nil, err
			}
			issued++
			cert := &arin.RPKIIssuedCertificate{Class: input.Class, SKI: input.CSRPEM, CertificatePEM: fmt.Sprintf("certificate-%d", issued), IssuerPEM: "issuer", CertificateURLs: "rsync://repo.example/module/child.cer", NotAfter: "2027-01-01T00:00:00Z"}
			objects[input.CSRPEM] = cert
			return cert, nil
		},
		read: func(_ context.Context, c arin.RPKIProvisioningReadConfig, input arin.RPKICertificateRequest, v arin.RPKICertificateValidation) (*arin.RPKIIssuedCertificate, error) {
			mu.Lock()
			defer mu.Unlock()
			if err := check(c, input, v); err != nil {
				return nil, err
			}
			if failRead {
				return nil, errors.New("validation unavailable")
			}
			return objects[input.CSRPEM], nil
		},
		revoke: func(_ context.Context, c arin.RPKIProvisioningReadConfig, class, ski string) error {
			mu.Lock()
			defer mu.Unlock()
			if class != "class" || objects[ski] == nil {
				return errors.New("wrong revocation target")
			}
			delete(objects, ski)
			revoked++
			return nil
		},
	}
	factories := map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(certificateTestProvider{Provider: New("test")(), resource: r})}
	config := func(csr, asn string) string {
		return fmt.Sprintf(`resource "arin_rpki_certificate" "test" {
 endpoint="https://parent.example/provisioning"
 child_handle="child"
 parent_handle="parent"
 journal_directory="/private/journal"
 signing_key_file="/private/signing.pem"
 signing_certificate_pem="ee"
 signing_ca_pem="local"
 signing_crls_pem="crl"
 peer_ca_pem="peer"
 class_name="class"
 csr_pem=%q
 requested_asn=%q
 requested_ipv4=""
 resource_anchor_pem="anchor"
 issuer_chain_pem="chain"
 rrdp_notifications=["https://repo.example/notification.xml"]
 rrdp_cache_directory="/private/cache"
 manifest_history_directory="/private/history"
}`, csr, asn)
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, CheckDestroy: func(_ *terraform.State) error {
		mu.Lock()
		defer mu.Unlock()
		if len(objects) != 0 || issued != 3 || revoked != 2 {
			return fmt.Errorf("unexpected lifecycle: %d issued, %d revoked", issued, revoked)
		}
		return nil
	}, Steps: []resource.TestStep{
		{Config: config("key-one", "64500"), Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr("arin_rpki_certificate.test", "ski", "key-one"), resource.TestCheckResourceAttr("arin_rpki_certificate.test", "certificate_pem", "certificate-1"))},
		{Config: config("key-one", "64500"), PlanOnly: true},
		{Config: config("key-one", "64500"), PlanOnly: true, PreConfig: func() { mu.Lock(); failRead = true; mu.Unlock() }, ExpectError: regexp.MustCompile("validation unavailable")},
		{Config: config("key-one", "64500"), PlanOnly: true, PreConfig: func() { mu.Lock(); failRead = false; mu.Unlock() }},
		{Config: config("key-one", "64500-64501"), Check: resource.TestCheckResourceAttr("arin_rpki_certificate.test", "certificate_pem", "certificate-2")},
		{Config: config("key-two", "64500"), Check: resource.TestCheckResourceAttr("arin_rpki_certificate.test", "ski", "key-two")},
		{Config: config("key-two", "64500"), PlanOnly: true},
	}})
}
