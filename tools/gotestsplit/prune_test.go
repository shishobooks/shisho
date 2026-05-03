package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestCmdPrune_DeletesOrphanShards(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Simulate a cache that was built when total=12, now we're pruning to total=8.
	// Shard indices 0-7 should survive; 8-11 should be deleted.
	files := map[string]bool{
		"junit-0-1.xml":  true,  // keep: shard 0 < 8
		"junit-0-2.xml":  true,  // keep: shard 0, second chunk
		"junit-3-1.xml":  true,  // keep: shard 3 < 8
		"junit-7-1.xml":  true,  // keep: shard 7 < 8 (boundary)
		"junit-8-1.xml":  false, // delete: shard 8 >= 8
		"junit-9-1.xml":  false, // delete: shard 9 >= 8
		"junit-11-1.xml": false, // delete: shard 11 >= 8
		"junit-11-2.xml": false, // delete: shard 11, second chunk
		"other.xml":      true,  // keep: doesn't match junit pattern
		"readme.txt":     true,  // keep: not even xml
	}

	for name := range files {
		os.WriteFile(filepath.Join(dir, name), []byte("<testsuites/>"), 0o600)
	}

	var stdout, stderr bytes.Buffer
	err := cmdPrune(context.Background(), []string{"-junit-dir=" + dir, "-total=8"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdPrune: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	var remaining []string
	for _, e := range entries {
		remaining = append(remaining, e.Name())
	}
	sort.Strings(remaining)

	var want []string
	for name, keep := range files {
		if keep {
			want = append(want, name)
		}
	}
	sort.Strings(want)

	if len(remaining) != len(want) {
		t.Fatalf("remaining files mismatch:\n got=%v\nwant=%v", remaining, want)
	}
	for i := range remaining {
		if remaining[i] != want[i] {
			t.Fatalf("remaining files mismatch:\n got=%v\nwant=%v", remaining, want)
		}
	}
}

func TestCmdPrune_ReportsDeletedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "junit-0-1.xml"), []byte("<testsuites/>"), 0o600)
	os.WriteFile(filepath.Join(dir, "junit-5-1.xml"), []byte("<testsuites/>"), 0o600)

	var stdout, stderr bytes.Buffer
	err := cmdPrune(context.Background(), []string{"-junit-dir=" + dir, "-total=4"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdPrune: %v", err)
	}

	out := stdout.String()
	if out == "" {
		t.Fatal("expected output reporting deleted files")
	}
	// Should mention the deleted file
	if !bytes.Contains([]byte(out), []byte("junit-5-1.xml")) {
		t.Errorf("expected output to mention junit-5-1.xml, got: %s", out)
	}
}

func TestCmdPrune_NothingToDelete(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "junit-0-1.xml"), []byte("<testsuites/>"), 0o600)
	os.WriteFile(filepath.Join(dir, "junit-1-1.xml"), []byte("<testsuites/>"), 0o600)

	var stdout, stderr bytes.Buffer
	err := cmdPrune(context.Background(), []string{"-junit-dir=" + dir, "-total=4"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("cmdPrune: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("expected 2 files, got %d", len(entries))
	}
}

func TestCmdPrune_RequiresFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"missing junit-dir", []string{"-total=8"}},
		{"missing total", []string{"-junit-dir=/tmp"}},
		{"total zero", []string{"-junit-dir=/tmp", "-total=0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			err := cmdPrune(context.Background(), tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected error for invalid flags")
			}
		})
	}
}
