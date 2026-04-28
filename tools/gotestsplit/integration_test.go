package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestRun_ProducesJUnit_AndShardsAreRunnable verifies end-to-end:
//  1. `run` succeeds against a fresh fixture module with no history → uses count fallback.
//  2. JUnit XML is written into the cache dir.
//  3. A second `run` reads that history without error.
func TestRun_ProducesJUnit_AndShardsAreRunnable(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	// Cannot t.Parallel() — uses t.Chdir.

	cacheDir := t.TempDir()
	fixture, err := filepath.Abs("testdata/fixturemod")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(fixture) // Go 1.24+, repo go.mod is 1.25.

	for shardIdx := 0; shardIdx < 2; shardIdx++ {
		var stdout, stderr bytes.Buffer
		err := cmdRun(context.Background(), []string{
			"-junit-dir=" + cacheDir,
			"-total=2", "-index=" + strconv.Itoa(shardIdx),
			"./...",
		}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("first pass shard %d: %v\nstderr:\n%s", shardIdx, err, stderr.String())
		}
		if !strings.Contains(stdout.String(), "PASS") {
			t.Errorf("shard %d stdout missing PASS:\n%s", shardIdx, stdout.String())
		}
	}

	// JUnit files should exist now.
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	junitCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".xml") {
			junitCount++
		}
	}
	if junitCount == 0 {
		t.Fatalf("no JUnit files written to %s", cacheDir)
	}

	// Second pass uses the history.
	var stdout, stderr bytes.Buffer
	err = cmdRun(context.Background(), []string{
		"-junit-dir=" + cacheDir,
		"-total=2", "-index=0",
		"./...",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("second pass: %v\nstderr:\n%s", err, stderr.String())
	}
}
