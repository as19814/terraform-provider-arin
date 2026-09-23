package arin

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func cmsSigningFixture(t *testing.T) (rpkiCMSSigningIdentity, []byte) {
	t.Helper()
	sd, caKey, ca := cmsTrustFixture(t)
	eeKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template, err := x509.ParseCertificate(sd.Certs.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.CreateCertificate(rand.Reader, template, ca, &eeKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ee, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	crl, err := x509.ParseRevocationList(sd.CRLs.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return rpkiCMSSigningIdentity{Signer: eeKey, Certificate: ee, Anchor: ca, CRLs: []*x509.RevocationList{crl}}, sd.Encap.Content
}

type cmsCountingSigner struct {
	key   *rsa.PrivateKey
	calls int
	mode  string
}

func (s *cmsCountingSigner) Public() crypto.PublicKey { return &s.key.PublicKey }
func (s *cmsCountingSigner) Sign(r io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	s.calls++
	if opts.HashFunc() != crypto.SHA256 || len(digest) != 32 {
		return nil, errors.New("incorrect signer contract")
	}
	if s.mode == "error" {
		return nil, errors.New("private-device-secret-detail")
	}
	if s.mode == "corrupt" {
		return []byte("invalid"), nil
	}
	return s.key.Sign(r, digest, opts)
}
func TestRPKICMSSigning(t *testing.T) {
	identity, content := cmsSigningFixture(t)
	signer := &cmsCountingSigner{key: identity.Signer.(*rsa.PrivateKey)}
	identity.Signer = signer
	now := cmsTrustNow().Add(123 * time.Millisecond)
	der, err := signRPKICMS(content, identity, now, cmsTrustNow())
	if err != nil {
		t.Fatal(err)
	}
	if signer.calls != 1 {
		t.Fatal("unexpected signing calls")
	}
	got, err := verifyRPKICMS(der, rpkiCMSTrust{Anchor: identity.Anchor, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Content, content) || !got.SigningTime.Equal(cmsTrustNow()) {
		t.Fatal("signed content or whole-second time changed")
	}
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("OpenSSL not installed")
	}
	dir := t.TempDir()
	message := filepath.Join(dir, "message.der")
	anchor := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(message, der, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(anchor, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: identity.Anchor.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(openssl, "cms", "-verify", "-binary", "-inform", "DER", "-in", message, "-CAfile", anchor, "-purpose", "any", "-attime", strconv.FormatInt(cmsTrustNow().Unix(), 10), "-crl_check")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("OpenSSL verification failed: %v: %s", err, stderr.String())
	}
	if !bytes.Equal(output, content) {
		t.Fatal("OpenSSL payload mismatch")
	}
}
func TestRPKICMSSigningRejectsBeforeSigning(t *testing.T) {
	for _, mode := range []string{"nil_certificate", "nil_signer", "typed_nil_signer", "nil_anchor", "wrong_key", "ca_signer", "no_crl", "bad_crl", "expired", "clock_rollback", "zero_clock", "malformed_xml", "doctype", "empty_xml", "oversized_xml", "nil_intermediate", "nil_crl"} {
		t.Run(mode, func(t *testing.T) {
			identity, content := cmsSigningFixture(t)
			now := cmsTrustNow()
			last := now
			counter := &cmsCountingSigner{key: identity.Signer.(*rsa.PrivateKey)}
			identity.Signer = counter
			switch mode {
			case "nil_certificate":
				identity.Certificate = nil
			case "nil_signer":
				identity.Signer = nil
			case "typed_nil_signer":
				identity.Signer = (*rsa.PrivateKey)(nil)
			case "nil_anchor":
				identity.Anchor = nil
			case "wrong_key":
				key, err := rsa.GenerateKey(rand.Reader, 2048)
				if err != nil {
					t.Fatal(err)
				}
				counter.key = key
			case "ca_signer":
				identity.Certificate = identity.Anchor
			case "no_crl":
				identity.CRLs = nil
			case "bad_crl":
				broken := bytes.Clone(identity.CRLs[0].Raw)
				broken[len(broken)-1] ^= 1
				parsed, err := x509.ParseRevocationList(broken)
				if err != nil {
					t.Fatal(err)
				}
				identity.CRLs = []*x509.RevocationList{parsed}
			case "expired":
				now = now.Add(2 * time.Hour)
			case "clock_rollback":
				last = now.Add(time.Second)
			case "zero_clock":
				now = time.Time{}
			case "malformed_xml":
				content = []byte("<message>")
			case "doctype":
				content = append([]byte(`<!DOCTYPE message SYSTEM "https://example.net">`), content...)
			case "empty_xml":
				content = nil
			case "oversized_xml":
				content = bytes.Repeat([]byte("x"), (4<<20)+1)
			case "nil_intermediate":
				identity.Intermediates = []*x509.Certificate{nil}
			case "nil_crl":
				identity.CRLs = []*x509.RevocationList{nil}
			}
			if _, err := signRPKICMS(content, identity, now, last); err == nil {
				t.Fatal("invalid signing request accepted")
			}
			if counter.calls != 0 {
				t.Fatal("invalid request reached signer")
			}
		})
	}
}
func TestRPKICMSSignerFailures(t *testing.T) {
	for _, mode := range []string{"error", "corrupt"} {
		identity, content := cmsSigningFixture(t)
		counter := &cmsCountingSigner{key: identity.Signer.(*rsa.PrivateKey), mode: mode}
		identity.Signer = counter
		output, err := signRPKICMS(content, identity, cmsTrustNow(), time.Time{})
		if err == nil || output != nil || counter.calls != 1 {
			t.Fatal("signer failure was not terminal")
		}
		if strings.Contains(err.Error(), "private-device-secret-detail") {
			t.Fatal("signer details leaked")
		}
	}
}

func TestRPKICMSSigningWithIntermediate(t *testing.T) {
	sd, rootKey, root := cmsTrustFixture(t)
	intermediateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := *root
	template.SerialNumber = big.NewInt(3)
	template.Subject = pkix.Name{CommonName: "Signing intermediate"}
	template.SubjectKeyId = []byte{7, 8, 9}
	template.AuthorityKeyId = nil
	raw, err := x509.CreateCertificate(rand.Reader, &template, root, &intermediateKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	intermediate, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	eeKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	eeTemplate, err := x509.ParseCertificate(sd.Certs.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	eeTemplate.AuthorityKeyId = nil
	raw, err = x509.CreateCertificate(rand.Reader, eeTemplate, intermediate, &eeKey.PublicKey, intermediateKey)
	if err != nil {
		t.Fatal(err)
	}
	ee, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	rootCRL, err := x509.ParseRevocationList(sd.CRLs.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	eeCRL, err := x509.ParseRevocationList(cmsTestCRL(t, intermediate, intermediateKey, nil))
	if err != nil {
		t.Fatal(err)
	}
	identity := rpkiCMSSigningIdentity{Signer: eeKey, Certificate: ee, Anchor: root, Intermediates: []*x509.Certificate{intermediate}, CRLs: []*x509.RevocationList{eeCRL, rootCRL}}
	der, err := signRPKICMS(sd.Encap.Content, identity, cmsTrustNow(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeRPKICMS(der)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Certificates) != 2 || len(decoded.CRLs) != 2 {
		t.Fatal("chain material missing")
	}
	if _, err := verifyRPKICMS(der, rpkiCMSTrust{Anchor: root, Now: cmsTrustNow()}); err != nil {
		t.Fatal(err)
	}
}
