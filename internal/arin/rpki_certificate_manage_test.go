package arin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
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

func TestRPKICertificateRequestKey(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	var previous string
	for _, name := range []string{"first", "renewed"} {
		der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: name}, ExtraExtensions: rpkiCSRTestExtensions(t)}, local.Signer)
		if err != nil {
			t.Fatal(err)
		}
		value := certificateTestPEM("CERTIFICATE REQUEST", der)
		got, err := RPKICertificateRequestKey(value)
		if err != nil || got == "" {
			t.Fatalf("valid CSR: %v", err)
		}
		if previous != "" && got != previous {
			t.Fatal("CSR changed resource key identity")
		}
		previous = got
		for _, bad := range []string{value + value, certificateTestPEM("PRIVATE KEY", der), "invalid"} {
			if _, err := RPKICertificateRequestKey(bad); err == nil {
				t.Fatal("invalid CSR accepted")
			}
		}
		der[len(der)-1] ^= 1
		if _, err := RPKICertificateRequestKey(certificateTestPEM("CERTIFICATE REQUEST", der)); err == nil {
			t.Fatal("invalid CSR signature accepted")
		}
	}
}
