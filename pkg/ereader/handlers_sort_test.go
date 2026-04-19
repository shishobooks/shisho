package ereader

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/books"
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

// setupEReaderDB creates an in-memory SQLite DB with migrations applied.
// MaxOpenConns=1 pins the pool to the migrated connection so subsequent
// queries don't land on a sibling connection with no schema.
func setupEReaderDB(t *testing.T) *bun.DB {
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

// These tests verify the sortspec resolver + books service integration at
// the seam the eReader handler uses internally, not the HTTP handler
// round-trip. The handlers render HTML templates; end-to-end testing would
// couple assertions to template structure and API-key middleware wiring,
// both of which add no sort-behavior signal. A future follow-up could add
// a thin handler-shape smoke test that asserts `Sort: sort` is threaded
// into `ListBooksWithTotal` (e.g. via a bookService seam / stub).

// TestStoredSortFlowsThroughBooksService confirms the eReader
// sort-resolution helper picks up a stored `(user, library)` preference.
//
// We exercise the resolution path at the service boundary (ListBooksWithTotal
// with the resolved Sort), which is what the handler does internally. This
// keeps the test independent of the HTML template layer and of whichever API
// key middleware wrapper is in use.
func TestStoredSortFlowsThroughBooksService(t *testing.T) {
	t.Parallel()

	db := setupEReaderDB(t)

	// Seed a library with two books distinguishable by created_at.
	lib := &models.Library{
		Name:                     "Books",
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
		Filepath:        "/a",
		CreatedAt:       now.Add(-2 * time.Hour),
	}
	cheese := &models.Book{
		LibraryID:       lib.ID,
		Title:           "Cheese",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Cheese",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/c",
		CreatedAt:       now,
	}
	_, err = db.NewInsert().Model(apple).Exec(context.Background())
	require.NoError(t, err)
	_, err = db.NewInsert().Model(cheese).Exec(context.Background())
	require.NoError(t, err)

	// Seed a user and save a stored preference of date_added:desc.
	user := &models.User{Username: "alice", PasswordHash: "x", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(context.Background())
	require.NoError(t, err)

	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
	_, err = settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	// Resolve via the same helper the handler uses.
	resolved := sortspec.ResolveForLibrary(
		context.Background(),
		settingsSvc,
		user.ID,
		lib.ID,
		nil, // eReader never carries an explicit URL sort
	)
	require.Equal(t, []sortspec.SortLevel{
		{Field: sortspec.FieldDateAdded, Direction: sortspec.DirDesc},
	}, resolved)

	// Feed it into the books service exactly as the handler does.
	bookSvc := books.NewService(db)
	got, _, err := bookSvc.ListBooksWithTotal(context.Background(), books.ListBooksOptions{
		LibraryID: &lib.ID,
		Sort:      resolved,
	})
	require.NoError(t, err)
	require.Len(t, got, 2)
	// date_added:desc → cheese (now) before apple (now-2h).
	assert.Equal(t, cheese.ID, got[0].ID)
	assert.Equal(t, apple.ID, got[1].ID)
}

// TestNoStoredSortResolvesToNil confirms that without a
// stored preference, ResolveForLibrary returns nil and the handler falls
// through to its existing default ordering.
func TestNoStoredSortResolvesToNil(t *testing.T) {
	t.Parallel()

	db := setupEReaderDB(t)
	lib := &models.Library{
		Name:                     "Books",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(context.Background())
	require.NoError(t, err)

	user := &models.User{Username: "alice", PasswordHash: "x", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(context.Background())
	require.NoError(t, err)

	settingsSvc := settings.NewService(db)
	resolved := sortspec.ResolveForLibrary(
		context.Background(), settingsSvc, user.ID, lib.ID, nil,
	)
	assert.Nil(t, resolved, "no stored preference → nil (handler uses its default)")
}
