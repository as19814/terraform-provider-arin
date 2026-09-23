// Command rpki-journal inspects exchange metadata or explicitly abandons one
// interrupted inventory request. It never clears pending mutations or locks.
package main

import (
	"flag"
	"fmt"
	"github.com/as19814/terraform-provider-arin/internal/arin"
	"os"
)

func main() {
	directory := flag.String("directory", "", "Existing private exchange journal directory")
	peer := flag.String("peer", "", "Exact peer ID from the journal filename")
	digest := flag.String("recover-read-sha256", "", "Explicitly abandon this pending inventory request digest")
	flag.Parse()
	if flag.NArg() != 0 || *directory == "" || *peer == "" {
		flag.Usage()
		os.Exit(2)
	}
	var result []byte
	var err error
	if *digest == "" {
		result, err = arin.InspectRPKIJournal(*directory, *peer)
	} else {
		result, err = arin.RecoverRPKIRead(*directory, *peer, *digest)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(result))
}
