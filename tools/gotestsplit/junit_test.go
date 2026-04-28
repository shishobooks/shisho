package main

import (
	"reflect"
	"testing"
)

func TestReadHistory_ParsesAndMergesFiles(t *testing.T) {
	t.Parallel()
	got, err := ReadHistory("testdata")
	if err != nil {
		t.Fatalf("ReadHistory: %v", err)
	}
	// sample-2 overrides sample-1 for TestAlpha (later file wins).
	// Subtests (TestBeta/subcase) are skipped — top-level only.
	want := History{
		"github.com/example/foo": {
			"TestAlpha": 4.0,
			"TestBeta":  1.2,
		},
		"github.com/example/bar": {
			"TestGamma": 0.1,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReadHistory mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestReadHistory_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := ReadHistory(dir)
	if err != nil {
		t.Fatalf("ReadHistory: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty history, got %v", got)
	}
}

func TestReadHistory_MissingDir(t *testing.T) {
	t.Parallel()
	got, err := ReadHistory("/nonexistent/path/xyz")
	if err != nil {
		t.Fatalf("ReadHistory should not error on missing dir: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty history, got %v", got)
	}
}
