package arin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIIssueWithRRDP(t *testing.T) {
	f, pubs := manifestPathFixture(t, "valid")
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "child"}, ExtraExtensions: rpkiCSRTestExtensions(t)}, f.keys[0])
	if err != nil {
		t.Fatal(err)
	}
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	for _, mode := range []string{"valid", "unavailable", "tampered", "wrong_issuer", "wrong_anchor", "insecure_url", "allocation_mismatch"} {
		t.Run(mode, func(t *testing.T) {
			var posts, gets atomic.Int32
			var listing atomic.Bool
			var listResponse atomic.Value
			reply := upDownTestReply("issue_response", fmt.Sprintf(`<class class_name="class" cert_url="rsync://repo.example/module/issuer.cer" resource_set_as="64500-64510" resource_set_ipv4="" resource_set_ipv6="" resource_set_notafter="%s"><certificate cert_url="%s">%s</certificate><issuer>%s</issuer></class>`, cmsTrustNow().Add(time.Hour).Format("2006-01-02T15:04:05Z"), pubs[0].ChildURI, base64.StdEncoding.EncodeToString(f.certs[0].Raw), base64.StdEncoding.EncodeToString(f.certs[1].Raw)))
			if mode == "allocation_mismatch" {
				reply = strings.Replace(reply, `resource_set_as="64500-64510"`, `resource_set_as="64500-64509"`, 1)
			}
			signed, err := signRPKICMS([]byte(reply), remote, cmsTrustNow(), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			listSigned, err := signRPKICMS([]byte(strings.Replace(reply, `type="issue_response"`, `type="list_response"`, 1)), remote, cmsTrustNow(), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			listResponse.Store(listSigned)
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				w.Header().Set("Content-Type", "application/rpki-updown")
				if listing.Load() {
					_, _ = w.Write(listResponse.Load().([]byte))
				} else {
					_, _ = w.Write(signed)
				}
			}))
			defer endpoint.Close()
			snapshots := make([][]byte, len(pubs))
			for i, p := range pubs {
				prefix := p.ManifestURI[:strings.LastIndexByte(p.ManifestURI, '/')+1]
				var content strings.Builder
				fmt.Fprintf(&content, `<publish uri="%s">%s</publish>`, p.ManifestURI, base64.StdEncoding.EncodeToString(p.ManifestDER))
				for name, data := range p.Files {
					if mode == "tampered" && i == 0 && name == "issuer.crl" {
						data = []byte("tampered")
					}
					fmt.Fprintf(&content, `<publish uri="%s">%s</publish>`, prefix+name, base64.StdEncoding.EncodeToString(data))
				}
				snapshots[i] = rrdpSnapshotTestBody(content.String())
			}
			var repository *httptest.Server
			repository = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gets.Add(1)
				if mode == "unavailable" {
					w.WriteHeader(503)
					return
				}
				i := 0
				if strings.HasPrefix(r.URL.Path, "/1/") {
					i = 1
				}
				if strings.HasSuffix(r.URL.Path, "snapshot.xml") {
					_, _ = w.Write(snapshots[i])
					return
				}
				_, _ = w.Write(rrdpNotificationTestBody("3", fmt.Sprintf(`<snapshot uri="%s/%d/snapshot.xml" hash="%x"/>`, repository.URL, i, sha256.Sum256(snapshots[i]))))
			}))
			defer repository.Close()
			exchange := rpkiHTTPExchange{Endpoint: endpoint.URL, MediaType: "application/rpki-updown", Directory: privateExchangeDir(t), Identity: local, PeerAnchor: remote.Anchor, Clock: cmsTrustNow}
			client := rpkiUpDownClient{Exchange: exchange, Child: "child", Parent: "parent"}
			validation := rpkiIssueRRDPValidation{Anchor: f.certs[2], Issuers: f.certs[1:], Notifications: []string{repository.URL + "/0/notification.xml", repository.URL + "/1/notification.xml"}, CacheDirectory: privateExchangeDir(t), HistoryDirectory: privateExchangeDir(t), Client: rrdpHTTPClient{Transport: repository.Client().Transport}}
			if mode == "wrong_anchor" {
				validation.Anchor = f.certs[1]
			}
			if mode == "wrong_issuer" {
				validation.Issuers = f.certs[2:]
				validation.Notifications = validation.Notifications[1:]
			}
			if mode == "insecure_url" {
				validation.Notifications[0] = "http://example.test/notification.xml"
			}
			apiConfig := certificateTestConfig(t, exchange)
			apiValidation := RPKICertificateValidation{AnchorPEM: certificateTestPEM("CERTIFICATE", validation.Anchor.Raw), Notifications: validation.Notifications, CacheDirectory: validation.CacheDirectory, HistoryDirectory: validation.HistoryDirectory}
			for _, issuer := range validation.Issuers {
				apiValidation.IssuerChainPEM += certificateTestPEM("CERTIFICATE", issuer.Raw)
			}
			valid := mode == "valid"
			for attempt := 0; attempt < 2; attempt++ {
				var result bool
				var err error
				if attempt == 0 {
					got, issueErr := client.IssueWithRRDP(context.Background(), rpkiIssueRequest{Class: "class", CSRDER: csr}, validation)
					result, err = got != nil, issueErr
				} else {
					got, issueErr := issueRPKICertificate(context.Background(), apiConfig, RPKICertificateRequest{Class: "class", CSRPEM: certificateTestPEM("CERTIFICATE REQUEST", csr)}, apiValidation, cmsTrustNow, validation.Client)
					result, err = got != nil, issueErr
					if got != nil && (got.Class != "class" || got.SKI != base64.RawURLEncoding.EncodeToString(f.certs[0].SubjectKeyId) || got.CertificatePEM != certificateTestPEM("CERTIFICATE", f.certs[0].Raw) || got.IssuerPEM != certificateTestPEM("CERTIFICATE", f.certs[1].Raw) || got.CertificateURLs != pubs[0].ChildURI || got.NotAfter != f.certs[0].NotAfter.UTC().Format(time.RFC3339)) {
						t.Fatal("API lost certificate identity")
					}
				}
				if (err == nil) != valid || result != valid {
					t.Fatalf("mode=%s result=%v err=%v", mode, result, err)
				}
			}
			if valid {
				listing.Store(true)
				got, err := readRPKICertificate(context.Background(), apiConfig, RPKICertificateRequest{Class: "class", CSRPEM: certificateTestPEM("CERTIFICATE REQUEST", csr)}, apiValidation, cmsTrustNow, validation.Client)
				if err != nil || got == nil || got.CertificatePEM != certificateTestPEM("CERTIFICATE", f.certs[0].Raw) {
					t.Fatalf("validated refresh failed: %v", err)
				}
				for _, mode := range []string{"absent", "duplicate", "wrong_request"} {
					body := strings.Replace(reply, `type="issue_response"`, `type="list_response"`, 1)
					switch mode {
					case "absent":
						body = upDownTestReply("list_response", "")
					case "duplicate":
						start, end := strings.Index(body, "<certificate "), strings.Index(body, "</certificate>")+len("</certificate>")
						body = body[:end] + body[start:end] + body[end:]
					case "wrong_request":
						body = strings.Replace(body, "<certificate ", `<certificate req_resource_set_as="" `, 1)
					}
					signed, err := signRPKICMS([]byte(body), remote, cmsTrustNow(), time.Time{})
					if err != nil {
						t.Fatal(err)
					}
					listResponse.Store(signed)
					got, err := readRPKICertificate(context.Background(), apiConfig, RPKICertificateRequest{Class: "class", CSRPEM: certificateTestPEM("CERTIFICATE REQUEST", csr)}, apiValidation, cmsTrustNow, validation.Client)
					if got != nil || (err == nil) != (mode == "absent") {
						t.Fatalf("read %s: got=%v err=%v", mode, got != nil, err)
					}
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
			histories, err := filepath.Glob(filepath.Join(validation.HistoryDirectory, "rpki-manifests-*.json"))
			if err != nil {
				t.Fatal(err)
			}
			if valid {
				if posts.Load() != 6 || gets.Load() != 4 || state.Pending != nil || len(histories) != 1 {
					t.Fatal("validated issuance did not complete with cache reuse and history")
				}
			} else if mode == "wrong_anchor" || mode == "insecure_url" {
				if posts.Load() != 0 || gets.Load() != 0 || state.Pending != nil {
					t.Fatal("invalid configuration dispatched")
				}
			} else {
				if posts.Load() != 1 || state.Pending == nil || len(histories) != 0 {
					t.Fatal("failed verification completed, retried issuance, or committed history")
				}
			}
		})
	}
}
