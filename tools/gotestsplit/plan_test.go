package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestPlan_PrintsAssignmentForShard(t *testing.T) {
	t.Parallel()
	// Use packagesFromHistory under the hood by passing -no-discover so we don't
	// shell out to `go test -list` in unit tests.
	var stdout, stderr bytes.Buffer
	err := cmdPlan(context.Background(),
		[]string{
			"-junit-dir=testdata",
			"-total=2", "-index=0",
			"-no-discover",
		},
		&stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdPlan: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "shard 1/2") {
		t.Errorf("missing 'shard 1/2' header; got:\n%s", out)
	}
}

func TestPlan_DetailMode_ShowsAllShards(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := cmdPlan(context.Background(),
		[]string{
			"-junit-dir=testdata",
			"-total=2", "-index=0",
			"-detail", "-no-discover",
		},
		&stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdPlan: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "shard 1/2") || !strings.Contains(out, "shard 2/2") {
		t.Errorf("detail mode should show both shards; got:\n%s", out)
	}
}
