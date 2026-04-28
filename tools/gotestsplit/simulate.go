package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
)

func cmdSimulate(ctx context.Context, argv []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("simulate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	junitDir := fs.String("junit-dir", "", "directory of JUnit XML files from prior runs (required)")
	min := fs.Int("min", 2, "minimum N to project")
	max := fs.Int("max", 10, "maximum N to project")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *junitDir == "" {
		return fmt.Errorf("-junit-dir is required")
	}
	if *min < 1 || *max < *min {
		return fmt.Errorf("invalid range: min=%d max=%d", *min, *max)
	}
	hist, err := ReadHistory(*junitDir)
	if err != nil {
		return err
	}
	if len(hist) == 0 {
		return fmt.Errorf("no JUnit data found in %s", *junitDir)
	}
	pkgs := packagesFromHistory(hist)
	fmt.Fprintf(stdout, "%-5s  %-13s  %s\n", "N", "slowest", "cost")
	for n := *min; n <= *max; n++ {
		shards := Pack(hist, pkgs, n)
		var slowest float64
		for _, s := range shards {
			var load float64
			for _, it := range s {
				load += it.Duration
			}
			if load > slowest {
				slowest = load
			}
		}
		cost := slowest * float64(n)
		fmt.Fprintf(stdout, "N=%-3d  slowest=%5.1fm  cost=%5.1f min\n",
			n, slowest/60, cost/60)
	}
	return nil
}

// packagesFromHistory builds Package list using only what's in JUnit history
// (simulate does not invoke `go test -list`).
func packagesFromHistory(hist History) []Package {
	pkgs := make([]Package, 0, len(hist))
	for path, tests := range hist {
		p := Package{Path: path}
		for name := range tests {
			p.Tests = append(p.Tests, name)
		}
		sort.Strings(p.Tests)
		pkgs = append(pkgs, p)
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path < pkgs[j].Path })
	return pkgs
}
