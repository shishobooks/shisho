package aliases

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupTestDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)

	db := bun.NewDB(sqldb, sqlitedialect.New())

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func createTestLibrary(t *testing.T, db *bun.DB) *models.Library {
	t.Helper()
	lib := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)
	return lib
}

func createTestGenre(t *testing.T, db *bun.DB, name string, libraryID int) *models.Genre {
	t.Helper()
	g := &models.Genre{
		LibraryID: libraryID,
		Name:      name,
	}
	_, err := db.NewInsert().Model(g).Exec(context.Background())
	require.NoError(t, err)
	return g
}

func TestAddAlias(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre := createTestGenre(t, db, "Science Fiction", lib.ID)

	err := svc.AddAlias(ctx, GenreConfig, genre.ID, "Sci-Fi", lib.ID)
	require.NoError(t, err)

	aliases, err := svc.ListAliases(ctx, GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"Sci-Fi"}, aliases)
}

func TestAddAlias_RejectsEmpty(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre := createTestGenre(t, db, "Science Fiction", lib.ID)

	err := svc.AddAlias(ctx, GenreConfig, genre.ID, "  ", lib.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestAddAlias_RejectsDuplicatePrimaryName(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre := createTestGenre(t, db, "Science Fiction", lib.ID)

	err := svc.AddAlias(ctx, GenreConfig, genre.ID, "science fiction", lib.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resource's own name")
}

func TestAddAlias_RejectsConflictWithOtherPrimaryName(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre := createTestGenre(t, db, "Science Fiction", lib.ID)
	createTestGenre(t, db, "Fantasy", lib.ID)

	err := svc.AddAlias(ctx, GenreConfig, genre.ID, "fantasy", lib.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "existing name")
}

func TestAddAlias_RejectsConflictWithExistingAlias(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre1 := createTestGenre(t, db, "Science Fiction", lib.ID)
	genre2 := createTestGenre(t, db, "Fantasy", lib.ID)

	err := svc.AddAlias(ctx, GenreConfig, genre1.ID, "SF", lib.ID)
	require.NoError(t, err)

	// Try adding same alias (case-insensitive) to another genre
	err = svc.AddAlias(ctx, GenreConfig, genre2.ID, "sf", lib.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "existing alias")
}

func TestRemoveAlias(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre := createTestGenre(t, db, "Science Fiction", lib.ID)

	err := svc.AddAlias(ctx, GenreConfig, genre.ID, "Sci-Fi", lib.ID)
	require.NoError(t, err)

	err = svc.RemoveAlias(ctx, GenreConfig, genre.ID, "Sci-Fi")
	require.NoError(t, err)

	aliases, err := svc.ListAliases(ctx, GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.Empty(t, aliases)
}

func TestListAliases_Empty(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre := createTestGenre(t, db, "Science Fiction", lib.ID)

	aliases, err := svc.ListAliases(ctx, GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.NotNil(t, aliases)
	assert.Empty(t, aliases)
}

func TestSyncAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre := createTestGenre(t, db, "Science Fiction", lib.ID)

	// Add initial aliases
	err := svc.AddAlias(ctx, GenreConfig, genre.ID, "Sci-Fi", lib.ID)
	require.NoError(t, err)
	err = svc.AddAlias(ctx, GenreConfig, genre.ID, "SF", lib.ID)
	require.NoError(t, err)

	// Sync: keep SF, remove Sci-Fi, add SciFi
	err = svc.SyncAliases(ctx, GenreConfig, genre.ID, lib.ID, []string{"SF", "SciFi"})
	require.NoError(t, err)

	aliases, err := svc.ListAliases(ctx, GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"SF", "SciFi"}, aliases)
}

func TestSyncAliases_RejectsConflict(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre := createTestGenre(t, db, "Science Fiction", lib.ID)
	createTestGenre(t, db, "Fantasy", lib.ID)

	err := svc.SyncAliases(ctx, GenreConfig, genre.ID, lib.ID, []string{"fantasy"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "existing name")
}

func TestAddAlias_CascadeDeletesOnParentRemoval(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre := createTestGenre(t, db, "Science Fiction", lib.ID)

	err := svc.AddAlias(ctx, GenreConfig, genre.ID, "Sci-Fi", lib.ID)
	require.NoError(t, err)

	// Delete the genre - aliases should cascade
	_, err = db.NewDelete().Model((*models.Genre)(nil)).Where("id = ?", genre.ID).Exec(ctx)
	require.NoError(t, err)

	aliases, err := svc.ListAliases(ctx, GenreConfig, genre.ID)
	require.NoError(t, err)
	assert.Empty(t, aliases)
}

func TestAddAlias_DifferentLibrariesAllowed(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib1 := createTestLibrary(t, db)
	lib2 := &models.Library{
		Name:                     "Second Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib2).Exec(ctx)
	require.NoError(t, err)

	genre1 := createTestGenre(t, db, "Science Fiction", lib1.ID)
	genre2 := createTestGenre(t, db, "Science Fiction", lib2.ID)

	err = svc.AddAlias(ctx, GenreConfig, genre1.ID, "Sci-Fi", lib1.ID)
	require.NoError(t, err)

	// Same alias name in different library should succeed
	err = svc.AddAlias(ctx, GenreConfig, genre2.ID, "Sci-Fi", lib2.ID)
	require.NoError(t, err)
}

func TestAddAlias_ReturnsValidationError(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)
	genre := createTestGenre(t, db, "Science Fiction", lib.ID)
	createTestGenre(t, db, "Fantasy", lib.ID)

	err := svc.AddAlias(ctx, GenreConfig, genre.ID, "Fantasy", lib.ID)
	require.Error(t, err)

	var validationErr *errcodes.Error
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, 422, validationErr.HTTPCode)
}
