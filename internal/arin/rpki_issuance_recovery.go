package arin

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"time"
)

type rpkiIssuanceRecoveryPlan struct {
	RequestSHA256      string
	SigningTime        time.Time
	Child, Parent, SKI string
	Request            RPKICertificateRequest
}

// pendingIssuancePlan recovers the exact requested key, class and resource sets
// from authenticated journal evidence. Inspection does not clear or resend it.
func (c rpkiUpDownClient) pendingIssuancePlan(lease *rpkiExchangeLease) (rpkiIssuanceRecoveryPlan, error) {
	var plan rpkiIssuanceRecoveryPlan
	child, parent := upDownToken(c.Child), upDownToken(c.Parent)
	if !upDownLabel(child) || !upDownLabel(parent) || c.Exchange.MediaType != "application/rpki-updown" {
		return plan, errRPKIUpDown
	}
	exchange := c.Exchange
	scope, _ := json.Marshal([]string{child, parent})
	exchange.PeerScope = string(scope)
	body, pending, err := exchange.pendingMutationContent(lease, "updown-issue")
	if err != nil {
		return plan, err
	}
	plan, err = parseIssuanceRecoveryPlan(body, child, parent)
	if err != nil {
		return rpkiIssuanceRecoveryPlan{}, err
	}
	plan.RequestSHA256, plan.SigningTime = pending.RequestSHA256, pending.SigningTime
	return plan, nil
}

func parseIssuanceRecoveryPlan(body []byte, child, parent string) (rpkiIssuanceRecoveryPlan, error) {
	fail := func() (rpkiIssuanceRecoveryPlan, error) { return rpkiIssuanceRecoveryPlan{}, errRPKIUpDown }
	if len(body) == 0 || len(body) > 4<<20 || !upDownLabel(child) || !upDownLabel(parent) {
		return fail()
	}
	root, err := parseXML(body)
	if err != nil {
		return fail()
	}
	attrs, err := upDownAttrs(root, "message", "version", "sender", "recipient", "type")
	if err != nil || attrs["version"] != "1" || attrs["type"] != "issue" || upDownToken(attrs["sender"]) != child || upDownToken(attrs["recipient"]) != parent || strings.Trim(root.Text, " \t\r\n") != "" || len(root.Children) != 1 {
		return fail()
	}
	node := root.Children[0]
	attrs, err = upDownAttrs(node, "request", "class_name", "req_resource_set_as", "req_resource_set_ipv4", "req_resource_set_ipv6")
	if err != nil || len(node.Children) != 0 || len(node.Text) > base64.StdEncoding.EncodedLen(512000) {
		return fail()
	}
	der, err := base64.StdEncoding.Strict().DecodeString(node.Text)
	if err != nil || base64.StdEncoding.EncodeToString(der) != node.Text {
		return fail()
	}
	request := rpkiIssueRequest{Class: upDownToken(attrs["class_name"]), CSRDER: der}
	for name, dest := range map[string]**string{"req_resource_set_as": &request.RequestedASN, "req_resource_set_ipv4": &request.RequestedIPv4, "req_resource_set_ipv6": &request.RequestedIPv6} {
		if value, ok := attrs[name]; ok {
			*dest = &value
		}
	}
	if _, err := buildUpDownIssue(child, parent, request); err != nil {
		return fail()
	}
	input := RPKICertificateRequest{Class: request.Class, CSRPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})), RequestedASN: request.RequestedASN, RequestedIPv4: request.RequestedIPv4, RequestedIPv6: request.RequestedIPv6}
	ski, err := RPKICertificateRequestKey(input.CSRPEM)
	if err != nil {
		return fail()
	}
	return rpkiIssuanceRecoveryPlan{Child: child, Parent: parent, SKI: ski, Request: input}, nil
}
