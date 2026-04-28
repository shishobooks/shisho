package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessScanJob_SupplementsInDirectory(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book directory with main file + supplements
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{
		Title:    "My Book",
		HasCover: true,
	})

	// Create supplement files
	supplementTXT1 := filepath.Join(bookDir, "companion.txt")
	require.NoError(t, os.WriteFile(supplementTXT1, []byte("Companion content"), 0644))

	supplementTXT2 := filepath.Join(bookDir, "notes.txt")
	require.NoError(t, os.WriteFile(supplementTXT2, []byte("Notes content"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	// Verify book was created
	books := tc.listBooks()
	require.Len(t, books, 1)

	// Verify files: 1 main + 2 supplements
	files := tc.listFiles()
	require.Len(t, files, 3)

	mainFiles := 0
	supplementFiles := 0
	for _, f := range files {
		switch f.FileRole {
		case models.FileRoleMain:
			mainFiles++
			assert.Equal(t, "m4b", f.FileType)
		case models.FileRoleSupplement:
			supplementFiles++
			assert.Equal(t, "txt", f.FileType)
		}
	}
	assert.Equal(t, 1, mainFiles)
	assert.Equal(t, 2, supplementFiles)
}

func TestProcessScanJob_SupplementsExcludeHiddenFiles(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{})

	// Create hidden file (should be excluded)
	hiddenFile := filepath.Join(bookDir, ".hidden")
	require.NoError(t, os.WriteFile(hiddenFile, []byte("hidden"), 0644))

	// Create .DS_Store (should be excluded)
	dsStore := filepath.Join(bookDir, ".DS_Store")
	require.NoError(t, os.WriteFile(dsStore, []byte("dsstore"), 0644))

	// Create normal supplement (should be included)
	supplement := filepath.Join(bookDir, "guide.txt")
	require.NoError(t, os.WriteFile(supplement, []byte("guide"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	// Should have 2: main epub + guide.txt supplement
	// Hidden files and .DS_Store should be excluded
	require.Len(t, files, 2)

	for _, f := range files {
		assert.NotContains(t, f.Filepath, ".hidden")
		assert.NotContains(t, f.Filepath, ".DS_Store")
	}
}

func TestProcessScanJob_SupplementsExcludeShishoFiles(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{})

	// Create shisho special files (should be excluded)
	coverFile := filepath.Join(bookDir, "book.cover.jpg")
	require.NoError(t, os.WriteFile(coverFile, []byte("cover"), 0644))

	metadataFile := filepath.Join(bookDir, "book.metadata.json")
	require.NoError(t, os.WriteFile(metadataFile, []byte("{}"), 0644))

	// Create normal supplement
	supplement := filepath.Join(bookDir, "appendix.txt")
	require.NoError(t, os.WriteFile(supplement, []byte("appendix"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	// Should have 2: main epub + appendix.txt
	require.Len(t, files, 2)

	for _, f := range files {
		assert.NotContains(t, f.Filepath, ".cover.")
		assert.NotContains(t, f.Filepath, ".metadata.json")
	}
}

func TestProcessScanJob_SupplementsInSubdirectory(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{})

	// Create subdirectory with supplements
	subDir := testgen.CreateSubDir(t, bookDir, "extras")
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "bonus.txt"), []byte("bonus"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "artwork.jpg"), []byte("art"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	files := tc.listFiles()
	// Should have 3: main m4b + 2 supplements in subdirectory
	require.Len(t, files, 3)

	supplementCount := 0
	for _, f := range files {
		if f.FileRole == models.FileRoleSupplement {
			supplementCount++
		}
	}
	assert.Equal(t, 2, supplementCount)
}

func TestProcessScanJob_RootLevelSupplements(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create root-level main file
	testgen.GenerateM4B(t, libraryPath, "My Book.m4b", testgen.M4BOptions{})

	// Create supplement with matching basename
	supplement := filepath.Join(libraryPath, "My Book.txt")
	require.NoError(t, os.WriteFile(supplement, []byte("supplement"), 0644))

	// Create unrelated file (different basename - should NOT be picked up)
	unrelated := filepath.Join(libraryPath, "Other Book.txt")
	require.NoError(t, os.WriteFile(unrelated, []byte("other"), 0644))

	err := tc.runScan()
	require.NoError(t, err)

	// Should have 1 book: "My Book" (Other Book.pdf has no main file so is ignored)
	books := tc.listBooks()
	require.Len(t, books, 1, "Only My Book should exist, Other Book.txt doesn't have a main file")

	files := tc.listFiles()
	// My Book.m4b (main) + My Book.txt (supplement)
	require.Len(t, files, 2)

	mainCount := 0
	suppCount := 0
	for _, f := range files {
		if f.FileRole == models.FileRoleMain {
			mainCount++
		} else {
			suppCount++
		}
	}
	assert.Equal(t, 1, mainCount)
	assert.Equal(t, 1, suppCount)
}

// TestProcessScanJob_ScannableSupplementNotRescannedAsMain reproduces the bug
// where a supplement sharing a scannable extension (e.g. .pdf next to .epub)
// is walked by the scan loop and re-created as a main file, hitting the
// UNIQUE constraint on (filepath, library_id).
//
// Repro path: main EPUB is scanned normally, then a PDF is added to the book
// directory and registered directly in the DB as a supplement (simulating a
// user demoting a file via the file-role update endpoint). A subsequent scan
// must not try to recreate the PDF as a main file.
func TestProcessScanJob_ScannableSupplementNotRescannedAsMain(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Book dir with main EPUB.
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Emily Oster] Cribsheet")
	testgen.GenerateEPUB(t, bookDir, "Cribsheet.epub", testgen.EPUBOptions{
		Title:   "Cribsheet",
		Authors: []string{"Emily Oster"},
	})

	// First scan: creates book + main EPUB.
	require.NoError(t, tc.runScan())

	booksList := tc.listBooks()
	require.Len(t, booksList, 1)
	bookID := booksList[0].ID

	files := tc.listFiles()
	require.Len(t, files, 1)

	// Now add a PDF to disk and register it as a supplement directly in the DB
	// (matches what the file-role update handler does).
	pdfPath := testgen.GeneratePDF(t, bookDir, "Cribsheet.pdf", testgen.PDFOptions{})
	pdfStat, err := os.Stat(pdfPath)
	require.NoError(t, err)

	supplementFile := &models.File{
		LibraryID:     1,
		BookID:        bookID,
		Filepath:      pdfPath,
		FileType:      "pdf",
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: pdfStat.Size(),
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, supplementFile))
	suppID := supplementFile.ID

	require.Len(t, tc.listFiles(), 2)

	// Second scan: the PDF already exists as a supplement. The scan walk
	// previously walked the PDF, missed it in the cache (cache is main-only),
	// and tried to recreate it as a main file — hitting UNIQUE(filepath, library_id).
	log := logger.FromContext(tc.ctx)
	rescanLog := tc.jobLogService.NewJobLogger(tc.ctx, 1, log)
	require.NoError(t, tc.worker.ProcessScanJob(tc.ctx, nil, rescanLog))

	// State should be unchanged: same files with same roles and same IDs.
	afterFiles := tc.listFiles()
	require.Len(t, afterFiles, 2, "rescan should not add or lose any files")

	var foundSupp bool
	for _, f := range afterFiles {
		if filepath.Base(f.Filepath) == "Cribsheet.pdf" {
			assert.Equal(t, models.FileRoleSupplement, f.FileRole, "PDF should still be a supplement after rescan")
			assert.Equal(t, suppID, f.ID, "supplement row should not have been recreated")
			foundSupp = true
		}
	}
	assert.True(t, foundSupp, "supplement PDF row should still exist after rescan")

	// When the bug is present, the scan walks Cribsheet.pdf, calls scanFileCreateNew,
	// parses PDF metadata, and writes Cribsheet.pdf.cover.jpg to disk before the
	// CreateFile UNIQUE constraint aborts the insert. A correct scan must early
	// return for supplements without touching the cover file.
	coverPath := filepath.Join(bookDir, "Cribsheet.pdf.cover.jpg")
	_, err = os.Stat(coverPath)
	assert.True(t, os.IsNotExist(err),
		"rescan should not extract a cover for an existing supplement PDF (cover file at %s)", coverPath)
}

// TestProcessScanJob_PDFNamedSupplementBecomesSupplement covers the core
// scenario: a PDF named "Supplement.pdf" alongside a main EPUB in the same
// directory must be classified as a supplement.
func TestProcessScanJob_PDFNamedSupplementBecomesSupplement(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{
		Title:   "My Book",
		Authors: []string{"Author"},
	})
	testgen.GeneratePDF(t, bookDir, "Supplement.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())

	booksList := tc.listBooks()
	require.Len(t, booksList, 1, "EPUB and supplement PDF should belong to one book")

	files := tc.listFiles()
	require.Len(t, files, 2)

	var mainCount, suppCount int
	for _, f := range files {
		switch f.FileRole {
		case models.FileRoleMain:
			mainCount++
			assert.Equal(t, "epub", f.FileType, "main file should be the EPUB")
		case models.FileRoleSupplement:
			suppCount++
			assert.Equal(t, "pdf", f.FileType, "supplement should be the PDF")
			assert.Equal(t, "Supplement.pdf", filepath.Base(f.Filepath))
		}
	}
	assert.Equal(t, 1, mainCount)
	assert.Equal(t, 1, suppCount)
}

// TestProcessScanJob_PDFAloneInDirStaysMain confirms the safety rule:
// a directory containing only a supplement-named PDF must still import as
// a main file so the book isn't silently dropped.
func TestProcessScanJob_PDFAloneInDirStaysMain(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] PDF Only Book")
	testgen.GeneratePDF(t, bookDir, "Supplement.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())

	require.Len(t, tc.listBooks(), 1)

	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, models.FileRoleMain, files[0].FileRole, "lone supplement-named PDF should be main")
	assert.Equal(t, "pdf", files[0].FileType)
}

// TestProcessScanJob_PDFCaseInsensitiveMatch confirms basename matching
// ignores case for both the filename and the configured names.
func TestProcessScanJob_PDFCaseInsensitiveMatch(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Casing Test")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Casing Test"})
	testgen.GeneratePDF(t, bookDir, "BONUS.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())

	require.Len(t, tc.listBooks(), 1)
	files := tc.listFiles()
	require.Len(t, files, 2)

	for _, f := range files {
		if f.FileType == "pdf" {
			assert.Equal(t, models.FileRoleSupplement, f.FileRole)
		}
	}
}

// TestProcessScanJob_PDFSubstringDoesNotMatch confirms substring matches do
// NOT trigger supplement classification — only exact basename matches.
func TestProcessScanJob_PDFSubstringDoesNotMatch(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Substring Test")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Substring Test"})
	testgen.GeneratePDF(t, bookDir, "My Book - Supplement.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())

	// PDF basename "My Book - Supplement" is NOT an exact match for any list
	// entry, so it stays main. Two main files end up on the same book (the
	// existing scan flow merges files with the same bookPath into one book).
	require.Len(t, tc.listBooks(), 1)
	files := tc.listFiles()
	require.Len(t, files, 2)
	for _, f := range files {
		assert.Equal(t, models.FileRoleMain, f.FileRole, "substring match should not classify as supplement")
	}
}

// TestProcessScanJob_PDFEmptyConfigDisablesFeature confirms setting
// PDFSupplementFilenames to an empty slice disables the auto-classification.
func TestProcessScanJob_PDFEmptyConfigDisablesFeature(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)
	tc.worker.config.PDFSupplementFilenames = nil

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Disabled Test")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Disabled Test"})
	testgen.GeneratePDF(t, bookDir, "Supplement.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 2)
	for _, f := range files {
		assert.Equal(t, models.FileRoleMain, f.FileRole, "feature should be disabled when config is empty")
	}
}

// TestProcessScanJob_PDFExistingMainNotReclassified confirms an existing
// PDF main file whose name happens to match the supplement list is NOT
// reclassified on rescan. The rule only applies at file creation.
func TestProcessScanJob_PDFExistingMainNotReclassified(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// First scan: only the PDF exists, so it imports as main.
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Rescan Test")
	testgen.GeneratePDF(t, bookDir, "Supplement.pdf", testgen.PDFOptions{})
	require.NoError(t, tc.runScan())

	files := tc.listFiles()
	require.Len(t, files, 1)
	require.Equal(t, models.FileRoleMain, files[0].FileRole)
	mainID := files[0].ID

	// Add an EPUB sibling and rescan.
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Rescan Test"})
	require.NoError(t, tc.runScan())

	afterFiles := tc.listFiles()
	require.Len(t, afterFiles, 2)

	for _, f := range afterFiles {
		if f.ID == mainID {
			assert.Equal(t, models.FileRoleMain, f.FileRole, "existing main PDF must not be reclassified on rescan")
		}
	}
}
