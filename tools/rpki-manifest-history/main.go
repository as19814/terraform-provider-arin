// Command rpki-manifest-history explicitly carries rollback history between
// independently trusted resource anchors. It performs no network requests.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/as19814/terraform-provider-arin/internal/arin"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, arin.MigrateRPKIManifestHistory)) }

func run(args []string, out, stderr io.Writer, migrate func(string, string, string) error) int {
	flags := flag.NewFlagSet("rpki-manifest-history", flag.ContinueOnError)
	flags.SetOutput(stderr)
	directory := flags.String("directory", "", "Existing private manifest-history directory")
	previous := flags.String("previous-anchor", "", "PEM file for the previously trusted resource anchor")
	replacement := flags.String("replacement-anchor", "", "PEM file for the independently trusted replacement resource anchor")
	apply := flags.Bool("migrate", false, "Explicitly merge rollback watermarks into the replacement anchor scope, retaining the source")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *directory == "" || *previous == "" || *replacement == "" || !*apply {
		flags.Usage()
		return 2
	}
	old, err := readAnchor(*previous)
	if err != nil {
		fmt.Fprintln(stderr, "Cannot read previous resource anchor:", err)
		return 1
	}
	next, err := readAnchor(*replacement)
	if err != nil {
		fmt.Fprintln(stderr, "Cannot read replacement resource anchor:", err)
		return 1
	}
	if err := migrate(*directory, old, next); err != nil {
		fmt.Fprintln(stderr, "Manifest-history migration failed:", err)
		return 1
	}
	fmt.Fprintln(out, "Manifest history migrated; source retained. Configure the independently trusted replacement anchor before resuming validation.")
	return 0
}

func readAnchor(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return "", fmt.Errorf("anchor must be a regular PEM file no larger than 1 MiB")
	}
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return "", err
	}
	if len(b) > 1<<20 {
		return "", fmt.Errorf("anchor exceeds 1 MiB")
	}
	return string(b), nil
}
