package settings

import (
	"context"
	"database/sql"
	"testing"

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

	t.Cleanup(func() { db.Close() })
	return db
}

func createTestUser(t *testing.T, db *bun.DB, username string) *models.User { //nolint:unparam // username is parameterized for flexibility
	t.Helper()
	u := &models.User{
		Username:     username,
		PasswordHash: "x",
		RoleID:       1,
		IsActive:     true,
	}
	_, err := db.NewInsert().Model(u).Exec(context.Background())
	require.NoError(t, err)
	return u
}

func createTestLibrary(t *testing.T, db *bun.DB, name string) *models.Library { //nolint:unparam // name is parameterized for flexibility
	t.Helper()
	l := &models.Library{
		Name:                     name,
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(l).Exec(context.Background())
	require.NoError(t, err)
	return l
}

func TestGetLibrarySettings_ReturnsNilWhenMissing(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")

	got, err := svc.GetLibrarySettings(context.Background(), user.ID, lib.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestUpsertLibrarySort_InsertsThenUpdates(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")

	spec := "title:asc"
	row, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &spec)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, user.ID, row.UserID)
	assert.Equal(t, lib.ID, row.LibraryID)
	require.NotNil(t, row.SortSpec)
	assert.Equal(t, "title:asc", *row.SortSpec)

	// Update — same (user, library), new spec. Should overwrite, not duplicate.
	newSpec := "author:asc,series:asc"
	row2, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &newSpec)
	require.NoError(t, err)
	assert.Equal(t, "author:asc,series:asc", *row2.SortSpec)

	// Only one row should exist for this pair.
	var count int
	count, err = db.NewSelect().
		Model((*models.UserLibrarySettings)(nil)).
		Where("user_id = ? AND library_id = ?", user.ID, lib.ID).
		Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestUpsertLibrarySort_ClearsWithNil(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")

	spec := "title:asc"
	_, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &spec)
	require.NoError(t, err)

	row, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, row.SortSpec)
}

func TestUserDelete_CascadesSettings(t *testing.T) {
	t.Parallel()

	db := setupTestDB(t)
	svc := NewService(db)
	user := createTestUser(t, db, "alice")
	lib := createTestLibrary(t, db, "Books")

	spec := "title:asc"
	_, err := svc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &spec)
	require.NoError(t, err)

	_, err = db.NewDelete().Model((*models.User)(nil)).Where("id = ?", user.ID).Exec(context.Background())
	require.NoError(t, err)

	var count int
	count, err = db.NewSelect().
		Model((*models.UserLibrarySettings)(nil)).
		Where("user_id = ?", user.ID).
		Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, count, "settings should cascade on user delete")
}
