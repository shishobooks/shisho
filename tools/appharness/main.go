// Command appharness runs the Shisho app's end-to-end flows (ADR 0021):
// Maestro drives the app through its accessibility identifiers against a
// Shisho server seeded with small fixtures made for the run.
//
//	go run ./tools/appharness ios [-app Shisho.app] [-flows dir] [-keep]
//	go run ./tools/appharness signoff
//	go run ./tools/appharness serve
//	go run ./tools/appharness base
//	go run ./tools/appharness fixtures DIR | -key
//	go run ./tools/appharness cleanup
//	go run ./tools/appharness simulator [-app Shisho.app] -- COMMAND...
//
// `ios` builds and seeds a throwaway server, installs the app on the
// worktree's simulator for flows, runs every flow, and removes the server's
// files afterwards. `serve` runs the same seeded server until interrupted,
// for QA without touching real data (`mise start:qa`). `simulator` runs a
// command against the worktree's simulator for tests, its UDID in
// SIMULATOR_UDID. Each worktree's simulators are its own, cloned from a base
// once and kept booted between runs (worktreesim.go), and the fixtures are
// cached between runs (fixtures.go); SHISHO_HARNESS_FRESH=1 reuses neither.
// `signoff` runs every flow on a committed checkout and posts the commit
// status this repository's pull requests merge on (signoff.go).
// `fixtures -key` prints the cache's key, and `cleanup` deletes the
// worktree's simulators (`mise cleanup`). `ios`, `serve`, and `cleanup`
// remove the temp folders of runs that were killed outright (rundir.go).
// Use the mise tasks (`mise ios:harness`, `mise ios:test`), which build what
// these need first.
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
	case "signoff":
		err = runSignoff(ctx, os.Args[2:])
	case "serve":
		err = runServe(ctx)
	case "base":
		// Makes the base simulator ahead of a run, so its first boot can
		// happen while the app builds (`mise ios:simulator`). On one of GitHub's
		// runners, whose runs use disposable clones, it also boots the run's
		// clone.
		_, err = baseSimulator(ctx)
		if err == nil && disposableRunner() && !freshRun() {
			prepareClone(ctx)
		}
	case "fixtures":
		if len(os.Args) != 3 {
			usage()
		}
		var root string
		if root, err = os.Getwd(); err != nil {
			break
		}
		if os.Args[2] == "-key" {
			// What CI caches the fixtures by, so its cache matches ours.
			var key string
			if key, err = fixtureKey(root); err == nil {
				fmt.Println(key)
			}
		} else {
			err = writeFixtures(ctx, root, os.Args[2])
		}
	case "cleanup":
		var root string
		if root, err = os.Getwd(); err == nil {
			err = runCleanup(ctx, root)
		}
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
	fmt.Fprintln(os.Stderr, "usage: appharness ios [flags] | signoff | serve | base | fixtures DIR | fixtures -key | cleanup | simulator [-app Shisho.app] -- COMMAND...")
	flag.PrintDefaults()
	os.Exit(2)
}
