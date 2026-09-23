package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/as19814/terraform-provider-arin/internal/arin"
)

func TestJournalCommandModes(t *testing.T) {
	for _, mode := range []string{"inspect", "read", "observe", "before", "after", "failure", "conflict", "missing", "invalid_expected", "positional"} {
		t.Run(mode, func(t *testing.T) {
			calls := []string{}
			actions := journalActions{
				inspect: func(d, p string) ([]byte, error) {
					calls = append(calls, "inspect")
					return []byte(`{"pending":true}`), nil
				},
				recoverRead: func(d, p, h string) ([]byte, error) {
					calls = append(calls, "read")
					return []byte(`{"recovered":true}`), nil
				},
				load: func(path string) (arin.RPKIPublicationReadConfig, error) {
					calls = append(calls, "load")
					if mode == "failure" {
						return arin.RPKIPublicationReadConfig{}, errors.New("invalid config")
					}
					return arin.RPKIPublicationReadConfig{Publisher: "publisher"}, nil
				},
				publication: func(_ context.Context, c arin.RPKIPublicationReadConfig, digest, expected string) (*arin.RPKIPublicationRecoveryReport, error) {
					calls = append(calls, "publication")
					want := ""
					if mode == "before" {
						want = "matches_before"
					}
					if mode == "after" {
						want = "matches_after"
					}
					if c.Publisher != "publisher" || digest != "digest" || expected != want {
						t.Fatal("recovery arguments changed")
					}
					return &arin.RPKIPublicationRecoveryReport{Outcome: "matches_after", Committed: expected != ""}, nil
				},
			}
			args := []string{"-publication-config", "/private/config.json", "-request-sha256", "digest"}
			switch mode {
			case "inspect":
				args = []string{"-directory", "/private/journal", "-peer", "peer"}
			case "read":
				args = []string{"-directory", "/private/journal", "-peer", "peer", "-recover-read-sha256", "digest"}
			case "before":
				args = append(args, "-expect", "matches_before")
			case "after":
				args = append(args, "-expect", "matches_after")
			case "conflict":
				args = append(args, "-directory", "/private/journal")
			case "missing":
				args = []string{"-publication-config", "/private/config.json"}
			case "invalid_expected":
				args = append(args, "-expect", "force")
			case "positional":
				args = append(args, "unexpected")
			}
			var out, stderr bytes.Buffer
			code := runJournal(args, &out, &stderr, actions)
			invalid := mode == "conflict" || mode == "missing" || mode == "invalid_expected" || mode == "positional"
			if invalid {
				if code != 2 || len(calls) != 0 || out.Len() != 0 {
					t.Fatal("invalid command performed an action")
				}
				return
			}
			if mode == "failure" {
				if code != 1 || strings.Join(calls, ",") != "load" || out.Len() != 0 {
					t.Fatal("failed configuration reached recovery")
				}
				return
			}
			if code != 0 {
				t.Fatalf("command failed: %s", stderr.String())
			}
			if mode == "observe" && !strings.Contains(out.String(), `"committed": false`) {
				t.Fatal("observation committed recovery")
			}
			if (mode == "after" || mode == "before") && !strings.Contains(out.String(), `"committed": true`) {
				t.Fatal("explicit recovery not reported")
			}
		})
	}
}
