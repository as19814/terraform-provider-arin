// Command rpki-journal inspects journals, abandons exact pending inventory reads,
// observes pending revocations, or explicitly reconciles a publication batch.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/as19814/terraform-provider-arin/internal/arin"
)

type journalActions struct {
	loadProvisioning func(string) (arin.RPKIProvisioningReadConfig, error)
	revocation       func(context.Context, arin.RPKIProvisioningReadConfig, string) (*arin.RPKIRevocationRecoveryReport, error)
	inspect          func(string, string) ([]byte, error)
	recoverRead      func(string, string, string) ([]byte, error)
	load             func(string) (arin.RPKIPublicationReadConfig, error)
	publication      func(context.Context, arin.RPKIPublicationReadConfig, string, string) (*arin.RPKIPublicationRecoveryReport, error)
}

func main() {
	os.Exit(runJournal(os.Args[1:], os.Stdout, os.Stderr, journalActions{loadProvisioning: arin.LoadRPKIProvisioningConfig, revocation: arin.ObserveRPKIRevocation, inspect: arin.InspectRPKIJournal, recoverRead: arin.RecoverRPKIRead, load: arin.LoadRPKIPublicationConfig, publication: arin.RecoverRPKIPublication}))
}
func runJournal(args []string, out, stderr io.Writer, actions journalActions) int {
	flags := flag.NewFlagSet("rpki-journal", flag.ContinueOnError)
	flags.SetOutput(stderr)
	directory := flags.String("directory", "", "Existing private exchange journal directory")
	peer := flags.String("peer", "", "Exact peer ID from the journal filename")
	readDigest := flags.String("recover-read-sha256", "", "Explicitly abandon this pending inventory request digest")
	configFile := flags.String("publication-config", "", "Absolute path to private BPKI JSON configuration for publication recovery")
	provisioningFile := flags.String("provisioning-config", "", "Absolute path to private BPKI JSON configuration for revocation observation")
	digest := flags.String("request-sha256", "", "Exact pending publication or revocation mutation digest")
	expected := flags.String("expect", "", "Commit only matches_before or matches_after; omit to observe without clearing")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	invalid := flags.NArg() != 0
	if *provisioningFile != "" {
		invalid = invalid || *configFile != "" || *directory != "" || *peer != "" || *readDigest != "" || *digest == "" || *expected != ""
	} else if *configFile != "" {
		invalid = invalid || *directory != "" || *peer != "" || *readDigest != "" || *digest == "" || (*expected != "" && *expected != "matches_before" && *expected != "matches_after")
	} else {
		invalid = invalid || *directory == "" || *peer == "" || *digest != "" || *expected != ""
	}
	if invalid {
		flags.Usage()
		return 2
	}
	var result []byte
	var err error
	if *provisioningFile != "" {
		var config arin.RPKIProvisioningReadConfig
		config, err = actions.loadProvisioning(*provisioningFile)
		if err == nil {
			var report *arin.RPKIRevocationRecoveryReport
			report, err = actions.revocation(context.Background(), config, *digest)
			if err == nil {
				result, err = json.MarshalIndent(report, "", "  ")
			}
		}
	} else if *configFile != "" {
		var config arin.RPKIPublicationReadConfig
		config, err = actions.load(*configFile)
		if err == nil {
			var report *arin.RPKIPublicationRecoveryReport
			report, err = actions.publication(context.Background(), config, *digest, *expected)
			if err == nil {
				result, err = json.MarshalIndent(report, "", "  ")
			}
		}
	} else if *readDigest != "" {
		result, err = actions.recoverRead(*directory, *peer, *readDigest)
	} else {
		result, err = actions.inspect(*directory, *peer)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(out, string(result))
	return 0
}
