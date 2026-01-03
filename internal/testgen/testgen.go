// Package testgen provides utilities for generating test files (EPUB, CBZ, M4B)
// with configurable metadata for testing the scan worker.
package testgen

import (
	"os"
	"path/filepath"
	"testing"
)

// EPUBOptions configures the generated EPUB file.
type EPUBOptions struct {
	Title         string
	Authors       []string
	Series        string
	SeriesNumber  *float64
	HasCover      bool
	CoverMimeType string // "image/jpeg" or "image/png", defaults to "image/png"
}

// CBZOptions configures the generated CBZ file.
type CBZOptions struct {
	Title           string
	Series          string
	SeriesNumber    *float64
	Writer          string
	PageCount       int    // defaults to 3
	HasComicInfo    bool   // whether to include ComicInfo.xml
	CoverPageType   string // "FrontCover", "InnerCover", or "" (none specified)
	ImageFormat     string // "png" or "jpeg", defaults to "png"
	ForceEmptyTitle bool   // if true, writes an empty <Title></Title> element (for testing empty title handling)
}

// M4BOptions configures the generated M4B file.
type M4BOptions struct {
	Title    string
	Artist   string  // Author
	Album    string  // Series/Grouping (e.g., "Series Name #7")
	Composer string  // Narrator
	Genre    string  // Genre text
	Duration float64 // Duration in seconds
	HasCover bool
}

// TempDir creates a temporary directory for testing and registers cleanup.
// The directory is automatically removed when the test completes.
func TempDir(t *testing.T, pattern string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// TempLibraryDir creates a temporary library directory structure for testing.
// Returns the library path that should be used when creating a library.
func TempLibraryDir(t *testing.T) string {
	t.Helper()
	return TempDir(t, "testgen-library-*")
}

// CreateSubDir creates a subdirectory within the given parent directory.
// Returns the full path to the created subdirectory.
func CreateSubDir(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory %s: %v", dir, err)
	}
	return dir
}

// WriteFile creates a file with the given content in the specified directory.
// Returns the full path to the created file.
func WriteFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
	return path
}

// FileExists checks if a file exists at the given path.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ReadFile reads and returns the contents of a file.
func ReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	return data
}

// StringPtr is a helper to create a pointer to a string.
func StringPtr(s string) *string {
	return &s
}
