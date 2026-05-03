package people

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/pkg/errors"
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

	// Enable foreign keys to match production behavior
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func TestGetAuthoredBooks_DeduplicatesMultipleRoles(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library
	library := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book
	book := &models.Book{
		LibraryID:       library.ID,
		Title:           "Test Book",
		TitleSource:     models.DataSourceFilepath,
		SortTitle:       "Test Book",
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceFilepath,
		Filepath:        t.TempDir(),
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create a person
	person := &models.Person{
		LibraryID:      library.ID,
		Name:           "Jane Doe",
		SortName:       "Doe, Jane",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	// Add the person as author with multiple roles (writer and penciller)
	writerRole := models.AuthorRoleWriter
	pencillerRole := models.AuthorRolePenciller
	authors := []*models.Author{
		{BookID: book.ID, PersonID: person.ID, SortOrder: 1, Role: &writerRole},
		{BookID: book.ID, PersonID: person.ID, SortOrder: 2, Role: &pencillerRole},
	}
	for _, author := range authors {
		_, err = db.NewInsert().Model(author).Exec(ctx)
		require.NoError(t, err)
	}

	// Call GetAuthoredBooks
	svc := NewService(db)
	books, err := svc.GetAuthoredBooks(ctx, person.ID)
	require.NoError(t, err)

	// Should return only 1 book, not 2 (duplicated for each role)
	assert.Len(t, books, 1, "Should return 1 book, not duplicated for each role")
	assert.Equal(t, book.ID, books[0].ID)
}

// TestRetrievePerson_LibraryScoping verifies that RetrievePerson applies
// the LibraryID filter as an independent predicate so a person ID from a
// sibling library cannot leak through. See
// pkg/ereader/handlers.go AuthorBooks for the motivating call site.
func TestRetrievePerson_LibraryScoping(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	libA := &models.Library{
		Name:                     "Library A",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(libA).Exec(ctx)
	require.NoError(t, err)

	libB := &models.Library{
		Name:                     "Library B",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err = db.NewInsert().Model(libB).Exec(ctx)
	require.NoError(t, err)

	personInB := &models.Person{
		LibraryID:      libB.ID,
		Name:           "Secret Author",
		SortName:       "Author, Secret",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(personInB).Exec(ctx)
	require.NoError(t, err)

	svc := NewService(db)

	// Lookup scoped to lib B succeeds.
	got, err := svc.RetrievePerson(ctx, RetrievePersonOptions{
		ID:        &personInB.ID,
		LibraryID: &libB.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, personInB.ID, got.ID)

	// Lookup scoped to lib A must NOT leak the person from lib B.
	_, err = svc.RetrievePerson(ctx, RetrievePersonOptions{
		ID:        &personInB.ID,
		LibraryID: &libA.ID,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errcodes.NotFound("Person")),
		"expected NotFound when person ID belongs to a different library, got: %v", err)
}

func TestFindOrCreatePerson_PrimaryNameMatch(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	person := &models.Person{
		LibraryID:      lib.ID,
		Name:           "Brandon Sanderson",
		SortName:       "Sanderson, Brandon",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	found, err := svc.FindOrCreatePerson(ctx, "Brandon Sanderson", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, person.ID, found.ID)
}

func TestFindOrCreatePerson_AliasMatch(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	person := &models.Person{
		LibraryID:      lib.ID,
		Name:           "Robert Jordan",
		SortName:       "Jordan, Robert",
		SortNameSource: models.DataSourceFilepath,
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewRaw(
		"INSERT INTO person_aliases (created_at, person_id, name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), person.ID, "James Oliver Rigney Jr.", lib.ID,
	).Exec(ctx)
	require.NoError(t, err)

	found, err := svc.FindOrCreatePerson(ctx, "James Oliver Rigney Jr.", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, person.ID, found.ID)
	assert.Equal(t, "Robert Jordan", found.Name)
}

func TestFindOrCreatePerson_NoMatch_CreatesNew(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()
	svc := NewService(db)

	lib := &models.Library{
		Name:                     "Test Library",
		CoverAspectRatio:         "book",
		DownloadFormatPreference: models.DownloadFormatOriginal,
	}
	_, err := db.NewInsert().Model(lib).Exec(ctx)
	require.NoError(t, err)

	found, err := svc.FindOrCreatePerson(ctx, "Terry Pratchett", lib.ID)
	require.NoError(t, err)
	assert.Equal(t, "Terry Pratchett", found.Name)
	assert.Equal(t, lib.ID, found.LibraryID)
}
