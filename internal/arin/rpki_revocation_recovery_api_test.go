package arin

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRPKIProvisioningRecoveryConfig(t *testing.T) {
	for _, mode := range []string{"valid", "duplicate", "unknown", "case", "null", "nested", "trailing", "missing", "public", "symlink", "publisher", "missing_parent", "empty_handle"} {
		t.Run(mode, func(t *testing.T) {
			raw := []byte(`{"endpoint":"https://repo.example/publication","child_handle":"child","parent_handle":"parent","journal_directory":"/private/journal","signing_key_file":"/private/key.pem","signing_certificate_pem":"ee","signing_ca_pem":"local","signing_crls_pem":"crl","peer_ca_pem":"peer"}`)
			switch mode {
			case "publisher":
				raw = append([]byte(`{"publisher_handle":"publisher",`), raw[1:]...)
			case "missing_parent":
				raw = []byte(strings.Replace(string(raw), `"parent_handle":"parent",`, "", 1))
			case "empty_handle":
				raw = []byte(strings.Replace(string(raw), `"parent_handle":"parent"`, `"parent_handle":"   "`, 1))
			case "duplicate":
				raw = append([]byte(`{"endpoint":"other",`), raw[1:]...)
			case "unknown":
				raw = append([]byte(`{"signing_key_pem":"secret",`), raw[1:]...)
			case "case":
				raw = append([]byte(`{"Endpoint":"other",`), raw[1:]...)
			case "null":
				raw = []byte("null")
			case "nested":
				raw = []byte(`{"endpoint":{}}`)
			case "trailing":
				raw = append(raw, []byte(" {}")...)
			case "missing":
				raw = []byte(`{"endpoint":"https://repo.example"}`)
			}
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
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
			got, err := LoadRPKIProvisioningConfig(path)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("unexpected config result: %v", err)
			}
			if err == nil && got.Child != "child" || err == nil && got.Parent != "parent" {
				t.Fatal("config mapping changed")
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("config content leaked")
			}
		})
	}
}
func TestRPKIRevocationRecoveryAPI(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	now := cmsTrustNow().Add(2 * time.Second)
	reply, err := signRPKICMS([]byte(upDownTestReply("list_response", "")), remote, now, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rpki-updown")
		_, _ = w.Write(reply)
	}))
	defer server.Close()
	encode := func(kind string, raw []byte) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: raw}))
	}
	keyFile := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(keyFile, []byte(encode("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(local.Signer.(*rsa.PrivateKey)))), 0600); err != nil {
		t.Fatal(err)
	}
	config := RPKIPublicationReadConfig{Endpoint: server.URL, Publisher: "publisher", JournalDirectory: privateExchangeDir(t), SigningKeyFile: keyFile, SigningCertificatePEM: encode("CERTIFICATE", local.Certificate.Raw), SigningAnchorPEM: encode("CERTIFICATE", local.Anchor.Raw), SigningCRLsPEM: encode("X509 CRL", local.CRLs[0].Raw), PeerAnchorPEM: encode("CERTIFICATE", remote.Anchor.Raw)}
	exchange, err := config.exchange()
	if err != nil {
		t.Fatal(err)
	}
	exchange.MediaType = "application/rpki-updown"
	scope, _ := json.Marshal([]string{"child", "parent"})
	exchange.PeerScope = string(scope)
	provisioning := RPKIProvisioningReadConfig{BPKI: config, Child: "child", Parent: "parent"}
	peer, err := exchange.peerID()
	if err != nil {
		t.Fatal(err)
	}
	batch := []byte(`<message xmlns="` + rpkiUpDownNamespace + `" version="1" sender="child" recipient="parent" type="revoke"><key class_name="class" ski="AAAAAAAAAAAAAAAAAAAAAAAAAAA"/></message>`)
	signed, err := signRPKICMS(batch, local, cmsTrustNow(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(signed))
	lease, err := openRPKIExchange(config.JournalDirectory, peer)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.BeginSigned(digest, "updown-revoke", cmsTrustNow(), signed); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := observeRPKIRevocation(context.Background(), provisioning, digest, func() time.Time { return now })
	if err != nil || report.Committed || report.Outcome != "class_absent" || report.Class != "class" || report.Child != "child" || report.Parent != "parent" || report.SKI != "AAAAAAAAAAAAAAAAAAAAAAAAAAA" || report.RequestSHA256 != digest || report.RecoveryPeerID == peer {
		t.Fatalf("report mismatch: %v", err)
	}
	raw, err := json.Marshal(report)
	if err != nil || strings.Contains(string(raw), "PRIVATE KEY") || strings.Contains(string(raw), config.SigningCertificatePEM) {
		t.Fatal("report leaked identity material")
	}
	lease, err = openRPKIExchange(config.JournalDirectory, peer)
	if err != nil {
		t.Fatal(err)
	}
	state, err := lease.State()
	if err != nil || state.Pending == nil || state.Pending.RequestSHA256 != digest {
		t.Fatal("observation cleared pending mutation")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ObserveRPKIRevocation(context.Background(), provisioning, "invalid"); err == nil {
		t.Fatal("invalid digest accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ObserveRPKIRevocation(canceled, provisioning, digest); err == nil {
		t.Fatal("cancellation ignored")
	}
}
