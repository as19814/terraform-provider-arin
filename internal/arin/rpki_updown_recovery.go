package arin

import (
	"crypto/x509"
	"encoding/json"
	"strings"
	"time"
)

type rpkiRevocationRecoveryPlan struct {
	RequestSHA256             string
	SigningTime               time.Time
	Child, Parent, Class, SKI string
}

// pendingRevocationPlan recovers authenticated intent only. It neither resends
// the request nor clears pending state, including scheduled revocations.
func (c rpkiUpDownClient) pendingRevocationPlan(lease *rpkiExchangeLease) (rpkiRevocationRecoveryPlan, error) {
	var plan rpkiRevocationRecoveryPlan
	child, parent := upDownToken(c.Child), upDownToken(c.Parent)
	if !upDownLabel(child) || !upDownLabel(parent) || c.Exchange.MediaType != "application/rpki-updown" {
		return plan, errRPKIUpDown
	}
	exchange := c.Exchange
	scope, _ := json.Marshal([]string{child, parent})
	exchange.PeerScope = string(scope)
	body, pending, err := exchange.pendingMutationContent(lease, "updown-revoke")
	if err != nil {
		return plan, err
	}
	plan, err = parseRevocationRecoveryPlan(body, child, parent)
	if err != nil {
		return rpkiRevocationRecoveryPlan{}, err
	}
	plan.RequestSHA256, plan.SigningTime = pending.RequestSHA256, pending.SigningTime
	return plan, nil
}

func parseRevocationRecoveryPlan(body []byte, child, parent string) (rpkiRevocationRecoveryPlan, error) {
	fail := func() (rpkiRevocationRecoveryPlan, error) { return rpkiRevocationRecoveryPlan{}, errRPKIUpDown }
	if len(body) == 0 || len(body) > 4<<20 || !upDownLabel(child) || !upDownLabel(parent) {
		return fail()
	}
	root, err := parseXML(body)
	if err != nil {
		return fail()
	}
	a, err := upDownAttrs(root, "message", "version", "sender", "recipient", "type")
	if err != nil || a["version"] != "1" || a["type"] != "revoke" || upDownToken(a["sender"]) != child || upDownToken(a["recipient"]) != parent || strings.Trim(root.Text, " \t\r\n") != "" || len(root.Children) != 1 {
		return fail()
	}
	key := root.Children[0]
	a, err = upDownAttrs(key, "key", "class_name", "ski")
	if err != nil || len(key.Children) != 0 || strings.Trim(key.Text, " \t\r\n") != "" {
		return fail()
	}
	class := upDownToken(a["class_name"])
	ski, err := upDownSKI(a["ski"])
	if err != nil || !upDownLabel(class) {
		return fail()
	}
	return rpkiRevocationRecoveryPlan{Child: child, Parent: parent, Class: class, SKI: ski}, nil
}

// classifyRevocationInventory consumes an authenticated, parsed parent list.
// Presence may mean scheduled processing; absence is not proof of CRL publication.
func classifyRevocationInventory(plan rpkiRevocationRecoveryPlan, classes []rpkiResourceClass) (string, error) {
	ski, err := upDownSKI(plan.SKI)
	if err != nil || ski != plan.SKI || !upDownLabel(plan.Class) || upDownToken(plan.Class) != plan.Class || len(classes) > 10000 {
		return "", errRPKIUpDown
	}
	seen := map[string]bool{}
	found, present := false, false
	total, count := 0, 0
	for _, class := range classes {
		if !upDownLabel(class.Name) || upDownToken(class.Name) != class.Name || seen[class.Name] {
			return "", errRPKIUpDown
		}
		seen[class.Name] = true
		count += len(class.Certificates)
		if count > 10000 {
			return "", errRPKIUpDown
		}
		if class.Name == plan.Class {
			found = true
		}
		for _, item := range class.Certificates {
			total += len(item.DER)
			if len(item.DER) > 512000 || total > 4<<20 {
				return "", errRPKIUpDown
			}
			cert, err := x509.ParseCertificate(item.DER)
			if err != nil || !cert.IsCA || !cert.BasicConstraintsValid {
				return "", errRPKIUpDown
			}
			identifier, err := rpkiPublicKeyIdentifier(cert.RawSubjectPublicKeyInfo)
			if err != nil {
				return "", err
			}
			if class.Name == plan.Class && identifier == ski {
				present = true
			}
		}
	}
	if present {
		return "key_present", nil
	}
	if !found {
		return "class_absent", nil
	}
	return "key_absent", nil
}
