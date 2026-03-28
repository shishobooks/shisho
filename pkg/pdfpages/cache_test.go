package pdfpages

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testPDFPath string

func TestMain(m *testing.M) {
	// Create a temp dir for test fixtures
	dir, err := os.MkdirTemp("", "pdfpages-test-*")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}

	// Create a minimal 3-page PDF for testing
	testPDFPath = filepath.Join(dir, "test.pdf")
	if err := writeTestPDF(testPDFPath, 3); err != nil {
		panic("failed to create test PDF: " + err.Error())
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// writeTestPDF writes a minimal valid PDF with the given page count.
func writeTestPDF(outPath string, pageCount int) error {
	var b strings.Builder
	var offsets []int
	objNum := 1

	b.WriteString("%PDF-1.4\n")

	// Obj 1: Catalog
	offsets = append(offsets, b.Len())
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n", objNum))
	objNum++

	// Obj 2: Pages
	offsets = append(offsets, b.Len())
	firstPageObj := objNum + 1
	kidsParts := make([]string, pageCount)
	for i := 0; i < pageCount; i++ {
		kidsParts[i] = fmt.Sprintf("%d 0 R", firstPageObj+i)
	}
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>\nendobj\n",
		objNum, strings.Join(kidsParts, " "), pageCount))
	objNum++

	// Page objects
	for i := 0; i < pageCount; i++ {
		offsets = append(offsets, b.Len())
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R >>\nendobj\n", objNum))
		objNum++
	}

	// Xref
	xrefOffset := b.Len()
	b.WriteString("xref\n")
	b.WriteString(fmt.Sprintf("0 %d\n", objNum))
	b.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		b.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	// Trailer
	b.WriteString("trailer\n")
	b.WriteString(fmt.Sprintf("<< /Size %d /Root 1 0 R >>\n", objNum))
	b.WriteString("startxref\n")
	b.WriteString(fmt.Sprintf("%d\n", xrefOffset))
	b.WriteString("%%EOF\n")

	return os.WriteFile(outPath, []byte(b.String()), 0644)
}

func TestGetPage_RendersAndCaches(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	// First call: render and cache
	cachedPath, mimeType, err := cache.GetPage(testPDFPath, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", mimeType)
	assert.FileExists(t, cachedPath)

	// Verify the cached file is a valid JPEG
	data, err := os.ReadFile(cachedPath)
	require.NoError(t, err)
	_, err = jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)

	// Second call: should return cached path (same result)
	cachedPath2, mimeType2, err := cache.GetPage(testPDFPath, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, cachedPath, cachedPath2)
	assert.Equal(t, "image/jpeg", mimeType2)
}

func TestGetPage_DifferentPages(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	path0, _, err := cache.GetPage(testPDFPath, 2, 0)
	require.NoError(t, err)

	path1, _, err := cache.GetPage(testPDFPath, 2, 1)
	require.NoError(t, err)

	path2, _, err := cache.GetPage(testPDFPath, 2, 2)
	require.NoError(t, err)

	// All paths should be different files
	assert.NotEqual(t, path0, path1)
	assert.NotEqual(t, path1, path2)
}

func TestGetPage_InvalidPageNumber(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	// Page number too high
	_, _, err := cache.GetPage(testPDFPath, 3, 99)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	// Negative page number
	_, _, err = cache.GetPage(testPDFPath, 3, -1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestGetPage_InvalidPDF(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	invalidPath := filepath.Join(t.TempDir(), "invalid.pdf")
	require.NoError(t, os.WriteFile(invalidPath, []byte("not a pdf"), 0644))

	_, _, err := cache.GetPage(invalidPath, 4, 0)
	assert.Error(t, err)
}

func TestGetPage_NonexistentFile(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	_, _, err := cache.GetPage("/nonexistent/file.pdf", 5, 0)
	assert.Error(t, err)
}

func TestInvalidate(t *testing.T) {
	t.Parallel()
	cacheDir := t.TempDir()
	cache := NewCache(cacheDir, 150, 85)

	// Render a page to create cache entry
	cachedPath, _, err := cache.GetPage(testPDFPath, 6, 0)
	require.NoError(t, err)
	assert.FileExists(t, cachedPath)

	// Invalidate
	err = cache.Invalidate(6)
	require.NoError(t, err)

	// Cached file should be gone
	_, err = os.Stat(cachedPath)
	assert.True(t, os.IsNotExist(err))
}
