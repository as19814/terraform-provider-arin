package arin

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRPKIRRDPPathDiscovery(t *testing.T) {
	for _, mode := range []string{"valid", "missing_issuer", "wrong_issuer", "ambiguous", "wrong_anchor", "unlisted_issuer", "invalid_notification"} {
		t.Run(mode, func(t *testing.T) {
			f, pubs := manifestPathFixture(t, "valid")
			leaf := *f.certs[0]
			leaf.ExtraExtensions = nil
			for _, e := range leaf.Extensions {
				if e.Id.String() != "1.3.6.1.5.5.7.1.1" {
					leaf.ExtraExtensions = append(leaf.ExtraExtensions, e)
				}
			}
			leaf.IssuingCertificateURL = []string{pubs[1].ChildURI}
			if mode == "ambiguous" {
				leaf.IssuingCertificateURL = append(leaf.IssuingCertificateURL, "rsync://repo.example/module/other.cer")
			}
			raw, err := x509.CreateCertificate(rand.Reader, &leaf, f.certs[1], &f.keys[0].PublicKey, f.keys[1])
			if err != nil {
				t.Fatal(err)
			}
			f.certs[0], err = x509.ParseCertificate(raw)
			if err != nil {
				t.Fatal(err)
			}
			pubs[0].Files["child.cer"] = raw
			pubs[0] = rewriteRevocationManifestFiles(t, pubs[0], f)
			if mode == "unlisted_issuer" {
				delete(pubs[1].Files, "child.cer")
				pubs[1] = rewriteRevocationManifestFiles(t, pubs[1], f)
				pubs[1].Files["child.cer"] = f.certs[1].Raw
			}
			snapshots := make([][]byte, 2)
			for i, p := range pubs {
				prefix := p.ManifestURI[:strings.LastIndexByte(p.ManifestURI, '/')+1]
				objects := map[string][]byte{p.ManifestURI: p.ManifestDER}
				for name, raw := range p.Files {
					objects[prefix+name] = raw
				}
				if i == 1 {
					switch mode {
					case "missing_issuer":
						delete(objects, p.ChildURI)
					case "wrong_issuer":
						objects[p.ChildURI] = f.certs[0].Raw
					case "ambiguous":
						objects["rsync://repo.example/module/other.cer"] = f.certs[2].Raw
					}
				}
				var body strings.Builder
				for uri, raw := range objects {
					fmt.Fprintf(&body, `<publish uri="%s">%s</publish>`, uri, base64.StdEncoding.EncodeToString(raw))
				}
				snapshots[i] = rrdpSnapshotTestBody(body.String())
			}
			var calls atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != "GET" || r.Header.Get("Authorization") != "" {
					t.Error("unexpected discovery request")
				}
				i := 0
				if strings.HasPrefix(r.URL.Path, "/1/") {
					i = 1
				}
				if strings.HasSuffix(r.URL.Path, "snapshot.xml") {
					_, _ = w.Write(snapshots[i])
					return
				}
				_, _ = w.Write(rrdpNotificationTestBody("3", fmt.Sprintf(`<snapshot uri="%s/%d/snapshot.xml" hash="%x"/>`, server.URL, i, sha256.Sum256(snapshots[i]))))
			}))
			defer server.Close()
			notifications := []string{server.URL + "/0/notification.xml", server.URL + "/1/notification.xml"}
			if mode == "invalid_notification" {
				notifications[0] = "http://invalid.example/notification.xml"
			}
			anchor := f.certs[2]
			if mode == "wrong_anchor" {
				anchor = f.certs[1]
			}
			client := rrdpHTTPClient{Transport: server.Client().Transport}
			directory := privateExchangeDir(t)
			path, err := client.DiscoverPath(context.Background(), directory, f.certs[0], anchor, notifications, cmsTrustNow())
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode=%s err=%v", mode, err)
			}
			if mode == "invalid_notification" && calls.Load() != 0 {
				t.Fatal("invalid input made network requests")
			}
			if err != nil {
				return
			}
			if calls.Load() != 4 || len(path.Certificates) != 3 || !bytes.Equal(path.Certificates[1].Raw, f.certs[1].Raw) {
				t.Fatal("discovery lost scoped issuer or bypassed cache")
			}
			validation := RPKICertificateValidation{AnchorPEM: certificateTestPEM("CERTIFICATE", anchor.Raw), Notifications: notifications, CacheDirectory: directory, HistoryDirectory: privateExchangeDir(t)}
			chain, apiErr := discoverRPKIIssuerChain(context.Background(), certificateTestPEM("CERTIFICATE", f.certs[0].Raw), validation, cmsTrustNow(), client)
			if apiErr != nil {
				t.Fatal(apiErr)
			}
			issuers, apiErr := rpkiPEMCertificates(chain, 31)
			if apiErr != nil || len(issuers) != 2 || !bytes.Equal(issuers[0].Raw, f.certs[1].Raw) || !bytes.Equal(issuers[1].Raw, anchor.Raw) || calls.Load() != 4 {
				t.Fatal("validated discovery API returned wrong chain or bypassed cache")
			}
			validation.IssuerChainPEM = chain
			if _, err := discoverRPKIIssuerChain(context.Background(), certificateTestPEM("CERTIFICATE", f.certs[0].Raw), validation, cmsTrustNow(), client); err == nil || calls.Load() != 4 {
				t.Fatal("conflicting discovery configuration dispatched")
			}
			if _, err := verifyAndRecordRPKIManifestPath(privateExchangeDir(t), path.Certificates, anchor, path.Publications, cmsTrustNow()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
