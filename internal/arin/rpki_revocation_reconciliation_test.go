package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPKIRevocationReconciliation(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	for _, mode := range []string{"valid", "valid_api", "observe_api", "observe_proof", "key_present", "class_absent", "wrong_issuer", "wrong_prior", "wrong_hash", "missing_crl", "not_revoked", "history_unavailable", "expired_api", "expired_observe_api", "expired_disabled", "expired_class_absent", "expired_wrong_hash"} {
		t.Run(mode, func(t *testing.T) {
			fixture := "revoked_child"
			if mode == "not_revoked" || strings.HasPrefix(mode, "expired_") {
				fixture = "valid"
			}
			f, pubs := manifestPathFixture(t, fixture)
			if strings.HasPrefix(mode, "expired_") {
				expireRevocationCertificate(t, &f, cmsTrustNow().Add(-time.Second))
			}
			delete(pubs[0].Files, "child.cer")
			pubs[0] = rewriteRevocationManifestFiles(t, pubs[0], f)
			ski, err := rpkiPublicKeyIdentifier(f.certs[0].RawSubjectPublicKeyInfo)
			if err != nil {
				t.Fatal(err)
			}
			original, err := signRPKICMS([]byte(`<message xmlns="`+rpkiUpDownNamespace+`" version="1" sender="child" recipient="parent" type="revoke"><key class_name="class" ski="`+ski+`"/></message>`), local, cmsTrustNow(), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(original))
			now := cmsTrustNow().Add(2 * time.Second)
			directory := privateExchangeDir(t)
			var peer string
			assertLocked := func() {
				if lease, err := openRPKIExchange(directory, peer); err == nil {
					_ = lease.Close()
					t.Error("original journal unlocked during recovery")
				}
			}
			issuer := f.certs[1].Raw
			if mode == "wrong_issuer" {
				issuer = f.certs[2].Raw
			}
			child := ""
			if mode == "key_present" {
				child = fmt.Sprintf(`<certificate cert_url="%s">%s</certificate>`, pubs[0].ChildURI, base64.StdEncoding.EncodeToString(f.certs[0].Raw))
			}
			inventory := fmt.Sprintf(`<class class_name="class" cert_url="rsync://repo.example/module/issuer.cer" resource_set_as="64500-64510" resource_set_ipv4="" resource_set_ipv6="" resource_set_notafter="%s">%s<issuer>%s</issuer></class>`, now.Add(time.Hour).Format("2006-01-02T15:04:05Z"), child, base64.StdEncoding.EncodeToString(issuer))
			if mode == "class_absent" || mode == "expired_class_absent" {
				inventory = ""
			}
			reply, err := signRPKICMS([]byte(upDownTestReply("list_response", inventory)), remote, now, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			var posts, gets atomic.Int32
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				assertLocked()
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				signed, err := verifyRPKICMS(raw, rpkiCMSTrust{Anchor: local.Anchor, Now: now})
				if err != nil {
					t.Error(err)
					return
				}
				root, err := parseXML(signed.Content)
				if err != nil {
					t.Error(err)
					return
				}
				attrs, err := upDownAttrs(root, "message", "version", "sender", "recipient", "type")
				if err != nil || attrs["type"] != "list" || len(root.Children) != 0 || !signed.SigningTime.After(cmsTrustNow()) {
					t.Error("recovery resent mutation or stale request")
					return
				}
				w.Header().Set("Content-Type", "application/rpki-updown")
				_, _ = w.Write(reply)
			}))
			defer endpoint.Close()
			snapshots := make([][]byte, len(pubs))
			for i, p := range pubs {
				prefix := p.ManifestURI[:strings.LastIndexByte(p.ManifestURI, '/')+1]
				var body strings.Builder
				fmt.Fprintf(&body, `<publish uri="%s">%s</publish>`, p.ManifestURI, base64.StdEncoding.EncodeToString(p.ManifestDER))
				for name, data := range p.Files {
					if mode == "missing_crl" && i == 0 && name == "issuer.crl" {
						continue
					}
					fmt.Fprintf(&body, `<publish uri="%s">%s</publish>`, prefix+name, base64.StdEncoding.EncodeToString(data))
				}
				snapshots[i] = rrdpSnapshotTestBody(body.String())
			}
			var repository *httptest.Server
			repository = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gets.Add(1)
				assertLocked()
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
			scope, _ := json.Marshal([]string{"child", "parent"})
			exchange := rpkiHTTPExchange{Endpoint: endpoint.URL, MediaType: "application/rpki-updown", Directory: directory, PeerScope: string(scope), Identity: local, PeerAnchor: remote.Anchor, Clock: func() time.Time { return now }}
			peer, err = exchange.peerID()
			if err != nil {
				t.Fatal(err)
			}
			lease, err := openRPKIExchange(directory, peer)
			if err != nil {
				t.Fatal(err)
			}
			if err := lease.BeginSigned(digest, "updown-revoke", cmsTrustNow(), original); err != nil {
				t.Fatal(err)
			}
			path := lease.path
			if err := lease.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			validation := RPKIRevocationValidation{PriorCertificatePEM: certificateTestPEM("CERTIFICATE", f.certs[0].Raw), Path: RPKICertificateValidation{AnchorPEM: certificateTestPEM("CERTIFICATE", f.certs[2].Raw), IssuerChainPEM: certificateTestPEM("CERTIFICATE", f.certs[1].Raw) + certificateTestPEM("CERTIFICATE", f.certs[2].Raw), Notifications: []string{repository.URL + "/0/notification.xml", repository.URL + "/1/notification.xml"}, CacheDirectory: privateExchangeDir(t), HistoryDirectory: privateExchangeDir(t)}}
			validation.AllowExpired = strings.HasPrefix(mode, "expired_") && mode != "expired_disabled"
			if mode == "wrong_prior" {
				validation.PriorCertificatePEM = certificateTestPEM("CERTIFICATE", f.certs[1].Raw)
			}
			if mode == "history_unavailable" {
				validation.Path.HistoryDirectory = "relative"
			}
			hash := rpkiManifestDigest(f.certs[0].Raw)
			expected := hash
			if mode == "wrong_hash" || mode == "expired_wrong_hash" {
				expected = strings.Repeat("f", 64)
			}
			client := rpkiUpDownClient{Exchange: exchange, Child: "child", Parent: "parent"}
			transport := rrdpHTTPClient{Transport: repository.Client().Transport}
			var o rpkiRevocationRecoveryObservation
			if mode == "valid_api" || mode == "observe_api" || mode == "expired_api" || mode == "expired_observe_api" {
				selected := expected
				if mode == "observe_api" || mode == "expired_observe_api" {
					selected = ""
				}
				var report *RPKIRevocationRecoveryReport
				report, err = recoverRPKIRevocation(context.Background(), certificateTestConfig(t, exchange), &validation, digest, selected, exchange.Clock, transport)
				if err == nil {
					if report.RequestSHA256 != digest || report.Class != "class" || report.SKI != ski || report.Child != "child" || report.Parent != "parent" || report.Committed != (mode == "valid_api" || mode == "expired_api") || report.Outcome != "key_absent" || report.IssuerSHA256 != rpkiManifestDigest(f.certs[1].Raw) || report.CRLSHA256 != rpkiManifestDigest(pubs[0].Files["issuer.crl"]) || report.ManifestSHA256 != rpkiManifestDigest(pubs[0].ManifestDER) || report.CRLURI == "" {
						t.Fatal("API report lost evidence binding")
					}
					if strings.HasPrefix(mode, "expired_") {
						if report.Evidence != "expired_withdrawn" || report.ExpiredAt != f.certs[0].NotAfter.Format(time.RFC3339Nano) || report.CheckedAt != now.Format(time.RFC3339Nano) {
							t.Fatal("expiry reported as revocation")
						}
					} else if report.Evidence != "revoked" || report.ExpiredAt != "" || report.CheckedAt != "" {
						t.Fatal("revocation evidence changed")
					}
					o = rpkiRevocationRecoveryObservation{Sent: report.Sent, Received: report.Received, Proof: &rpkiRevocationProof{CertificateSHA256: report.CertificateSHA256, SKI: report.SKI}}
				}
			} else if mode == "observe_proof" {
				o, err = client.readPendingRevocation(context.Background(), digest, "", &validation, transport)
			} else {
				o, err = client.reconcilePendingRevocation(context.Background(), digest, expected, validation, transport)
			}
			committed := mode == "valid" || mode == "valid_api" || mode == "expired_api"
			success := committed || mode == "observe_proof" || mode == "observe_api" || mode == "expired_observe_api"
			if (err == nil) != success {
				t.Fatalf("mode=%s err=%v", mode, err)
			}
			if posts.Load() != 1 {
				t.Fatal("unexpected inventory request count")
			}
			if success && (o.Proof == nil || o.Proof.CertificateSHA256 != hash || o.Proof.SKI != ski || gets.Load() != 4) {
				t.Fatal("missing bound proof")
			}
			if mode == "key_present" || mode == "class_absent" || mode == "wrong_issuer" || mode == "wrong_prior" {
				if gets.Load() != 0 {
					t.Fatal("unbound inventory fetched repository")
				}
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !committed && !bytes.Equal(before, after) {
				t.Fatal("observation or rejected reconciliation altered journal")
			}
			reopened, err := openRPKIExchange(directory, peer)
			if err != nil {
				t.Fatal(err)
			}
			state, err := reopened.State()
			if err != nil {
				t.Fatal(err)
			}
			if committed {
				if state.Pending != nil || state.ReconciledRevocation == nil || state.ReconciledRevocation.Proof.CertificateSHA256 != hash || !state.LastSent.Equal(o.Sent) || !state.LastReceived.Equal(o.Received) {
					t.Fatal("reconciliation not durably saved")
				}
				if mode == "expired_api" && (state.ReconciledRevocation.Proof.ExpiredAt != f.certs[0].NotAfter.Format(time.RFC3339Nano) || state.ReconciledRevocation.Proof.CheckedAt != now.Format(time.RFC3339Nano)) {
					t.Fatal("durable receipt lost expiry evidence")
				}
				state.ReconciledRevocation.Proof.CertificateSHA256 = "changed"
				copy, err := reopened.State()
				if err != nil || copy.ReconciledRevocation.Proof.CertificateSHA256 != hash {
					t.Fatal("receipt aliases journal state")
				}
			} else if state.Pending == nil || state.ReconciledRevocation != nil {
				t.Fatal("pending state lost")
			}
			if err := reopened.Close(); err != nil {
				t.Fatal(err)
			}
			if committed {
				if _, err := client.reconcilePendingRevocation(context.Background(), digest, expected, validation, transport); err == nil || posts.Load() != 1 {
					t.Fatal("completed mutation recovery repeated network calls")
				}
			}
		})
	}
}
