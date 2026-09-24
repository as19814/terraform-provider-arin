package arin

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIRRDPPathRetrieval(t *testing.T) {
	f, pubs := manifestPathFixture(t, "valid")
	for _, mode := range []string{"valid", "missing_file", "wrong_child", "swapped_repository", "changed_crl", "wrong_anchor"} {
		t.Run(mode, func(t *testing.T) {
			snapshots := make([][]byte, 2)
			for i, p := range pubs {
				prefix := p.ManifestURI[:strings.LastIndexByte(p.ManifestURI, '/')+1]
				objects := map[string][]byte{p.ManifestURI: p.ManifestDER}
				for name, data := range p.Files {
					objects[prefix+name] = data
				}
				if i == 0 {
					switch mode {
					case "missing_file":
						delete(objects, prefix+"issuer.crl")
					case "wrong_child":
						objects[prefix+"child.cer"] = f.certs[1].Raw
					case "changed_crl":
						objects[prefix+"issuer.crl"] = []byte("changed")
					}
				}
				var content strings.Builder
				for uri, data := range objects {
					fmt.Fprintf(&content, `<publish uri="%s">%s</publish>`, uri, base64.StdEncoding.EncodeToString(data))
				}
				snapshots[i] = rrdpSnapshotTestBody(content.String())
			}
			var calls atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				index := 0
				if strings.HasPrefix(r.URL.Path, "/1/") {
					index = 1
				}
				if strings.HasSuffix(r.URL.Path, "snapshot.xml") {
					_, _ = w.Write(snapshots[index])
					return
				}
				_, _ = w.Write(rrdpNotificationTestBody("3", fmt.Sprintf(`<snapshot uri="%s/%d/snapshot.xml" hash="%x"/>`, server.URL, index, sha256.Sum256(snapshots[index]))))
			}))
			defer server.Close()
			notifications := []string{server.URL + "/0/notification.xml", server.URL + "/1/notification.xml"}
			if mode == "swapped_repository" {
				notifications[0], notifications[1] = notifications[1], notifications[0]
			}
			client := rrdpHTTPClient{Transport: server.Client().Transport}
			directory := privateExchangeDir(t)
			path, err := client.RetrievePath(context.Background(), directory, f.certs, notifications, cmsTrustNow())
			rejected := mode == "missing_file" || mode == "wrong_child" || mode == "swapped_repository"
			if (err != nil) != rejected {
				t.Fatalf("retrieval rejection=%v: %v", rejected, err)
			}
			if rejected {
				return
			}
			anchor := f.certs[2]
			if mode == "wrong_anchor" {
				anchor = f.certs[1]
			}
			_, err = verifyAndRecordRPKIManifestPath(privateExchangeDir(t), path.Certificates, anchor, path.Publications, cmsTrustNow())
			if (err == nil) != (mode == "valid") {
				t.Fatalf("path validation mode=%s: %v", mode, err)
			}
			if mode != "valid" {
				return
			}
			if calls.Load() != 4 {
				t.Fatal("unexpected retrieval count")
			}
			path.Publications[0].Files["child.cer"][0] ^= 0xff
			path.Certificates[0].Raw[0] ^= 0xff
			reopened, err := client.RetrievePath(context.Background(), directory, f.certs, notifications, cmsTrustNow())
			if err != nil || calls.Load() != 4 {
				t.Fatalf("cache reuse failed: %v", err)
			}
			if _, err := verifyRPKIManifestPath(reopened.Certificates, anchor, reopened.Publications, cmsTrustNow()); err != nil {
				t.Fatalf("caller mutation corrupted persisted or input data: %v", err)
			}
		})
	}
}

func TestRPKIRRDPPathPreflight(t *testing.T) {
	f, _ := manifestPathFixture(t, "valid")
	client := rrdpHTTPClient{Transport: rrdpTestTransport(func(*http.Request) (*http.Response, error) {
		t.Error("invalid configuration dispatched HTTP")
		return nil, errRPKIRRDP
	})}
	for _, retrieve := range []func(context.Context, string, []*x509.Certificate, []string, time.Time) (rpkiIssuePath, error){client.RetrievePath, client.RetrieveRevocationPath} {
		for _, certs := range [][]*x509.Certificate{nil, {f.certs[0]}, {f.certs[0], nil, f.certs[2]}} {
			if _, err := retrieve(context.Background(), privateExchangeDir(t), certs, []string{"https://repo.example/0", "https://repo.example/1"}, cmsTrustNow()); err == nil {
				t.Fatal("invalid chain accepted")
			}
		}
		if _, err := retrieve(context.Background(), privateExchangeDir(t), f.certs, []string{"https://repo.example/0", "http://repo.example/1"}, cmsTrustNow()); err == nil {
			t.Fatal("insecure notification accepted")
		}
	}
}

func TestRPKIRRDPRevocationRetrieval(t *testing.T) {
	for _, mode := range []string{"valid", "direct_anchor", "missing_file", "missing_upstream", "still_published", "swapped_repository", "changed_crl", "wrong_anchor"} {
		t.Run(mode, func(t *testing.T) {
			f, pubs := manifestPathFixture(t, "revoked_child")
			if mode == "direct_anchor" {
				f.certs = f.certs[1:]
				pubs = pubs[1:]
				pubs[0].Files["issuer.crl"] = resourcePathCRL(t, f.certs[1], f.keys[2], f.certs[0].SerialNumber).Raw
			}
			if mode != "still_published" {
				delete(pubs[0].Files, "child.cer")
			}
			pubs[0] = rewriteRevocationManifestFiles(t, pubs[0], f)
			snapshots := make([][]byte, len(pubs))
			for i, p := range pubs {
				prefix := p.ManifestURI[:strings.LastIndexByte(p.ManifestURI, '/')+1]
				objects := map[string][]byte{p.ManifestURI: p.ManifestDER}
				for name, data := range p.Files {
					objects[prefix+name] = data
				}
				if i == 0 {
					switch mode {
					case "missing_file":
						delete(objects, prefix+"issuer.crl")
					case "changed_crl":
						objects[prefix+"issuer.crl"] = []byte("changed")
					}
				}
				if i == 1 && mode == "missing_upstream" {
					delete(objects, prefix+"child.cer")
				}
				var content strings.Builder
				for uri, data := range objects {
					fmt.Fprintf(&content, `<publish uri="%s">%s</publish>`, uri, base64.StdEncoding.EncodeToString(data))
				}
				snapshots[i] = rrdpSnapshotTestBody(content.String())
			}
			var calls atomic.Int32
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				index := 0
				if strings.HasPrefix(r.URL.Path, "/1/") {
					index = 1
				}
				if strings.HasSuffix(r.URL.Path, "snapshot.xml") {
					_, _ = w.Write(snapshots[index])
					return
				}
				_, _ = w.Write(rrdpNotificationTestBody("3", fmt.Sprintf(`<snapshot uri="%s/%d/snapshot.xml" hash="%x"/>`, server.URL, index, sha256.Sum256(snapshots[index]))))
			}))
			defer server.Close()
			notifications := make([]string, len(pubs))
			for i := range notifications {
				notifications[i] = fmt.Sprintf("%s/%d/notification.xml", server.URL, i)
			}
			if mode == "swapped_repository" {
				notifications[0], notifications[1] = notifications[1], notifications[0]
			}
			client := rrdpHTTPClient{Transport: server.Client().Transport}
			directory := privateExchangeDir(t)
			path, err := client.RetrieveRevocationPath(context.Background(), directory, f.certs, notifications, cmsTrustNow())
			rejected := mode == "missing_file" || mode == "missing_upstream" || mode == "swapped_repository"
			if (err != nil) != rejected {
				t.Fatalf("retrieval rejection=%v: %v", rejected, err)
			}
			if rejected {
				return
			}
			anchor := f.certs[len(f.certs)-1]
			if mode == "wrong_anchor" {
				anchor = f.certs[1]
			}
			ski, err := rpkiPublicKeyIdentifier(f.certs[0].RawSubjectPublicKeyInfo)
			if err != nil {
				t.Fatal(err)
			}
			history := privateExchangeDir(t)
			_, err = verifyAndRecordRPKIRevocationProof(history, path.Certificates[0].Raw, ski, path.Certificates[1:], anchor, path.Publications[1:], path.Publications[0], cmsTrustNow())
			if (err == nil) != (mode == "valid" || mode == "direct_anchor") {
				t.Fatalf("path validation mode=%s: %v", mode, err)
			}
			if mode != "valid" && mode != "direct_anchor" {
				return
			}
			if calls.Load() != int32(2*len(pubs)) {
				t.Fatal("unexpected retrieval count")
			}
			path.Publications[0].Files["issuer.crl"][0] ^= 0xff
			path.Certificates[0].Raw[0] ^= 0xff
			reopened, err := client.RetrieveRevocationPath(context.Background(), directory, f.certs, notifications, cmsTrustNow())
			if err != nil || calls.Load() != int32(2*len(pubs)) {
				t.Fatalf("cache reuse failed: %v", err)
			}
			if _, err := verifyAndRecordRPKIRevocationProof(history, reopened.Certificates[0].Raw, ski, reopened.Certificates[1:], anchor, reopened.Publications[1:], reopened.Publications[0], cmsTrustNow()); err != nil {
				t.Fatalf("caller mutation corrupted persisted or input data: %v", err)
			}
		})
	}
}
