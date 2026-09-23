package arin

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRPKIPublicationBatchValidation(t *testing.T) {
	uri := "rsync://repo.example/module/a.cer"
	hash := strings.Repeat("ab", 32)
	changes := []rpkiPublicationChange{{URI: uri, DER: []byte{0x30, 0}}, {URI: uri + "2", OldSHA256: hash, DER: []byte{0x30, 0}}, {URI: uri + "3", OldSHA256: hash, Withdraw: true}}
	query, err := buildRPKIPublicationBatch(changes)
	if err != nil {
		t.Fatal(err)
	}
	root, err := parseXML(query)
	if err != nil || len(root.Children) != 3 {
		t.Fatal("invalid batch XML")
	}
	for i, p := range root.Children {
		a, _ := publicationAttrs(p, p.Name.Local, "tag", "uri", "hash")
		if a["uri"] != changes[i].URI || a["hash"] != changes[i].OldSHA256 || a["tag"] == "" {
			t.Fatal("lost mutation attributes")
		}
		if i < 2 && (p.Name.Local != "publish" || p.Text != "MAA=") {
			t.Fatal("lost publish bytes")
		}
		if i == 2 && !strings.Contains(string(query), "withdraw") {
			t.Fatal("lost withdrawal")
		}
	}
	a, _ := publicationAttrs(root.Children[0], "publish", "tag", "uri", "hash")
	tag := a["tag"]
	echoed := `<publish tag="` + tag + `" uri="` + uri + `">MAA=</publish>`
	report := `<report_error error_code="object_already_present" tag="` + tag + `"><failed_pdu>` + echoed + `</failed_pdu></report_error>`
	for _, pdu := range []string{`<success/>`, report, `<report_error error_code="xml_error"/>`, report + report} {
		rejected, err := validateRPKIPublicationBatch(query, []byte(publicationTestReply(pdu)))
		if err != nil {
			t.Fatalf("valid reply rejected: %v", err)
		}
		if (pdu == `<success/>`) != (rejected == nil) {
			t.Fatal("wrong result classification")
		}
	}
	for name, pdu := range map[string]string{
		"empty": "", "inventory": `<list/>`, "two_success": `<success/><success/>`, "mixed": `<success/>` + report,
		"wrong_tag":        strings.ReplaceAll(report, tag, "unknown"),
		"wrong_uri":        strings.Replace(report, uri, uri+"wrong", 1),
		"wrong_bytes":      strings.Replace(report, "MAA=", "MAE=", 1),
		"wrong_operation":  strings.ReplaceAll(report, "publish", "withdraw"),
		"missing_echo_tag": strings.Replace(report, `<publish tag="`+tag+`"`, `<publish`, 1),
		"unknown_code":     strings.Replace(report, "object_already_present", "surprise", 1),
	} {
		t.Run(name, func(t *testing.T) {
			rejected, err := validateRPKIPublicationBatch(query, []byte(publicationTestReply(pdu)))
			if err == nil || rejected != nil {
				t.Fatal("invalid reply accepted")
			}
		})
	}
	for name, changes := range map[string][]rpkiPublicationChange{
		"empty":            nil,
		"duplicate":        {{URI: uri, DER: []byte{1}}, {URI: uri, DER: []byte{2}}},
		"missing_bytes":    {{URI: uri}},
		"missing_old_hash": {{URI: uri, Withdraw: true}},
		"withdraw_bytes":   {{URI: uri, Withdraw: true, OldSHA256: hash, DER: []byte{1}}},
		"invalid_uri":      {{URI: "https://example.com/a", DER: []byte{1}}},
		"invalid_hash":     {{URI: uri, OldSHA256: "bad", DER: []byte{1}}},
		"oversized":        {{URI: uri, DER: make([]byte, 4<<20)}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := buildRPKIPublicationBatch(changes); err == nil {
				t.Fatal("invalid batch accepted")
			}
		})
	}
}

func TestRPKIPublicationBatchLifecycle(t *testing.T) {
	local, _ := cmsSigningFixture(t)
	remote, _ := cmsSigningFixture(t)
	var mu sync.Mutex
	objects := map[string][]byte{}
	calls := 0
	malformed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		verified, err := verifyRPKICMS(raw, rpkiCMSTrust{Anchor: local.Anchor, Now: cmsTrustNow()})
		if err != nil {
			t.Error(err)
			return
		}
		query, err := parseXML(verified.Content)
		if err != nil {
			t.Error(err)
			return
		}
		if r.Header.Get("Content-Type") != "application/rpki-publication" || r.Header.Get("Authorization") != "" {
			t.Error("wrong headers")
			return
		}
		// Stage all changes in a copy; only a successful batch changes inventory.
		staged := maps.Clone(objects)
		reply := `<success/>`
		for _, p := range query.Children {
			attrs, err := publicationAttrs(p, p.Name.Local, "tag", "uri", "hash")
			if err != nil {
				t.Error(err)
				return
			}
			uri, old := attrs["uri"], attrs["hash"]
			prior, exists := staged[uri]
			code := ""
			if old == "" && exists {
				code = "object_already_present"
			}
			if old != "" && !exists {
				code = "no_object_present"
			}
			if old != "" && exists && fmt.Sprintf("%x", sha256.Sum256(prior)) != old {
				code = "no_object_matching_hash"
			}
			if code != "" {
				reply = `<report_error error_code="` + code + `" tag="` + attrs["tag"] + `"/>`
				break
			}
			switch p.Name.Local {
			case "publish":
				data, err := base64.StdEncoding.DecodeString(p.Text)
				if err != nil {
					t.Error(err)
					return
				}
				staged[uri] = data
			case "withdraw":
				delete(staged, uri)
			default:
				t.Error("wrong mutation")
				return
			}
		}
		if reply == `<success/>` {
			objects = staged
		}
		if malformed {
			reply = `<success/><success/>`
		}
		signed, err := signRPKICMS([]byte(publicationTestReply(reply)), remote, cmsTrustNow(), time.Time{})
		if err != nil {
			t.Error(err)
			return
		}
		w.Header().Set("Content-Type", "application/rpki-publication")
		_, _ = w.Write(signed)
	}))
	defer server.Close()
	config := rpkiHTTPExchange{Endpoint: server.URL, MediaType: "application/rpki-publication", Directory: privateExchangeDir(t), PeerScope: "publisher", Identity: local, PeerAnchor: remote.Anchor, Clock: cmsTrustNow}
	client := rpkiPublicationClient{Exchange: config}
	ctx := context.Background()
	uri := "rsync://repo.example/module/a.cer"
	first, second := []byte{0x30, 0}, []byte{0x30, 3, 2, 1, 1}
	digest := func(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
	if err := client.Apply(ctx, []rpkiPublicationChange{{URI: uri, DER: first}}); err != nil {
		t.Fatal(err)
	}
	planned, err := planRPKIPublicationBundle(map[string]string{uri: base64.StdEncoding.EncodeToString(second)}, map[string]string{uri: digest(first)})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Apply(ctx, planned); err != nil {
		t.Fatal(err)
	}
	// The planner must guard an unchanged member while replacing another one.
	guard := uri + "-guard"
	mu.Lock()
	objects[guard] = first
	mu.Unlock()
	planned, err = planRPKIPublicationBundle(map[string]string{uri: base64.StdEncoding.EncodeToString(first), guard: base64.StdEncoding.EncodeToString(first)}, map[string]string{uri: digest(second), guard: digest(first)})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	objects[guard] = second // Concurrent edit after the inventory refresh.
	mu.Unlock()
	err = client.Apply(ctx, planned)
	var guardRejected *rpkiPublicationError
	if !errors.As(err, &guardRejected) || guardRejected.Codes[0] != "no_object_matching_hash" {
		t.Fatalf("concurrent edit to unchanged member accepted: %v", err)
	}
	mu.Lock()
	guardAtomic := digest(objects[uri]) == digest(second) && digest(objects[guard]) == digest(second)
	delete(objects, guard)
	mu.Unlock()
	if !guardAtomic {
		t.Fatal("stale unchanged member allowed a partial bundle update")
	}
	// First PDU would create a new object, second has a stale hash. Neither commits.
	err = client.Apply(ctx, []rpkiPublicationChange{{URI: uri + "2", DER: first}, {URI: uri, OldSHA256: digest(first), Withdraw: true}})
	var rejected *rpkiPublicationError
	if !errors.As(err, &rejected) || rejected.Codes[0] != "no_object_matching_hash" || len(rejected.OperationIndexes) != 1 || rejected.OperationIndexes[0] != 1 {
		t.Fatalf("wrong rejection: %v", err)
	}
	mu.Lock()
	unchanged := len(objects) == 1 && digest(objects[uri]) == digest(second)
	mu.Unlock()
	if !unchanged {
		t.Fatal("failed batch changed repository")
	}
	if err := client.Apply(ctx, []rpkiPublicationChange{{URI: uri, OldSHA256: digest(second), Withdraw: true}}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	empty, count := len(objects) == 0, calls
	mu.Unlock()
	if !empty || count != 5 {
		t.Fatal("incorrect lifecycle or retry")
	}
	peer, _ := config.peerID()
	lease, err := openRPKIExchange(config.Directory, peer)
	if err != nil {
		t.Fatal(err)
	}
	state, err := lease.State()
	if err != nil || state.Pending != nil || !state.LastReceived.Equal(cmsTrustNow()) {
		t.Fatal("exchange not durably completed")
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	malformed = true
	mu.Unlock()
	for i := 0; i < 2; i++ {
		if err := client.Apply(ctx, []rpkiPublicationChange{{URI: uri, DER: first}}); err == nil {
			t.Fatal("malformed success accepted")
		}
	}
	mu.Lock()
	count = calls
	mu.Unlock()
	if count != 6 {
		t.Fatal("uncertain mutation was retried")
	}
	lease, err = openRPKIExchange(config.Directory, peer)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	state, err = lease.State()
	if err != nil || state.Pending == nil {
		t.Fatal("uncertain mutation not retained")
	}
}
