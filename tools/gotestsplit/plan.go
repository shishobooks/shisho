package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

func cmdPlan(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	junitDir := fs.String("junit-dir", "", "directory of JUnit XML files (required)")
	total := fs.Int("total", 0, "total number of shards (required)")
	index := fs.Int("index", 0, "zero-based shard index")
	detail := fs.Bool("detail", false, "show all shards' assignments, not just -index")
	noDiscover := fs.Bool("no-discover", false, "(test-only) build packages from JUnit history instead of running `go test -list`")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *junitDir == "" || *total <= 0 || *index < 0 || *index >= *total {
		return fmt.Errorf("plan requires -junit-dir, -total>0, 0<=-index<-total")
	}
	pkgPatterns := fs.Args()

	hist, err := ReadHistory(*junitDir)
	if err != nil {
		return err
	}
	var pkgs []Package
	if *noDiscover {
		pkgs = packagesFromHistory(hist)
	} else {
		if len(pkgPatterns) == 0 {
			return fmt.Errorf("plan requires at least one package pattern")
		}
		pkgs, err = DiscoverPackages(ctx, pkgPatterns, nil)
		if err != nil {
			return err
		}
	}
	shards := Pack(hist, pkgs, *total)
	if *detail {
		for i, s := range shards {
			printShard(stdout, i, *total, s)
		}
		return nil
	}
	printShard(stdout, *index, *total, shards[*index])
	return nil
}

func printShard(w io.Writer, i, total int, items []Item) {
	var load float64
	for _, it := range items {
		load += it.Duration
	}
	fmt.Fprintf(w, "shard %d/%d  load=%.1fs (%.1fm)  items=%d\n",
		i+1, total, load, load/60, len(items))
	for _, it := range items {
		if len(it.Tests) == 0 {
			fmt.Fprintf(w, "  %-60s  whole pkg  (%.1fs)\n", it.Pkg, it.Duration)
		} else {
			fmt.Fprintf(w, "  %-60s  %d tests   (%.1fs)\n", it.Pkg, len(it.Tests), it.Duration)
		}
	}
}
