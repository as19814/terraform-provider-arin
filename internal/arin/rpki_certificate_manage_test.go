package arin

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func certificateTestPEM(kind string, raw []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: raw}))
}
func certificateTestConfig(t *testing.T, exchange rpkiHTTPExchange) RPKIProvisioningReadConfig {
	t.Helper()
	identity := exchange.Identity
	path := filepath.Join(t.TempDir(), "signing.pem")
	if err := os.WriteFile(path, []byte(certificateTestPEM("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(identity.Signer.(*rsa.PrivateKey)))), 0600); err != nil {
		t.Fatal(err)
	}
	return RPKIProvisioningReadConfig{Child: "child", Parent: "parent", BPKI: RPKIPublicationReadConfig{
		Endpoint: exchange.Endpoint, JournalDirectory: exchange.Directory, SigningKeyFile: path,
		SigningCertificatePEM: certificateTestPEM("CERTIFICATE", identity.Certificate.Raw), SigningAnchorPEM: certificateTestPEM("CERTIFICATE", identity.Anchor.Raw), SigningCRLsPEM: certificateTestPEM("X509 CRL", identity.CRLs[0].Raw), PeerAnchorPEM: certificateTestPEM("CERTIFICATE", exchange.PeerAnchor.Raw),
	}}
}
func TestRPKICertificateManagePreflight(t *testing.T) {
	config := RPKIProvisioningReadConfig{Child: "child", Parent: "parent"}
	for _, input := range []RPKICertificateRequest{{}, {Class: "class", CSRPEM: "invalid"}, {Class: "class", CSRPEM: certificateTestPEM("PRIVATE KEY", []byte("secret"))}} {
		if _, err := IssueRPKICertificate(context.Background(), config, input, RPKICertificateValidation{}); !errors.Is(err, errRPKIUpDown) {
			t.Fatalf("bad request reached key loading: %v", err)
		}
	}
	for _, tc := range []struct{ class, ski string }{{"", "AAAAAAAAAAAAAAAAAAAAAAAAAAA"}, {"class", "invalid"}} {
		if err := RevokeRPKICertificate(context.Background(), config, tc.class, tc.ski); !errors.Is(err, errRPKIUpDown) {
			t.Fatalf("invalid revocation reached key loading: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := IssueRPKICertificate(ctx, config, RPKICertificateRequest{}, RPKICertificateValidation{}); !errors.Is(err, context.Canceled) {
		t.Fatal("issuance ignored cancellation")
	}
	if err := RevokeRPKICertificate(ctx, config, "class", "AAAAAAAAAAAAAAAAAAAAAAAAAAA"); !errors.Is(err, context.Canceled) {
		t.Fatal("revocation ignored cancellation")
	}
}
