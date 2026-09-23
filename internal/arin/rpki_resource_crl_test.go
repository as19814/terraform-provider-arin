package arin

import (
	"crypto/rand"
	"crypto/x509"
	"math/big"
	"testing"
	"time"
)

func TestRPKIResourceCRLIgnoresNumberForSelection(t *testing.T) {
	f := resourceCertificateFixture(t)
	cert, issuer, key := f.certs[0], f.certs[1], f.keys[1]
	create := func(number *big.Int, revoked bool) *x509.RevocationList {
		t.Helper()
		template := &x509.RevocationList{Number: number, ThisUpdate: cmsTrustNow().Add(-time.Minute), NextUpdate: cmsTrustNow().Add(time.Hour)}
		if revoked {
			template.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: cert.SerialNumber, RevocationTime: template.ThisUpdate}}
		}
		raw, err := x509.CreateRevocationList(rand.Reader, template, issuer, key)
		if err != nil {
			t.Fatal(err)
		}
		crl, err := x509.ParseRevocationList(raw)
		if err != nil {
			t.Fatal(err)
		}
		return crl
	}
	low := create(big.NewInt(0), false)
	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 159), big.NewInt(1))
	high := create(max, false)
	for _, crl := range []*x509.RevocationList{low, high} {
		if !resourceCertificateUnrevoked(cert, issuer, []*x509.RevocationList{crl}, cmsTrustNow()) {
			t.Fatal("valid selected CRL rejected based on its number")
		}
		if _, err := verifyRPKICertificatePath(f.certs, f.certs[2], []*x509.RevocationList{crl, f.crls[1]}, cmsTrustNow()); err != nil {
			t.Fatal(err)
		}
	}
	for _, crls := range [][]*x509.RevocationList{{low, high}, {high, low}, {create(big.NewInt(0), true), high}} {
		if resourceCertificateUnrevoked(cert, issuer, crls, cmsTrustNow()) {
			t.Fatal("ambiguous CRLs selected using their numbers")
		}
		if _, err := verifyRPKICertificatePath(f.certs, f.certs[2], append(crls, f.crls[1]), cmsTrustNow()); err == nil {
			t.Fatal("path selected an ambiguous CRL")
		}
	}
	if resourceCertificateUnrevoked(cert, issuer, []*x509.RevocationList{create(big.NewInt(0), true)}, cmsTrustNow()) {
		t.Fatal("selected low-number CRL revocation ignored")
	}
	// Repeating the same DER is not a second candidate.
	if !resourceCertificateUnrevoked(cert, issuer, []*x509.RevocationList{low, low}, cmsTrustNow()) {
		t.Fatal("identical CRL copies treated as ambiguous")
	}
}
