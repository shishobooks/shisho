package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestSimulate_PrintsTableAcrossRange(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := cmdSimulate(context.Background(),
		[]string{"-junit-dir=testdata", "-min=2", "-max=3"},
		&stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdSimulate: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"N=2", "N=3", "slowest=", "cost="} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestSimulate_NoHistory_ErrorsClearly(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	err := cmdSimulate(context.Background(),
		[]string{"-junit-dir=" + t.TempDir()},
		&stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no JUnit") {
		t.Errorf("error should mention missing JUnit data, got: %v", err)
	}
}
