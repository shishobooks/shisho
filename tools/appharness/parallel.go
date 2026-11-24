package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

// maxParallelShards caps `ios -parallel`: each shard boots a simulator of
// its own, which holds 1 to 3 GB, and runs a server and Maestro beside it.
const maxParallelShards = 2

// flowsPurpose names the worktree simulator a run's shard uses: the
// worktree's flows simulator for the first, and one more of its own for
// each other shard run at once (worktreePurposes lists them all).
func flowsPurpose(shard int) string {
	if shard <= 1 {
		return "flows"
	}
	return "flows-" + strconv.Itoa(shard)
}

// parallelShards returns the shard ("i/n", or "" for every flow) each
// Maestro run takes: one run for a plain or `-shard` run, or `-parallel`'s
// split.
func parallelShards(parallel int, shard string) ([]string, error) {
	switch {
	case parallel < 1 || parallel > maxParallelShards:
		return nil, errors.Errorf("-parallel %d: want 1 to %d", parallel, maxParallelShards)
	case parallel == 1:
		return []string{shard}, nil
	case shard != "":
		return nil, errors.New("-parallel splits the flows itself, so it takes no -shard")
	}
	shards := make([]string, parallel)
	for i := range shards {
		shards[i] = fmt.Sprintf("%d/%d", i+1, parallel)
	}
	return shards, nil
}

// runParallel runs one flowRun per shard at once, each on its own
// simulator, server, and folder (under r.out, when set, as shard-i), with
// its output's lines labelled by shard. Every shard runs to the end, so a
// failure in one still shows the other's results.
func runParallel(ctx context.Context, r flowRun, shards []string) error {
	var mu sync.Mutex
	errs := make([]error, len(shards))
	var wg sync.WaitGroup
	for i, shard := range shards {
		run := r
		run.shard, run.purpose = shard, flowsPurpose(i+1)
		if r.out != "" {
			run.out = filepath.Join(r.out, "shard-"+strconv.Itoa(i+1))
		}
		label := "[" + shard + "] "
		stdout := &labelWriter{label: label, out: os.Stdout, mu: &mu}
		stderr := &labelWriter{label: label, out: os.Stderr, mu: &mu}
		run.stdout, run.stderr = stdout, stderr
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = runFlows(ctx, run)
			stdout.Flush()
			stderr.Flush()
		}()
	}
	wg.Wait()
	var failed []string
	for i, err := range errs {
		if err != nil {
			failed = append(failed, fmt.Sprintf("shard %s: %v", shards[i], err))
		}
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}

// labelWriter writes each whole line to out with label in front, so the
// lines of runs going at once don't break into each other. mu is shared by
// every writer to the same terminal. Flush writes a last line that never
// got its newline.
type labelWriter struct {
	label   string
	out     io.Writer
	mu      *sync.Mutex
	pending []byte
}

func (w *labelWriter) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			return len(p), nil
		}
		if err := w.writeLine(w.pending[:end+1]); err != nil {
			return len(p), err
		}
		w.pending = w.pending[end+1:]
	}
}

func (w *labelWriter) Flush() {
	if len(w.pending) > 0 {
		_ = w.writeLine(append(w.pending, '\n'))
		w.pending = nil
	}
}

func (w *labelWriter) writeLine(line []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := io.WriteString(w.out, w.label+string(line))
	return errors.WithStack(err)
}
