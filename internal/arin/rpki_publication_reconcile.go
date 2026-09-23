package arin

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type rpkiPublicationRecoveryPlan struct {
	RequestSHA256 string
	SigningTime   time.Time
	// Every owned URI appears in both maps. An empty hash means absent.
	Before, After map[string]string
}

// pendingPublicationPlan authenticates the persisted request at its recorded
// signing time and binds it to this exact protocol peer. It does not send a
// request, clear the pending journal or infer a remote transaction outcome.
func (c rpkiHTTPExchange) pendingPublicationPlan(lease *rpkiExchangeLease) (rpkiPublicationRecoveryPlan, error) {
	var plan rpkiPublicationRecoveryPlan
	if lease == nil || c.MediaType != "application/rpki-publication" {
		return plan, errRPKIExchangeState
	}
	verified, pending, err := c.pendingMutationContent(lease, "publication-batch")
	if err != nil {
		return plan, err
	}
	plan, err = parsePublicationRecoveryPlan(verified)
	if err != nil {
		return rpkiPublicationRecoveryPlan{}, err
	}
	plan.RequestSHA256 = pending.RequestSHA256
	plan.SigningTime = pending.SigningTime
	return plan, nil
}
func parsePublicationRecoveryPlan(body []byte) (rpkiPublicationRecoveryPlan, error) {
	fail := func() (rpkiPublicationRecoveryPlan, error) {
		return rpkiPublicationRecoveryPlan{}, errRPKIPublicationBatch
	}
	if len(body) == 0 || len(body) > 4<<20 {
		return fail()
	}
	root, err := parseXML(body)
	if err != nil {
		return fail()
	}
	attrs, err := publicationAttrs(root, "msg", "type", "version")
	if err != nil || attrs["type"] != "query" || attrs["version"] != "4" || strings.TrimSpace(root.Text) != "" || len(root.Children) == 0 || len(root.Children) > 10000 {
		return fail()
	}
	plan := rpkiPublicationRecoveryPlan{Before: map[string]string{}, After: map[string]string{}}
	tags := map[string]bool{}
	for _, pdu := range root.Children {
		if pdu.Name.Local != "publish" && pdu.Name.Local != "withdraw" {
			return fail()
		}
		a, err := publicationAttrs(pdu, pdu.Name.Local, "uri", "hash", "tag")
		uri, old, tag := a["uri"], a["hash"], a["tag"]
		if err != nil || !publicationURI(uri) || strings.HasSuffix(uri, "/") || len(pdu.Children) != 0 || tag == "" || len(tag) > 4096 || tags[tag] {
			return fail()
		}
		if _, duplicate := plan.Before[uri]; duplicate {
			return fail()
		}
		tags[tag] = true
		if _, present := a["hash"]; present {
			if len(old) != 64 {
				return fail()
			}
			if _, err := hex.DecodeString(old); err != nil {
				return fail()
			}
		}
		plan.Before[uri] = strings.ToLower(old)
		if pdu.Name.Local == "withdraw" {
			if old == "" || strings.TrimSpace(pdu.Text) != "" {
				return fail()
			}
			plan.After[uri] = ""
		} else {
			data, err := base64.StdEncoding.Strict().DecodeString(pdu.Text)
			if err != nil || len(data) == 0 || len(data) > 3<<20 || base64.StdEncoding.EncodeToString(data) != pdu.Text {
				return fail()
			}
			plan.After[uri] = fmt.Sprintf("%x", sha256.Sum256(data))
		}
	}
	return plan, nil
}

// classifyPublicationInventory compares a caller-authenticated inventory with
// the recorded batch. Its result alone never authorizes journal completion:
// recovery must also authenticate/bind a fresh inventory exchange durably.
func classifyPublicationInventory(plan rpkiPublicationRecoveryPlan, objects []rpkiPublicationObject) (string, error) {
	if len(plan.Before) == 0 || len(plan.Before) > 10000 || len(plan.Before) != len(plan.After) {
		return "", errRPKIPublicationBatch
	}
	if len(objects) > 65536 {
		return "", errRPKIPublicationReply
	}
	total := 0
	observed := make(map[string]string, len(objects))
	for _, object := range objects {
		total += len(object.URI) + len(object.SHA256)
		if total > 4<<20 {
			return "", errRPKIPublicationReply
		}
		if !publicationURI(object.URI) || strings.HasSuffix(object.URI, "/") || !exchangeDigest.MatchString(object.SHA256) {
			return "", errRPKIPublicationReply
		}
		if _, duplicate := observed[object.URI]; duplicate {
			return "", errRPKIPublicationReply
		}
		observed[object.URI] = object.SHA256
	}
	before, after := true, true
	for uri, old := range plan.Before {
		next, ok := plan.After[uri]
		if !ok || !publicationURI(uri) || strings.HasSuffix(uri, "/") || (old != "" && !exchangeDigest.MatchString(old)) || (next != "" && !exchangeDigest.MatchString(next)) || (old == "" && next == "") {
			return "", errRPKIPublicationBatch
		}
		before = before && observed[uri] == old
		after = after && observed[uri] == next
	}
	switch {
	case before && after:
		return "ambiguous", nil
	case after:
		return "matches_after", nil
	case before:
		return "matches_before", nil
	default:
		return "conflict", nil
	}
}
