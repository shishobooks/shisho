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

func TestGlobalSearch_ReturnsFileTypes(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/test/audiobook",
		Title:           "My Audiobook",
		TitleSource:     "file",
		SortTitle:       "My Audiobook",
		SortTitleSource: "file",
		AuthorSource:    "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create an M4B file (audiobook)
	m4bFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/test/audiobook/test.m4b",
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	_, err = db.NewInsert().Model(m4bFile).Exec(ctx)
	require.NoError(t, err)

	// Index the book in FTS
	svc := NewService(db)
	book.Files = []*models.File{m4bFile}
	err = svc.IndexBook(ctx, book)
	require.NoError(t, err)

	// Search for the book
	results, err := svc.GlobalSearch(ctx, library.ID, "Audiobook")
	require.NoError(t, err)
	require.Len(t, results.Books, 1, "Should find one book")
	require.Equal(t, "My Audiobook", results.Books[0].Title)
	require.Equal(t, []string{"m4b"}, results.Books[0].FileTypes, "Should include file types")
}

func TestGlobalSearch_ReturnsMultipleFileTypes(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a book with both EPUB and M4B files
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/test/multiformat",
		Title:           "Multi Format Book",
		TitleSource:     "file",
		SortTitle:       "Multi Format Book",
		SortTitleSource: "file",
		AuthorSource:    "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create EPUB file
	epubFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/test/multiformat/test.epub",
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	_, err = db.NewInsert().Model(epubFile).Exec(ctx)
	require.NoError(t, err)

	// Create M4B file
	m4bFile := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/test/multiformat/test.m4b",
		FileType:      models.FileTypeM4B,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 2000,
	}
	_, err = db.NewInsert().Model(m4bFile).Exec(ctx)
	require.NoError(t, err)

	// Index the book in FTS
	svc := NewService(db)
	book.Files = []*models.File{epubFile, m4bFile}
	err = svc.IndexBook(ctx, book)
	require.NoError(t, err)

	// Search for the book
	results, err := svc.GlobalSearch(ctx, library.ID, "Multi")
	require.NoError(t, err)
	require.Len(t, results.Books, 1, "Should find one book")
	require.Equal(t, "Multi Format Book", results.Books[0].Title)
	require.ElementsMatch(t, []string{"epub", "m4b"}, results.Books[0].FileTypes, "Should include all file types")
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

func TestGlobalSearch_DeduplicatesAuthorNames(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a person who has multiple roles on the same book
	person := &models.Person{
		LibraryID:      library.ID,
		Name:           "Stan Lee",
		SortName:       "Lee, Stan",
		SortNameSource: "file",
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	// Create a book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/test/comic",
		Title:           "Amazing Spider-Man",
		TitleSource:     "file",
		SortTitle:       "Amazing Spider-Man",
		SortTitleSource: "file",
		AuthorSource:    "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create author associations with different roles for the same person
	// This can happen with CBZ files where someone is both writer and editor
	_, err = db.NewInsert().
		Model(&models.Author{}).
		Value("book_id", "?", book.ID).
		Value("person_id", "?", person.ID).
		Value("sort_order", "?", 0).
		Value("role", "?", models.AuthorRoleWriter).
		Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().
		Model(&models.Author{}).
		Value("book_id", "?", book.ID).
		Value("person_id", "?", person.ID).
		Value("sort_order", "?", 1).
		Value("role", "?", models.AuthorRoleEditor).
		Exec(ctx)
	require.NoError(t, err)

	// Create a file
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/test/comic/test.cbz",
		FileType:      models.FileTypeCBZ,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Index the book in FTS
	svc := NewService(db)
	writerRole := models.AuthorRoleWriter
	editorRole := models.AuthorRoleEditor
	book.Authors = []*models.Author{
		{PersonID: person.ID, Person: person, SortOrder: 0, Role: &writerRole},
		{PersonID: person.ID, Person: person, SortOrder: 1, Role: &editorRole},
	}
	book.Files = []*models.File{file}
	err = svc.IndexBook(ctx, book)
	require.NoError(t, err)

	// Search for the book
	results, err := svc.GlobalSearch(ctx, library.ID, "Spider")
	require.NoError(t, err)
	require.Len(t, results.Books, 1, "Should find one book")

	// The author name should appear only once, not duplicated
	require.Equal(t, "Stan Lee", results.Books[0].Authors, "Author name should not be duplicated")
}

func TestSearchBooksByIdentifier_DeduplicatesAuthorNames(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a person who has multiple roles on the same book
	person := &models.Person{
		LibraryID:      library.ID,
		Name:           "Stan Lee",
		SortName:       "Lee, Stan",
		SortNameSource: "file",
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	// Create a book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/test/comic",
		Title:           "Amazing Spider-Man",
		TitleSource:     "file",
		SortTitle:       "Amazing Spider-Man",
		SortTitleSource: "file",
		AuthorSource:    "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create author associations with different roles for the same person
	_, err = db.NewInsert().
		Model(&models.Author{}).
		Value("book_id", "?", book.ID).
		Value("person_id", "?", person.ID).
		Value("sort_order", "?", 0).
		Value("role", "?", models.AuthorRoleWriter).
		Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().
		Model(&models.Author{}).
		Value("book_id", "?", book.ID).
		Value("person_id", "?", person.ID).
		Value("sort_order", "?", 1).
		Value("role", "?", models.AuthorRoleEditor).
		Exec(ctx)
	require.NoError(t, err)

	// Create a file with an identifier
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/test/comic/test.cbz",
		FileType:      models.FileTypeCBZ,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Create a file identifier (e.g., ISBN)
	identifier := &models.FileIdentifier{
		FileID: file.ID,
		Type:   "isbn",
		Value:  "978-0-13-468599-9",
		Source: "file",
	}
	_, err = db.NewInsert().Model(identifier).Exec(ctx)
	require.NoError(t, err)

	// Search by identifier
	svc := NewService(db)
	results, err := svc.searchBooksByIdentifier(ctx, "978-0-13-468599-9", library.ID, nil, 10)
	require.NoError(t, err)
	require.Len(t, results, 1, "Should find one book by identifier")

	// The author name should appear only once, not duplicated
	require.Equal(t, "Stan Lee", results[0].Authors, "Author name should not be duplicated")
}

func TestRebuildAllIndexes_DeduplicatesAuthorNames(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	// Create a library
	library := &models.Library{
		Name:             "Test Library",
		CoverAspectRatio: "book",
	}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	// Create a person who has multiple roles on the same book
	person := &models.Person{
		LibraryID:      library.ID,
		Name:           "Stan Lee",
		SortName:       "Lee, Stan",
		SortNameSource: "file",
	}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	// Create a book
	book := &models.Book{
		LibraryID:       library.ID,
		Filepath:        "/test/comic",
		Title:           "Amazing Spider-Man",
		TitleSource:     "file",
		SortTitle:       "Amazing Spider-Man",
		SortTitleSource: "file",
		AuthorSource:    "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	// Create author associations with different roles for the same person
	_, err = db.NewInsert().
		Model(&models.Author{}).
		Value("book_id", "?", book.ID).
		Value("person_id", "?", person.ID).
		Value("sort_order", "?", 0).
		Value("role", "?", models.AuthorRoleWriter).
		Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().
		Model(&models.Author{}).
		Value("book_id", "?", book.ID).
		Value("person_id", "?", person.ID).
		Value("sort_order", "?", 1).
		Value("role", "?", models.AuthorRoleEditor).
		Exec(ctx)
	require.NoError(t, err)

	// Create a file
	file := &models.File{
		LibraryID:     library.ID,
		BookID:        book.ID,
		Filepath:      "/test/comic/test.cbz",
		FileType:      models.FileTypeCBZ,
		FileRole:      models.FileRoleMain,
		FilesizeBytes: 1000,
	}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	// Rebuild all indexes (this is called after a scan)
	svc := NewService(db)
	err = svc.RebuildAllIndexes(ctx)
	require.NoError(t, err, "RebuildAllIndexes should not fail")

	// Search for the book
	results, err := svc.GlobalSearch(ctx, library.ID, "Spider")
	require.NoError(t, err)
	require.Len(t, results.Books, 1, "Should find one book")

	// The author name should appear only once, not duplicated
	require.Equal(t, "Stan Lee", results.Books[0].Authors, "Author name should not be duplicated")
}
