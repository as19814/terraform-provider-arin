package arin

import "bytes"

// historicalRevocationClass reauthenticates evidence retained before the pending
// revoke. Neither a caller-provided class nor a matching public key is enough:
// the signed inventory must contain the exact prior certificate.
func (client rpkiUpDownClient) historicalRevocationClass(lease *rpkiExchangeLease, plan rpkiRevocationRecoveryPlan, validation RPKIRevocationValidation) (rpkiResourceClass, string, error) {
	receipt, body, err := lease.CompletedResponse()
	if err != nil || receipt.Request.Operation != "updown-list" || !receipt.Request.SigningTime.Before(plan.SigningTime) {
		return rpkiResourceClass{}, "", errRPKIUpDown
	}
	c := client.Exchange
	verified, err := verifyRPKICMS(body, rpkiCMSTrust{Anchor: c.PeerAnchor, Intermediates: c.PeerIntermediates, Now: receipt.SigningTime})
	if err != nil || !verified.SigningTime.Equal(receipt.SigningTime) {
		return rpkiResourceClass{}, "", errRPKIUpDown
	}
	classes, rejection, err := parseUpDownList(verified.Content, upDownToken(client.Child), upDownToken(client.Parent))
	if err != nil || rejection != nil {
		return rpkiResourceClass{}, "", errRPKIUpDown
	}
	prior, err := rpkiPEMCertificates(validation.PriorCertificatePEM, 1)
	if err != nil || len(prior) != 1 {
		return rpkiResourceClass{}, "", errRPKIUpDown
	}
	for _, class := range classes {
		if class.Name != plan.Class {
			continue
		}
		for _, certificate := range class.Certificates {
			if bytes.Equal(certificate.DER, prior[0].Raw) {
				return class, receipt.SHA256, nil
			}
		}
	}
	return rpkiResourceClass{}, "", errRPKIUpDown
}
