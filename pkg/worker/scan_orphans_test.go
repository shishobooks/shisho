package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/robinjoseph08/golib/logger"
	"github.com/shishobooks/shisho/internal/testgen"
	"github.com/shishobooks/shisho/pkg/libraries"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func TestCleanupOrphanedFiles_PartialOrphan(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	// Create library with temp path
	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	// Create a book dir with 2 EPUB files
	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Two Files")
	testgen.GenerateEPUB(t, bookDir, "keep.epub", testgen.EPUBOptions{
		Title:   "Two Files",
		Authors: []string{"Author"},
	})
	testgen.GenerateEPUB(t, bookDir, "remove.epub", testgen.EPUBOptions{
		Title:   "Two Files",
		Authors: []string{"Author"},
	})

	// Run initial scan
	err := tc.runScan()
	require.NoError(t, err)

	allBooks := tc.listBooks()
	require.Len(t, allBooks, 1)
	files := tc.listFiles()
	require.Len(t, files, 2)

	// Identify which file to orphan
	var keepFile, removeFile *models.File
	for _, f := range files {
		if filepath.Base(f.Filepath) == "keep.epub" {
			keepFile = f
		} else {
			removeFile = f
		}
	}
	require.NotNil(t, keepFile)
	require.NotNil(t, removeFile)

	// Build existingFiles (what ListFilesForLibrary returns — main files)
	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)
	require.Len(t, existingFiles, 2)

	// Build scannedPaths — only include the file we're keeping (simulates remove.epub being deleted from disk)
	scannedPaths := map[string]struct{}{
		keepFile.Filepath: {},
	}

	// Get library for the method call
	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	// Run orphan cleanup
	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Verify: removed file is gone, kept file remains
	remainingFiles := tc.listFiles()
	require.Len(t, remainingFiles, 1)
	assert.Equal(t, keepFile.ID, remainingFiles[0].ID)

	// Verify: book still exists
	remainingBooks := tc.listBooks()
	require.Len(t, remainingBooks, 1)
}

func TestCleanupOrphanedFiles_FullOrphan_NoSupplements(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Single File")
	testgen.GenerateEPUB(t, bookDir, "only.epub", testgen.EPUBOptions{
		Title:   "Single File",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)
	require.Len(t, tc.listBooks(), 1)
	require.Len(t, tc.listFiles(), 1)

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)

	// No files in scannedPaths = all orphaned
	scannedPaths := map[string]struct{}{}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Book and file should both be deleted
	assert.Empty(t, tc.listBooks())
	assert.Empty(t, tc.listFiles())
}

func TestCleanupOrphanedFiles_MissingParentBookDeletesOrphanedRows(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Missing Parent")
	testgen.GenerateEPUB(t, bookDir, "orphan.epub", testgen.EPUBOptions{
		Title:   "Missing Parent",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)
	require.Len(t, tc.listBooks(), 1)
	require.Len(t, tc.listFiles(), 1)

	book := tc.listBooks()[0]
	file := tc.listFiles()[0]
	now := time.Now()

	person := &models.Person{
		LibraryID:      book.LibraryID,
		Name:           "Dangling Author",
		SortName:       "Author, Dangling",
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = tc.db.NewInsert().Model(person).Exec(tc.ctx)
	require.NoError(t, err)

	author := &models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: 1}
	_, err = tc.db.NewInsert().Model(author).Exec(tc.ctx)
	require.NoError(t, err)

	genre := &models.Genre{LibraryID: book.LibraryID, Name: "Dangling Genre", CreatedAt: now, UpdatedAt: now}
	_, err = tc.db.NewInsert().Model(genre).Exec(tc.ctx)
	require.NoError(t, err)
	_, err = tc.db.NewInsert().Model(&models.BookGenre{BookID: book.ID, GenreID: genre.ID}).Exec(tc.ctx)
	require.NoError(t, err)

	tag := &models.Tag{LibraryID: book.LibraryID, Name: "dangling-tag", CreatedAt: now, UpdatedAt: now}
	_, err = tc.db.NewInsert().Model(tag).Exec(tc.ctx)
	require.NoError(t, err)
	_, err = tc.db.NewInsert().Model(&models.BookTag{BookID: book.ID, TagID: tag.ID}).Exec(tc.ctx)
	require.NoError(t, err)

	series := &models.Series{
		LibraryID:      book.LibraryID,
		Name:           "Dangling Series",
		NameSource:     models.DataSourceFilepath,
		SortName:       "Dangling Series",
		SortNameSource: models.DataSourceFilepath,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	_, err = tc.db.NewInsert().Model(series).Exec(tc.ctx)
	require.NoError(t, err)
	_, err = tc.db.NewInsert().Model(&models.BookSeries{BookID: book.ID, SeriesID: series.ID, SortOrder: 1}).Exec(tc.ctx)
	require.NoError(t, err)

	chapter := &models.Chapter{FileID: file.ID, Title: "Dangling Chapter", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	_, err = tc.db.NewInsert().Model(chapter).Exec(tc.ctx)
	require.NoError(t, err)
	identifier := &models.FileIdentifier{
		FileID:    file.ID,
		Type:      models.IdentifierTypeISBN13,
		Value:     "9781234567890",
		Source:    models.DataSourceFileMetadata,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = tc.db.NewInsert().Model(identifier).Exec(tc.ctx)
	require.NoError(t, err)
	_, err = tc.db.NewInsert().Model(&models.FileFingerprint{
		FileID:    file.ID,
		Algorithm: models.FingerprintAlgorithmSHA256,
		Value:     "dangling-fingerprint",
		CreatedAt: now,
	}).Exec(tc.ctx)
	require.NoError(t, err)

	require.NoError(t, deleteBookWithoutCascades(tc.ctx, tc.db, book.ID))
	assert.Positive(t, foreignKeyViolationCount(tc.ctx, t, tc.db))

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, book.LibraryID)
	require.NoError(t, err)
	require.Len(t, existingFiles, 1)

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: &book.LibraryID})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, map[string]struct{}{}, library, jobLog)

	assert.Empty(t, tc.listBooks())
	assert.Empty(t, tc.listFiles())
	assertNoRowsForColumn(tc.ctx, t, tc.db, "chapters", "file_id", file.ID)
	assertNoRowsForColumn(tc.ctx, t, tc.db, "file_identifiers", "file_id", file.ID)
	assertNoRowsForColumn(tc.ctx, t, tc.db, "file_fingerprints", "file_id", file.ID)
	assertNoRowsForColumn(tc.ctx, t, tc.db, "authors", "book_id", book.ID)
	assertNoRowsForColumn(tc.ctx, t, tc.db, "book_genres", "book_id", book.ID)
	assertNoRowsForColumn(tc.ctx, t, tc.db, "book_tags", "book_id", book.ID)
	assertNoRowsForColumn(tc.ctx, t, tc.db, "book_series", "book_id", book.ID)
	assertNoForeignKeyViolations(tc.ctx, t, tc.db)
}

func TestCleanupOrphanedFiles_MissingParentBookWithScannedMainFileDeletesAllRows(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Missing Parent Mixed")
	testgen.GenerateEPUB(t, bookDir, "keep.epub", testgen.EPUBOptions{
		Title:   "Missing Parent Mixed",
		Authors: []string{"Author"},
	})
	testgen.GenerateEPUB(t, bookDir, "remove.epub", testgen.EPUBOptions{
		Title:   "Missing Parent Mixed",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)
	require.Len(t, tc.listBooks(), 1)
	require.Len(t, tc.listFiles(), 2)

	book := tc.listBooks()[0]
	files := tc.listFiles()

	var keepFile, removeFile *models.File
	for _, file := range files {
		switch filepath.Base(file.Filepath) {
		case "keep.epub":
			keepFile = file
		case "remove.epub":
			removeFile = file
		}
	}
	require.NotNil(t, keepFile)
	require.NotNil(t, removeFile)

	now := time.Now()
	chapter := &models.Chapter{FileID: keepFile.ID, Title: "Still Scanned", SortOrder: 0, CreatedAt: now, UpdatedAt: now}
	_, err = tc.db.NewInsert().Model(chapter).Exec(tc.ctx)
	require.NoError(t, err)

	require.NoError(t, deleteBookWithoutCascades(tc.ctx, tc.db, book.ID))
	assert.Positive(t, foreignKeyViolationCount(tc.ctx, t, tc.db))

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, book.LibraryID)
	require.NoError(t, err)
	require.Len(t, existingFiles, 2)

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: &book.LibraryID})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, map[string]struct{}{keepFile.Filepath: {}}, library, jobLog)

	assert.Empty(t, tc.listBooks())
	assert.Empty(t, tc.listFiles())
	assertNoRowsForColumn(tc.ctx, t, tc.db, "chapters", "file_id", keepFile.ID)
	assertNoForeignKeyViolations(tc.ctx, t, tc.db)
}

func TestProcessScanJob_MissingParentBookWithScannedMainFileRecreatesLiveFile(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Missing Parent Rescan")
	testgen.GenerateEPUB(t, bookDir, "keep.epub", testgen.EPUBOptions{
		Title:   "Missing Parent Rescan",
		Authors: []string{"Author"},
	})
	testgen.GenerateEPUB(t, bookDir, "remove.epub", testgen.EPUBOptions{
		Title:   "Missing Parent Rescan",
		Authors: []string{"Author"},
	})

	require.NoError(t, tc.runScan())
	require.Len(t, tc.listBooks(), 1)
	require.Len(t, tc.listFiles(), 2)

	book := tc.listBooks()[0]
	files := tc.listFiles()

	var keepPath, removePath string
	for _, file := range files {
		switch filepath.Base(file.Filepath) {
		case "keep.epub":
			keepPath = file.Filepath
		case "remove.epub":
			removePath = file.Filepath
		}
	}
	require.NotEmpty(t, keepPath)
	require.NotEmpty(t, removePath)

	require.NoError(t, deleteBookWithoutCascades(tc.ctx, tc.db, book.ID))
	require.NoError(t, os.Remove(removePath))

	require.NoError(t, tc.runScan())

	booksAfter := tc.listBooks()
	require.Len(t, booksAfter, 1)

	filesAfter := tc.listFiles()
	require.Len(t, filesAfter, 1)
	assert.Equal(t, keepPath, filesAfter[0].Filepath)
	assert.Equal(t, booksAfter[0].ID, filesAfter[0].BookID)
	assertNoForeignKeyViolations(tc.ctx, t, tc.db)
}

func TestProcessScanJob_MissingParentBookAllMainFilesStillOnDiskRecreatesBook(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Missing Parent Healthy Files")
	testgen.GenerateEPUB(t, bookDir, "first.epub", testgen.EPUBOptions{
		Title:   "Missing Parent Healthy Files",
		Authors: []string{"Author"},
	})
	testgen.GenerateEPUB(t, bookDir, "second.epub", testgen.EPUBOptions{
		Title:   "Missing Parent Healthy Files",
		Authors: []string{"Author"},
	})

	require.NoError(t, tc.runScan())
	require.Len(t, tc.listBooks(), 1)
	require.Len(t, tc.listFiles(), 2)

	book := tc.listBooks()[0]
	require.NoError(t, deleteBookWithoutCascades(tc.ctx, tc.db, book.ID))

	require.NoError(t, tc.runScan())

	booksAfter := tc.listBooks()
	require.Len(t, booksAfter, 1)

	filesAfter := tc.listFiles()
	require.Len(t, filesAfter, 2)
	for _, file := range filesAfter {
		assert.Equal(t, booksAfter[0].ID, file.BookID)
	}
	assertNoForeignKeyViolations(tc.ctx, t, tc.db)
}

func TestProcessScanJob_MissingParentBookWithLiveScannableSupplementPreservesRole(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Missing Parent Supplement")
	testgen.GenerateEPUB(t, bookDir, "main.epub", testgen.EPUBOptions{
		Title:   "Missing Parent Supplement",
		Authors: []string{"Author"},
	})

	require.NoError(t, tc.runScan())
	require.Len(t, tc.listBooks(), 1)
	require.Len(t, tc.listFiles(), 1)

	book := tc.listBooks()[0]
	mainFile := tc.listFiles()[0]

	supplementPath := filepath.Join(bookDir, "supplement.epub")
	testgen.GenerateEPUB(t, bookDir, "supplement.epub", testgen.EPUBOptions{
		Title:   "Supplement Payload",
		Authors: []string{"Author"},
	})
	supplement := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		Filepath:      supplementPath,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 123,
	}
	require.NoError(t, tc.bookService.CreateFile(tc.ctx, supplement))

	require.NoError(t, deleteBookWithoutCascades(tc.ctx, tc.db, book.ID))
	require.NoError(t, tc.runScan())

	booksAfter := tc.listBooks()
	require.Len(t, booksAfter, 1)

	filesAfter := tc.listFiles()
	require.Len(t, filesAfter, 2)

	rolesByPath := make(map[string]string, len(filesAfter))
	for _, file := range filesAfter {
		assert.Equal(t, booksAfter[0].ID, file.BookID)
		rolesByPath[file.Filepath] = file.FileRole
	}

	assert.Equal(t, models.FileRoleMain, rolesByPath[mainFile.Filepath])
	assert.Equal(t, models.FileRoleSupplement, rolesByPath[supplementPath])
	assertNoForeignKeyViolations(tc.ctx, t, tc.db)
}

func TestCleanupOrphanedFiles_FullOrphan_PromotesSupplement(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] With Supplement")
	testgen.GenerateEPUB(t, bookDir, "main.epub", testgen.EPUBOptions{
		Title:   "With Supplement",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)
	require.Len(t, tc.listBooks(), 1)

	allBooks := tc.listBooks()
	bookID := allBooks[0].ID

	// Manually add a supplement file in the DB (a CBZ supplement that can be promoted)
	supplement := &models.File{
		LibraryID:     1,
		BookID:        bookID,
		Filepath:      filepath.Join(bookDir, "supplement.cbz"),
		FileType:      models.FileTypeCBZ,
		FileRole:      models.FileRoleSupplement,
		FilesizeBytes: 100,
	}
	err = tc.bookService.CreateFile(tc.ctx, supplement)
	require.NoError(t, err)

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)
	require.Len(t, existingFiles, 1, "only main files are returned")

	// No main files in scannedPaths = all main files orphaned
	scannedPaths := map[string]struct{}{}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Book should still exist
	remainingBooks := tc.listBooks()
	require.Len(t, remainingBooks, 1)

	// Main file should be deleted, supplement should remain and be promoted
	remainingFiles := tc.listFiles()
	require.Len(t, remainingFiles, 1)
	assert.Equal(t, supplement.ID, remainingFiles[0].ID)
	assert.Equal(t, models.FileRoleMain, remainingFiles[0].FileRole)
}

func TestCleanupOrphanedFiles_NoOrphans(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Healthy Book")
	testgen.GenerateEPUB(t, bookDir, "file.epub", testgen.EPUBOptions{
		Title:   "Healthy Book",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)

	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)

	// All files are in scannedPaths — no orphans
	scannedPaths := make(map[string]struct{})
	for _, f := range existingFiles {
		scannedPaths[f.Filepath] = struct{}{}
	}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Everything should remain unchanged
	assert.Len(t, tc.listBooks(), 1)
	assert.Len(t, tc.listFiles(), 1)
}

func TestCleanupOrphanedFiles_FullOrphan_NewMainFileAddedDuringScan(t *testing.T) {
	t.Parallel()
	tc := newTestContext(t)

	libraryPath := testgen.TempLibraryDir(t)
	tc.createLibrary([]string{libraryPath})

	bookDir := testgen.CreateSubDir(t, libraryPath, "[Author] Replaced File")
	testgen.GenerateEPUB(t, bookDir, "original.epub", testgen.EPUBOptions{
		Title:   "Replaced File",
		Authors: []string{"Author"},
	})

	err := tc.runScan()
	require.NoError(t, err)
	require.Len(t, tc.listBooks(), 1)
	require.Len(t, tc.listFiles(), 1)

	allBooks := tc.listBooks()
	bookID := allBooks[0].ID

	// Snapshot existingFiles BEFORE the parallel scan adds a new file.
	// This simulates the pre-scan ListFilesForLibrary call.
	existingFiles, err := tc.bookService.ListFilesForLibrary(tc.ctx, 1)
	require.NoError(t, err)
	require.Len(t, existingFiles, 1)

	// Simulate what the parallel scan does: add a new main file to the same book.
	// In production, this happens when a user replaces original.epub with replacement.epub on disk.
	newFile := &models.File{
		LibraryID:     1,
		BookID:        bookID,
		Filepath:      filepath.Join(bookDir, "replacement.epub"),
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 200,
	}
	err = tc.bookService.CreateFile(tc.ctx, newFile)
	require.NoError(t, err)

	// Original file is NOT in scannedPaths (deleted from disk).
	// replacement.epub IS on disk but was not in existingFiles (added during scan).
	scannedPaths := map[string]struct{}{
		newFile.Filepath: {},
	}

	library, err := tc.libraryService.RetrieveLibrary(tc.ctx, libraries.RetrieveLibraryOptions{ID: intPtr(1)})
	require.NoError(t, err)

	log := logger.FromContext(tc.ctx)
	jobLog := tc.jobLogService.NewJobLogger(tc.ctx, 0, log)
	tc.worker.cleanupOrphanedFiles(tc.ctx, existingFiles, scannedPaths, library, jobLog)

	// Book should survive — it gained a new main file during the scan
	remainingBooks := tc.listBooks()
	require.Len(t, remainingBooks, 1)

	// Original file deleted, new file remains
	remainingFiles := tc.listFiles()
	require.Len(t, remainingFiles, 1)
	assert.Equal(t, newFile.ID, remainingFiles[0].ID)
	assert.Equal(t, models.FileRoleMain, remainingFiles[0].FileRole)
}

func intPtr(i int) *int {
	return &i
}

func deleteBookWithoutCascades(ctx context.Context, db *bun.DB, bookID int) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "DELETE FROM books WHERE id = ?", bookID); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	return err
}

func assertNoRowsForColumn(ctx context.Context, t *testing.T, db *bun.DB, table, column string, value int) {
	t.Helper()

	var count int
	err := db.NewSelect().
		TableExpr(table).
		Where("? = ?", bun.Ident(column), value).
		ColumnExpr("count(*)").
		Scan(ctx, &count)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func assertNoForeignKeyViolations(ctx context.Context, t *testing.T, db *bun.DB) {
	t.Helper()

	assert.Zero(t, foreignKeyViolationCount(ctx, t, db))
}

func foreignKeyViolationCount(ctx context.Context, t *testing.T, db *bun.DB) int {
	t.Helper()

	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	require.NoError(t, err)
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	require.NoError(t, rows.Err())
	return count
}
