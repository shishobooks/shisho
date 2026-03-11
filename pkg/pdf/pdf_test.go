package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testdataDir string

func TestMain(m *testing.M) {
	// Create test fixtures as raw PDF files with full control over the info dict.
	dir, err := os.MkdirTemp("", "pdf-test-*")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}
	testdataDir = dir

	if err := createWithMetadata(dir); err != nil {
		panic("failed to create with-metadata.pdf: " + err.Error())
	}
	if err := createNoMetadata(dir); err != nil {
		panic("failed to create no-metadata.pdf: " + err.Error())
	}
	if err := createMultipleAuthors(dir); err != nil {
		panic("failed to create multiple-authors.pdf: " + err.Error())
	}
	if err := createInvalidPDF(dir); err != nil {
		panic("failed to create invalid.pdf: " + err.Error())
	}

	code := m.Run()

	os.RemoveAll(dir)
	os.Exit(code)
}

// writeRawPDF writes a minimal PDF with exact control over page count and info dict.
// infoEntries maps PDF info dict keys (Title, Author, Subject, Keywords, CreationDate)
// to their string values. Pass nil for no info dict.
func writeRawPDF(outPath string, pageCount int, infoEntries map[string]string) error {
	// Build the PDF incrementally, tracking object numbers and byte offsets.
	// PDF spec: header, body (objects), xref table, trailer.
	var b strings.Builder

	// Track offsets of each object for the xref table
	var offsets []int
	objNum := 1

	// Write header
	b.WriteString("%PDF-1.4\n")

	// Object 1: Catalog
	catalogObj := objNum
	objNum++
	offsets = append(offsets, b.Len())
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n", catalogObj))

	// Object 2: Pages (root page tree node)
	pagesObj := objNum
	objNum++
	offsets = append(offsets, b.Len())

	// Build Kids array
	firstPageObj := objNum
	kidsArr := ""
	for i := 0; i < pageCount; i++ {
		if i > 0 {
			kidsArr += " "
		}
		kidsArr += fmt.Sprintf("%d 0 R", firstPageObj+i)
	}

	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>\nendobj\n",
		pagesObj, kidsArr, pageCount))

	// Page objects
	for i := 0; i < pageCount; i++ {
		offsets = append(offsets, b.Len())
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent %d 0 R >>\nendobj\n",
			objNum, pagesObj))
		objNum++
	}

	// Info dict object (optional)
	infoObj := 0
	if infoEntries != nil && len(infoEntries) > 0 {
		infoObj = objNum
		objNum++
		offsets = append(offsets, b.Len())

		var infoStr strings.Builder
		infoStr.WriteString(fmt.Sprintf("%d 0 obj\n<< ", infoObj))
		for k, v := range infoEntries {
			// PDF string literal: wrap in parentheses, escape special chars
			escaped := strings.ReplaceAll(v, "\\", "\\\\")
			escaped = strings.ReplaceAll(escaped, "(", "\\(")
			escaped = strings.ReplaceAll(escaped, ")", "\\)")
			infoStr.WriteString(fmt.Sprintf("/%s (%s) ", k, escaped))
		}
		infoStr.WriteString(">>\nendobj\n")
		b.WriteString(infoStr.String())
	}

	// Xref table
	xrefOffset := b.Len()
	b.WriteString("xref\n")
	b.WriteString(fmt.Sprintf("0 %d\n", objNum))
	b.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		b.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	// Trailer
	b.WriteString("trailer\n")
	b.WriteString(fmt.Sprintf("<< /Size %d /Root %d 0 R", objNum, catalogObj))
	if infoObj > 0 {
		b.WriteString(fmt.Sprintf(" /Info %d 0 R", infoObj))
	}
	b.WriteString(" >>\n")
	b.WriteString("startxref\n")
	b.WriteString(fmt.Sprintf("%d\n", xrefOffset))
	b.WriteString("%%EOF\n")

	return os.WriteFile(outPath, []byte(b.String()), 0644)
}

func createWithMetadata(dir string) error {
	return writeRawPDF(
		filepath.Join(dir, "with-metadata.pdf"),
		3,
		map[string]string{
			"Title":        "Test PDF Title",
			"Author":       "Test Author",
			"Subject":      "A test PDF description",
			"Keywords":     "fiction, sci-fi",
			"CreationDate": "D:20240615103000+00'00'",
		},
	)
}

func createNoMetadata(dir string) error {
	// No info dict at all
	return writeRawPDF(filepath.Join(dir, "no-metadata.pdf"), 2, nil)
}

func createMultipleAuthors(dir string) error {
	return writeRawPDF(
		filepath.Join(dir, "multiple-authors.pdf"),
		1,
		map[string]string{
			"Author": "Alpha & Beta; Gamma, Delta",
		},
	)
}

func createInvalidPDF(dir string) error {
	return os.WriteFile(filepath.Join(dir, "invalid.pdf"), []byte("this is not a valid PDF file"), 0644)
}

func TestParse_WithMetadata(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testdataDir, "with-metadata.pdf")
	meta, err := Parse(path)
	require.NoError(t, err)

	assert.Equal(t, "Test PDF Title", meta.Title)
	require.Len(t, meta.Authors, 1)
	assert.Equal(t, "Test Author", meta.Authors[0].Name)
	assert.Empty(t, meta.Authors[0].Role, "PDF authors should have no role")
	assert.Equal(t, "A test PDF description", meta.Description)
	require.NotNil(t, meta.ReleaseDate)
	assert.Equal(t, 2024, meta.ReleaseDate.Year())
	assert.Equal(t, time.Month(6), meta.ReleaseDate.Month())
	assert.Equal(t, 15, meta.ReleaseDate.Day())
	require.NotNil(t, meta.PageCount)
	assert.Equal(t, 3, *meta.PageCount)
	assert.Equal(t, models.DataSourcePDFMetadata, meta.DataSource)
}

func TestParse_NoMetadata(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testdataDir, "no-metadata.pdf")
	meta, err := Parse(path)
	require.NoError(t, err)

	assert.Empty(t, meta.Title)
	assert.Empty(t, meta.Authors)
	assert.Empty(t, meta.Description)
	assert.Nil(t, meta.ReleaseDate)
	require.NotNil(t, meta.PageCount)
	assert.Equal(t, 2, *meta.PageCount)
	assert.Equal(t, models.DataSourcePDFMetadata, meta.DataSource)
}

func TestParse_MultipleAuthors(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testdataDir, "multiple-authors.pdf")
	meta, err := Parse(path)
	require.NoError(t, err)

	require.Len(t, meta.Authors, 4)
	assert.Equal(t, "Alpha", meta.Authors[0].Name)
	assert.Equal(t, "Beta", meta.Authors[1].Name)
	assert.Equal(t, "Gamma", meta.Authors[2].Name)
	assert.Equal(t, "Delta", meta.Authors[3].Name)
}

func TestParse_Keywords(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testdataDir, "with-metadata.pdf")
	meta, err := Parse(path)
	require.NoError(t, err)

	require.Len(t, meta.Tags, 2)
	assert.Equal(t, "fiction", meta.Tags[0])
	assert.Equal(t, "sci-fi", meta.Tags[1])
}

func TestParse_InvalidPDF(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testdataDir, "invalid.pdf")
	_, err := Parse(path)
	assert.Error(t, err)
}
