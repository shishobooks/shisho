package opds

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// setupOPDSDB creates an in-memory SQLite DB with migrations applied.
// MaxOpenConns=1 pins the pool to the migrated connection so subsequent
// queries don't land on a sibling connection with no schema.
func setupOPDSDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.ShimName, ":memory:")
	require.NoError(t, err)
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func mustParseSortSpec(t *testing.T, s string) []sortspec.SortLevel {
	t.Helper()
	levels, err := sortspec.Parse(s)
	require.NoError(t, err)
	return levels
}

// TestLibraryAllBooksFeed_HonorsStoredSort confirms the OPDS "all books"
// feed applies the user's stored library sort preference. Apple has the
// older created_at but the alphabetically-earlier title, so the default
// (sort_title ASC) would return Apple, Cheese. The stored date_added:desc
// inverts that, returning Cheese, Apple — which only holds if the sort
// parameter was actually threaded through.
func TestLibraryAllBooksFeed_HonorsStoredSort(t *testing.T) {
	t.Parallel()

	db := setupOPDSDB(t)

	lib := &models.Library{
		Name:                     "Library A",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)

	now := time.Now()
	apple := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Apple",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Apple",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/tmp/apple",
		CreatedAt:       now.Add(-2 * time.Hour),
	}
	cheese := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Cheese",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Cheese",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/tmp/cheese",
		CreatedAt:       now,
	}
	_, err = db.NewInsert().Model(apple).Exec(context.Background())
	require.NoError(t, err)
	_, err = db.NewInsert().Model(cheese).Exec(context.Background())
	require.NoError(t, err)

	user := &models.User{Username: "alice", PasswordHash: "x", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(context.Background())
	require.NoError(t, err)

	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
	_, err = settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	opdsSvc := NewService(db)

	resolved, err := settingsSvc.GetLibrarySettings(context.Background(), user.ID, lib.ID)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.NotNil(t, resolved.SortSpec)

	// Pass empty fileTypes so the books-with-files filter doesn't drop
	// our test books (which have no file rows). The actual content of
	// the feed entries doesn't matter for this test — only their order.
	feed, err := opdsSvc.BuildLibraryAllBooksFeed(
		context.Background(),
		"http://x",
		"",
		lib.ID,
		10,
		0,
		mustParseSortSpec(t, *resolved.SortSpec),
	)
	require.NoError(t, err)
	require.Len(t, feed.Entries, 2)
	// date_added:desc → cheese (newer) first, apple (older) second.
	// Under the default sort_title ASC this would be reversed, so this
	// assertion proves the sort param was threaded through.
	assert.Contains(t, feed.Entries[0].Title, "Cheese")
	assert.Contains(t, feed.Entries[1].Title, "Apple")
}

// TestLibraryAllBooksFeed_NilSortUsesDefault preserves backward compat.
func TestLibraryAllBooksFeed_NilSortUsesDefault(t *testing.T) {
	t.Parallel()

	db := setupOPDSDB(t)

	lib := &models.Library{
		Name:                     "Library B",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)

	now := time.Now()
	cheese := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Cheese",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Cheese",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/tmp/cheese2",
		CreatedAt:       now,
	}
	apple := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Apple",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Apple",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/tmp/apple2",
		CreatedAt:       now.Add(-time.Hour),
	}
	_, err = db.NewInsert().Model(cheese).Exec(context.Background())
	require.NoError(t, err)
	_, err = db.NewInsert().Model(apple).Exec(context.Background())
	require.NoError(t, err)

	opdsSvc := NewService(db)

	// Pass empty fileTypes so the books-with-files filter doesn't drop
	// our test books (which have no file rows).
	feed, err := opdsSvc.BuildLibraryAllBooksFeed(
		context.Background(),
		"http://x",
		"",
		lib.ID,
		10,
		0,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, feed.Entries, 2)
	// Default sort_title ASC → Apple, Cheese.
	assert.Contains(t, feed.Entries[0].Title, "Apple")
	assert.Contains(t, feed.Entries[1].Title, "Cheese")
}
