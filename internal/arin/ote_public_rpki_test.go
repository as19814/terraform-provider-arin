package arin

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"fmt"
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
	_, _, uri := otePublicRPKIBootstrap(t)
	client := rrdpHTTPClient{Transport: otePublicRPKITransport{}, Timeout: time.Minute}
	result, err := client.FetchNotification(context.Background(), uri, time.Time{})
	if err != nil || result.Notification == nil || result.NotModified {
		t.Fatalf("notification: %v", err)
	}
	t.Log("parsed public RRDP notification")
}

func otePublicRPKIBootstrap(t *testing.T) (*x509.Certificate, string, string) {
	t.Helper()
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
	t.Log("validated TAL-pinned trust anchor")
	return anchor, manifestURI, notificationURI
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

// This opt-in downloads the full public repository, potentially over 700 MiB.
// Disk staging is removed at the end; no BPKI or management API calls are made.
func TestOTEPublicRPKIRepository(t *testing.T) {
	if os.Getenv("ARIN_OTE_REPOSITORY_TESTS") != "1" {
		t.Skip("requires ARIN_OTE_REPOSITORY_TESTS=1")
	}
	anchor, manifestURI, notificationURI := otePublicRPKIBootstrap(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client := rrdpHTTPClient{Transport: otePublicRPKITransport{}, Timeout: 5 * time.Minute}
	directory := privateExchangeDir(t)
	fetchedAt := time.Now().UTC()
	cache, err := client.RefreshDiskPersistent(ctx, directory, notificationURI, fetchedAt)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	spool := cache.Objects
	t.Logf("verified snapshot digest: %d objects, %d decoded bytes", len(spool.entries), spool.size)
	// This census reports advertised certificate profiles, not validation of
	// every chain or CMS object in the repository.
	original, reconsidered, mixed, unknown := 0, 0, 0, 0
	for uri := range spool.entries {
		if !strings.HasSuffix(uri, ".cer") {
			continue
		}
		raw, err := spool.ReadObject(uri)
		if err != nil {
			t.Fatal("certificate census: corrupt repository object")
		}
		cert, err := x509.ParseCertificate(raw)
		if err != nil {
			t.Fatal("certificate census: malformed certificate")
		}
		oldPolicy, newPolicy, oldResources, newResources := false, false, false, false
		for _, policy := range cert.PolicyIdentifiers {
			oldPolicy = oldPolicy || policy.String() == "1.3.6.1.5.5.7.14.2"
			newPolicy = newPolicy || policy.String() == "1.3.6.1.5.5.7.14.3"
		}
		for _, extension := range cert.Extensions {
			switch extension.Id.String() {
			case "1.3.6.1.5.5.7.1.7", "1.3.6.1.5.5.7.1.8":
				oldResources = true
			case "1.3.6.1.5.5.7.1.28", "1.3.6.1.5.5.7.1.29":
				newResources = true
			}
		}
		switch {
		case (oldPolicy || oldResources) && (newPolicy || newResources):
			mixed++
		case oldPolicy && oldResources:
			original++
		case newPolicy && newResources:
			reconsidered++
		default:
			unknown++
		}
	}
	if original+reconsidered+mixed+unknown == 0 {
		t.Fatal("certificate census found no certificates")
	}
	t.Logf("certificate profile census: original=%d reconsidered=%d mixed=%d unknown=%d", original, reconsidered, mixed, unknown)
	manifestDER, err := spool.ReadObject(manifestURI)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := decodeRPKIManifest(manifestDER)
	if err != nil {
		t.Fatalf("root manifest: %v", err)
	}
	prefix := manifestURI[:strings.LastIndexByte(manifestURI, '/')+1]
	publication := rpkiPathPublication{ManifestDER: manifestDER, ManifestURI: manifestURI, Files: map[string][]byte{}}
	total := len(manifestDER)
	for name := range manifest.Content.Files {
		data, err := spool.ReadObject(prefix + name)
		if err != nil {
			t.Fatal("root manifest references an absent or corrupt object")
		}
		total += len(data)
		if total > 128<<20 {
			t.Fatal("root publication exceeds path validation limit")
		}
		publication.Files[name] = data
	}
	now := time.Now().UTC()
	if _, err := checkRPKIManifestForIssuer(manifestDER, anchor.Raw, manifestURI, publication.Files, now); err != nil {
		t.Fatalf("root manifest validation: %v", err)
	}
	history := privateExchangeDir(t)
	children := 0
	for name, data := range publication.Files {
		if !strings.HasSuffix(name, ".cer") {
			continue
		}
		child, err := x509.ParseCertificate(data)
		if err != nil {
			t.Fatal(err)
		}
		publication.ChildURI = prefix + name
		for attempt := 0; attempt < 2; attempt++ {
			path, err := client.RetrievePath(ctx, directory, []*x509.Certificate{child, anchor}, []string{notificationURI}, fetchedAt)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := verifyAndRecordRPKIManifestPath(history, path.Certificates, anchor, path.Publications, now); err != nil {
				t.Fatal(fmt.Errorf("published child path validation: %w", err))
			}
		}
		children++
	}
	if children == 0 {
		t.Fatal("no published child CA to validate")
	}
	t.Logf("validated root manifest/CRL and %d child CA paths; reopened durable history", children)
}
