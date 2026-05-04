package imprints

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

func TestFindOrCreateImprint_PrimaryNameMatch(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	imp := &models.Imprint{LibraryID: lib.ID, Name: "Vintage Books"}
	err := svc.CreateImprint(ctx, imp)
	require.NoError(t, err)

	found, err := svc.FindOrCreateImprint(ctx, "Vintage Books", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, imp.ID, found.ID)
}

func TestFindOrCreateImprint_AliasMatch(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	imp := &models.Imprint{LibraryID: lib.ID, Name: "Vintage Books"}
	err := svc.CreateImprint(ctx, imp)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO imprint_aliases (created_at, imprint_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), imp.ID, "Vintage", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	found, err := svc.FindOrCreateImprint(ctx, "Vintage", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, imp.ID, found.ID)
	assert.Equal(t, "Vintage Books", found.Name)
}

func TestFindOrCreateImprint_NoMatch_CreatesNew(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	found, err := svc.FindOrCreateImprint(ctx, "Tor Books", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "Tor Books", found.Name)
	assert.Equal(t, lib.ID, found.LibraryID)
}

func TestListImprints_SearchMatchesAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	imp := &models.Imprint{LibraryID: lib.ID, Name: "Vintage Books"}
	err := svc.CreateImprint(ctx, imp)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO imprint_aliases (created_at, imprint_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), imp.ID, "Vintage Classics", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	search := "Vintage Classics"
	results, err := svc.ListImprints(ctx, ListImprintsOptions{
		LibraryID: &lib.ID,
		Search:    &search,
	})
	require.NoError(t, err)
	require.Len(t, results, 1, "Should find imprint by alias 'Vintage Classics'")
	assert.Equal(t, "Vintage Books", results[0].Name)
}
