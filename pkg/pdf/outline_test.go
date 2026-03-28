package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractOutline_WithBookmarks(t *testing.T) {
	t.Parallel()

	pdfPath := filepath.Join(t.TempDir(), "with-outline.pdf")
	require.NoError(t, writeRawPDFWithOutline(pdfPath, 5, []outlineFixture{
		{title: "Chapter 1", pageIndex: 0},
		{title: "Chapter 2", pageIndex: 2},
		{title: "Chapter 3", pageIndex: 4},
	}))

	entries, err := ExtractOutline(pdfPath)
	require.NoError(t, err)

	require.Len(t, entries, 3)
	assert.Equal(t, "Chapter 1", entries[0].Title)
	assert.Equal(t, 0, entries[0].StartPage)
	assert.Equal(t, "Chapter 2", entries[1].Title)
	assert.Equal(t, 2, entries[1].StartPage)
	assert.Equal(t, "Chapter 3", entries[2].Title)
	assert.Equal(t, 4, entries[2].StartPage)
}

func TestExtractOutline_NoBookmarks(t *testing.T) {
	t.Parallel()

	// Use the standard no-metadata test PDF (no outline)
	path := filepath.Join(testdataDir, "no-metadata.pdf")
	entries, err := ExtractOutline(path)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestExtractOutline_InvalidPDF(t *testing.T) {
	t.Parallel()

	path := filepath.Join(testdataDir, "invalid.pdf")
	_, err := ExtractOutline(path)
	assert.Error(t, err)
}

func TestParse_IncludesChaptersFromOutline(t *testing.T) {
	t.Parallel()

	pdfPath := filepath.Join(t.TempDir(), "with-chapters.pdf")
	require.NoError(t, writeRawPDFWithOutline(pdfPath, 3, []outlineFixture{
		{title: "Introduction", pageIndex: 0},
		{title: "Main Content", pageIndex: 1},
	}))

	meta, err := Parse(pdfPath)
	require.NoError(t, err)

	require.Len(t, meta.Chapters, 2)
	assert.Equal(t, "Introduction", meta.Chapters[0].Title)
	require.NotNil(t, meta.Chapters[0].StartPage)
	assert.Equal(t, 0, *meta.Chapters[0].StartPage)
	assert.Equal(t, "Main Content", meta.Chapters[1].Title)
	require.NotNil(t, meta.Chapters[1].StartPage)
	assert.Equal(t, 1, *meta.Chapters[1].StartPage)
}

// outlineFixture describes a bookmark entry for test PDF generation.
type outlineFixture struct {
	title     string
	pageIndex int // 0-indexed
}

// writeRawPDFWithOutline creates a minimal PDF with an outline (bookmark) tree.
// Each bookmark uses an explicit /Dest [pageRef /Fit] to point to a page.
func writeRawPDFWithOutline(outPath string, pageCount int, bookmarks []outlineFixture) error {
	return writeCleanPDFWithOutline(outPath, pageCount, bookmarks)
}

// writeCleanPDFWithOutline builds a PDF with bookmarks in a single pass.
func writeCleanPDFWithOutline(outPath string, pageCount int, bookmarks []outlineFixture) error {
	var b strings.Builder
	var offsets []int
	objNum := 1

	b.WriteString("%PDF-1.4\n")

	// Pre-compute object numbers
	catalogObj := objNum // 1
	objNum++
	pagesObj := objNum // 2
	objNum++

	// Page objects: 3 .. 3+pageCount-1
	pageObjNums := make([]int, pageCount)
	for i := 0; i < pageCount; i++ {
		pageObjNums[i] = objNum
		objNum++
	}

	outlinesObj := objNum // after pages
	objNum++

	// Bookmark objects
	bookmarkObjNums := make([]int, len(bookmarks))
	for i := range bookmarks {
		bookmarkObjNums[i] = objNum
		objNum++
	}

	// Write Catalog (obj 1)
	offsets = append(offsets, b.Len())
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Catalog /Pages %d 0 R /Outlines %d 0 R >>\nendobj\n",
		catalogObj, pagesObj, outlinesObj))

	// Write Pages (obj 2)
	offsets = append(offsets, b.Len())
	kidsParts := make([]string, pageCount)
	for i := 0; i < pageCount; i++ {
		kidsParts[i] = fmt.Sprintf("%d 0 R", pageObjNums[i])
	}
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>\nendobj\n",
		pagesObj, strings.Join(kidsParts, " "), pageCount))

	// Write Page objects
	for i := 0; i < pageCount; i++ {
		offsets = append(offsets, b.Len())
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent %d 0 R >>\nendobj\n",
			pageObjNums[i], pagesObj))
	}

	// Write Outlines root
	offsets = append(offsets, b.Len())
	if len(bookmarks) > 0 {
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>\nendobj\n",
			outlinesObj, bookmarkObjNums[0], bookmarkObjNums[len(bookmarkObjNums)-1], len(bookmarks)))
	} else {
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Outlines /Count 0 >>\nendobj\n", outlinesObj))
	}

	// Write Bookmark objects
	for i, bm := range bookmarks {
		offsets = append(offsets, b.Len())
		pageRef := pageObjNums[bm.pageIndex]

		var parts []string
		parts = append(parts, fmt.Sprintf("/Title (%s)", bm.title))
		parts = append(parts, fmt.Sprintf("/Parent %d 0 R", outlinesObj))
		parts = append(parts, fmt.Sprintf("/Dest [%d 0 R /Fit]", pageRef))

		if i > 0 {
			parts = append(parts, fmt.Sprintf("/Prev %d 0 R", bookmarkObjNums[i-1]))
		}
		if i < len(bookmarks)-1 {
			parts = append(parts, fmt.Sprintf("/Next %d 0 R", bookmarkObjNums[i+1]))
		}

		b.WriteString(fmt.Sprintf("%d 0 obj\n<< %s >>\nendobj\n", bookmarkObjNums[i], strings.Join(parts, " ")))
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
	b.WriteString(fmt.Sprintf("<< /Size %d /Root %d 0 R >>\n", objNum, catalogObj))
	b.WriteString("startxref\n")
	b.WriteString(fmt.Sprintf("%d\n", xrefOffset))
	b.WriteString("%%EOF\n")

	return os.WriteFile(outPath, []byte(b.String()), 0644)
}
