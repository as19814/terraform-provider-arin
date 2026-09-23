package arin

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"sort"
	"time"
)

// RPKIPublicationReadConfig uses an existing BPKI identity. Private key bytes are
// loaded from disk for the request and never returned to Terraform state.
type RPKIPublicationReadConfig struct {
	Endpoint, Publisher, JournalDirectory, SigningKeyFile                            string
	SigningCertificatePEM, SigningAnchorPEM, SigningIntermediatesPEM, SigningCRLsPEM string
	PeerAnchorPEM, PeerIntermediatesPEM                                              string
}

type RPKIPublicationObject struct{ URI, SHA256 string }
type RPKIPublicationInventory struct {
	ID      string
	Objects []RPKIPublicationObject
}

var errRPKIIdentityConfig = errors.New("invalid RPKI BPKI identity configuration")

func ReadRPKIPublication(ctx context.Context, config RPKIPublicationReadConfig) (*RPKIPublicationInventory, error) {
	return readRPKIPublication(ctx, config, nil)
}

func readRPKIPublication(ctx context.Context, config RPKIPublicationReadConfig, clock func() time.Time) (*RPKIPublicationInventory, error) {
	exchange, err := config.exchange()
	if err != nil {
		return nil, err
	}
	exchange.Clock = clock
	id, err := exchange.peerID()
	if err != nil {
		return nil, err
	}
	objects, err := (rpkiPublicationClient{Exchange: exchange}).List(ctx)
	if err != nil {
		return nil, err
	}
	result := &RPKIPublicationInventory{ID: id, Objects: make([]RPKIPublicationObject, 0, len(objects))}
	for _, object := range objects {
		result.Objects = append(result.Objects, RPKIPublicationObject{URI: object.URI, SHA256: object.SHA256})
	}
	sort.Slice(result.Objects, func(i, j int) bool { return result.Objects[i].URI < result.Objects[j].URI })
	return result, nil
}

func (config RPKIPublicationReadConfig) exchange() (rpkiHTTPExchange, error) {
	fail := func() (rpkiHTTPExchange, error) { return rpkiHTTPExchange{}, errRPKIIdentityConfig }
	certs, err := rpkiPEMCertificates(config.SigningCertificatePEM, 1)
	if err != nil || len(certs) != 1 {
		return fail()
	}
	anchors, err := rpkiPEMCertificates(config.SigningAnchorPEM, 1)
	if err != nil || len(anchors) != 1 {
		return fail()
	}
	intermediates, err := rpkiPEMCertificates(config.SigningIntermediatesPEM, 31)
	if err != nil {
		return fail()
	}
	peers, err := rpkiPEMCertificates(config.PeerAnchorPEM, 1)
	if err != nil || len(peers) != 1 {
		return fail()
	}
	peerIntermediates, err := rpkiPEMCertificates(config.PeerIntermediatesPEM, 31)
	if err != nil {
		return fail()
	}
	blocks, err := rpkiPEMBlocks(config.SigningCRLsPEM, "X509 CRL", 32)
	if err != nil || len(blocks) == 0 {
		return fail()
	}
	var crls []*x509.RevocationList
	for _, block := range blocks {
		crl, err := x509.ParseRevocationList(block)
		if err != nil {
			return fail()
		}
		crls = append(crls, crl)
	}
	key, err := readRPKISigningKey(config.SigningKeyFile)
	if err != nil {
		return fail()
	}
	return rpkiHTTPExchange{Endpoint: config.Endpoint, MediaType: "application/rpki-publication", Directory: config.JournalDirectory, PeerScope: config.Publisher, Identity: rpkiCMSSigningIdentity{Signer: key, Certificate: certs[0], Anchor: anchors[0], Intermediates: intermediates, CRLs: crls}, PeerAnchor: peers[0], PeerIntermediates: peerIntermediates}, nil
}

func rpkiPEMCertificates(value string, limit int) ([]*x509.Certificate, error) {
	blocks, err := rpkiPEMBlocks(value, "CERTIFICATE", limit)
	if err != nil {
		return nil, err
	}
	var certs []*x509.Certificate
	for _, block := range blocks {
		cert, err := x509.ParseCertificate(block)
		if err != nil {
			return nil, errRPKIIdentityConfig
		}
		certs = append(certs, cert)
	}
	return certs, nil
}
func rpkiPEMBlocks(value, kind string, limit int) ([][]byte, error) {
	if len(value) > 4<<20 {
		return nil, errRPKIIdentityConfig
	}
	rest := bytes.TrimSpace([]byte(value))
	var blocks [][]byte
	for len(rest) > 0 {
		if len(blocks) >= limit || !bytes.HasPrefix(rest, []byte("-----BEGIN "+kind+"-----")) {
			return nil, errRPKIIdentityConfig
		}
		block, tail := pem.Decode(rest)
		if block == nil || block.Type != kind || len(block.Headers) != 0 {
			return nil, errRPKIIdentityConfig
		}
		blocks = append(blocks, block.Bytes)
		rest = bytes.TrimSpace(tail)
	}
	return blocks, nil
}
func readRPKISigningKey(path string) (*rsa.PrivateKey, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 65536 {
		return nil, errRPKIIdentityConfig
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errRPKIIdentityConfig
	}
	actual, statErr := file.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(file, 65537))
	closeErr := file.Close()
	if statErr != nil || !os.SameFile(info, actual) || readErr != nil || closeErr != nil || len(raw) > 65536 {
		return nil, errRPKIIdentityConfig
	}
	defer clear(raw)
	raw = bytes.TrimSpace(raw)
	kind := "PRIVATE KEY"
	if bytes.HasPrefix(raw, []byte("-----BEGIN RSA PRIVATE KEY-----")) {
		kind = "RSA PRIVATE KEY"
	}
	if !bytes.HasPrefix(raw, []byte("-----BEGIN "+kind+"-----")) {
		return nil, errRPKIIdentityConfig
	}
	block, rest := pem.Decode(raw)
	if block == nil || block.Type != kind || len(block.Headers) != 0 || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errRPKIIdentityConfig
	}
	defer clear(block.Bytes)
	var key *rsa.PrivateKey
	if kind == "RSA PRIVATE KEY" {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	} else {
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		key, _ = parsed.(*rsa.PrivateKey)
	}
	if err != nil || key == nil || key.N.BitLen() < 2048 || key.N.BitLen() > 8192 || key.Validate() != nil {
		return nil, errRPKIIdentityConfig
	}
	return key, nil
}
