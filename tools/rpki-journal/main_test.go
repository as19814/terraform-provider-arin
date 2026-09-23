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

func TestJournalRevocationModes(t *testing.T) {
	for _, mode := range []string{"observe", "load_error", "read_error", "expect", "publication", "directory", "read", "missing"} {
		t.Run(mode, func(t *testing.T) {
			calls := []string{}
			actions := journalActions{
				loadProvisioning: func(path string) (arin.RPKIProvisioningReadConfig, error) {
					calls = append(calls, "load")
					if path != "/private/provisioning.json" {
						t.Fatal("config path changed")
					}
					if mode == "load_error" {
						return arin.RPKIProvisioningReadConfig{}, errors.New("invalid config")
					}
					return arin.RPKIProvisioningReadConfig{Child: "child", Parent: "parent"}, nil
				},
				revocation: func(_ context.Context, config arin.RPKIProvisioningReadConfig, digest string) (*arin.RPKIRevocationRecoveryReport, error) {
					calls = append(calls, "observe")
					if config.Child != "child" || config.Parent != "parent" || digest != "digest" {
						t.Fatal("observation arguments changed")
					}
					if mode == "read_error" {
						return nil, errors.New("read failed")
					}
					return &arin.RPKIRevocationRecoveryReport{Outcome: "key_absent"}, nil
				},
			}
			args := []string{"-provisioning-config", "/private/provisioning.json", "-request-sha256", "digest"}
			switch mode {
			case "expect":
				args = append(args, "-expect", "matches_after")
			case "publication":
				args = append(args, "-publication-config", "/private/publication.json")
			case "directory":
				args = append(args, "-directory", "/private/journal")
			case "read":
				args = append(args, "-recover-read-sha256", "digest")
			case "missing":
				args = args[:2]
			}
			var out, stderr bytes.Buffer
			code := runJournal(args, &out, &stderr, actions)
			switch mode {
			case "observe":
				if code != 0 || strings.Join(calls, ",") != "load,observe" || !strings.Contains(out.String(), `"committed": false`) || !strings.Contains(out.String(), `"outcome": "key_absent"`) {
					t.Fatalf("observation failed: %s", stderr.String())
				}
			case "load_error", "read_error":
				want := "load"
				if mode == "read_error" {
					want = "load,observe"
				}
				if code != 1 || strings.Join(calls, ",") != want || out.Len() != 0 {
					t.Fatal("failed operation produced report")
				}
			default:
				if code != 2 || len(calls) != 0 || out.Len() != 0 {
					t.Fatal("invalid mode performed action")
				}
			}
		})
	}
}

func TestJournalIssuanceModes(t *testing.T) {
	for _, mode := range []string{"observe", "commit", "load_error", "read_error", "missing_provisioning", "missing_validation", "publication", "expect", "read"} {
		t.Run(mode, func(t *testing.T) {
			calls := []string{}
			actions := journalActions{
				loadProvisioning: func(path string) (arin.RPKIProvisioningReadConfig, error) {
					calls = append(calls, "provisioning")
					return arin.RPKIProvisioningReadConfig{Child: "child"}, nil
				},
				loadValidation: func(path string) (arin.RPKICertificateValidation, error) {
					calls = append(calls, "validation")
					if mode == "load_error" {
						return arin.RPKICertificateValidation{}, errors.New("bad validation")
					}
					return arin.RPKICertificateValidation{AnchorPEM: "anchor"}, nil
				},
				issuance: func(_ context.Context, c arin.RPKIProvisioningReadConfig, v arin.RPKICertificateValidation, digest, expected string) (*arin.RPKIIssuanceRecoveryReport, error) {
					calls = append(calls, "issuance")
					want := ""
					if mode == "commit" {
						want = "hash"
					}
					if c.Child != "child" || v.AnchorPEM != "anchor" || digest != "digest" || expected != want {
						t.Fatal("issuance recovery arguments changed")
					}
					if mode == "read_error" {
						return nil, errors.New("recovery failed")
					}
					return &arin.RPKIIssuanceRecoveryReport{Outcome: "matches_request", CertificateSHA256: "hash", Committed: expected != ""}, nil
				},
			}
			args := []string{"-provisioning-config", "/private/provisioning.json", "-issuance-validation", "/private/validation.json", "-request-sha256", "digest"}
			switch mode {
			case "commit":
				args = append(args, "-expect-certificate-sha256", "hash")
			case "missing_provisioning":
				args = args[2:]
			case "missing_validation":
				args = []string{"-provisioning-config", "/private/config.json", "-request-sha256", "digest", "-expect-certificate-sha256", "hash"}
			case "publication":
				args = append(args, "-publication-config", "/private/publication.json")
			case "expect":
				args = append(args, "-expect", "matches_after")
			case "read":
				args = append(args, "-recover-read-sha256", "digest")
			}
			var out, stderr bytes.Buffer
			code := runJournal(args, &out, &stderr, actions)
			switch mode {
			case "observe", "commit":
				want := `"committed": false`
				if mode == "commit" {
					want = `"committed": true`
				}
				if code != 0 || strings.Join(calls, ",") != "provisioning,validation,issuance" || !strings.Contains(out.String(), want) {
					t.Fatalf("bad recovery mode: %s", stderr.String())
				}
			case "load_error", "read_error":
				want := "provisioning,validation"
				if mode == "read_error" {
					want += ",issuance"
				}
				if code != 1 || strings.Join(calls, ",") != want || out.Len() != 0 {
					t.Fatal("failed operation produced result")
				}
			default:
				if code != 2 || len(calls) != 0 || out.Len() != 0 {
					t.Fatal("invalid flags performed an action")
				}
			}
		})
	}
}

func TestJournalProvenRevocationModes(t *testing.T) {
	for _, mode := range []string{"observe", "commit", "load_error", "read_error", "missing_provisioning", "missing_validation", "publication", "expect", "read", "issuance", "expiry_observe", "expiry_commit", "expiry_without_validation", "class_observe", "class_commit", "class_without_validation"} {
		t.Run(mode, func(t *testing.T) {
			calls := []string{}
			actions := journalActions{
				loadProvisioning: func(path string) (arin.RPKIProvisioningReadConfig, error) {
					calls = append(calls, "provisioning")
					return arin.RPKIProvisioningReadConfig{Child: "child"}, nil
				},
				loadRevocationValidation: func(path string) (arin.RPKIRevocationValidation, error) {
					calls = append(calls, "validation")
					if mode == "load_error" {
						return arin.RPKIRevocationValidation{}, errors.New("bad validation")
					}
					return arin.RPKIRevocationValidation{PriorCertificatePEM: "prior"}, nil
				},
				recoverRevocation: func(_ context.Context, c arin.RPKIProvisioningReadConfig, v arin.RPKIRevocationValidation, digest, expected string) (*arin.RPKIRevocationRecoveryReport, error) {
					calls = append(calls, "revocation")
					want := ""
					if mode == "commit" || mode == "expiry_commit" || mode == "class_commit" {
						want = "hash"
					}
					if v.AllowClassAbsent != strings.HasPrefix(mode, "class_") {
						t.Fatal("class opt-in changed")
					}
					if v.AllowExpired != strings.HasPrefix(mode, "expiry_") {
						t.Fatal("expiry opt-in changed")
					}
					if c.Child != "child" || v.PriorCertificatePEM != "prior" || digest != "digest" || expected != want {
						t.Fatal("revocation recovery arguments changed")
					}
					if mode == "read_error" {
						return nil, errors.New("recovery failed")
					}
					return &arin.RPKIRevocationRecoveryReport{Outcome: "key_absent", CertificateSHA256: "hash", Committed: expected != ""}, nil
				},
			}
			args := []string{"-provisioning-config", "/private/provisioning.json", "-revocation-validation", "/private/validation.json", "-request-sha256", "digest"}
			switch mode {
			case "commit", "expiry_commit", "class_commit":
				args = append(args, "-expect-certificate-sha256", "hash")
			case "missing_provisioning":
				args = args[2:]
			case "missing_validation":
				args = []string{"-provisioning-config", "/private/config.json", "-request-sha256", "digest", "-expect-certificate-sha256", "hash"}
			case "issuance":
				args = append(args, "-issuance-validation", "/private/issuance.json")
			case "publication":
				args = append(args, "-publication-config", "/private/publication.json")
			case "expect":
				args = append(args, "-expect", "matches_after")
			case "read":
				args = append(args, "-recover-read-sha256", "digest")
			}
			if strings.HasPrefix(mode, "expiry_") {
				args = append(args, "-accept-expired-certificate")
			}
			if strings.HasPrefix(mode, "class_") {
				args = append(args, "-accept-missing-class")
			}
			if mode == "class_without_validation" {
				args = []string{"-provisioning-config", "/private/config.json", "-request-sha256", "digest", "-accept-missing-class"}
			}
			if mode == "expiry_without_validation" {
				args = []string{"-provisioning-config", "/private/config.json", "-request-sha256", "digest", "-accept-expired-certificate"}
			}
			var out, stderr bytes.Buffer
			code := runJournal(args, &out, &stderr, actions)
			switch mode {
			case "observe", "commit", "expiry_observe", "expiry_commit", "class_observe", "class_commit":
				want := `"committed": false`
				if mode == "commit" || mode == "expiry_commit" || mode == "class_commit" {
					want = `"committed": true`
				}
				if code != 0 || strings.Join(calls, ",") != "provisioning,validation,revocation" || !strings.Contains(out.String(), want) {
					t.Fatalf("bad recovery mode: %s", stderr.String())
				}
			case "load_error", "read_error":
				want := "provisioning,validation"
				if mode == "read_error" {
					want += ",revocation"
				}
				if code != 1 || strings.Join(calls, ",") != want || out.Len() != 0 {
					t.Fatal("failed operation produced result")
				}
			default:
				if code != 2 || len(calls) != 0 || out.Len() != 0 {
					t.Fatal("invalid flags performed an action")
				}
			}
		})
	}
}
