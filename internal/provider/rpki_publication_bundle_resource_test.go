package provider

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"
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

type publicationBundleTestProvider struct {
	frameworkprovider.Provider
	resource *rpkiPublicationBundleResource
}

func (p publicationBundleTestProvider) Resources(context.Context) []func() frameworkresource.Resource {
	return []func() frameworkresource.Resource{func() frameworkresource.Resource { return p.resource }}
}
func TestAccRPKIPublicationBundle(t *testing.T) {
	var mu sync.Mutex
	const a = "rsync://repo.example/module/a.cer"
	const b = "rsync://repo.example/module/b.mft"
	const outside = "rsync://repo.example/other/unmanaged.cer"
	objects := map[string][]byte{outside: []byte("untouched")}
	hash := func(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
	r := &rpkiPublicationBundleResource{
		read: func(_ context.Context, _ arin.RPKIPublicationReadConfig) (*arin.RPKIPublicationInventory, error) {
			mu.Lock()
			defer mu.Unlock()
			out := &arin.RPKIPublicationInventory{}
			for uri, data := range objects {
				out.Objects = append(out.Objects, arin.RPKIPublicationObject{URI: uri, SHA256: hash(data)})
			}
			return out, nil
		},
		apply: func(_ context.Context, c arin.RPKIPublicationReadConfig, desired, prior map[string]string) error {
			mu.Lock()
			defer mu.Unlock()
			if c.Publisher != "publisher" || c.Endpoint != "https://repo.example/publication" || c.SigningKeyFile != "/private/key.pem" {
				return errors.New("configuration mapping failed")
			}
			decoded, _, err := arin.DecodeRPKIPublicationObjects(desired)
			if err != nil {
				return err
			}
			for uri, old := range prior {
				data, ok := objects[uri]
				if !ok || hash(data) != old {
					return errors.New("stale precondition")
				}
			}
			for uri := range decoded {
				if _, exists := objects[uri]; exists && prior[uri] == "" {
					return errors.New("object already present")
				}
			}
			next := maps.Clone(objects)
			for uri := range prior {
				delete(next, uri)
			}
			for uri, data := range decoded {
				next[uri] = data
			}
			objects = next
			return nil
		},
	}
	factories := map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(publicationBundleTestProvider{Provider: New("test")(), resource: r})}
	config := func(body string) string {
		return `resource "arin_rpki_publication_bundle" "test" {
 endpoint="https://repo.example/publication"
 publisher_handle="publisher"
 journal_directory="/private/journal"
 signing_key_file="/private/key.pem"
 signing_certificate_pem="ee"
 signing_ca_pem="local"
 signing_crls_pem="crl"
 peer_ca_pem="peer"
 objects={` + body + `}
}`
	}
	first := config(fmt.Sprintf(`%q=base64encode("first")`, a))
	second := config(fmt.Sprintf("%q=base64encode(\"second\")\n%q=base64encode(\"manifest\")", a, b))
	third := config(fmt.Sprintf(`%q=base64encode("manifest2")`, b))
	unchanged := func(_ *terraform.State) error {
		mu.Lock()
		defer mu.Unlock()
		if string(objects[outside]) != "untouched" {
			return errors.New("unmanaged object changed")
		}
		return nil
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, CheckDestroy: func(s *terraform.State) error {
		mu.Lock()
		defer mu.Unlock()
		if len(objects) != 1 || string(objects[outside]) != "untouched" {
			return errors.New("owned objects not withdrawn")
		}
		return nil
	}, Steps: []resource.TestStep{
		{Config: first, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_rpki_publication_bundle.test", "hashes."+a, hash([]byte("first"))), unchanged)},
		{Config: first, PlanOnly: true},
		{Config: second, Check: resource.TestCheckResourceAttr("arin_rpki_publication_bundle.test", "hashes.%", "2")},
		{PreConfig: func() { mu.Lock(); defer mu.Unlock(); objects[a] = []byte("external") }, Config: second, Check: resource.TestCheckResourceAttr("arin_rpki_publication_bundle.test", "hashes."+a, hash([]byte("second")))},
		{PreConfig: func() { mu.Lock(); defer mu.Unlock(); delete(objects, b) }, Config: second, Check: resource.TestCheckResourceAttr("arin_rpki_publication_bundle.test", "hashes.%", "2")},
		{Config: third, Check: resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("arin_rpki_publication_bundle.test", "hashes.%", "1"), unchanged)},
		{Config: third, PlanOnly: true},
	}})
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{{Config: strings.Replace(first, `base64encode("first")`, `"invalid"`, 1), ExpectError: regexp.MustCompile("Invalid publication bundle")}}})
}
