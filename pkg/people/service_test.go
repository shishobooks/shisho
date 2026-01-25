package people

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

	_, err = migrations.BringUpToDate(context.Background(), db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func TestGetAuthoredBooks_DeduplicatesMultipleRoles(t *testing.T) {
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
