package arin

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// This transport permits only credential-free reads from the public sandbox
// repository, including redirects followed by the RRDP client.
type otePublicRPKITransport struct{}

func (otePublicRPKITransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet || r.URL.Scheme != "https" || r.URL.Host != "rrdp.ote.arin.net" || r.URL.User != nil || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
		return nil, errors.New("public OT&E repository request rejected")
	}
	return http.DefaultTransport.RoundTrip(r)
}

// No enrollment or API key is needed. The checked-in TAL pins the trust anchor;
// downloaded objects are temporary and are never added to the repository.
func TestOTEPublicRPKIBootstrap(t *testing.T) {
	if os.Getenv("ARIN_OTE_PUBLIC_TESTS") != "1" {
		t.Skip("requires ARIN_OTE_PUBLIC_TESTS=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := rrdpHTTPClient{Transport: otePublicRPKITransport{}, Timeout: time.Minute}
	tal, err := os.ReadFile("../../docs/reference/arin-api/rpki/arin_ote.tal")
	if err != nil {
		t.Fatal(err)
	}
	_, key, ok := strings.Cut(strings.ReplaceAll(string(tal), "\r\n", "\n"), "\n\n")
	if !ok {
		t.Fatal("TAL has no public key")
	}
	spki, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(key), ""))
	if err != nil {
		t.Fatal(err)
	}
	der, _, _, err := client.get(ctx, "https://rrdp.ote.arin.net/arin-rpki-ta.cer", 512000, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	anchor, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(anchor.RawSubjectPublicKeyInfo, spki) {
		t.Fatal("sandbox trust anchor does not match saved TAL")
	}
	now := time.Now().UTC()
	if _, err := verifyRPKIManifestPath([]*x509.Certificate{anchor}, anchor, nil, now); err != nil {
		t.Fatalf("trust anchor validation: %v", err)
	}
	sia, err := rpkiCASIA(anchor.Extensions)
	var descriptions []rpkiAccessDescription
	if err != nil || !rpkiCSRDER(sia, &descriptions) {
		t.Fatal("invalid trust anchor SIA")
	}
	var manifestURI, notificationURI string
	for _, d := range descriptions {
		if d.Method.Equal(oidRPKIManifest) {
			manifestURI = string(d.Location.Bytes)
		}
		if d.Method.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 13}) {
			notificationURI = string(d.Location.Bytes)
		}
	}
	if manifestURI == "" || notificationURI == "" {
		t.Fatal("missing trust anchor repository locations")
	}
	result, err := client.FetchNotification(ctx, notificationURI, time.Time{})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if result.Notification == nil || result.NotModified || result.Notification.Snapshot.URI == "" {
		t.Fatal("missing initial repository snapshot")
	}
	t.Log("validated TAL-pinned trust anchor and parsed its RRDP notification; repository objects are not validated by this bootstrap test")
}

func TestOTEPublicRPKITransportGuard(t *testing.T) {
	for _, tc := range []struct{ method, uri, header string }{
		{"POST", "https://rrdp.ote.arin.net/notification.xml", ""},
		{"GET", "http://rrdp.ote.arin.net/notification.xml", ""},
		{"GET", "https://rrdp.arin.net/notification.xml", ""},
		{"GET", "https://rrdp.ote.arin.net:443/notification.xml", ""},
		{"GET", "https://user@rrdp.ote.arin.net/notification.xml", ""},
		{"GET", "https://rrdp.ote.arin.net/notification.xml?apikey=sentinel", ""},
		{"GET", "https://rrdp.ote.arin.net/notification.xml", "Authorization"},
		{"GET", "https://rrdp.ote.arin.net/notification.xml", "Cookie"},
	} {
		r, err := http.NewRequest(tc.method, tc.uri, nil)
		if err != nil {
			t.Fatal(err)
		}
		if tc.header != "" {
			r.Header.Set(tc.header, "sentinel")
		}
		if _, err := (otePublicRPKITransport{}).RoundTrip(r); err == nil {
			t.Fatal("forbidden public repository request accepted")
		}
	}
}
