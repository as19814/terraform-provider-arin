package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManifestHistoryCommand(t *testing.T) {
	dir := t.TempDir()
	old, next := filepath.Join(dir, "old.pem"), filepath.Join(dir, "new.pem")
	for path, value := range map[string]string{old: "previous", next: "replacement"} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	base := []string{"-directory", dir, "-previous-anchor", old, "-replacement-anchor", next}
	for _, mode := range []string{"missing_opt_in", "success", "failed", "missing_file", "extra_argument", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			args := append([]string(nil), base...)
			if mode != "missing_opt_in" {
				args = append(args, "-migrate")
			}
			if mode == "missing_file" {
				args[3] = filepath.Join(dir, "absent")
			}
			if mode == "extra_argument" {
				args = append(args, "extra")
			}
			if mode == "oversized" {
				p := filepath.Join(dir, "large")
				if err := os.WriteFile(p, make([]byte, (1<<20)+1), 0600); err != nil {
					t.Fatal(err)
				}
				args[3] = p
			}
			calls := 0
			var out, stderr bytes.Buffer
			code := run(args, &out, &stderr, func(d, a, b string) error {
				calls++
				if d != dir || a != "previous" || b != "replacement" {
					t.Fatal("incorrect migration inputs")
				}
				if mode == "failed" {
					return errors.New("conflicting history")
				}
				return nil
			})
			if mode == "success" {
				if code != 0 || calls != 1 || out.Len() == 0 {
					t.Fatal("migration failed")
				}
			} else {
				if code == 0 || out.Len() != 0 {
					t.Fatal("failure reported success")
				}
				want := 0
				if mode == "failed" {
					want = 1
				}
				if calls != want {
					t.Fatal("unexpected migration call")
				}
			}
		})
	}
}
