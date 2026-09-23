package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const rrdpTestNotificationURI = "https://repo.example/notification.xml"
const rrdpTestObjectURI = "rsync://repo.example/module/a.cer"

func rrdpDeltaTestBody(serial, content string) []byte {
	return []byte(fmt.Sprintf(`<delta xmlns="%s" version="1" session_id="%s" serial="%s">%s</delta>`, rrdpNamespace, rrdpTestSession, serial, content))
}
func rrdpDeltaPublish(uri string, old, data []byte) string {
	attr := ""
	if old != nil {
		attr = fmt.Sprintf(` hash="%x"`, sha256.Sum256(old))
	}
	return fmt.Sprintf(`<publish uri="%s"%s>%s</publish>`, uri, attr, base64.StdEncoding.EncodeToString(data))
}
func rrdpDeltaWithdraw(uri string, old []byte) string {
	return fmt.Sprintf(`<withdraw uri="%s" hash="%x"/>`, uri, sha256.Sum256(old))
}

func TestRPKIRRDPDelta(t *testing.T) {
	base := rrdpRepository{NotificationURI: rrdpTestNotificationURI, Session: rrdpTestSession, Serial: "2", Objects: map[string][]byte{rrdpTestObjectURI: []byte("old"), "rsync://repo.example/module/keep.cer": []byte("keep")}}
	extra := "rsync://repo.example/module/new.cer"
	body := rrdpDeltaTestBody("3", rrdpDeltaPublish(rrdpTestObjectURI, []byte("old"), []byte("new"))+rrdpDeltaPublish(extra, nil, []byte("added")))
	ref := rrdpDeltaReference{Serial: "3", File: rrdpFileReference{Hash: sha256.Sum256(body)}}
	next, err := applyRRDPDelta(body, rrdpTestNotificationURI, rrdpTestSession, ref, base)
	if err != nil || next.Serial != "3" || string(next.Objects[rrdpTestObjectURI]) != "new" || string(next.Objects[extra]) != "added" {
		t.Fatalf("valid delta failed: %v", err)
	}
	if string(base.Objects[rrdpTestObjectURI]) != "old" || len(base.Objects) != 2 {
		t.Fatal("base map changed")
	}
	next.Objects["rsync://repo.example/module/keep.cer"][0] = 'X'
	if string(base.Objects["rsync://repo.example/module/keep.cer"]) != "keep" {
		t.Fatal("unchanged objects alias base")
	}
	body = rrdpDeltaTestBody("4", rrdpDeltaWithdraw(extra, []byte("added")))
	ref = rrdpDeltaReference{Serial: "4", File: rrdpFileReference{Hash: sha256.Sum256(body)}}
	removed, err := applyRRDPDelta(body, rrdpTestNotificationURI, rrdpTestSession, ref, next)
	if err != nil || len(removed.Objects) != 2 || len(next.Objects) != 3 {
		t.Fatalf("withdraw failed: %v", err)
	}
}

func TestRPKIRRDPDeltaRejectsInvalid(t *testing.T) {
	base := rrdpRepository{NotificationURI: rrdpTestNotificationURI, Session: rrdpTestSession, Serial: "2", Objects: map[string][]byte{rrdpTestObjectURI: []byte("old")}}
	good := rrdpDeltaPublish("rsync://repo.example/module/new.cer", nil, []byte("added"))
	for _, tt := range []struct{ name, content string }{
		{"empty", ""},
		{"overwrite_without_hash", rrdpDeltaPublish(rrdpTestObjectURI, nil, []byte("new"))},
		{"wrong_old_hash", good + rrdpDeltaPublish(rrdpTestObjectURI, []byte("wrong"), []byte("new"))},
		{"missing_replacement", rrdpDeltaPublish("rsync://repo.example/module/missing.cer", []byte("old"), []byte("new"))},
		{"missing_withdrawal", rrdpDeltaWithdraw("rsync://repo.example/module/missing.cer", []byte("old"))},
		{"withdraw_hash", rrdpDeltaWithdraw(rrdpTestObjectURI, []byte("wrong"))},
		{"withdraw_no_hash", fmt.Sprintf(`<withdraw uri="%s"/>`, rrdpTestObjectURI)},
		{"withdraw_payload", strings.Replace(rrdpDeltaWithdraw(rrdpTestObjectURI, []byte("old")), "/>", ">YQ==</withdraw>", 1)},
		{"bad_hash", fmt.Sprintf(`<publish uri="%s" hash="abc">YQ==</publish>`, rrdpTestObjectURI)},
		{"bad_base64", `<publish uri="rsync://repo.example/module/new.cer">***</publish>`},
		{"unknown_element", `<unknown/>`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := rrdpDeltaTestBody("3", tt.content)
			ref := rrdpDeltaReference{Serial: "3", File: rrdpFileReference{Hash: sha256.Sum256(body)}}
			got, err := applyRRDPDelta(body, rrdpTestNotificationURI, rrdpTestSession, ref, base)
			if err == nil || got.Objects != nil {
				t.Fatal("invalid delta returned partial state")
			}
			if !reflect.DeepEqual(base.Objects, map[string][]byte{rrdpTestObjectURI: []byte("old")}) {
				t.Fatal("failed delta mutated base")
			}
		})
	}
	for _, mode := range []string{"digest", "session", "repository", "serial_gap", "serial_replay"} {
		t.Run(mode, func(t *testing.T) {
			body := rrdpDeltaTestBody("3", good)
			ref := rrdpDeltaReference{Serial: "3", File: rrdpFileReference{Hash: sha256.Sum256(body)}}
			session, uri := rrdpTestSession, rrdpTestNotificationURI
			switch mode {
			case "digest":
				ref.File.Hash[0] ^= 1
			case "session":
				session = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
			case "repository":
				uri = "https://other.example/notification.xml"
			case "serial_gap":
				ref.Serial = "4"
			case "serial_replay":
				ref.Serial = "2"
			}
			if _, err := applyRRDPDelta(body, uri, session, ref, base); err == nil {
				t.Fatal("invalid delta scope accepted")
			}
		})
	}
}

func TestRPKIRRDPDeltaHTTPS(t *testing.T) {
	body := rrdpDeltaTestBody("3", rrdpDeltaPublish(rrdpTestObjectURI, nil, []byte("new")))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()
	client := rrdpHTTPClient{Transport: server.Client().Transport}
	base := rrdpRepository{NotificationURI: rrdpTestNotificationURI, Session: rrdpTestSession, Serial: "2", Objects: map[string][]byte{}}
	ref := rrdpDeltaReference{Serial: "3", File: rrdpFileReference{URI: server.URL, Hash: sha256.Sum256(body)}}
	got, err := client.FetchDelta(context.Background(), rrdpTestNotificationURI, rrdpTestSession, ref, base)
	if err != nil || !bytes.Equal(got.Objects[rrdpTestObjectURI], []byte("new")) {
		t.Fatalf("HTTPS delta failed: %v", err)
	}
}
