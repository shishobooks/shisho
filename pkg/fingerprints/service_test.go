package fingerprints_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/fingerprints"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// newTestDB opens an in-memory SQLite database with all migrations applied.
func newTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	// Enable foreign keys to match production behavior.
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// insertTestLibrary creates a library row and returns it.
func insertTestLibrary(t *testing.T, db *bun.DB, name string) *models.Library {
	t.Helper()
	lib := &models.Library{
		Name:                     name,
		CoverAspectRatio:         "portrait",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)
	return lib
}

// insertTestBook creates a book row for the given library and returns it.
func insertTestBook(t *testing.T, db *bun.DB, lib *models.Library) *models.Book {
	t.Helper()
	book := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err := db.NewInsert().Model(book).Exec(context.Background())
	require.NoError(t, err)
	return book
}

// insertTestFile creates a file row for the given book and returns it.
func insertTestFile(t *testing.T, db *bun.DB, book *models.Book) *models.File {
	t.Helper()
	file := &models.File{
		LibraryID:     book.LibraryID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/library/book.epub",
		FilesizeBytes: 1024,
	}
	_, err := db.NewInsert().Model(file).Exec(context.Background())
	require.NoError(t, err)
	return file
}

// TestInsert_NewFingerprint verifies that Insert stores a fingerprint row.
func TestInsert_NewFingerprint(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")
	book := insertTestBook(t, db, lib)
	file := insertTestFile(t, db, book)

	err := svc.Insert(ctx, file.ID, models.FingerprintAlgorithmSHA256, "abc123")
	require.NoError(t, err)

	count, err := svc.CountForFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// TestInsert_Idempotent verifies that calling Insert twice with the same
// (file_id, algorithm) pair does not error and does not create a duplicate row.
func TestInsert_Idempotent(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")
	book := insertTestBook(t, db, lib)
	file := insertTestFile(t, db, book)

	err := svc.Insert(ctx, file.ID, models.FingerprintAlgorithmSHA256, "abc123")
	require.NoError(t, err)

	// Second insert with same (file_id, algorithm) should be a no-op.
	err = svc.Insert(ctx, file.ID, models.FingerprintAlgorithmSHA256, "abc123")
	require.NoError(t, err)

	count, err := svc.CountForFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "duplicate insert should not create a second row")
}

// TestInsert_ConflictDoesNotOverwrite verifies that Insert's ON CONFLICT
// clause is DO NOTHING, not DO UPDATE — a second insert with a different
// value MUST NOT replace the stored value. Callers that need to replace a
// stale fingerprint have to call DeleteForFile first.
func TestInsert_ConflictDoesNotOverwrite(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")
	book := insertTestBook(t, db, lib)
	file := insertTestFile(t, db, book)

	require.NoError(t, svc.Insert(ctx, file.ID, models.FingerprintAlgorithmSHA256, "original-value"))

	// A second insert with a DIFFERENT value must not overwrite the stored value.
	require.NoError(t, svc.Insert(ctx, file.ID, models.FingerprintAlgorithmSHA256, "replacement-value"))

	stored, err := svc.ListForFile(ctx, file.ID, models.FingerprintAlgorithmSHA256)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "original-value", stored[0].Value, "ON CONFLICT DO NOTHING must preserve the original value")
}

// TestFindFilesByHash_FindsMatch verifies that a file can be looked up by hash.
func TestFindFilesByHash_FindsMatch(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")
	book := insertTestBook(t, db, lib)
	file := insertTestFile(t, db, book)

	hash := "deadbeef1234"
	err := svc.Insert(ctx, file.ID, models.FingerprintAlgorithmSHA256, hash)
	require.NoError(t, err)

	found, err := svc.FindFilesByHash(ctx, lib.ID, models.FingerprintAlgorithmSHA256, hash)
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, file.ID, found[0].ID)
}

// TestFindFilesByHash_DifferentLibrary verifies that files in a different library
// are not returned even if they have the same hash.
func TestFindFilesByHash_DifferentLibrary(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib1 := insertTestLibrary(t, db, "Library 1")
	book1 := insertTestBook(t, db, lib1)
	file1 := insertTestFile(t, db, book1)

	lib2 := insertTestLibrary(t, db, "Library 2")

	hash := "deadbeef1234"
	err := svc.Insert(ctx, file1.ID, models.FingerprintAlgorithmSHA256, hash)
	require.NoError(t, err)

	// Querying lib2 should return nothing even though lib1 has the hash.
	found, err := svc.FindFilesByHash(ctx, lib2.ID, models.FingerprintAlgorithmSHA256, hash)
	require.NoError(t, err)
	assert.Empty(t, found, "files from a different library should not be returned")
}

// TestFindFilesByHash_UnknownHash verifies that an empty slice is returned when
// no file matches the hash.
func TestFindFilesByHash_UnknownHash(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")

	found, err := svc.FindFilesByHash(ctx, lib.ID, models.FingerprintAlgorithmSHA256, "nosuchhash")
	require.NoError(t, err)
	assert.Empty(t, found)
}

// TestDeleteForFile verifies that DeleteForFile removes all fingerprints for
// the specified file and leaves other files' fingerprints intact.
func TestDeleteForFile(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")
	book := insertTestBook(t, db, lib)
	file1 := insertTestFile(t, db, book)

	// Insert a second file.
	file2 := &models.File{
		LibraryID:     lib.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/library/book2.epub",
		FilesizeBytes: 2048,
	}
	_, err := db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	err = svc.Insert(ctx, file1.ID, models.FingerprintAlgorithmSHA256, "hash1")
	require.NoError(t, err)
	err = svc.Insert(ctx, file2.ID, models.FingerprintAlgorithmSHA256, "hash2")
	require.NoError(t, err)

	// Delete only file1's fingerprints.
	err = svc.DeleteForFile(ctx, file1.ID)
	require.NoError(t, err)

	count1, err := svc.CountForFile(ctx, file1.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count1, "file1 fingerprints should be deleted")

	count2, err := svc.CountForFile(ctx, file2.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count2, "file2 fingerprints should be untouched")
}

// TestCountForFile verifies CountForFile returns the correct tally.
func TestCountForFile(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")
	book := insertTestBook(t, db, lib)
	file := insertTestFile(t, db, book)

	count, err := svc.CountForFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "no fingerprints yet")

	err = svc.Insert(ctx, file.ID, models.FingerprintAlgorithmSHA256, "sha_value")
	require.NoError(t, err)

	count, err = svc.CountForFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "one fingerprint inserted")

	// Insert a second algorithm to confirm multi-algorithm counting.
	err = svc.Insert(ctx, file.ID, "phash", "phash_value")
	require.NoError(t, err)

	count, err = svc.CountForFile(ctx, file.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "two fingerprints across two algorithms")
}

// TestListFilesMissingAlgorithm verifies that only file IDs without a given
// algorithm fingerprint are returned.
func TestListFilesMissingAlgorithm(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")
	book := insertTestBook(t, db, lib)

	file1 := insertTestFile(t, db, book)
	file2 := &models.File{
		LibraryID:     lib.ID,
		BookID:        book.ID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      "/test/library/book2.epub",
		FilesizeBytes: 2048,
	}
	_, err := db.NewInsert().Model(file2).Exec(ctx)
	require.NoError(t, err)

	// Give file1 a sha256 but leave file2 without one.
	err = svc.Insert(ctx, file1.ID, models.FingerprintAlgorithmSHA256, "hash1")
	require.NoError(t, err)

	missing, err := svc.ListFilesMissingAlgorithm(ctx, lib.ID, models.FingerprintAlgorithmSHA256)
	require.NoError(t, err)

	assert.NotContains(t, missing, file1.ID, "file1 already has a sha256, should not appear")
	assert.Contains(t, missing, file2.ID, "file2 is missing sha256, should appear")
}

// TestListFilesMissingAlgorithm_AllPresent verifies an empty slice when all
// files already have the algorithm.
func TestListFilesMissingAlgorithm_AllPresent(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")
	book := insertTestBook(t, db, lib)
	file := insertTestFile(t, db, book)

	err := svc.Insert(ctx, file.ID, models.FingerprintAlgorithmSHA256, "hash")
	require.NoError(t, err)

	missing, err := svc.ListFilesMissingAlgorithm(ctx, lib.ID, models.FingerprintAlgorithmSHA256)
	require.NoError(t, err)
	assert.Empty(t, missing, "no files should be missing the algorithm")
}

// TestListForFile verifies that ListForFile returns matching fingerprints for a
// specific file+algorithm and excludes rows for other algorithms.
func TestListForFile(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")
	book := insertTestBook(t, db, lib)
	file := insertTestFile(t, db, book)

	err := svc.Insert(ctx, file.ID, models.FingerprintAlgorithmSHA256, "sha_value")
	require.NoError(t, err)
	err = svc.Insert(ctx, file.ID, "phash", "phash_value")
	require.NoError(t, err)

	fps, err := svc.ListForFile(ctx, file.ID, models.FingerprintAlgorithmSHA256)
	require.NoError(t, err)
	require.Len(t, fps, 1)
	assert.Equal(t, models.FingerprintAlgorithmSHA256, fps[0].Algorithm)
	assert.Equal(t, "sha_value", fps[0].Value)
	assert.Equal(t, file.ID, fps[0].FileID)
}

// TestListForFile_Empty verifies that an empty slice is returned when no
// fingerprints exist for the requested file+algorithm.
func TestListForFile_Empty(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	svc := fingerprints.NewService(db)

	lib := insertTestLibrary(t, db, "Library")
	book := insertTestBook(t, db, lib)
	file := insertTestFile(t, db, book)

	fps, err := svc.ListForFile(ctx, file.ID, models.FingerprintAlgorithmSHA256)
	require.NoError(t, err)
	assert.Empty(t, fps)
}
