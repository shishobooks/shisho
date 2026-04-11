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

// writeTestPDF writes a minimal valid single-page PDF to outPath with the given
// info dict entries. infoEntries maps info dict key names (e.g., "Title", "Author")
// to string values. Pass nil or empty map for no info dict entries.
func writeTestPDF(t *testing.T, outPath string, infoEntries map[string]string) {
	t.Helper()
	writeTestPDFWithPages(t, outPath, 1, infoEntries)
}

// writeTestPDFWithPages writes a minimal valid PDF with pageCount pages to outPath
// with the given info dict entries.
func writeTestPDFWithPages(t *testing.T, outPath string, pageCount int, infoEntries map[string]string) {
	t.Helper()
	require.Positive(t, pageCount, "pageCount must be > 0")

	var b strings.Builder
	var offsets []int
	objNum := 1

	b.WriteString("%PDF-1.4\n")

	// Obj 1: Catalog
	catalogObj := objNum
	objNum++
	// Obj 2: Pages
	pagesObj := objNum
	objNum++
	// Obj 3..: Pages
	pageObjNums := make([]int, pageCount)
	for i := 0; i < pageCount; i++ {
		pageObjNums[i] = objNum
		objNum++
	}

	// Write Catalog
	offsets = append(offsets, b.Len())
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Catalog /Pages %d 0 R >>\nendobj\n", catalogObj, pagesObj))

	// Write Pages
	offsets = append(offsets, b.Len())
	kidsParts := make([]string, pageCount)
	for i := 0; i < pageCount; i++ {
		kidsParts[i] = fmt.Sprintf("%d 0 R", pageObjNums[i])
	}
	b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d /MediaBox [0 0 612 792] >>\nendobj\n",
		pagesObj, strings.Join(kidsParts, " "), pageCount))

	// Write each Page
	for i := 0; i < pageCount; i++ {
		offsets = append(offsets, b.Len())
		b.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent %d 0 R >>\nendobj\n",
			pageObjNums[i], pagesObj))
	}

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
	require.NoError(t, err, "writeTestPDFWithPages: failed to write %s", outPath)
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

func TestPDFGenerator_Generate_Chapters(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDFWithPages(t, srcPath, 5, map[string]string{"Title": "Test Book"})

	destPath := filepath.Join(tmpDir, "dest.pdf")

	startPage0 := 0
	startPage2 := 2
	startPage4 := 4
	book := &models.Book{Title: "Test Book"}
	file := &models.File{
		FileType: models.FileTypePDF,
		Chapters: []*models.Chapter{
			{Title: "Introduction", SortOrder: 0, StartPage: &startPage0},
			{Title: "Main Content", SortOrder: 1, StartPage: &startPage2},
			{Title: "Conclusion", SortOrder: 2, StartPage: &startPage4},
		},
	}

	gen := &PDFGenerator{}
	err := gen.Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Extract outline from dest and verify the bookmarks match file.Chapters.
	entries, err := pdf.ExtractOutline(destPath)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, "Introduction", entries[0].Title)
	assert.Equal(t, 0, entries[0].StartPage)
	assert.Equal(t, "Main Content", entries[1].Title)
	assert.Equal(t, 2, entries[1].StartPage)
	assert.Equal(t, "Conclusion", entries[2].Title)
	assert.Equal(t, 4, entries[2].StartPage)
}

func TestPDFGenerator_Generate_Chapters_SortOrder(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDFWithPages(t, srcPath, 5, nil)

	destPath := filepath.Join(tmpDir, "dest.pdf")

	// Chapters provided in the wrong order should be written in SortOrder order.
	startPage0 := 0
	startPage2 := 2
	startPage4 := 4
	book := &models.Book{Title: "Test"}
	file := &models.File{
		FileType: models.FileTypePDF,
		Chapters: []*models.Chapter{
			{Title: "Third", SortOrder: 2, StartPage: &startPage4},
			{Title: "First", SortOrder: 0, StartPage: &startPage0},
			{Title: "Second", SortOrder: 1, StartPage: &startPage2},
		},
	}

	err := (&PDFGenerator{}).Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	entries, err := pdf.ExtractOutline(destPath)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, "First", entries[0].Title)
	assert.Equal(t, "Second", entries[1].Title)
	assert.Equal(t, "Third", entries[2].Title)
}

func TestPDFGenerator_Generate_Chapters_Nested(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDFWithPages(t, srcPath, 6, nil)

	destPath := filepath.Join(tmpDir, "dest.pdf")

	startPage0 := 0
	startPage1 := 1
	startPage3 := 3
	startPage4 := 4
	book := &models.Book{Title: "Test"}
	file := &models.File{
		FileType: models.FileTypePDF,
		Chapters: []*models.Chapter{
			{
				Title:     "Part 1",
				SortOrder: 0,
				StartPage: &startPage0,
				Children: []*models.Chapter{
					{Title: "Part 1.1", SortOrder: 0, StartPage: &startPage1},
				},
			},
			{
				Title:     "Part 2",
				SortOrder: 1,
				StartPage: &startPage3,
				Children: []*models.Chapter{
					{Title: "Part 2.1", SortOrder: 0, StartPage: &startPage4},
				},
			},
		},
	}

	err := (&PDFGenerator{}).Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// ExtractOutline flattens nested bookmarks — we expect all four to show up.
	entries, err := pdf.ExtractOutline(destPath)
	require.NoError(t, err)
	require.Len(t, entries, 4)
	assert.Equal(t, "Part 1", entries[0].Title)
	assert.Equal(t, 0, entries[0].StartPage)
	assert.Equal(t, "Part 1.1", entries[1].Title)
	assert.Equal(t, 1, entries[1].StartPage)
	assert.Equal(t, "Part 2", entries[2].Title)
	assert.Equal(t, 3, entries[2].StartPage)
	assert.Equal(t, "Part 2.1", entries[3].Title)
	assert.Equal(t, 4, entries[3].StartPage)
}

func TestPDFGenerator_Generate_Chapters_FiltersInvalid(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDFWithPages(t, srcPath, 3, nil)

	destPath := filepath.Join(tmpDir, "dest.pdf")

	startPage0 := 0
	startPageOOB := 99 // beyond PageCount
	pageCount := 3
	book := &models.Book{Title: "Test"}
	file := &models.File{
		FileType:  models.FileTypePDF,
		PageCount: &pageCount,
		Chapters: []*models.Chapter{
			{Title: "Valid", SortOrder: 0, StartPage: &startPage0},
			{Title: "Missing Page", SortOrder: 1, StartPage: nil},
			{Title: "Out of Range", SortOrder: 2, StartPage: &startPageOOB},
		},
	}

	err := (&PDFGenerator{}).Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	entries, err := pdf.ExtractOutline(destPath)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the valid chapter should be written")
	assert.Equal(t, "Valid", entries[0].Title)
}

func TestPDFGenerator_Generate_NoChapters_LeavesSourceBookmarks(t *testing.T) {
	t.Parallel()

	// An empty Chapters slice should skip the bookmark write entirely (no-op)
	// rather than clearing bookmarks. This covers the case where a caller
	// passes a file model without chapter relations loaded.
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDFWithPages(t, srcPath, 2, nil)

	destPath := filepath.Join(tmpDir, "dest.pdf")

	book := &models.Book{Title: "Test"}
	file := &models.File{FileType: models.FileTypePDF, Chapters: nil}

	err := (&PDFGenerator{}).Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	// Source PDF had no bookmarks, dest should also have none.
	entries, err := pdf.ExtractOutline(destPath)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestPDFGenerator_Generate_Chapters_SourceUnmodified(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.pdf")
	writeTestPDFWithPages(t, srcPath, 3, nil)

	srcBefore, err := os.ReadFile(srcPath)
	require.NoError(t, err)

	destPath := filepath.Join(tmpDir, "dest.pdf")

	startPage0 := 0
	book := &models.Book{Title: "Test"}
	file := &models.File{
		FileType: models.FileTypePDF,
		Chapters: []*models.Chapter{
			{Title: "Chapter 1", SortOrder: 0, StartPage: &startPage0},
		},
	}

	err = (&PDFGenerator{}).Generate(context.Background(), srcPath, destPath, book, file)
	require.NoError(t, err)

	srcAfter, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, srcBefore, srcAfter, "source file must not be modified when writing chapters")
}

func TestConvertModelChaptersToPDFBookmarks(t *testing.T) {
	t.Parallel()

	startPage0 := 0
	startPage2 := 2
	startPage5 := 5
	startPageNeg := -1

	t.Run("empty input returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, convertModelChaptersToPDFBookmarks(nil, 10))
		assert.Nil(t, convertModelChaptersToPDFBookmarks([]*models.Chapter{}, 10))
	})

	t.Run("converts 0-indexed to 1-indexed pages", func(t *testing.T) {
		t.Parallel()
		result := convertModelChaptersToPDFBookmarks([]*models.Chapter{
			{Title: "A", SortOrder: 0, StartPage: &startPage0},
			{Title: "B", SortOrder: 1, StartPage: &startPage2},
		}, 10)
		require.Len(t, result, 2)
		assert.Equal(t, "A", result[0].Title)
		assert.Equal(t, 1, result[0].PageFrom)
		assert.Equal(t, "B", result[1].Title)
		assert.Equal(t, 3, result[1].PageFrom)
	})

	t.Run("drops chapters with nil StartPage", func(t *testing.T) {
		t.Parallel()
		result := convertModelChaptersToPDFBookmarks([]*models.Chapter{
			{Title: "A", SortOrder: 0, StartPage: &startPage0},
			{Title: "B", SortOrder: 1, StartPage: nil},
		}, 10)
		require.Len(t, result, 1)
		assert.Equal(t, "A", result[0].Title)
	})

	t.Run("drops chapters with negative StartPage", func(t *testing.T) {
		t.Parallel()
		result := convertModelChaptersToPDFBookmarks([]*models.Chapter{
			{Title: "Bad", SortOrder: 0, StartPage: &startPageNeg},
			{Title: "Good", SortOrder: 1, StartPage: &startPage0},
		}, 10)
		require.Len(t, result, 1)
		assert.Equal(t, "Good", result[0].Title)
	})

	t.Run("drops chapters beyond pageCount", func(t *testing.T) {
		t.Parallel()
		result := convertModelChaptersToPDFBookmarks([]*models.Chapter{
			{Title: "In range", SortOrder: 0, StartPage: &startPage2},
			{Title: "Out of range", SortOrder: 1, StartPage: &startPage5},
		}, 3)
		require.Len(t, result, 1)
		assert.Equal(t, "In range", result[0].Title)
	})

	t.Run("pageCount 0 disables upper-bound filter", func(t *testing.T) {
		t.Parallel()
		result := convertModelChaptersToPDFBookmarks([]*models.Chapter{
			{Title: "A", SortOrder: 0, StartPage: &startPage5},
		}, 0)
		require.Len(t, result, 1)
		assert.Equal(t, 6, result[0].PageFrom)
	})

	t.Run("preserves nested chapters", func(t *testing.T) {
		t.Parallel()
		result := convertModelChaptersToPDFBookmarks([]*models.Chapter{
			{
				Title:     "Parent",
				SortOrder: 0,
				StartPage: &startPage0,
				Children: []*models.Chapter{
					{Title: "Child", SortOrder: 0, StartPage: &startPage2},
				},
			},
		}, 10)
		require.Len(t, result, 1)
		assert.Equal(t, "Parent", result[0].Title)
		require.Len(t, result[0].Kids, 1)
		assert.Equal(t, "Child", result[0].Kids[0].Title)
		assert.Equal(t, 3, result[0].Kids[0].PageFrom)
	})

	t.Run("sorts by SortOrder", func(t *testing.T) {
		t.Parallel()
		result := convertModelChaptersToPDFBookmarks([]*models.Chapter{
			{Title: "Third", SortOrder: 2, StartPage: &startPage5},
			{Title: "First", SortOrder: 0, StartPage: &startPage0},
			{Title: "Second", SortOrder: 1, StartPage: &startPage2},
		}, 10)
		require.Len(t, result, 3)
		assert.Equal(t, "First", result[0].Title)
		assert.Equal(t, "Second", result[1].Title)
		assert.Equal(t, "Third", result[2].Title)
	})
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
