package search

import (
	"context"
	"database/sql"
	"testing"

	"github.com/shishobooks/shisho/pkg/migrations"
	"github.com/shishobooks/shisho/pkg/models"
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

func TestSearchBooksByIdentifier_ReturnsAuthors(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "2:3",
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a person (author)
	person := &models.Person{
		LibraryID:      library.ID,
		Name:           "Jane Doe",
		SortName:       "Doe, Jane",
		SortNameSource: "file",
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	// Create a book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/test/book",
		Title:           "Test Book",
		TitleSource:     "file",
		SortTitle:       "Test Book",
		SortTitleSource: "file",
		AuthorSource:    "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create author association
	_, err = db.NewInsert().
		Model(&models.Author{}).
		Value("book_id", "?", book.ID).
		Value("person_id", "?", person.ID).
		Value("sort_order", "?", 0).
		Exec(ctx)
	require.NoError(t, err)

	// Create a file with an identifier
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/test/book/test.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create a file identifier (e.g., ISBN)
	identifier := &models.FileIdentifier{
		FileID: file.ID,
		Type:   "isbn",
		Value:  "978-0-13-468599-1",
		Source: "file",
	}
	_, err = db.NewInsert().Model(identifier).Exec(ctx)
	require.NoError(t, err)

	// Search by identifier
	svc := NewService(db)
	results, err := svc.searchBooksByIdentifier(ctx, "978-0-13-468599-1", library.ID, nil, 10)
	require.NoError(t, err)
	require.Len(t, results, 1, "Should find one book by identifier")
	require.Equal(t, "Test Book", results[0].Title)
	require.Equal(t, "Jane Doe", results[0].Authors, "Should include author name")
}
