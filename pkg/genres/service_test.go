package genres

import (
	"context"
	"database/sql"
	"testing"
	"time"

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

func TestFindOrCreateGenre_PrimaryNameMatch(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	// Create a genre directly
	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	err := svc.CreateGenre(ctx, genre)
	require.NoError(t, err)

	// FindOrCreate with the same name should return the existing genre
	found, err := svc.FindOrCreateGenre(ctx, "Science Fiction", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, genre.ID, found.ID)
	assert.Equal(t, "Science Fiction", found.Name)
}

func TestFindOrCreateGenre_PrimaryNameMatch_CaseInsensitive(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	err := svc.CreateGenre(ctx, genre)
	require.NoError(t, err)

	// FindOrCreate with different case should return the existing genre
	found, err := svc.FindOrCreateGenre(ctx, "science fiction", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, genre.ID, found.ID)
}

func TestFindOrCreateGenre_AliasMatch(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	// Create a genre and add an alias
	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	err := svc.CreateGenre(ctx, genre)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO genre_aliases (created_at, genre_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), genre.ID, "Sci-Fi", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	// FindOrCreate with the alias name should return the canonical genre
	found, err := svc.FindOrCreateGenre(ctx, "Sci-Fi", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, genre.ID, found.ID)
	assert.Equal(t, "Science Fiction", found.Name)
}

func TestFindOrCreateGenre_AliasMatch_CaseInsensitive(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	genre := &models.Genre{LibraryID: lib.ID, Name: "Science Fiction"}
	err := svc.CreateGenre(ctx, genre)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO genre_aliases (created_at, genre_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), genre.ID, "Sci-Fi", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	// Case-insensitive alias match
	found, err := svc.FindOrCreateGenre(ctx, "SCI-FI", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, genre.ID, found.ID)
}

func TestFindOrCreateGenre_NoMatch_CreatesNew(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	// FindOrCreate with a name that doesn't match any primary or alias
	found, err := svc.FindOrCreateGenre(ctx, "Mystery", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "Mystery", found.Name)
	assert.Equal(t, lib.ID, found.LibraryID)

	// Verify it was actually created
	retrieved, err := svc.RetrieveGenre(ctx, RetrieveGenreOptions{ID: &found.ID})
	require.NoError(t, err)
	assert.Equal(t, "Mystery", retrieved.Name)
}

func TestFindOrCreateGenre_AliasMatch_LibraryScoped(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib1 := createTestLibrary(t, db)
	lib2 := &models.Library{
		Name:                     "Other Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib2).Exec(ctx)
	require.NoError(t, err)

	// Create genre with alias in lib1
	genre := &models.Genre{LibraryID: lib1.ID, Name: "Science Fiction"}
	err = svc.CreateGenre(ctx, genre)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO genre_aliases (created_at, genre_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), genre.ID, "Sci-Fi", lib1.ID,
	).Exec(ctx)
	require.NoError(t, err)

	// FindOrCreate in lib2 with the alias name should NOT match lib1's alias
	found, err := svc.FindOrCreateGenre(ctx, "Sci-Fi", lib2.ID)
	require.NoError(t, err)
	assert.NotEqual(t, genre.ID, found.ID, "Should not match alias from different library")
	assert.Equal(t, "Sci-Fi", found.Name)
	assert.Equal(t, lib2.ID, found.LibraryID)
}
