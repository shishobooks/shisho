package worker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// emptyFileSupplement returns the single supplement row the scan created.
func emptyFileSupplement(t *testing.T, tc *testContext) *models.File {
	t.Helper()
	var supplements []*models.File
	for _, f := range tc.listFiles() {
		if f.FileRole == models.FileRoleSupplement {
			supplements = append(supplements, f)
		}
	}
	require.Len(t, supplements, 1)
	return supplements[0]
}

// An empty supplement, such as a padding .txt in a book folder, is a real
// file of size 0 and imports like any other supplement.
func TestScan_EmptySupplement_ImportsWithZeroSize(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "My Book"})
	notesPath := filepath.Join(bookDir, "notes.txt")
	require.NoError(t, os.WriteFile(notesPath, nil, 0644))

	require.NoError(t, tc.runScan())

	supplement := emptyFileSupplement(t, tc)
	assert.Equal(t, notesPath, supplement.Filepath)
	assert.Equal(t, int64(0), supplement.FilesizeBytes)
}

// A tracked supplement truncated to 0 bytes must rescan and record its new
// size instead of failing on the files.filesize_bytes NOT NULL constraint.
// The library scan only revisits supplements with a scannable extension, so
// this uses a PDF, inserted the way the scanner creates one.
func TestScan_SupplementTruncatedToEmpty_RecordsZeroSize(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] My Book")
	testgen.GenerateEPUB(t, bookDir, "book.epub", testgen.EPUBOptions{Title: "My Book"})
	require.NoError(t, tc.runScan())
	files := tc.listFiles()
	require.Len(t, files, 1)

	extrasPath := testgen.GeneratePDF(t, bookDir, "extras.pdf", testgen.PDFOptions{PageCount: 2})
	extrasStat, err := os.Stat(extrasPath)
	require.NoError(t, err)
	require.NotZero(t, extrasStat.Size())
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, &models.File{
		LibraryID:     files[0].LibraryID,
		BookID:        files[0].BookID,
		Filepath:      extrasPath,
		FileType:      models.FileTypePDF,
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: extrasStat.Size(),
	}))

	require.NoError(t, os.Truncate(extrasPath, 0))
	require.NoError(t, tc.runScan())

	supplement := emptyFileSupplement(t, tc)
	assert.Equal(t, int64(0), supplement.FilesizeBytes)
	assert.NotNil(t, supplement.FileModifiedAt, "the rescan must record the truncated supplement's mtime")
}

// An empty main file still fails at parsing, so the scan imports nothing for
// it, exactly as before 0-byte sizes could be stored.
func TestScan_EmptyMainFile_IsNotImported(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Empty Book")
	require.NoError(t, os.WriteFile(filepath.Join(bookDir, "book.epub"), nil, 0644))

	require.NoError(t, tc.runScan())

	assert.Empty(t, tc.listFiles())
	assert.Empty(t, tc.listBooks())
}

// Every empty file has the same sha256, so a new empty file says nothing
// about which missing file it replaced. When one Book's empty supplement is
// removed and an unrelated empty file appears in another Book in the same
// batch, move detection must not repoint the removed row at the newcomer.
func TestMonitor_EmptyFile_IsNotDetectedAsMove(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libDir := t.TempDir()
	tc.createLibrary([]string{libDir})
	dirA := testgen.CreateSubDir(t, libDir, "[Author] Book A")
	testgen.GenerateEPUB(t, dirA, "book.epub", testgen.EPUBOptions{Title: "Book A"})
	dirB := testgen.CreateSubDir(t, libDir, "[Author] Book B")
	testgen.GenerateEPUB(t, dirB, "book.epub", testgen.EPUBOptions{Title: "Book B"})
	require.NoError(t, tc.runScan())
	mainFiles := tc.listFiles()
	require.Len(t, mainFiles, 2)
	bookAID := mainFiles[0].BookID
	if mainFiles[1].Filepath == filepath.Join(dirA, "book.epub") {
		bookAID = mainFiles[1].BookID
	}

	// Book A has a fingerprinted empty supplement.
	removedPath := filepath.Join(dirA, "extras.pdf")
	require.NoError(t, os.WriteFile(removedPath, nil, 0644))
	hash, err := computeFileSHA256(removedPath)
	require.NoError(t, err)
	removed := &models.File{
		LibraryID: mainFiles[0].LibraryID,
		BookID:    bookAID,
		Filepath:  removedPath,
		FileType:  models.FileTypePDF,
		FileRole:  models.FileRoleSupplement,
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, removed))
	require.NoError(t, tc.fingerprintService.Insert(tc.ctx, removed.ID, models.FingerprintAlgorithmSHA256, hash))

	// In one batch, Book A's empty supplement goes away and an unrelated
	// empty file appears in Book B.
	m, libID := newTestMonitorWithWorker(tc, libDir)
	require.NoError(t, os.Remove(removedPath))
	createdPath := filepath.Join(dirB, "extras.pdf")
	require.NoError(t, os.WriteFile(createdPath, nil, 0644))
	injectMonitorEvent(m, removedPath, fsnotify.Remove, libID, false)
	injectMonitorEvent(m, createdPath, fsnotify.Create, libID, false)
	m.processPendingEvents()

	for _, f := range tc.listFiles() {
		if f.ID == removed.ID {
			assert.NotEqual(t, createdPath, f.Filepath,
				"Book A's removed empty supplement must not be repointed at Book B's new empty file")
		}
		if f.Filepath == createdPath {
			assert.NotEqual(t, bookAID, f.BookID, "Book B's new empty file must not belong to Book A")
		}
	}
}
