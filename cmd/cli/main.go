// Command fullwa-cli is the admin CLI for the fullWA platform.
//
// Subcommands (added incrementally through phases):
//
//	migrate    — apply or roll back schema migrations
//	seed       — seed dev-mode fixtures
//	tenant     — create / list / suspend organizations
//	user       — invite / disable platform users
//	key        — rotate the credential KEK
//
// Phase 0 ships the skeleton; individual subcommands land in later phases.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "migrate", "seed", "tenant", "user", "key":
		fmt.Fprintf(os.Stderr, "subcommand %q is not implemented yet (Phase 0 skeleton).\n", os.Args[1])
		os.Exit(1)
	case "-h", "--help", "help":
		usage()
	case "version":
		fmt.Println("fullwa-cli 0.0.0-dev")
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	_ = flag.CommandLine
}

func usage() {
	fmt.Fprintln(os.Stderr, `fullwa-cli — fullWA admin CLI

usage:
  fullwa-cli <subcommand> [flags]

subcommands:
  migrate    apply or roll back schema migrations
  seed       seed dev-mode fixtures
  tenant     create / list / suspend organizations
  user       invite / disable platform users
  key        rotate the credential KEK
  version    print version
  help       show this message`)
}
