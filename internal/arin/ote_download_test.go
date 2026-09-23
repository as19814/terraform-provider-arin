package arin

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

// Read existing artifacts only. No enrollment, agreement or report submission.
func TestOTEDownloadClientLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit sandbox test opt-in")
	}
	key := os.Getenv("ARIN_OTE_API_KEY")
	if key == "" {
		t.Fatal("requires sandbox API key")
	}
	client, err := New(Config{APIKey: key, BaseURL: OTEURL, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []DownloadRequest{{Kind: "bulk_whois", Format: "xml", Objects: []string{"asns"}}, {Kind: "invalid_pocs", Format: "zip"}} {
		t.Run(request.Kind, func(t *testing.T) {
			result, err := client.DownloadTo(context.Background(), request, io.Discard, 64<<20)
			var api *APIError
			if errors.As(err, &api) && (api.StatusCode == 401 || api.StatusCode == 403) {
				t.Skipf("sandbox account download access denied: HTTP %d; successful retrieval remains unverified", api.StatusCode)
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.SizeBytes <= 0 || result.SHA256 == "" {
				t.Fatal("empty download metadata")
			}
			t.Logf("download completed with %d bytes; contents were discarded", result.SizeBytes)
		})
	}
}
