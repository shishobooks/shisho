// Package main is gotestsplit, a timing-aware fork of gotesplit
// (https://github.com/Songmu/gotesplit, MIT). It splits Go tests across CI
// shards based on measured per-test duration rather than test count.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	if len(argv) == 0 {
		printUsage(stderr)
		return flag.ErrHelp
	}
	switch argv[0] {
	case "simulate":
		return cmdSimulate(ctx, argv[1:], stdout, stderr)
	case "plan":
		return cmdPlan(ctx, argv[1:], stdout, stderr)
	case "run":
		return cmdRun(ctx, argv[1:], stdout, stderr)
	case "prune":
		return cmdPrune(ctx, argv[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return nil
	default:
		// Bare `gotestsplit -total=N -index=I ...` is a shortcut for `run`.
		return cmdRun(ctx, argv, stdout, stderr)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `gotestsplit — timing-aware Go test splitter

Usage:
  gotestsplit simulate -junit-dir=<dir> [-min=N] [-max=N]
  gotestsplit plan -junit-dir=<dir> -total=N -index=I [-detail] <pkg>...
  gotestsplit run -junit-dir=<dir> [-junit-out=<dir>] -total=N -index=I <pkg>... [-- go-test-args]
  gotestsplit prune -junit-dir=<dir> -total=N
  gotestsplit [-junit-out=<dir>] -total=N -index=I <pkg>... [-- go-test-args]   # shortcut for "run"`)
}
