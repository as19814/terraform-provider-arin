package arin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"sort"
	"time"
)

type RPKIProvisioningReadConfig struct {
	// BPKI supplies transport and signing configuration. Publisher is unused.
	BPKI          RPKIPublicationReadConfig
	Child, Parent string
}
type RPKIProvisioningCertificate struct {
	URLs, PEM, SHA256                          string
	RequestedASN, RequestedIPv4, RequestedIPv6 *string
}
type RPKIProvisioningClass struct {
	Name, CertificateURLs, ASN, IPv4, IPv6, SuggestedSIA, NotAfter, IssuerPEM string
	Certificates                                                              []RPKIProvisioningCertificate
}
type RPKIProvisioningInventory struct {
	ID      string
	Classes []RPKIProvisioningClass
}

// ReadRPKIProvisioning authenticates the BPKI list response, not the returned
// resource certificates' RPKI trust paths. It never issues or revokes a key.
func ReadRPKIProvisioning(ctx context.Context, config RPKIProvisioningReadConfig) (*RPKIProvisioningInventory, error) {
	return readRPKIProvisioning(ctx, config, nil)
}
func readRPKIProvisioning(ctx context.Context, config RPKIProvisioningReadConfig, clock func() time.Time) (*RPKIProvisioningInventory, error) {
	child, parent := upDownToken(config.Child), upDownToken(config.Parent)
	if !upDownLabel(child) || !upDownLabel(parent) {
		return nil, errRPKIUpDown
	}
	exchange, err := config.BPKI.exchange()
	if err != nil {
		return nil, err
	}
	exchange.MediaType = "application/rpki-updown"
	scope, _ := json.Marshal([]string{child, parent})
	exchange.PeerScope = string(scope)
	exchange.Clock = clock
	id, err := exchange.peerID()
	if err != nil {
		return nil, err
	}
	classes, err := (rpkiUpDownClient{Exchange: exchange, Child: child, Parent: parent}).List(ctx)
	if err != nil {
		return nil, err
	}
	encode := func(raw []byte) string {
		return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}))
	}
	out := &RPKIProvisioningInventory{ID: id, Classes: make([]RPKIProvisioningClass, 0, len(classes))}
	for _, c := range classes {
		converted := RPKIProvisioningClass{Name: c.Name, CertificateURLs: c.CertificateURLs, ASN: c.ASN, IPv4: c.IPv4, IPv6: c.IPv6, SuggestedSIA: c.SuggestedSIA, NotAfter: c.NotAfter.UTC().Format(time.RFC3339), IssuerPEM: encode(c.IssuerDER), Certificates: make([]RPKIProvisioningCertificate, 0, len(c.Certificates))}
		for _, cert := range c.Certificates {
			converted.Certificates = append(converted.Certificates, RPKIProvisioningCertificate{URLs: cert.URLs, PEM: encode(cert.DER), SHA256: fmt.Sprintf("%x", sha256.Sum256(cert.DER)), RequestedASN: cloneIssueString(cert.RequestedASN), RequestedIPv4: cloneIssueString(cert.RequestedIPv4), RequestedIPv6: cloneIssueString(cert.RequestedIPv6)})
		}
		sort.Slice(converted.Certificates, func(i, j int) bool {
			a, b := converted.Certificates[i], converted.Certificates[j]
			if a.URLs != b.URLs {
				return a.URLs < b.URLs
			}
			return a.SHA256 < b.SHA256
		})
		out.Classes = append(out.Classes, converted)
	}
	sort.Slice(out.Classes, func(i, j int) bool { return out.Classes[i].Name < out.Classes[j].Name })
	return out, nil
}
