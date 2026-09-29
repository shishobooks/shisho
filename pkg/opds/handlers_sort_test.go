package opds

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/books"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParseSortSpec(t *testing.T, s string) []sortspec.SortLevel {
	t.Helper()
	levels, err := sortspec.Parse(s)
	require.NoError(t, err)
	return levels
}

// TestLibraryAllBooksFeed_HonorsStoredSort confirms the OPDS "all books"
// feed applies the user's stored library sort preference. Apple has the
// older created_at but the alphabetically-earlier title, so the builtin
// default (date_added DESC) would return Cheese, Apple. The stored
// title:asc inverts that, returning Apple, Cheese — which only holds if
// the sort parameter was actually threaded through.
func TestLibraryAllBooksFeed_HonorsStoredSort(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)

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
	// Pick a stored preference that orders DIFFERENTLY from the builtin
	// default (date_added DESC). title:asc → Apple, Cheese; default →
	// Cheese, Apple. The distinct orderings are what makes the assertion
	// prove the sort was threaded through rather than silently falling
	// back to the builtin default.
	stored := "title:asc"
	_, err = settingsSvc.UpsertLibrarySort(context.Background(), user.ID, lib.ID, &stored)
	require.NoError(t, err)

	opdsSvc := NewService(db, books.NewService(db))

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
	// title:asc → Apple first, Cheese second. Under the builtin default
	// (date_added DESC) this would be reversed, so this assertion proves
	// the sort param was threaded through.
	assert.Contains(t, feed.Entries[0].Title, "Apple")
	assert.Contains(t, feed.Entries[1].Title, "Cheese")
}

// TestLibraryAllBooksFeed_NilSortUsesBuiltinDefault confirms that when
// a caller passes no explicit sort (e.g., the resolver returned nil), the
// books service falls back to sortspec.BuiltinDefault — date_added DESC.
// This keeps OPDS consistent with the /books REST endpoint and the
// gallery for callers that haven't layered BuiltinDefault themselves.
func TestLibraryAllBooksFeed_NilSortUsesBuiltinDefault(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)

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

	opdsSvc := NewService(db, books.NewService(db))

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
	// Builtin default is date_added DESC → Cheese (newer) before Apple.
	assert.Contains(t, feed.Entries[0].Title, "Cheese")
	assert.Contains(t, feed.Entries[1].Title, "Apple")
}

// TestHandlerResolveSort_FallsBackToBuiltinDefault confirms the OPDS
// handler's resolveSort returns sortspec.BuiltinDefault when
// ResolveForLibrary finds no stored preference. The books service also
// applies BuiltinDefault when Sort is nil, so this is belt-and-
// suspenders — it keeps the OPDS surface explicit about the sort it
// applies and insulates OPDS from a future change to the service's
// default.
func TestHandlerResolveSort_FallsBackToBuiltinDefault(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	user := &models.User{Username: "alice", PasswordHash: "x", RoleID: 1, IsActive: true}
	_, err := db.NewInsert().Model(user).Exec(context.Background())
	require.NoError(t, err)

	settingsSvc := settings.NewService(db)
	h := &handler{settingsService: settingsSvc}

	got := h.resolveSort(context.Background(), user, 1)

	assert.Equal(t, sortspec.BuiltinDefault(), got,
		"no stored preference → handler falls back to BuiltinDefault")
}
