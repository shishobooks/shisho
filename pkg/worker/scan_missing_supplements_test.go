package worker

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover a full scan removing supplement rows whose files are gone
// from disk (#625).

// missingSupplementFileIDsByName returns the IDs of the library's files keyed
// by base name, failing the test on a duplicate name.
func (tc *testContext) missingSupplementFileIDsByName() map[string]int {
	tc.t.Helper()
	ids := make(map[string]int)
	for _, f := range tc.listFiles() {
		name := filepath.Base(f.Filepath)
		require.NotContains(tc.t, ids, name, "duplicate file name %s", name)
		ids[name] = f.ID
	}
	return ids
}

// missingSupplementFileByName returns the library file with the given base
// name, or nil.
func (tc *testContext) missingSupplementFileByName(name string) *models.File {
	tc.t.Helper()
	for _, f := range tc.listFiles() {
		if filepath.Base(f.Filepath) == name {
			return f
		}
	}
	return nil
}

// missingSupplementBookFilenames returns the filenames column of a Book's
// search index row.
func (tc *testContext) missingSupplementBookFilenames(bookID int) string {
	tc.t.Helper()
	var filenames string
	err := tc.db.NewRaw("SELECT filenames FROM books_fts WHERE rowid = ?", bookID).Scan(tc.ctx, &filenames)
	require.NoError(tc.t, err)
	return filenames
}

func TestProcessScanJob_MissingTextSupplement_RemovesOnlyItsRow(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{Title: "My Book"})
	companionPath := filepath.Join(bookDir, "companion.txt")
	require.NoError(t, os.WriteFile(companionPath, []byte("Companion content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "notes.txt"), []byte("Notes content"), 0o644))

	require.NoError(t, tc.runScan())
	require.Len(t, tc.listBooks(), 1)
	before := tc.missingSupplementFileIDsByName()
	require.Len(t, before, 3)

	require.NoError(t, os.Remove(companionPath))
	require.NoError(t, tc.runScan())

	after := tc.missingSupplementFileIDsByName()
	assert.Equal(t, map[string]int{
		"book.m4b":  before["book.m4b"],
		"notes.txt": before["notes.txt"],
	}, after)
	books := tc.listBooks()
	require.Len(t, books, 1)
	assert.Equal(t, models.FileRoleSupplement, tc.missingSupplementFileByName("notes.txt").FileRole)

	filenames := tc.missingSupplementBookFilenames(books[0].ID)
	assert.NotContains(t, filenames, companionPath, "the search index must drop the removed supplement's path")
	assert.Contains(t, filenames, filepath.Join(bookDir, "notes.txt"))
}

// A supplement with a scannable extension is removed too, while a sibling of
// the same type that is still on disk is kept.
func TestProcessScanJob_MissingPDFSupplement_Removed(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] PDF Extras")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "PDF Extras", Authors: []string{"Author"}})
	bonusPath := testgen.GeneratePDF(t, bookDir, "bonus.pdf", testgen.PDFOptions{})
	testgen.GeneratePDF(t, bookDir, "booklet.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())
	before := tc.missingSupplementFileIDsByName()
	require.Len(t, before, 3)
	require.Equal(t, models.FileRoleSupplement, tc.missingSupplementFileByName("bonus.pdf").FileRole)
	require.Equal(t, models.FileRoleSupplement, tc.missingSupplementFileByName("booklet.pdf").FileRole)

	require.NoError(t, os.Remove(bonusPath))
	require.NoError(t, tc.runScan())

	assert.Equal(t, map[string]int{
		"book.epub":   before["book.epub"],
		"booklet.pdf": before["booklet.pdf"],
	}, tc.missingSupplementFileIDsByName())
	assert.Equal(t, models.FileRoleSupplement, tc.missingSupplementFileByName("booklet.pdf").FileRole)
	assert.Len(t, tc.listBooks(), 1)
}

// Supplements still on disk are never removed, whatever their extension.
// Deciding "missing" from the scannable-file list deletes every non-scannable
// supplement on every scan.
func TestProcessScanJob_PresentSupplementsSurviveRepeatedScans(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Kept Extras")
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{Title: "Kept Extras"})
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "notes.txt"), []byte("notes"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "map.jpg"), []byte("not really a jpeg"), 0o644))
	extrasDir := testgen.CreateSubDir(t, bookDir, "extras")
	require.NoError(t, os.WriteFile(filepath.Join(extrasDir, "track list.txt"), []byte("tracks"), 0o644))
	testgen.GeneratePDF(t, bookDir, "booklet.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())
	before := tc.missingSupplementFileIDsByName()
	require.Len(t, before, 5)

	for range 3 {
		require.NoError(t, tc.runScan())
		assert.Equal(t, before, tc.missingSupplementFileIDsByName())
	}
	for _, f := range tc.listFiles() {
		if filepath.Base(f.Filepath) != "book.m4b" {
			assert.Equal(t, models.FileRoleSupplement, f.FileRole, f.Filepath)
		}
	}
}

// When every main file is gone, a promotable supplement that is also gone
// must not be promoted: the Book is deleted as if it had no supplements.
func TestProcessScanJob_MissingSupplementNotPromoted(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] All Gone")
	epubPath := testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "All Gone", Authors: []string{"Author"}})
	bonusPath := testgen.GeneratePDF(t, bookDir, "bonus.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())
	require.Len(t, tc.listBooks(), 1)
	require.Equal(t, models.FileRoleSupplement, tc.missingSupplementFileByName("bonus.pdf").FileRole)

	require.NoError(t, os.Remove(epubPath))
	require.NoError(t, os.Remove(bonusPath))
	require.NoError(t, tc.runScan())

	assert.Empty(t, tc.listBooks())
	assert.Empty(t, tc.listFiles())
}

// The promotion path still works when the supplement is on disk.
func TestProcessScanJob_PresentSupplementPromotedWhenMainFilesGone(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Main Gone")
	epubPath := testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Main Gone", Authors: []string{"Author"}})
	testgen.GeneratePDF(t, bookDir, "bonus.pdf", testgen.PDFOptions{})

	require.NoError(t, tc.runScan())
	bonus := tc.missingSupplementFileByName("bonus.pdf")
	require.NotNil(t, bonus)
	require.Equal(t, models.FileRoleSupplement, bonus.FileRole)

	require.NoError(t, os.Remove(epubPath))
	require.NoError(t, tc.runScan())

	require.Len(t, tc.listBooks(), 1)
	files := tc.listFiles()
	require.Len(t, files, 1)
	assert.Equal(t, bonus.ID, files[0].ID)
	assert.Equal(t, models.FileRoleMain, files[0].FileRole)
}

// Only a definite not-exist counts as missing. Any other stat failure, such as
// a permission error, leaves the row alone.
func TestProcessScanJob_SupplementStatFailureKeepsRow(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Unreadable")
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{Title: "Unreadable"})
	companionPath := filepath.Join(bookDir, "companion.txt")
	require.NoError(t, os.WriteFile(companionPath, []byte("Companion content"), 0o644))

	require.NoError(t, tc.runScan())
	before := tc.missingSupplementFileIDsByName()
	require.Len(t, before, 2)

	// The file is gone, but stat fails with something other than not-exist.
	require.NoError(t, os.Remove(companionPath))
	tc.worker.statFile = func(name string) (fs.FileInfo, error) {
		if name == companionPath {
			return nil, &fs.PathError{Op: "stat", Path: name, Err: syscall.EACCES}
		}
		return os.Stat(name)
	}
	require.NoError(t, tc.runScan())

	assert.Equal(t, before, tc.missingSupplementFileIDsByName())
}

// Supplement discovery runs only when a main file is first imported, so a
// Book that loses a missing supplement is searched again for supplements.
// Otherwise a renamed supplement would drop off the Book for good.
func TestProcessScanJob_RenamedSupplementRediscovered(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Renamed Notes")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Renamed Notes", Authors: []string{"Author"}})
	oldPath := filepath.Join(bookDir, "notes.txt")
	require.NoError(t, os.WriteFile(oldPath, []byte("notes"), 0o644))

	require.NoError(t, tc.runScan())
	before := tc.missingSupplementFileIDsByName()
	require.Len(t, before, 2)

	require.NoError(t, os.Rename(oldPath, filepath.Join(bookDir, "notes v2.txt")))
	require.NoError(t, tc.runScan())

	after := tc.missingSupplementFileIDsByName()
	require.Len(t, after, 2)
	assert.Equal(t, before["book.epub"], after["book.epub"])
	assert.NotContains(t, after, "notes.txt")
	renamed := tc.missingSupplementFileByName("notes v2.txt")
	require.NotNil(t, renamed)
	assert.Equal(t, models.FileRoleSupplement, renamed.FileRole)
	assert.Equal(t, tc.listBooks()[0].ID, renamed.BookID)
}

// A book folder renamed while Shisho was not watching keeps its main file
// through move reconciliation, and its supplements are found at the new path.
func TestProcessScanJob_MovedBookFolderKeepsSupplements(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	oldDir := testgen.CreateSubDir(t, libraryPath, "[Author] Old Folder")
	epubPath := testgen.GenerateEPUB(t, oldDir, "book.epub", testgen.EPUBOptions{Title: "Old Folder", Authors: []string{"Author"}})
	require.NoError(t, os.WriteFile(filepath.Join(oldDir, "notes.txt"), []byte("notes"), 0o644))

	require.NoError(t, tc.runScan())
	before := tc.missingSupplementFileIDsByName()
	require.Len(t, before, 2)
	hash, err := computeSHA256ForTest(epubPath)
	require.NoError(t, err)
	require.NoError(t, tc.fingerprintService.Insert(tc.ctx, before["book.epub"], models.FingerprintAlgorithmSHA256, hash))

	newDir := filepath.Join(libraryPath, "[Author] New Folder")
	require.NoError(t, os.Rename(oldDir, newDir))
	require.NoError(t, tc.runScan())

	after := tc.missingSupplementFileIDsByName()
	require.Len(t, after, 2)
	assert.Equal(t, before["book.epub"], after["book.epub"], "move reconciliation keeps the main file row")
	notes := tc.missingSupplementFileByName("notes.txt")
	require.NotNil(t, notes)
	assert.Equal(t, filepath.Join(newDir, "notes.txt"), notes.Filepath)
	assert.Equal(t, models.FileRoleSupplement, notes.FileRole)
	assert.Len(t, tc.listBooks(), 1)
}

// A supplement moved after the scan loaded its file list, for example by a
// Book edit that organizes files, is checked at its current path and kept.
func TestCleanupOrphanedFiles_SupplementMovedDuringScanKept(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Moving Notes")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "Moving Notes", Authors: []string{"Author"}})
	oldPath := filepath.Join(bookDir, "notes.txt")
	require.NoError(t, os.WriteFile(oldPath, []byte("notes"), 0o644))
	require.NoError(t, tc.runScan())

	// The main files as the scan loaded them, before the move.
	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)
	notes := tc.missingSupplementFileByName("notes.txt")
	require.NotNil(t, notes)

	newPath := filepath.Join(bookDir, "Moving Notes - notes.txt")
	require.NoError(t, os.Rename(oldPath, newPath))
	_, err = tc.db.NewUpdate().Model((*models.File)(nil)).Set("filepath = ?", newPath).Where("id = ?", notes.ID).Exec(tc.ctx)
	require.NoError(t, err)

	scannedPaths := map[string]struct{}{}
	for _, f := range existingFiles {
		scannedPaths[f.Filepath] = struct{}{}
	}
	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, logger.FromContext(tc.ctx))
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	kept := tc.missingSupplementFileByName("Moving Notes - notes.txt")
	require.NotNil(t, kept, "the moved supplement's row must survive")
	assert.Equal(t, notes.ID, kept.ID)
}

// An unreadable library root, such as an unmounted drive, aborts the scan
// before orphan cleanup, so no supplement is removed.
func TestProcessScanJob_MissingLibraryRootKeepsSupplements(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Unmounted")
	testgen.GenerateM4B(t, bookDir, "book.m4b", testgen.M4BOptions{Title: "Unmounted"})
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "notes.txt"), []byte("notes"), 0o644))

	require.NoError(t, tc.runScan())
	before := tc.missingSupplementFileIDsByName()
	require.Len(t, before, 2)

	unmounted := libraryPath + "-unmounted"
	require.NoError(t, os.Rename(libraryPath, unmounted))
	t.Cleanup(func() { _ = os.Rename(unmounted, libraryPath) })

	require.Error(t, tc.runScan())
	assert.Equal(t, before, tc.missingSupplementFileIDsByName())
	assert.Len(t, tc.listBooks(), 1)
}
