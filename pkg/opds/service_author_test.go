package opds

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/sortspec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// authorTestSeeds packages the DB rows shared across the
// ListBooksByAuthor tests so each test reads cleanly.
type authorTestSeeds struct {
	library *models.Library
	apple   *models.Book
	cheese  *models.Book
	other   *models.Book // by a different author — proves the filter works
	alice   *models.Person
}

func seedAuthorTestData(t *testing.T, db *bun.DB) authorTestSeeds {
	t.Helper()
	ctx := context.Background()

	lib := &models.Library{
		Name:                     "Library A",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
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
	other := &models.Book{
		LibraryID:       lib.ID,
		Title:           "OtherAuthorBook",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "OtherAuthorBook",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        "/tmp/other",
		CreatedAt:       now.Add(-time.Hour),
	}
	for _, b := range []*models.Book{apple, cheese, other} {
		_, err = db.NewInsert().Model(b).Exec(ctx)
		require.NoError(t, err)
	}

	alice := &models.Person{
		LibraryID:      lib.ID,
		Name:           "Alice",
		SortName:       "Alice",
		SortNameSource: models.DataSourceFilepath,
	}
	bob := &models.Person{
		LibraryID:      lib.ID,
		Name:           "Bob",
		SortName:       "Bob",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(alice).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(bob).Exec(ctx)
	require.NoError(t, err)

	// authors.sort_order is NOT NULL with no default; nullzero would
	// strip a 0, so seed all rows with 1.
	for _, link := range []*models.Author{
		{BookID: apple.ID, PersonID: alice.ID, SortOrder: 1},
		{BookID: cheese.ID, PersonID: alice.ID, SortOrder: 1},
		{BookID: other.ID, PersonID: bob.ID, SortOrder: 1},
	} {
		_, err = db.NewInsert().Model(link).Exec(ctx)
		require.NoError(t, err)
	}

	return authorTestSeeds{
		library: lib,
		apple:   apple,
		cheese:  cheese,
		other:   other,
		alice:   alice,
	}
}

// TestListBooksByAuthor_HonorsSort confirms M14: the per-author ID
// query now applies the user's sort. Apple has the older created_at
// but the alphabetically-earlier title, so default `sort_title ASC`
// would return Apple, Cheese. The explicit date_added:desc inverts
// that — proving the sort is threaded into the filtered query rather
// than being applied to a different (broader) result set as before.
func TestListBooksByAuthor_HonorsSort(t *testing.T) {
	t.Parallel()

	db := setupOPDSDB(t)
	seeds := seedAuthorTestData(t, db)

	svc := NewService(db)
	got, total, err := svc.ListBooksByAuthor(
		context.Background(),
		seeds.library.ID,
		seeds.alice.Name,
		nil, // no file-type filter
		10,
		0,
		[]sortspec.SortLevel{{Field: sortspec.FieldDateAdded, Direction: sortspec.DirDesc}},
	)
	require.NoError(t, err)
	require.Equal(t, 2, total, "alice has two books, bob's book filtered out")
	require.Len(t, got, 2)
	assert.Equal(t, seeds.cheese.ID, got[0].ID, "date_added DESC: cheese first")
	assert.Equal(t, seeds.apple.ID, got[1].ID, "date_added DESC: apple second")
}

// TestListBooksByAuthor_RespectsLimitOffset confirms pagination is
// SQL-driven and ordering-aware. The previous implementation paginated
// an unsorted ID slice before applying the sort, so page 1 wasn't
// guaranteed to contain the "first N in sort order".
func TestListBooksByAuthor_RespectsLimitOffset(t *testing.T) {
	t.Parallel()

	db := setupOPDSDB(t)
	seeds := seedAuthorTestData(t, db)

	svc := NewService(db)

	// limit=1, offset=0 with date_added DESC → just cheese.
	page1, total, err := svc.ListBooksByAuthor(
		context.Background(),
		seeds.library.ID,
		seeds.alice.Name,
		nil,
		1,
		0,
		[]sortspec.SortLevel{{Field: sortspec.FieldDateAdded, Direction: sortspec.DirDesc}},
	)
	require.NoError(t, err)
	require.Equal(t, 2, total, "total still reports the full filtered count")
	require.Len(t, page1, 1)
	assert.Equal(t, seeds.cheese.ID, page1[0].ID)

	// limit=1, offset=1 → just apple.
	page2, _, err := svc.ListBooksByAuthor(
		context.Background(),
		seeds.library.ID,
		seeds.alice.Name,
		nil,
		1,
		1,
		[]sortspec.SortLevel{{Field: sortspec.FieldDateAdded, Direction: sortspec.DirDesc}},
	)
	require.NoError(t, err)
	require.Len(t, page2, 1)
	assert.Equal(t, seeds.apple.ID, page2[0].ID)
}

// TestListBooksByAuthor_UnknownAuthor confirms the "no such person in
// this library" branch returns an empty result rather than bubbling up
// a 404 — preserving the previous behavior where the unsorted-ID
// query returned an empty slice when nothing joined.
func TestListBooksByAuthor_UnknownAuthor(t *testing.T) {
	t.Parallel()

	db := setupOPDSDB(t)
	seeds := seedAuthorTestData(t, db)

	svc := NewService(db)
	got, total, err := svc.ListBooksByAuthor(
		context.Background(),
		seeds.library.ID,
		"Nobody",
		nil,
		10,
		0,
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, got)
}
