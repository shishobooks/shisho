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
	Description   string // dc:description
	Publisher     string // dc:publisher
	Language      string // dc:language, defaults to "en"
	Date          string // dc:date, e.g. "2020-01-02"
	HasCover      bool
	CoverMimeType string // "image/jpeg" or "image/png", defaults to "image/png"
	// Identifiers are extra dc:identifier elements written after the default
	// urn:uuid:test-book-id, which the parser skips as an unknown type.
	Identifiers []EPUBIdentifier
}

// EPUBIdentifier is one dc:identifier element. Scheme becomes opf:scheme
// when set.
type EPUBIdentifier struct {
	Scheme string
	Value  string
}

// CBZOptions configures the generated CBZ file.
type CBZOptions struct {
	Title           string
	Series          string
	SeriesNumber    *float64
	Writer          string
	Penciller       string
	Inker           string
	Colorist        string
	Letterer        string
	CoverArtist     string
	Editor          string
	Translator      string
	PageCount       int    // defaults to 3
	HasComicInfo    bool   // whether to include ComicInfo.xml
	CoverPageType   string // "FrontCover", "InnerCover", or "" (none specified)
	CoverPageIndex  int    // 0-indexed page number for cover (used with CoverPageType)
	ImageFormat     string // "png" or "jpeg", defaults to "png"
	ForceEmptyTitle bool   // if true, writes an empty <Title></Title> element (for testing empty title handling)
}

// M4BChapter represents a chapter for test M4B generation.
type M4BChapter struct {
	Title string
	Start float64 // Start time in seconds
}

// M4BOptions configures the generated M4B file.
type M4BOptions struct {
	Title       string
	Artist      string  // Author
	Album       string  // Album title (©alb atom); use Grouping for series info
	Grouping    string  // Grouping (©grp atom) — series info, e.g., "Series Name #7"
	Composer    string  // Narrator
	Genre       string  // Genre text
	Duration    float64 // Duration in seconds
	HasCover    bool
	Copyright   string // Copyright notice
	Date        string // Year/date (e.g., "2024")
	AlbumArtist string // Album artist (different from artist)
	Comment     string // Comment/description
	Chapters    []M4BChapter
	// Faststart writes the file with `-movflags +faststart`, placing the moov
	// box before mdat. This mirrors the layout Audible/Apple Books exports use
	// and is required to exercise the chunk-offset rewrite path in mp4.Write.
	Faststart bool
}

// MOBIKind is the layout of a generated MOBI file.
type MOBIKind int

const (
	// MOBIKindMOBI6 is a Mobipocket file with only the old MOBI6 book (.mobi).
	MOBIKindMOBI6 MOBIKind = iota
	// MOBIKindKF8 is a file with only the KF8 book (.azw3).
	MOBIKindKF8
	// MOBIKindCombo is a .mobi holding a MOBI6 book, a BOUNDARY record, and
	// a KF8 book, as Calibre writes with --mobi-file-type both.
	MOBIKindCombo
)

// MOBICompression is the text compression a generated MOBI file declares.
type MOBICompression int

const (
	MOBICompressionPalmDOC MOBICompression = iota // what Calibre writes
	MOBICompressionNone
	MOBICompressionHuffCDIC // what kindlegen writes for large books
)

// MOBIOptions configures the generated MOBI file. Each string field is one
// EXTH record and is left out when empty.
type MOBIOptions struct {
	Kind MOBIKind
	// Title is the updated-title record (503). FullName is the title stored
	// in the header and defaults to Title.
	Title    string
	FullName string
	// MOBI6Title replaces Title in the MOBI6 half of a combo file, so a test
	// can tell which half was read.
	MOBI6Title     string
	Authors        []string // one author record (100) each, written as given
	Description    string   // 103, may hold HTML
	Publisher      string   // 101
	Imprint        string   // 102
	PublishingDate string   // 106, e.g. "2021-03-04T06:00:00+00:00"
	Language       string   // 524
	ISBN           string   // 104
	// Source is the source record (112). Calibre writes "calibre:<uuid>";
	// it holds "urn:isbn:..." in some files.
	Source string
	// ASIN is the ASIN record (113). Calibre writes the book's UUID here.
	ASIN     string
	Subjects []string // one subject record (105) each, written as given
	HasCover bool
	// CoverMimeType is "image/jpeg" (default, what Calibre writes for MOBI6)
	// or "image/png".
	CoverMimeType string
	// Encrypted marks every book in the file as DRM-protected.
	Encrypted   bool
	Compression MOBICompression
	// CP1252 writes the header and EXTH text as Windows-1252 instead of UTF-8.
	CP1252 bool
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
