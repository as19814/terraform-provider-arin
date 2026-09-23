package arin

import (
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
