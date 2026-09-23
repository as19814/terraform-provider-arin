package arin

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIIssueWithResourcePath(t *testing.T) {
	f, pubs := manifestPathFixture(t, "valid")
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "child"}, ExtraExtensions: rpkiCSRTestExtensions(t)}, f.keys[0])
	if err != nil {
		t.Fatal(err)
	}
	input := rpkiIssueRequest{Class: "class", CSRDER: csr}
	reply := upDownTestReply("issue_response", fmt.Sprintf(`<class class_name="class" cert_url="rsync://repo.example/module/issuer.cer" resource_set_as="64500-64510" resource_set_ipv4="" resource_set_ipv6="" resource_set_notafter="%s"><certificate cert_url="%s">%s</certificate><issuer>%s</issuer></class>`, cmsTrustNow().Add(time.Hour).Format("2006-01-02T15:04:05Z"), pubs[0].ChildURI, base64.StdEncoding.EncodeToString(f.certs[0].Raw), base64.StdEncoding.EncodeToString(f.certs[1].Raw)))
	for _, mode := range []string{"valid", "retrieval_failure", "wrong_certificate", "wrong_issuer", "wrong_location", "missing_publication", "invalid_configuration", "resolver_mutation"} {
		t.Run(mode, func(t *testing.T) {
			local, _ := cmsSigningFixture(t)
			remote, _ := cmsSigningFixture(t)
			var calls, resolves atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				signed, err := signRPKICMS([]byte(reply), remote, cmsTrustNow(), time.Time{})
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/rpki-updown")
				_, _ = w.Write(signed)
			}))
			defer server.Close()
			exchange := rpkiHTTPExchange{Endpoint: server.URL, MediaType: "application/rpki-updown", Directory: privateExchangeDir(t), Identity: local, PeerAnchor: remote.Anchor, Clock: cmsTrustNow}
			client := rpkiUpDownClient{Exchange: exchange, Child: "child", Parent: "parent"}
			validation := rpkiIssuePathValidation{Anchor: f.certs[2], Directory: privateExchangeDir(t), Resolve: func(ctx context.Context, class rpkiResourceClass) (rpkiIssuePath, error) {
				resolves.Add(1)
				path := rpkiIssuePath{Certificates: append([]*x509.Certificate(nil), f.certs...), Publications: append([]rpkiPathPublication(nil), pubs...)}
				switch mode {
				case "retrieval_failure":
					return rpkiIssuePath{}, errors.New("repository not available")
				case "wrong_certificate":
					path.Certificates[0] = f.certs[1]
				case "wrong_issuer":
					path.Certificates[1] = f.certs[2]
				case "wrong_location":
					path.Publications[0].ChildURI = "rsync://repo.example/module/elsewhere.cer"
				case "missing_publication":
					path.Publications = nil
				case "resolver_mutation":
					class.IssuerDER[0] ^= 1
					class.Certificates[0].DER[0] ^= 1
				}
				return path, nil
			}}
			if mode == "invalid_configuration" {
				validation.Resolve = nil
			}
			valid := mode == "valid" || mode == "resolver_mutation"
			for i := 0; i < 2; i++ {
				got, err := client.IssueWithResourcePath(context.Background(), input, validation)
				if (err == nil) != valid || (got != nil) != valid {
					t.Fatalf("valid=%v result=%v err=%v", valid, got != nil, err)
				}
			}
			scope, _ := json.Marshal([]string{"child", "parent"})
			exchange.PeerScope = string(scope)
			peer, err := exchange.peerID()
			if err != nil {
				t.Fatal(err)
			}
			lease, err := openRPKIExchange(exchange.Directory, peer)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			state, err := lease.State()
			if err != nil {
				t.Fatal(err)
			}
			if valid {
				if calls.Load() != 2 || resolves.Load() != 2 || state.Pending != nil {
					t.Fatal("verified issue was not completed")
				}
			} else if mode == "invalid_configuration" {
				if calls.Load() != 0 || resolves.Load() != 0 || state.Pending != nil {
					t.Fatal("invalid config dispatched")
				}
			} else if calls.Load() != 1 || resolves.Load() != 1 || state.Pending == nil {
				t.Fatal("failed path was completed or retried")
			}
		})
	}
}
