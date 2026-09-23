package provider

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDownloads(t *testing.T) {
	var generation atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Query().Get("apikey") != "test-key" || r.Header.Get("Authorization") != "" {
			t.Error("invalid download authentication")
			w.WriteHeader(403)
			return
		}
		if r.URL.Path != "/public/rest/downloads/bulkwhois/asns+nets.xml" && r.URL.Path != "/public/rest/downloads/nvpr" && r.URL.Path != "/public/rest/downloads/bulkwhois" {
			t.Errorf("unexpected download path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="artifact.bin"`)
		fmt.Fprintf(w, "artifact-%d", generation.Load())
	}))
	defer server.Close()
	t.Setenv("ARIN_API_KEY", "test-key")
	t.Setenv("ARIN_DOWNLOAD_BASE_URL", server.URL)
	config := `provider "arin" {}
data "arin_bulk_whois" "content" {
 objects = ["nets", "asns"]
 format = "xml"
}
data "arin_invalid_pocs" "metadata" { include_content = false }
data "arin_bulk_whois" "default_archive" { include_content = false }
`
	check := func(n int) resource.TestCheckFunc {
		body := fmt.Sprintf("artifact-%d", n)
		return resource.ComposeAggregateTestCheckFunc(resource.TestCheckResourceAttr("data.arin_bulk_whois.content", "content_base64", base64.StdEncoding.EncodeToString([]byte(body))), resource.TestCheckResourceAttr("data.arin_bulk_whois.content", "id", "bulk_whois/asns+nets.xml"), resource.TestCheckResourceAttr("data.arin_invalid_pocs.metadata", "sha256", fmt.Sprintf("%x", sha256.Sum256([]byte(body)))), resource.TestCheckNoResourceAttr("data.arin_invalid_pocs.metadata", "content_base64"), resource.TestCheckResourceAttr("data.arin_bulk_whois.default_archive", "id", "bulk_whois/all.zip"), resource.TestCheckResourceAttr("data.arin_bulk_whois.content", "size_bytes", fmt.Sprint(len(body))))
	}
	resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, Check: check(0)}, {Config: config, PlanOnly: true}, {Config: config, PreConfig: func() { generation.Store(1) }, Check: check(1)}, {Config: config, PlanOnly: true}}})
}
func TestAccDownloadErrors(t *testing.T) {
	for _, mode := range []string{"denied", "oversized", "bad_format", "bad_objects", "bad_limit", "redirect", "html"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if mode == "denied" {
					w.WriteHeader(403)
					fmt.Fprint(w, "test-key")
					return
				}
				if mode == "redirect" {
					w.Header().Set("Location", "/other?apikey=test-key")
					w.WriteHeader(302)
					return
				}
				media := "application/octet-stream"
				if mode == "html" {
					media = "text/html"
				}
				w.Header().Set("Content-Type", media)
				fmt.Fprint(w, "large artifact")
			}))
			defer server.Close()
			t.Setenv("ARIN_API_KEY", "test-key")
			t.Setenv("ARIN_DOWNLOAD_BASE_URL", "")
			options, pattern := "", "HTTP 403"
			switch mode {
			case "oversized":
				options = "max_bytes=1"
				pattern = "byte limit"
			case "bad_format":
				options = `format="json"`
				pattern = "format must"
			case "bad_objects":
				options = `objects=["../nets"]`
				pattern = "objects must"
			case "bad_limit":
				options = "max_bytes=0"
				pattern = "must be positive"
			case "redirect":
				pattern = "HTTP 302"
			case "html":
				pattern = "content type"
			}
			config := fmt.Sprintf("provider \"arin\" { download_base_url=%q }\ndata \"arin_bulk_whois\" \"test\" { %s }", server.URL, options)
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(New("test")())}, Steps: []resource.TestStep{{Config: config, ExpectError: regexp.MustCompile(pattern)}}})
			if strings.HasPrefix(mode, "bad_") && calls.Load() != 0 {
				t.Fatal("invalid options reached download service")
			}
		})
	}
}
