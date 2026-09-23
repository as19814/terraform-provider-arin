package arin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func rrdpNotificationTestBody(serial, children string) []byte {
	return []byte(fmt.Sprintf(`<notification xmlns="%s" version="1" session_id="%s" serial="%s">%s</notification>`, rrdpNamespace, rrdpTestSession, serial, children))
}
func rrdpTestReference(kind, serial string) string {
	attr := ""
	if serial != "" {
		attr = ` serial="` + serial + `"`
	}
	return fmt.Sprintf(`<%s%s uri="https://repo.example/%s/%s.xml" hash="%s"/>`, kind, attr, serial, kind, strings.Repeat("AB", 32))
}

func TestRPKIRRDPNotification(t *testing.T) {
	snapshot := rrdpTestReference("snapshot", "")
	for _, body := range [][]byte{rrdpNotificationTestBody("3", snapshot), rrdpNotificationTestBody("3", snapshot+rrdpTestReference("delta", "3")+rrdpTestReference("delta", "2")), append([]byte(`<?xml version="1.0" encoding="US-ASCII"?>`), rrdpNotificationTestBody("3", snapshot)...)} {
		n, err := parseRRDPNotification(body)
		if err != nil {
			t.Fatal(err)
		}
		if n.Session != rrdpTestSession || n.Serial != "3" || n.Snapshot.Hash[0] != 0xab {
			t.Fatal("metadata changed")
		}
		if len(n.Deltas) > 0 && (n.Deltas[0].Serial != "2" || n.Deltas[1].Serial != "3") {
			t.Fatal("deltas not sorted")
		}
	}
	// Serial arithmetic must not truncate to uint64.
	low, high := "18446744073709551615", "18446744073709551616"
	if _, err := parseRRDPNotification(rrdpNotificationTestBody(high, snapshot+rrdpTestReference("delta", high)+rrdpTestReference("delta", low))); err != nil {
		t.Fatal(err)
	}
	// Feed snapshot bytes through the exact reference selected by the notification.
	body := rrdpSnapshotTestBody(`<publish uri="rsync://repo.example/module/a.cer">YWJj</publish>`)
	sum := sha256.Sum256(body)
	reference := strings.Replace(snapshot, strings.Repeat("AB", 32), hex.EncodeToString(sum[:]), 1)
	n, err := parseRRDPNotification(rrdpNotificationTestBody("3", reference))
	if err != nil {
		t.Fatal(err)
	}
	objects, err := parseRRDPSnapshot(body, n.Session, n.Serial, n.Snapshot.Hash)
	if err != nil || string(objects["rsync://repo.example/module/a.cer"]) != "abc" {
		t.Fatalf("snapshot reference failed: %v", err)
	}
}

func TestRPKIRRDPNotificationRejectsInvalid(t *testing.T) {
	snapshot := rrdpTestReference("snapshot", "")
	for _, tt := range []struct{ name, children string }{
		{"missing", ""},
		{"duplicate_snapshot", snapshot + snapshot},
		{"delta_before_snapshot", rrdpTestReference("delta", "3") + snapshot},
		{"duplicate_serial", snapshot + rrdpTestReference("delta", "3") + rrdpTestReference("delta", "3")},
		{"gap", snapshot + rrdpTestReference("delta", "1") + rrdpTestReference("delta", "3")},
		{"missing_latest", snapshot + rrdpTestReference("delta", "2")},
		{"future_delta", snapshot + rrdpTestReference("delta", "4")},
		{"bad_hash", strings.Replace(snapshot, strings.Repeat("AB", 32), "abc", 1)},
		{"http", strings.Replace(snapshot, "https:", "http:", 1)},
		{"relative", strings.Replace(snapshot, "https://repo.example/", "/", 1)},
		{"userinfo", strings.Replace(snapshot, "repo.example", "user@repo.example", 1)},
		{"fragment", strings.Replace(snapshot, "snapshot.xml", "snapshot.xml#x", 1)},
		{"unknown_attribute", strings.Replace(snapshot, " hash=", ` tag="a" hash=`, 1)},
		{"duplicate_attribute", strings.Replace(snapshot, " hash=", ` uri="https://other.example/a" hash=`, 1)},
		{"nested", strings.Replace(snapshot, "/>", "><nested/></snapshot>", 1)},
		{"wrong_namespace", strings.Replace(snapshot, "<snapshot ", `<snapshot xmlns="other" `, 1)},
		{"text", "extra" + snapshot},
		{"entity_space", "&#160;" + snapshot},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseRRDPNotification(rrdpNotificationTestBody("3", tt.children)); err == nil {
				t.Fatal("invalid notification accepted")
			}
		})
	}
	for _, serial := range []string{"0", "-1", "01", "+1", strings.Repeat("1", 129)} {
		if _, err := parseRRDPNotification(rrdpNotificationTestBody(serial, snapshot)); err == nil {
			t.Fatal("invalid serial accepted")
		}
	}
	base := rrdpNotificationTestBody("3", snapshot)
	for _, body := range [][]byte{nil, base[:len(base)-1], append(append([]byte(nil), base...), base...), append([]byte(`<!DOCTYPE notification>`), base...), []byte(strings.Replace(string(base), rrdpTestSession, "not-a-uuid", 1)), []byte(strings.Replace(string(base), `version="1"`, `version="2"`, 1)), make([]byte, (4<<20)+1)} {
		if _, err := parseRRDPNotification(body); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
	var many strings.Builder
	many.WriteString(snapshot)
	for i := 1; i <= 10001; i++ {
		many.WriteString(rrdpTestReference("delta", fmt.Sprint(i)))
	}
	if _, err := parseRRDPNotification(rrdpNotificationTestBody("10001", many.String())); err == nil {
		t.Fatal("delta limit ignored")
	}
}
