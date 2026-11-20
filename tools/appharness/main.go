// Command appharness runs the Shisho app's end-to-end flows (ADR 0021):
// Maestro drives the app through its accessibility identifiers against a
// Shisho server seeded with small fixtures made for the run.
//
//	go run ./tools/appharness ios [-app Shisho.app] [-flows dir] [-keep]
//	go run ./tools/appharness fixtures DIR
//	go run ./tools/appharness simulator -- COMMAND...
//
// `ios` builds and seeds a throwaway server, clones a base iOS simulator,
// installs the app, runs every flow, and removes the clone and the server's
// files afterwards. `simulator` runs a command against a disposable
// simulator clone, its UDID in SIMULATOR_UDID, so several worktrees can run
// tests at once. Use the mise tasks (`mise ios:harness`, `mise ios:test`),
// which build what these need first.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "ios":
		err = runIOS(ctx, os.Args[2:])
	case "fixtures":
		if len(os.Args) != 3 {
			usage()
		}
		err = writeFixtures(ctx, os.Args[2])
	case "simulator":
		err = runOnSimulator(ctx, os.Args[2:])
	default:
		usage()
	}
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "appharness:", err)
		os.Exit(1) //nolint:gocritic // stop already ran.
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: appharness ios [flags] | fixtures DIR | simulator -- COMMAND...")
	flag.PrintDefaults()
	os.Exit(2)
}
