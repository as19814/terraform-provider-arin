package arin

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

const rrdpTestSession = "9df4b597-af9e-4dca-bdda-719cce2c4e28"

func rrdpSnapshotTestBody(content string) []byte {
	return []byte(fmt.Sprintf(`<snapshot xmlns="%s" version="1" session_id="%s" serial="3">%s</snapshot>`, rrdpNamespace, rrdpTestSession, content))
}

func TestRPKIRRDPSnapshot(t *testing.T) {
	publish := `<publish uri="rsync://repo.example/module/a.cer">Y W\nJj</publish>`
	publish = strings.ReplaceAll(publish, `\n`, "\n")
	for _, prefix := range []string{"", `<?xml version="1.0" encoding="US-ASCII"?>`} {
		body := append([]byte(prefix), rrdpSnapshotTestBody(publish)...)
		got, err := parseRRDPSnapshot(body, rrdpTestSession, "3", sha256.Sum256(body))
		if err != nil || !bytes.Equal(got["rsync://repo.example/module/a.cer"], []byte("abc")) {
			t.Fatalf("valid snapshot failed: %v", err)
		}
		body[len(body)/2] ^= 1
		if !bytes.Equal(got["rsync://repo.example/module/a.cer"], []byte("abc")) {
			t.Fatal("objects alias snapshot input")
		}
	}
	empty := rrdpSnapshotTestBody("")
	got, err := parseRRDPSnapshot(empty, rrdpTestSession, "3", sha256.Sum256(empty))
	if err != nil || len(got) != 0 {
		t.Fatal("empty snapshot rejected")
	}
}

func TestRPKIRRDPSnapshotRejectsInvalid(t *testing.T) {
	valid := `<publish uri="rsync://repo.example/module/a.cer">YWJj</publish>`
	for _, tt := range []struct{ name, content string }{
		{"duplicate", valid + valid},
		{"withdraw", `<withdraw uri="rsync://repo.example/module/a.cer"/>`},
		{"publish_hash", `<publish uri="rsync://repo.example/module/a.cer" hash="ab">YWJj</publish>`},
		{"publish_tag", `<publish uri="rsync://repo.example/module/a.cer" tag="a">YWJj</publish>`},
		{"nested", `<publish uri="rsync://repo.example/module/a.cer"><x/></publish>`},
		{"namespace", `<publish xmlns="other" uri="rsync://repo.example/module/a.cer">YWJj</publish>`},
		{"missing_uri", `<publish>YWJj</publish>`},
		{"http_uri", `<publish uri="https://repo.example/a.cer">YWJj</publish>`},
		{"directory_uri", `<publish uri="rsync://repo.example/module/">YWJj</publish>`},
		{"bad_base64", `<publish uri="rsync://repo.example/module/a.cer">***</publish>`},
		{"pad_bits", `<publish uri="rsync://repo.example/module/a.cer">YR==</publish>`},
		{"empty_object", `<publish uri="rsync://repo.example/module/a.cer"/>`},
		{"extra_text", "not whitespace" + valid},
		{"entity_space", "&#160;" + valid},
		{"non_ascii", "é" + valid},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := rrdpSnapshotTestBody(tt.content)
			if _, err := parseRRDPSnapshot(body, rrdpTestSession, "3", sha256.Sum256(body)); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
	for _, mode := range []string{"hash", "session", "serial", "version", "duplicate_attribute", "doctype", "second_root", "truncated", "bad_uuid", "zero_serial"} {
		t.Run(mode, func(t *testing.T) {
			body := rrdpSnapshotTestBody(valid)
			session, serial := rrdpTestSession, "3"
			switch mode {
			case "session":
				session = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
			case "serial":
				serial = "4"
			case "version":
				body = bytes.Replace(body, []byte(`version="1"`), []byte(`version="2"`), 1)
			case "duplicate_attribute":
				body = bytes.Replace(body, []byte(`version="1"`), []byte(`version="1" version="1"`), 1)
			case "doctype":
				body = append([]byte(`<!DOCTYPE snapshot>`), body...)
			case "second_root":
				body = append(body, rrdpSnapshotTestBody("")...)
			case "truncated":
				body = body[:len(body)-1]
			case "bad_uuid":
				session = "not-a-uuid"
			case "zero_serial":
				serial = "0"
			}
			digest := sha256.Sum256(body)
			if mode == "hash" {
				digest[0] ^= 1
			}
			if _, err := parseRRDPSnapshot(body, session, serial, digest); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
}

func TestRPKIRRDPSnapshotBounds(t *testing.T) {
	for _, size := range []int{(4 << 20) - 1, 4 << 20, (4 << 20) + 1} {
		data := make([]byte, size)
		body := rrdpSnapshotTestBody(`<publish uri="rsync://repo.example/module/a.cer">` + base64.StdEncoding.EncodeToString(data) + `</publish>`)
		got, err := parseRRDPSnapshot(body, rrdpTestSession, "3", sha256.Sum256(body))
		if (err == nil) != (size <= 4<<20) {
			t.Fatalf("size=%d err=%v", size, err)
		}
		if err == nil && len(got["rsync://repo.example/module/a.cer"]) != size {
			t.Fatal("object truncated")
		}
	}
	var many strings.Builder
	for i := 0; i < 10001; i++ {
		fmt.Fprintf(&many, `<publish uri="rsync://repo.example/module/%d.cer">YQ==</publish>`, i)
	}
	body := rrdpSnapshotTestBody(many.String())
	if _, err := parseRRDPSnapshot(body, rrdpTestSession, "3", sha256.Sum256(body)); err == nil {
		t.Fatal("object count limit ignored")
	}
}
