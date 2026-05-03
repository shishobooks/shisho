package publishers

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

func TestFindOrCreatePublisher_PrimaryNameMatch(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pub := &models.Publisher{LibraryID: lib.ID, Name: "Penguin Random House"}
	err := svc.CreatePublisher(ctx, pub)
	require.NoError(t, err)

	found, err := svc.FindOrCreatePublisher(ctx, "Penguin Random House", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, pub.ID, found.ID)
}

func TestFindOrCreatePublisher_AliasMatch(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	pub := &models.Publisher{LibraryID: lib.ID, Name: "Penguin Random House"}
	err := svc.CreatePublisher(ctx, pub)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO publisher_aliases (created_at, publisher_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), pub.ID, "PRH", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	found, err := svc.FindOrCreatePublisher(ctx, "PRH", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, pub.ID, found.ID)
	assert.Equal(t, "Penguin Random House", found.Name)
}

func TestFindOrCreatePublisher_NoMatch_CreatesNew(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := createTestLibrary(t, db)

	found, err := svc.FindOrCreatePublisher(ctx, "HarperCollins", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "HarperCollins", found.Name)
	assert.Equal(t, lib.ID, found.LibraryID)
}
