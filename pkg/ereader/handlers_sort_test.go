package ereader

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/apikeys"
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

// TestHandlerResolveSort_FallsBackToBuiltinDefault confirms the handler's
// resolveSort layers sortspec.BuiltinDefault on top of ResolveForLibrary.
// Without this layering an eReader user with no saved preference would
// see books in `b.sort_title ASC` order (the books service's hard-coded
// fallback) while the React gallery shows the same library in
// `date_added:desc` order — the inconsistency the M6 review caught.
func TestHandlerResolveSort_FallsBackToBuiltinDefault(t *testing.T) {
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
	// Other deps are unused by resolveSort; nil keeps the test focused.
	h := newHandler(db, nil, nil, nil, nil, nil, settingsSvc)

	apiKey := &apikeys.APIKey{UserID: user.ID}
	got := h.resolveSort(context.Background(), apiKey, lib.ID)

	assert.Equal(t, sortspec.BuiltinDefault(), got,
		"no stored preference → handler falls back to BuiltinDefault")
}

// TestHandlerResolveSort_NilApiKeyFallsBackToBuiltinDefault is a
// belt-and-suspenders check: the handler's other call sites already
// short-circuit on missing API keys with an Unauthorized response, but
// resolveSort is independently safe — it never returns nil, so callers
// don't have to guard.
func TestHandlerResolveSort_NilApiKeyFallsBackToBuiltinDefault(t *testing.T) {
	t.Parallel()

	db := setupEReaderDB(t)
	settingsSvc := settings.NewService(db)
	h := newHandler(db, nil, nil, nil, nil, nil, settingsSvc)

	got := h.resolveSort(context.Background(), nil, 1)

	assert.Equal(t, sortspec.BuiltinDefault(), got)
}

// TestNoStoredSortResolvesToNil pins the lower-level
// ResolveForLibrary contract: without a stored preference, it returns
// nil. The eReader handler then layers BuiltinDefault on top — see
// TestHandlerResolveSort_FallsBackToBuiltinDefault above for the
// handler-level behavior.
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
	assert.Nil(t, resolved, "no stored preference → nil (handler layers BuiltinDefault on top)")
}

// TestAuthorBooks_PersonIDAndSortFlow exercises the seam the AuthorBooks
// handler uses after the M8 refactor: instead of fetching every book by
// the author across all libraries (peopleService.GetAuthoredBooks) and
// filtering in Go, the handler now calls bookService.ListBooksWithTotal
// with LibraryID + PersonID + the resolved sort. This test pins that
// composition: PersonID restricts to one author's books inside the
// chosen library, and the user's stored sort preference flips the order
// from the legacy default.
func TestAuthorBooks_PersonIDAndSortFlow(t *testing.T) {
	t.Parallel()

	db := setupEReaderDB(t)
	ctx := context.Background()

	lib := &models.Library{
		Name:                     "Books",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	now := time.Now()
	apple := &models.Book{
		LibraryID: lib.ID, Title: "Apple", TitleSource: models.DataSourceFilepath,
		SortTitle: "Apple", SortTitleSource: models.DataSourceFilepath,
		AuthorSource: models.DataSourceFilepath,
		Filepath:     "/a", CreatedAt: now.Add(-2 * time.Hour),
	}
	cheese := &models.Book{
		LibraryID: lib.ID, Title: "Cheese", TitleSource: models.DataSourceFilepath,
		SortTitle: "Cheese", SortTitleSource: models.DataSourceFilepath,
		AuthorSource: models.DataSourceFilepath,
		Filepath:     "/c", CreatedAt: now,
	}
	other := &models.Book{
		LibraryID: lib.ID, Title: "BobsOnly", TitleSource: models.DataSourceFilepath,
		SortTitle: "BobsOnly", SortTitleSource: models.DataSourceFilepath,
		AuthorSource: models.DataSourceFilepath,
		Filepath:     "/o", CreatedAt: now.Add(-time.Hour),
	}
	for _, b := range []*models.Book{apple, cheese, other} {
		_, err = db.NewInsert().Model(b).Exec(ctx)
		require.NoError(t, err)
	}

	alice := &models.Person{
		LibraryID: lib.ID, Name: "Alice", SortName: "Alice",
		SortNameSource: models.DataSourceFilepath,
	}
	bob := &models.Person{
		LibraryID: lib.ID, Name: "Bob", SortName: "Bob",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(alice).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(bob).Exec(ctx)
	require.NoError(t, err)

	// authors.sort_order is NOT NULL; nullzero would strip a 0, so use 1.
	for _, link := range []*models.Author{
		{BookID: apple.ID, PersonID: alice.ID, SortOrder: 1},
		{BookID: cheese.ID, PersonID: alice.ID, SortOrder: 1},
		{BookID: other.ID, PersonID: bob.ID, SortOrder: 1},
	} {
		_, err = db.NewInsert().Model(link).Exec(ctx)
		require.NoError(t, err)
	}

	user := &models.User{Username: "alice", PasswordHash: "x", RoleID: 1, IsActive: true}
	_, err = db.NewInsert().Model(user).Exec(ctx)
	require.NoError(t, err)

	settingsSvc := settings.NewService(db)
	stored := "date_added:desc"
	_, err = settingsSvc.UpsertLibrarySort(ctx, user.ID, lib.ID, &stored)
	require.NoError(t, err)

	resolved := sortspec.ResolveForLibrary(ctx, settingsSvc, user.ID, lib.ID, nil)
	require.NotNil(t, resolved)

	bookSvc := books.NewService(db)
	got, total, err := bookSvc.ListBooksWithTotal(ctx, books.ListBooksOptions{
		LibraryID: &lib.ID,
		PersonID:  &alice.ID,
		Sort:      resolved,
	})
	require.NoError(t, err)
	require.Equal(t, 2, total, "PersonID excludes Bob's book")
	require.Len(t, got, 2)
	// date_added:desc → cheese (newer) before apple (older).
	assert.Equal(t, cheese.ID, got[0].ID)
	assert.Equal(t, apple.ID, got[1].ID)
}
