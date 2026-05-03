package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

var (
	errPruneFlagsInvalid = errors.New("prune requires -junit-dir and -total>0")
	junitFileRE          = regexp.MustCompile(`^junit-(\d+)-(\d+)\.xml$`)
)

func cmdPrune(_ context.Context, argv []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(stderr)
	junitDir := fs.String("junit-dir", "", "directory of JUnit XML files to prune (required)")
	total := fs.Int("total", 0, "current total number of shards (required)")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	if *junitDir == "" || *total <= 0 {
		return errPruneFlagsInvalid
	}

	entries, err := os.ReadDir(*junitDir)
	if err != nil {
		return err
	}

	var deleted int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := junitFileRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		shard, _ := strconv.Atoi(m[1])
		if shard < *total {
			continue
		}
		path := filepath.Join(*junitDir, e.Name())
		if err := os.Remove(path); err != nil {
			fmt.Fprintf(stderr, "warning: failed to remove %s: %v\n", e.Name(), err)
			continue
		}
		fmt.Fprintf(stdout, "pruned %s (shard %d >= total %d)\n", e.Name(), shard, *total)
		deleted++
	}

	if deleted == 0 {
		fmt.Fprintln(stderr, "prune: nothing to delete")
	} else {
		fmt.Fprintf(stderr, "prune: deleted %d stale file(s)\n", deleted)
	}
	return nil
}
