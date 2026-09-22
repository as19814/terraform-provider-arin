package main

import (
	"context"
	"flag"
	"log"

	"github.com/as19814/terraform-provider-arin/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// version is set at build time with -ldflags.
var version = "dev"

//go:generate go tool tfplugindocs generate --provider-name arin

func main() {
	debug := flag.Bool("debug", false, "Run with debugger attachment support")
	flag.Parse()
	if err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/as19814/arin", ProtocolVersion: 6, Debug: *debug,
	}); err != nil {
		log.Fatal(err)
	}
}
