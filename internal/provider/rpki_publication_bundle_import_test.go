package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicationBundleImportManifest(t *testing.T) {
	for _, mode := range []string{"valid", "unknown", "duplicate", "case_alias", "null", "empty", "bad_content", "bad_id", "public", "symlink", "trailing", "nested"} {
		t.Run(mode, func(t *testing.T) {
			manifest := publicationBundleImport{Endpoint: "https://repo.example/publication", Publisher: "publisher", Journal: "/private/journal", KeyFile: "/private/key.pem", Certificate: "ee", Anchor: "local", CRLs: "crl", Peer: "peer", Objects: map[string]string{"rsync://repo.example/module/a.cer": "b2xk"}}
			switch mode {
			case "empty":
				manifest.Objects = nil
			case "bad_content":
				manifest.Objects["rsync://repo.example/module/a.cer"] = "invalid"
			case "bad_id":
				manifest.ID = "wrong"
			}
			raw, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "unknown":
				raw = append([]byte(`{"signing_key_pem":"secret",`), raw[1:]...)
			case "duplicate":
				raw = append([]byte(`{"publisher_handle":"different",`), raw[1:]...)
			case "case_alias":
				raw = append([]byte(`{"Endpoint":"shadow",`), raw[1:]...)
			case "null":
				raw = []byte("null")
			case "trailing":
				raw = append(raw, []byte(" {}")...)
			case "nested":
				raw = []byte(strings.Repeat("[", 30) + "0" + strings.Repeat("]", 30))
			}
			path := filepath.Join(t.TempDir(), "import.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if mode == "public" {
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "symlink" {
				if err := os.Symlink(path, path+".link"); err != nil {
					t.Fatal(err)
				}
				path += ".link"
			}
			got, err := readPublicationBundleImport(path)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("unexpected import validation: %v", err)
			}
			if err == nil && got.Objects["rsync://repo.example/module/a.cer"] != "b2xk" {
				t.Fatal("object content lost")
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("manifest content leaked")
			}
		})
	}
	if _, err := readPublicationBundleImport("relative.json"); err == nil {
		t.Fatal("relative import path accepted")
	}
}
