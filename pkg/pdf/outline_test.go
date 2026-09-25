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

// Some document tooling writes outline items whose destination has no page,
// such as /Dest [null 0 0 1]. PDFium reports these with a non-nil DestInfo
// and PageIndex -1. They must be skipped like items with no destination,
// while their children, which can still point at real pages, are kept.
func TestExtractOutline_SkipsNullDestinations(t *testing.T) {
	t.Parallel()

	pdfPath := filepath.Join(t.TempDir(), "null-dest-outline.pdf")
	require.NoError(t, writeRawPDFWithOutline(pdfPath, 4, []outlineFixture{
		{title: "Cover", pageIndex: 0},
		{title: "Dangling", nullDest: true},
		{title: "Part One", nullDest: true, children: []outlineFixture{
			{title: "Chapter 1", pageIndex: 1},
			{title: "Dangling Child", nullDest: true},
			{title: "Chapter 2", pageIndex: 3},
		}},
	}))

	entries, err := ExtractOutline(pdfPath)
	require.NoError(t, err)

	assert.Equal(t, []OutlineEntry{
		{Title: "Cover", StartPage: 0},
		{Title: "Chapter 1", StartPage: 1},
		{Title: "Chapter 2", StartPage: 3},
	}, entries)
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
	pageIndex int  // 0-indexed; ignored when nullDest is set
	nullDest  bool // write /Dest [null 0 0 1], a destination with no page
	children  []outlineFixture
}

// outlineNode is an outlineFixture with its PDF object number and the object
// numbers of its outline neighbours (0 when absent).
type outlineNode struct {
	fixture            outlineFixture
	objNum             int
	parent, prev, next int
	children           []*outlineNode
}

// countOutlineNodes returns the total number of nodes in the given subtrees,
// used for the /Count entry of an open outline item.
func countOutlineNodes(nodes []*outlineNode) int {
	count := len(nodes)
	for _, n := range nodes {
		count += countOutlineNodes(n.children)
	}
	return count
}

// writeRawPDFWithOutline creates a minimal PDF with an outline (bookmark) tree.
// Each bookmark uses an explicit /Dest [pageRef /Fit] to point to a page, or
// /Dest [null 0 0 1] when nullDest is set. Children are written as nested
// outline items.
func writeRawPDFWithOutline(outPath string, pageCount int, bookmarks []outlineFixture) error {
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

	// Bookmark objects, numbered depth-first so they are written in
	// increasing object order.
	var ordered []*outlineNode
	var assign func(parent int, fixtures []outlineFixture) []*outlineNode
	assign = func(parent int, fixtures []outlineFixture) []*outlineNode {
		nodes := make([]*outlineNode, len(fixtures))
		for i, f := range fixtures {
			n := &outlineNode{fixture: f, objNum: objNum, parent: parent}
			objNum++
			if i > 0 {
				n.prev = nodes[i-1].objNum
				nodes[i-1].next = n.objNum
			}
			ordered = append(ordered, n)
			n.children = assign(n.objNum, f.children)
			nodes[i] = n
		}
		return nodes
	}
	roots := assign(outlinesObj, bookmarks)

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
	if len(roots) > 0 {
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>\nendobj\n",
			outlinesObj, roots[0].objNum, roots[len(roots)-1].objNum, countOutlineNodes(roots)))
	} else {
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Outlines /Count 0 >>\nendobj\n", outlinesObj))
	}

	// Write Bookmark objects
	for _, n := range ordered {
		offsets = append(offsets, b.Len())

		var parts []string
		parts = append(parts, fmt.Sprintf("/Title (%s)", n.fixture.title))
		parts = append(parts, fmt.Sprintf("/Parent %d 0 R", n.parent))
		if n.fixture.nullDest {
			parts = append(parts, "/Dest [null 0 0 1]")
		} else {
			parts = append(parts, fmt.Sprintf("/Dest [%d 0 R /Fit]", pageObjNums[n.fixture.pageIndex]))
		}

		if n.prev != 0 {
			parts = append(parts, fmt.Sprintf("/Prev %d 0 R", n.prev))
		}
		if n.next != 0 {
			parts = append(parts, fmt.Sprintf("/Next %d 0 R", n.next))
		}

		if len(n.children) > 0 {
			parts = append(parts, fmt.Sprintf("/First %d 0 R /Last %d 0 R /Count %d",
				n.children[0].objNum, n.children[len(n.children)-1].objNum, countOutlineNodes(n.children)))
		}

		b.WriteString(fmt.Sprintf("%d 0 obj\n<< %s >>\nendobj\n", n.objNum, strings.Join(parts, " ")))
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
