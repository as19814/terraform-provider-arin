package arin

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRPKIPendingPublicationPlan(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	uri := "rsync://repo.example/module/a.cer"
	old := strings.Repeat("a", 64)
	body, err := buildRPKIPublicationBatch([]rpkiPublicationChange{{URI: uri, OldSHA256: old, DER: []byte("new")}, {URI: uri + "-gone", OldSHA256: old, Withdraw: true}, {URI: uri + "-created", DER: []byte("created")}})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"valid", "wrong_peer", "wrong_operation", "wrong_time", "corrupt_signature", "wrong_payload", "missing_evidence"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			exchange := rpkiHTTPExchange{Endpoint: "https://repo.example/publication", MediaType: "application/rpki-publication", Directory: directory, PeerScope: "publisher", Identity: local, PeerAnchor: remote.Anchor}
			peer, err := exchange.peerID()
			if err != nil {
				t.Fatal(err)
			}
			lease, err := openRPKIExchange(directory, peer)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			payload := body
			if mode == "wrong_payload" {
				payload = []byte(rpkiPublicationListQuery)
			}
			signed, err := signRPKICMS(payload, local, cmsTrustNow(), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "corrupt_signature" {
				signed[len(signed)-1] ^= 1
			}
			digest := fmt.Sprintf("%x", sha256.Sum256(signed))
			operation := "publication-batch"
			if mode == "wrong_operation" {
				operation = "updown-issue"
			}
			recorded := cmsTrustNow()
			if mode == "wrong_time" {
				recorded = recorded.Add(time.Second)
			}
			if mode == "missing_evidence" {
				err = lease.Begin(digest, operation, recorded)
			} else {
				err = lease.BeginSigned(digest, operation, recorded, signed)
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "wrong_peer" {
				exchange.PeerScope = "other-publisher"
			}
			plan, err := exchange.pendingPublicationPlan(lease)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode=%s err=%v", mode, err)
			}
			if err == nil {
				if plan.RequestSHA256 != digest || !plan.SigningTime.Equal(recorded) || plan.Before[uri] != old || plan.After[uri] != fmt.Sprintf("%x", sha256.Sum256([]byte("new"))) || plan.After[uri+"-gone"] != "" || plan.Before[uri+"-created"] != "" || len(plan.Before) != 3 {
					t.Fatal("recorded batch intent lost")
				}
			}
			state, err := lease.State()
			if err != nil || state.Pending == nil || state.Pending.RequestSHA256 != digest {
				t.Fatal("inspection cleared pending mutation")
			}
		})
	}
}
func TestRPKIPublicationRecoveryInventory(t *testing.T) {
	a, b, c, g := "rsync://repo.example/module/a", "rsync://repo.example/module/b", "rsync://repo.example/module/c", "rsync://repo.example/module/guard"
	old, next, other := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	plan := rpkiPublicationRecoveryPlan{Before: map[string]string{a: old, b: old, c: "", g: old}, After: map[string]string{a: next, b: "", c: next, g: old}}
	for _, tc := range []struct {
		name    string
		objects []rpkiPublicationObject
		outcome string
	}{
		{"before", []rpkiPublicationObject{{a, old}, {b, old}, {g, old}}, "matches_before"},
		{"after", []rpkiPublicationObject{{a, next}, {c, next}, {g, old}}, "matches_after"},
		{"unmanaged", []rpkiPublicationObject{{a, next}, {c, next}, {g, old}, {"rsync://elsewhere.example/module/unmanaged", other}}, "matches_after"},
		{"partial", []rpkiPublicationObject{{a, next}, {b, old}, {g, old}}, "conflict"},
		{"changed_guard", []rpkiPublicationObject{{a, next}, {c, next}, {g, other}}, "conflict"},
		{"empty", nil, "conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := classifyPublicationInventory(plan, tc.objects)
			if err != nil || got != tc.outcome {
				t.Fatalf("got %q err=%v", got, err)
			}
		})
	}
	same := rpkiPublicationRecoveryPlan{Before: map[string]string{a: old}, After: map[string]string{a: old}}
	if got, err := classifyPublicationInventory(same, []rpkiPublicationObject{{a, old}}); err != nil || got != "ambiguous" {
		t.Fatal("no-op request falsely proved applied")
	}
	for _, objects := range [][]rpkiPublicationObject{{{a, old}, {a, next}}, {{a, "invalid"}}, {{"https://repo.example/object", old}}} {
		if _, err := classifyPublicationInventory(plan, objects); err == nil {
			t.Fatal("malformed inventory accepted")
		}
	}
	if _, err := classifyPublicationInventory(rpkiPublicationRecoveryPlan{Before: map[string]string{a: old}, After: map[string]string{b: next}}, nil); err == nil {
		t.Fatal("mismatched ownership maps accepted")
	}
}
func TestRPKIPublicationRecoveryQueryValidation(t *testing.T) {
	uri := "rsync://repo.example/module/a.cer"
	query := func(pdu string) []byte {
		return []byte(`<msg xmlns="` + rpkiPublicationNamespace + `" version="4" type="query">` + pdu + `</msg>`)
	}
	good := `<publish tag="one" uri="` + uri + `">bmV3</publish>`
	for _, pdu := range []string{"", `<list/>`, good + good, strings.Replace(good, `tag="one"`, "", 1), strings.Replace(good, `uri="`+uri+`"`, `uri="https://repo.example/object"`, 1), strings.Replace(good, ">bmV3<", ` hash="">bmV3<`, 1), `<withdraw tag="one" uri="` + uri + `"/>`, `<withdraw tag="one" uri="` + uri + `" hash="` + strings.Repeat("a", 64) + `">payload</withdraw>`, strings.Replace(good, "bmV3", "invalid", 1), strings.Replace(good, "bmV3", "<nested/>", 1)} {
		if _, err := parsePublicationRecoveryPlan(query(pdu)); err == nil {
			t.Fatalf("invalid pending query accepted: %s", pdu)
		}
	}
	if _, err := parsePublicationRecoveryPlan(query(good)); err != nil {
		t.Fatal(err)
	}
}
