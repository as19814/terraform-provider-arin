package arin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRPKIIssuanceValidationConfig(t *testing.T) {
	for _, mode := range []string{"valid", "missing", "null", "empty", "nested", "number", "too_many", "duplicate", "unknown", "alias", "http", "public", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			raw := `{"resource_anchor_pem":"anchor","issuer_chain_pem":"issuers","rrdp_cache_directory":"/private/cache","manifest_history_directory":"/private/history","rrdp_notifications":["https://repo.example/notification.xml"]}`
			switch mode {
			case "missing":
				raw = strings.Replace(raw, `,"rrdp_notifications":["https://repo.example/notification.xml"]`, "", 1)
			case "null", "empty", "nested", "number", "too_many":
				v := map[string]string{"null": "null", "empty": "[]", "nested": "[[]]", "number": "[1]"}[mode]
				if mode == "too_many" {
					urls := make([]string, 32)
					for i := range urls {
						urls[i] = "https://repo.example/notification.xml"
					}
					b, _ := json.Marshal(urls)
					v = string(b)
				}
				raw = strings.Replace(raw, `["https://repo.example/notification.xml"]`, v, 1)
			case "duplicate":
				raw = `{"rrdp_notifications":["https://other.example/n.xml"],` + raw[1:]
			case "unknown":
				raw = `{"private_key":"secret",` + raw[1:]
			case "alias":
				raw = strings.Replace(raw, "rrdp_notifications", "RRDP_NOTIFICATIONS", 1)
			case "http":
				raw = strings.Replace(raw, "https://", "http://", 1)
			}
			path := filepath.Join(t.TempDir(), "validation.json")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if mode == "public" {
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "symlink" {
				if err := os.Symlink(path, path+".link"); err != nil {
					t.Fatal(err)
				}
				path += ".link"
			}
			got, err := LoadRPKICertificateValidation(path)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode %s: %v", mode, err)
			}
			if err == nil && (len(got.Notifications) != 1 || got.AnchorPEM != "anchor" || got.CacheDirectory != "/private/cache") {
				t.Fatal("configuration mapping changed")
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("configuration contents leaked")
			}
		})
	}
}
func TestRPKIIssuanceRecoveryAPIPreflight(t *testing.T) {
	for _, pair := range [][2]string{{"invalid", ""}, {strings.Repeat("a", 64), "invalid"}} {
		if _, err := RecoverRPKIIssuance(context.Background(), RPKIProvisioningReadConfig{}, RPKICertificateValidation{}, pair[0], pair[1]); !errors.Is(err, errRPKIExchangeState) {
			t.Fatalf("invalid input reached key loading: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RecoverRPKIIssuance(ctx, RPKIProvisioningReadConfig{}, RPKICertificateValidation{}, strings.Repeat("a", 64), ""); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
}
