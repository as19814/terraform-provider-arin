package arin

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIPublicationRead(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	encode := func(kind string, raw []byte) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: raw}))
	}
	keyFile := filepath.Join(t.TempDir(), "key.pem")
	keyPEM := encode("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(local.Signer.(*rsa.PrivateKey)))
	if err := os.WriteFile(keyFile, []byte(keyPEM), 0600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		request, err := verifyRPKICMS(raw, rpkiCMSTrust{Anchor: local.Anchor, Now: cmsTrustNow()})
		if err != nil || string(request.Content) != rpkiPublicationListQuery {
			t.Error("invalid signed list request")
			w.WriteHeader(400)
			return
		}
		if r.Header.Get("X-ARIN-APIKey") != "" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected credentials")
		}
		reply := `<msg xmlns="` + rpkiPublicationNamespace + `" version="4" type="reply"><list uri="rsync://repo.example/module/z.cer" hash="` + strings.Repeat("AB", 32) + `"/><list uri="rsync://repo.example/module/a.cer" hash="` + strings.Repeat("CD", 32) + `"/></msg>`
		signed, err := signRPKICMS([]byte(reply), remote, cmsTrustNow(), time.Time{})
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/rpki-publication")
		_, _ = w.Write(signed)
	}))
	defer server.Close()
	config := RPKIPublicationReadConfig{Endpoint: server.URL, Publisher: "publisher", JournalDirectory: privateExchangeDir(t), SigningKeyFile: keyFile, SigningCertificatePEM: encode("CERTIFICATE", local.Certificate.Raw), SigningAnchorPEM: encode("CERTIFICATE", local.Anchor.Raw), SigningCRLsPEM: encode("X509 CRL", local.CRLs[0].Raw), PeerAnchorPEM: encode("CERTIFICATE", remote.Anchor.Raw)}
	for i := 0; i < 2; i++ {
		out, err := readRPKIPublication(context.Background(), config, cmsTrustNow)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.ID) != 64 || len(out.Objects) != 2 || !strings.HasSuffix(out.Objects[0].URI, "a.cer") || out.Objects[1].SHA256 != strings.Repeat("ab", 32) {
			t.Fatal("inventory sorting/hash normalization failed")
		}
	}
	if calls.Load() != 2 {
		t.Fatal("inventory not refreshed")
	}
	if err := os.Chmod(keyFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readRPKIPublication(context.Background(), config, cmsTrustNow); err == nil || calls.Load() != 2 {
		t.Fatal("insecure key file accepted")
	}
}

func TestRPKISigningKeyAndPEMInputs(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(local.Signer)
	if err != nil {
		t.Fatal(err)
	}
	raw := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	for _, mode := range []string{"valid", "public", "symlink", "trailing", "malformed", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "key.pem")
			data := append([]byte(nil), raw...)
			switch mode {
			case "trailing":
				data = append(data, []byte("secret-garbage")...)
			case "malformed":
				data = []byte("secret-invalid-key")
			case "oversize":
				data = []byte(strings.Repeat("x", 65537))
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
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
			key, err := readRPKISigningKey(path)
			if (err == nil) != (mode == "valid") || (key != nil) != (mode == "valid") {
				t.Fatalf("unexpected key result: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("key contents leaked")
			}
		})
	}
	for _, value := range []string{"garbage", string(raw), "-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----"} {
		if _, err := rpkiPEMCertificates(value, 1); err == nil {
			t.Fatal("invalid certificate accepted")
		}
	}
}
