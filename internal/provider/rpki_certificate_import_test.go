package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func certificateImportFixture(csr, asn string) certificateImport {
	empty := ""
	return certificateImport{Endpoint: "https://parent.example/provisioning", Child: "child", Parent: "parent", Journal: "/private/journal", KeyFile: "/private/signing.pem", Certificate: "ee", Anchor: "local", CRLs: "crl", Peer: "peer", Class: "class", CSR: csr, RequestedASN: &asn, RequestedIPv4: &empty, ResourceAnchor: "anchor", Issuers: "chain", Notifications: []string{"https://repo.example/notification.xml"}, Cache: "/private/cache", History: "/private/history"}
}
func writeCertificateImport(t *testing.T, csr, asn string) string {
	t.Helper()
	raw, err := json.Marshal(certificateImportFixture(csr, asn))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "certificate.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestCertificateImportManifest(t *testing.T) {
	for _, mode := range []string{"valid", "unknown", "duplicate", "alias", "missing", "empty_urls", "null_url", "null", "trailing", "nested", "public", "symlink", "relative", "directory", "oversized", "too_many_urls", "computed"} {
		t.Run(mode, func(t *testing.T) {
			raw, err := json.Marshal(certificateImportFixture("csr", "64500"))
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "computed":
				raw = append([]byte(`{"id":"chosen",`), raw[1:]...)
			case "too_many_urls":
				fixture := certificateImportFixture("csr", "64500")
				fixture.Notifications = make([]string, 32)
				for i := range fixture.Notifications {
					fixture.Notifications[i] = "https://repo.example/notification.xml"
				}
				raw, err = json.Marshal(fixture)
				if err != nil {
					t.Fatal(err)
				}
			case "unknown":
				raw = append([]byte(`{"signing_key_pem":"secret",`), raw[1:]...)
			case "duplicate":
				raw = append([]byte(`{"child_handle":"other",`), raw[1:]...)
			case "alias":
				raw = append([]byte(`{"Child_Handle":"other",`), raw[1:]...)
			case "missing":
				raw = []byte(strings.Replace(string(raw), `"class_name":"class"`, `"class_name":null`, 1))
			case "empty_urls":
				raw = []byte(strings.Replace(string(raw), `["https://repo.example/notification.xml"]`, `[]`, 1))
			case "null_url":
				raw = []byte(strings.Replace(string(raw), `["https://repo.example/notification.xml"]`, `[null]`, 1))
			case "null":
				raw = []byte("null")
			case "trailing":
				raw = append(raw, []byte(" {}")...)
			case "nested":
				raw = []byte(strings.Replace(string(raw), `"requested_asn":"64500"`, `"requested_asn":{}`, 1))
			}
			path := filepath.Join(t.TempDir(), "certificate.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "oversized":
				if err := os.Truncate(path, (16<<20)+1); err != nil {
					t.Fatal(err)
				}
			case "public":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(path, path+".link"); err != nil {
					t.Fatal(err)
				}
				path += ".link"
			case "relative":
				path = "certificate.json"
			case "directory":
				path = filepath.Dir(path)
			}
			got, err := readCertificateImport(path)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode %s: %v", mode, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("manifest contents leaked")
			}
			if err == nil && (got.RequestedIPv4 == nil || *got.RequestedIPv4 != "" || got.RequestedIPv6 != nil || got.CSR != "csr") {
				t.Fatal("manifest lost optional resource semantics")
			}
		})
	}
}
