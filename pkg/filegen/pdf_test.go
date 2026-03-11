package filegen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/pdf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestPDF writes a minimal valid PDF to outPath with the given info dict entries.
// infoEntries maps info dict key names (e.g., "Title", "Author") to string values.
// Pass nil or empty map for no info dict entries.
func writeTestPDF(t *testing.T, outPath string, infoEntries map[string]string) {
	t.Helper()
	var b strings.Builder

	// Track byte offsets for the xref table.
	var offsets []int
	objNum := 1

	b.WriteString("%PDF-1.4\n")

	// Obj 1: Catalog
	catalogObj := objNum
	objNum++
	offsets = append(offsets, b.Len())
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n", catalogObj))

	// Obj 2: Pages
	pagesObj := objNum
	objNum++
	offsets = append(offsets, b.Len())
	firstPageObj := objNum
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%d 0 R] /Count 1 /MediaBox [0 0 612 792] >>\nendobj\n",
		pagesObj, firstPageObj))

	// Obj 3: Page
	offsets = append(offsets, b.Len())
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent %d 0 R >>\nendobj\n",
		objNum, pagesObj))
	objNum++

	// Info dict object (optional)
	infoObj := 0
	if len(infoEntries) > 0 {
		infoObj = objNum
		objNum++
		offsets = append(offsets, b.Len())
		var infoStr strings.Builder
		infoStr.WriteString(fmt.Sprintf("%d 0 obj\n<< ", infoObj))
		for k, v := range infoEntries {
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

	err := os.WriteFile(outPath, []byte(b.String()), 0644)
	require.NoError(t, err, "writeTestPDF: failed to write %s", outPath)
}

func TestPDFGenerator_SupportedType(t *testing.T) {
	t.Parallel()
	gen := &PDFGenerator{}
	assert.Equal(t, models.FileTypePDF, gen.SupportedType())
}

func TestPDFGenerator_Generate(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDF(t, srcPath, map[string]string{
		"Title":  "Original Title",
		"Author": "Original Author",
	})

	destPath := filepath.Join(tmpDir, "dest.pdf")

	book := &models.Book{
		Title: "Updated Title",
		Authors: []*models.Author{
			{SortOrder: 0, Person: &models.Person{Name: "New Author"}},
		},
	}
	file := &models.File{FileType: models.FileTypePDF}

	gen := &PDFGenerator{}
	err := gen.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Destination file must exist.
	_, err = os.Stat(destPath)
	require.NoError(t, err)

	// Re-parse the destination to verify the title was updated.
	meta, err := pdf.Parse(destPath)
	require.NoError(t, err)
	assert.Equal(t, "Updated Title", meta.Title)
	require.Len(t, meta.Authors, 1)
	assert.Equal(t, "New Author", meta.Authors[0].Name)
}

func TestPDFGenerator_Generate_MultipleAuthors(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDF(t, srcPath, map[string]string{
		"Title":  "Test Book",
		"Author": "Old Author",
	})

	destPath := filepath.Join(tmpDir, "dest.pdf")

	book := &models.Book{
		Title: "Test Book",
		Authors: []*models.Author{
			{SortOrder: 1, Person: &models.Person{Name: "Second Author"}},
			{SortOrder: 0, Person: &models.Person{Name: "First Author"}},
		},
	}
	file := &models.File{FileType: models.FileTypePDF}

	gen := &PDFGenerator{}
	err := gen.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	meta, err := pdf.Parse(destPath)
	require.NoError(t, err)
	// Authors should be joined in sort order.
	assert.Equal(t, "First Author, Second Author", meta.Authors[0].Name+", "+meta.Authors[1].Name)
}

func TestPDFGenerator_Generate_Description(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDF(t, srcPath, map[string]string{
		"Title": "My Book",
	})

	destPath := filepath.Join(tmpDir, "dest.pdf")

	desc := "A fascinating description."
	book := &models.Book{
		Title:       "My Book",
		Description: &desc,
	}
	file := &models.File{FileType: models.FileTypePDF}

	gen := &PDFGenerator{}
	err := gen.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	meta, err := pdf.Parse(destPath)
	require.NoError(t, err)
	assert.Equal(t, "A fascinating description.", meta.Description)
}

func TestPDFGenerator_Generate_Tags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDF(t, srcPath, map[string]string{
		"Title": "My Book",
	})

	destPath := filepath.Join(tmpDir, "dest.pdf")

	book := &models.Book{
		Title: "My Book",
		BookTags: []*models.BookTag{
			{Tag: &models.Tag{Name: "fiction"}},
			{Tag: &models.Tag{Name: "sci-fi"}},
		},
	}
	file := &models.File{FileType: models.FileTypePDF}

	gen := &PDFGenerator{}
	err := gen.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	meta, err := pdf.Parse(destPath)
	require.NoError(t, err)
	require.Len(t, meta.Tags, 2)
	assert.Equal(t, "fiction", meta.Tags[0])
	assert.Equal(t, "sci-fi", meta.Tags[1])
}

func TestPDFGenerator_Generate_ReleaseDate(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDF(t, srcPath, map[string]string{
		"Title": "My Book",
	})

	destPath := filepath.Join(tmpDir, "dest.pdf")

	releaseDate := time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC)
	book := &models.Book{
		Title: "My Book",
	}
	file := &models.File{
		FileType:    models.FileTypePDF,
		ReleaseDate: &releaseDate,
	}

	gen := &PDFGenerator{}
	err := gen.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Note: pdfcpu always overwrites CreationDate with the current time when writing,
	// so we cannot verify the round-tripped value. We just verify generation succeeds.
	_, err = os.Stat(destPath)
	require.NoError(t, err)
}

func TestPDFGenerator_Generate_SourceUnmodified(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDF(t, srcPath, map[string]string{
		"Title":  "Original Title",
		"Author": "Original Author",
	})

	// Record source file content before generation.
	srcContentBefore, err := os.ReadFile(srcPath)
	require.NoError(t, err)

	destPath := filepath.Join(tmpDir, "dest.pdf")

	book := &models.Book{
		Title: "New Title",
	}
	file := &models.File{FileType: models.FileTypePDF}

	gen := &PDFGenerator{}
	err = gen.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Source must not be modified.
	srcContentAfter, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, srcContentBefore, srcContentAfter, "source file must not be modified")
}

func TestPDFGenerator_PreservesCreator(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDF(t, srcPath, map[string]string{
		"Title":   "Original Title",
		"Creator": "My PDF Creator App",
	})

	destPath := filepath.Join(tmpDir, "dest.pdf")

	book := &models.Book{
		Title: "New Title",
	}
	file := &models.File{FileType: models.FileTypePDF}

	gen := &PDFGenerator{}
	err := gen.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Read the destination info dict directly via pdfcpu to check Creator is preserved.
	// Note: pdfcpu always overwrites Producer with its own identifier when writing,
	// so Producer is not preserved. Creator, however, is not touched by pdfcpu.
	f, err := os.Open(destPath)
	require.NoError(t, err)
	defer f.Close()

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	pdfCtx, err := api.ReadAndValidate(f, conf)
	require.NoError(t, err)

	xrt := pdfCtx.XRefTable
	assert.Equal(t, "My PDF Creator App", xrt.Creator, "Creator must be preserved from original")

	// Title must be updated.
	assert.Equal(t, "New Title", xrt.Title)
}

func TestPDFGenerator_Generate_CancelledContext(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDF(t, srcPath, nil)

	destPath := filepath.Join(tmpDir, "dest.pdf")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	book := &models.Book{Title: "Test"}
	file := &models.File{FileType: models.FileTypePDF}

	gen := &PDFGenerator{}
	err := gen.Generate(ctx, srcPath, destPath, book, file)
	require.Error(t, err)

	var genErr *GenerationError
	require.ErrorAs(t, err, &genErr)
	assert.Equal(t, models.FileTypePDF, genErr.FileType)
}
