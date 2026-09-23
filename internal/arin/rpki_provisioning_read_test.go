package arin

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
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

func TestRPKIProvisioningRead(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	resources := resourceCertificateFixture(t)
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
		if err != nil || !strings.Contains(string(request.Content), `type="list"`) {
			t.Error("invalid signed list request")
			w.WriteHeader(400)
			return
		}
		if r.Header.Get("X-ARIN-APIKey") != "" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected credentials")
		}
		reply := upDownTestReply("list_response", fmt.Sprintf(`<class class_name="class" cert_url="rsync://repo.example/module/issuer.cer" resource_set_as="64500-64510" resource_set_ipv4="" resource_set_ipv6="" resource_set_notafter="%s"><certificate cert_url="rsync://repo.example/module/child.cer" req_resource_set_as="">%s</certificate><issuer>%s</issuer></class>`, cmsTrustNow().Add(time.Hour).Format("2006-01-02T15:04:05Z"), base64.StdEncoding.EncodeToString(resources.certs[0].Raw), base64.StdEncoding.EncodeToString(resources.certs[1].Raw)))
		signed, err := signRPKICMS([]byte(reply), remote, cmsTrustNow(), time.Time{})
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/rpki-updown")
		_, _ = w.Write(signed)
	}))
	defer server.Close()
	identity := RPKIPublicationReadConfig{Endpoint: server.URL, Publisher: "publisher", JournalDirectory: privateExchangeDir(t), SigningKeyFile: keyFile, SigningCertificatePEM: encode("CERTIFICATE", local.Certificate.Raw), SigningAnchorPEM: encode("CERTIFICATE", local.Anchor.Raw), SigningCRLsPEM: encode("X509 CRL", local.CRLs[0].Raw), PeerAnchorPEM: encode("CERTIFICATE", remote.Anchor.Raw)}
	config := RPKIProvisioningReadConfig{BPKI: identity, Child: "child", Parent: "parent"}
	for i := 0; i < 2; i++ {
		out, err := readRPKIProvisioning(context.Background(), config, cmsTrustNow)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.ID) != 64 || len(out.Classes) != 1 || out.Classes[0].Name != "class" || out.Classes[0].ASN != "64500-64510" || len(out.Classes[0].Certificates) != 1 {
			t.Fatal("resource inventory mapping failed")
		}
		cert := out.Classes[0].Certificates[0]
		if cert.RequestedASN == nil || *cert.RequestedASN != "" || cert.RequestedIPv4 != nil || len(cert.SHA256) != 64 || !strings.Contains(cert.PEM, "BEGIN CERTIFICATE") {
			t.Fatal("certificate mapping lost absent/empty distinction")
		}

	}
	if calls.Load() != 2 {
		t.Fatal("inventory not refreshed")
	}
	if err := os.Chmod(keyFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readRPKIProvisioning(context.Background(), config, cmsTrustNow); err == nil || calls.Load() != 2 {
		t.Fatal("insecure key file accepted")
	}
}
