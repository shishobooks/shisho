package testgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// PDFOptions configures the generated PDF file.
type PDFOptions struct {
	PageCount int // defaults to 3
}

// GeneratePDF writes a minimal valid PDF with the given page count to the
// specified directory. The resulting file is parseable by pdfcpu and
// renderable by go-pdfium, so it can be used in scanner and cover-page tests.
func GeneratePDF(t *testing.T, dir, filename string, opts PDFOptions) string {
	t.Helper()
	pageCount := opts.PageCount
	if pageCount <= 0 {
		pageCount = 3
	}

	path := filepath.Join(dir, filename)
	if err := writeMinimalPDF(path, pageCount); err != nil {
		t.Fatalf("failed to generate pdf %s: %v", path, err)
	}
	return path
}

// writeMinimalPDF constructs a minimal PDF (header, catalog, pages, xref,
// trailer) with the given number of empty pages. Mirrors the fixture helper
// used in pkg/pdf tests.
func writeMinimalPDF(outPath string, pageCount int) error {
	var b strings.Builder
	var offsets []int
	objNum := 1

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

	firstPageObj := objNum
	kidsParts := make([]string, pageCount)
	for i := 0; i < pageCount; i++ {
		kidsParts[i] = fmt.Sprintf("%d 0 R", firstPageObj+i)
	}
	kidsArr := strings.Join(kidsParts, " ")

	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>\nendobj\n",
		pagesObj, kidsArr, pageCount))

	// Page objects
	for i := 0; i < pageCount; i++ {
		offsets = append(offsets, b.Len())
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent %d 0 R >>\nendobj\n",
			objNum, pagesObj))
		objNum++
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

	return os.WriteFile(outPath, []byte(b.String()), 0600)
}
