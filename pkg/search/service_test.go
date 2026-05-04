package search

import (
	"context"
	"database/sql"
	"testing"
	"time"

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

// --- Helper to insert an alias row directly ---

func insertAlias(t *testing.T, db *bun.DB, table, fkColumn string, resourceID, libraryID int, name string) {
	t.Helper()
	_, err := db.NewRaw(
		"INSERT INTO "+table+" (created_at, "+fkColumn+", name, library_id) VALUES (?, ?, ?, ?)",
		time.Now(), resourceID, name, libraryID,
	).Exec(context.Background())
	require.NoError(t, err)
}

// --- Alias FTS tests ---

func TestIndexGenre_IncludesAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	genre := &models.Genre{LibraryID: library.ID, Name: "Science Fiction"}
	_, err = db.NewInsert().Model(genre).Exec(ctx)
	require.NoError(t, err)

	insertAlias(t, db, "genre_aliases", "genre_id", genre.ID, library.ID, "SciFi")
	insertAlias(t, db, "genre_aliases", "genre_id", genre.ID, library.ID, "SF")

	svc := NewService(db)
	err = svc.IndexGenre(ctx, genre)
	require.NoError(t, err)

	var count int
	err = db.NewSelect().TableExpr("genres_fts").
		ColumnExpr("COUNT(*)").
		Where("genres_fts MATCH ?", `"SciFi"`).
		Where("library_id = ?", library.ID).
		Scan(ctx, &count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "Should find genre by alias 'SciFi'")
}

func TestIndexTag_IncludesAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	tag := &models.Tag{LibraryID: library.ID, Name: "Artificial Intelligence"}
	_, err = db.NewInsert().Model(tag).Exec(ctx)
	require.NoError(t, err)

	insertAlias(t, db, "tag_aliases", "tag_id", tag.ID, library.ID, "AI")

	svc := NewService(db)
	err = svc.IndexTag(ctx, tag)
	require.NoError(t, err)

	var count int
	err = db.NewSelect().TableExpr("tags_fts").
		ColumnExpr("COUNT(*)").
		Where("tags_fts MATCH ?", `"AI"`).
		Where("library_id = ?", library.ID).
		Scan(ctx, &count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "Should find tag by alias 'AI'")
}

func TestIndexPerson_IncludesAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	person := &models.Person{LibraryID: library.ID, Name: "Stephen King", SortName: "King, Stephen", SortNameSource: "file"}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	insertAlias(t, db, "person_aliases", "person_id", person.ID, library.ID, "Richard Bachman")

	svc := NewService(db)
	err = svc.IndexPerson(ctx, person)
	require.NoError(t, err)

	results, _, err := svc.SearchPeople(ctx, library.ID, "Bachman", 10, 0)
	require.NoError(t, err)
	require.Len(t, results, 1, "Should find person by alias 'Bachman'")
	require.Equal(t, "Stephen King", results[0].Name)
}

func TestIndexSeries_IncludesAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	series := &models.Series{LibraryID: library.ID, Name: "A Song of Ice and Fire", NameSource: "file", SortName: "Song of Ice and Fire, A", SortNameSource: "file"}
	_, err = db.NewInsert().Model(series).Exec(ctx)
	require.NoError(t, err)

	insertAlias(t, db, "series_aliases", "series_id", series.ID, library.ID, "Game of Thrones")

	svc := NewService(db)
	err = svc.IndexSeries(ctx, series)
	require.NoError(t, err)

	results, _, err := svc.SearchSeries(ctx, library.ID, "Thrones", 10, 0)
	require.NoError(t, err)
	require.Len(t, results, 1, "Should find series by alias 'Thrones'")
	require.Equal(t, "A Song of Ice and Fire", results[0].Name)
}

func TestIndexBook_IncludesAuthorAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	person := &models.Person{LibraryID: library.ID, Name: "Stephen King", SortName: "King, Stephen", SortNameSource: "file"}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)

	insertAlias(t, db, "person_aliases", "person_id", person.ID, library.ID, "Richard Bachman")

	book := &models.Book{
		LibraryID: library.ID, Filepath: "/test/book", Title: "Thinner",
		TitleSource: "file", SortTitle: "Thinner", SortTitleSource: "file", AuthorSource: "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewInsert().Model(&models.Author{}).
		Value("book_id", "?", book.ID).
		Value("person_id", "?", person.ID).
		Value("sort_order", "?", 0).
		Exec(ctx)
	require.NoError(t, err)

	file := &models.File{LibraryID: library.ID, BookID: book.ID, Filepath: "/test/book/thinner.epub", FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, FilesizeBytes: 1000}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	book.Authors = []*models.Author{{PersonID: person.ID, Person: person, SortOrder: 0}}
	book.Files = []*models.File{file}

	svc := NewService(db)
	err = svc.IndexBook(ctx, book)
	require.NoError(t, err)

	results, err := svc.GlobalSearch(ctx, library.ID, "Bachman")
	require.NoError(t, err)
	require.Len(t, results.Books, 1, "Should find book by author alias 'Bachman'")
	require.Equal(t, "Thinner", results.Books[0].Title)
}

func TestIndexBook_IncludesNarratorAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	narrator := &models.Person{LibraryID: library.ID, Name: "Jim Dale", SortName: "Dale, Jim", SortNameSource: "file"}
	_, err = db.NewInsert().Model(narrator).Exec(ctx)
	require.NoError(t, err)

	insertAlias(t, db, "person_aliases", "person_id", narrator.ID, library.ID, "James Dale")

	book := &models.Book{
		LibraryID: library.ID, Filepath: "/test/audiobook", Title: "Harry Potter",
		TitleSource: "file", SortTitle: "Harry Potter", SortTitleSource: "file", AuthorSource: "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	m4bFile := &models.File{LibraryID: library.ID, BookID: book.ID, Filepath: "/test/audiobook/hp.m4b", FileType: models.FileTypeM4B, FileRole: models.FileRoleMain, FilesizeBytes: 1000}
	_, err = db.NewInsert().Model(m4bFile).Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewInsert().Model(&models.Narrator{}).
		Value("file_id", "?", m4bFile.ID).
		Value("person_id", "?", narrator.ID).
		Value("sort_order", "?", 0).
		Exec(ctx)
	require.NoError(t, err)

	m4bFile.Narrators = []*models.Narrator{{PersonID: narrator.ID, Person: narrator, SortOrder: 0}}
	book.Files = []*models.File{m4bFile}

	svc := NewService(db)
	err = svc.IndexBook(ctx, book)
	require.NoError(t, err)

	results, err := svc.GlobalSearch(ctx, library.ID, "James Dale")
	require.NoError(t, err)
	require.Len(t, results.Books, 1, "Should find book by narrator alias 'James Dale'")
}

func TestIndexBook_IncludesSeriesAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	series := &models.Series{LibraryID: library.ID, Name: "The Dark Tower", NameSource: "file", SortName: "Dark Tower, The", SortNameSource: "file"}
	_, err = db.NewInsert().Model(series).Exec(ctx)
	require.NoError(t, err)

	insertAlias(t, db, "series_aliases", "series_id", series.ID, library.ID, "DT Series")

	book := &models.Book{
		LibraryID: library.ID, Filepath: "/test/dt", Title: "The Gunslinger",
		TitleSource: "file", SortTitle: "Gunslinger, The", SortTitleSource: "file", AuthorSource: "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewInsert().Model(&models.BookSeries{}).
		Value("book_id", "?", book.ID).
		Value("series_id", "?", series.ID).
		Value("sort_order", "?", 0).
		Exec(ctx)
	require.NoError(t, err)

	file := &models.File{LibraryID: library.ID, BookID: book.ID, Filepath: "/test/dt/gunslinger.epub", FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, FilesizeBytes: 1000}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	book.BookSeries = []*models.BookSeries{{SeriesID: series.ID, Series: series, SortOrder: 0}}
	book.Files = []*models.File{file}

	svc := NewService(db)
	err = svc.IndexBook(ctx, book)
	require.NoError(t, err)

	results, err := svc.GlobalSearch(ctx, library.ID, "DT Series")
	require.NoError(t, err)
	require.Len(t, results.Books, 1, "Should find book by series alias 'DT Series'")
}

func TestIndexBook_ExcludesGenreAndTagAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	genre := &models.Genre{LibraryID: library.ID, Name: "Horror"}
	_, err = db.NewInsert().Model(genre).Exec(ctx)
	require.NoError(t, err)
	insertAlias(t, db, "genre_aliases", "genre_id", genre.ID, library.ID, "Spooky")

	book := &models.Book{
		LibraryID: library.ID, Filepath: "/test/horror", Title: "It",
		TitleSource: "file", SortTitle: "It", SortTitleSource: "file", AuthorSource: "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{LibraryID: library.ID, BookID: book.ID, Filepath: "/test/horror/it.epub", FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, FilesizeBytes: 1000}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	book.Files = []*models.File{file}

	svc := NewService(db)
	err = svc.IndexBook(ctx, book)
	require.NoError(t, err)

	results, err := svc.GlobalSearch(ctx, library.ID, "Spooky")
	require.NoError(t, err)
	require.Empty(t, results.Books, "Genre aliases should not be in books_fts")
}

func TestIndexPublisher_IncludesAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	publisher := &models.Publisher{LibraryID: library.ID, Name: "Penguin Random House"}
	_, err = db.NewInsert().Model(publisher).Exec(ctx)
	require.NoError(t, err)

	insertAlias(t, db, "publisher_aliases", "publisher_id", publisher.ID, library.ID, "PRH")

	svc := NewService(db)
	err = svc.IndexPublisher(ctx, publisher)
	require.NoError(t, err)

	var count int
	err = db.NewSelect().TableExpr("publishers_fts").
		ColumnExpr("COUNT(*)").
		Where("publishers_fts MATCH ?", `"PRH"`).
		Where("library_id = ?", library.ID).
		Scan(ctx, &count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "Should find publisher by alias 'PRH'")
}

func TestIndexImprint_IncludesAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	imprint := &models.Imprint{LibraryID: library.ID, Name: "Del Rey"}
	_, err = db.NewInsert().Model(imprint).Exec(ctx)
	require.NoError(t, err)

	insertAlias(t, db, "imprint_aliases", "imprint_id", imprint.ID, library.ID, "DelRey Books")

	svc := NewService(db)
	err = svc.IndexImprint(ctx, imprint)
	require.NoError(t, err)

	var count int
	err = db.NewSelect().TableExpr("imprints_fts").
		ColumnExpr("COUNT(*)").
		Where("imprints_fts MATCH ?", `"DelRey"`).
		Where("library_id = ?", library.ID).
		Scan(ctx, &count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "Should find imprint by alias 'DelRey Books'")
}

func TestRebuildAllIndexes_IncludesAliases(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t)
	ctx := context.Background()

	library := &models.Library{Name: "Lib", CoverAspectRatio: "book"}
	_, err := db.NewInsert().Model(library).Exec(ctx)
	require.NoError(t, err)

	person := &models.Person{LibraryID: library.ID, Name: "Stephen King", SortName: "King, Stephen", SortNameSource: "file"}
	_, err = db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)
	insertAlias(t, db, "person_aliases", "person_id", person.ID, library.ID, "Richard Bachman")

	series := &models.Series{LibraryID: library.ID, Name: "The Dark Tower", NameSource: "file", SortName: "Dark Tower, The", SortNameSource: "file"}
	_, err = db.NewInsert().Model(series).Exec(ctx)
	require.NoError(t, err)
	insertAlias(t, db, "series_aliases", "series_id", series.ID, library.ID, "DT Series")

	genre := &models.Genre{LibraryID: library.ID, Name: "Horror"}
	_, err = db.NewInsert().Model(genre).Exec(ctx)
	require.NoError(t, err)
	insertAlias(t, db, "genre_aliases", "genre_id", genre.ID, library.ID, "Spooky")

	tag := &models.Tag{LibraryID: library.ID, Name: "Bestseller"}
	_, err = db.NewInsert().Model(tag).Exec(ctx)
	require.NoError(t, err)
	insertAlias(t, db, "tag_aliases", "tag_id", tag.ID, library.ID, "Popular")

	publisher := &models.Publisher{LibraryID: library.ID, Name: "Simon & Schuster"}
	_, err = db.NewInsert().Model(publisher).Exec(ctx)
	require.NoError(t, err)
	insertAlias(t, db, "publisher_aliases", "publisher_id", publisher.ID, library.ID, "S&S")

	imprint := &models.Imprint{LibraryID: library.ID, Name: "Scribner"}
	_, err = db.NewInsert().Model(imprint).Exec(ctx)
	require.NoError(t, err)
	insertAlias(t, db, "imprint_aliases", "imprint_id", imprint.ID, library.ID, "Scribner Books")

	book := &models.Book{
		LibraryID: library.ID, Filepath: "/test/dt", Title: "The Gunslinger",
		TitleSource: "file", SortTitle: "Gunslinger, The", SortTitleSource: "file", AuthorSource: "file",
	}
	_, err = db.NewInsert().Model(book).Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewInsert().Model(&models.Author{}).
		Value("book_id", "?", book.ID).Value("person_id", "?", person.ID).Value("sort_order", "?", 0).Exec(ctx)
	require.NoError(t, err)

	_, err = db.NewInsert().Model(&models.BookSeries{}).
		Value("book_id", "?", book.ID).Value("series_id", "?", series.ID).Value("sort_order", "?", 0).Exec(ctx)
	require.NoError(t, err)

	file := &models.File{LibraryID: library.ID, BookID: book.ID, Filepath: "/test/dt/gunslinger.epub", FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, FilesizeBytes: 1000}
	_, err = db.NewInsert().Model(file).Exec(ctx)
	require.NoError(t, err)

	svc := NewService(db)
	err = svc.RebuildAllIndexes(ctx)
	require.NoError(t, err)

	people, _, err := svc.SearchPeople(ctx, library.ID, "Bachman", 10, 0)
	require.NoError(t, err)
	require.Len(t, people, 1, "RebuildAllIndexes should include person aliases")

	seriesResults, _, err := svc.SearchSeries(ctx, library.ID, "DT Series", 10, 0)
	require.NoError(t, err)
	require.Len(t, seriesResults, 1, "RebuildAllIndexes should include series aliases")

	var genreCount int
	err = db.NewSelect().TableExpr("genres_fts").ColumnExpr("COUNT(*)").
		Where("genres_fts MATCH ?", `"Spooky"`).Where("library_id = ?", library.ID).Scan(ctx, &genreCount)
	require.NoError(t, err)
	require.Equal(t, 1, genreCount, "RebuildAllIndexes should include genre aliases")

	var tagCount int
	err = db.NewSelect().TableExpr("tags_fts").ColumnExpr("COUNT(*)").
		Where("tags_fts MATCH ?", `"Popular"`).Where("library_id = ?", library.ID).Scan(ctx, &tagCount)
	require.NoError(t, err)
	require.Equal(t, 1, tagCount, "RebuildAllIndexes should include tag aliases")

	bookResults, err := svc.GlobalSearch(ctx, library.ID, "Bachman")
	require.NoError(t, err)
	require.Len(t, bookResults.Books, 1, "RebuildAllIndexes should include author aliases in books_fts")

	bookResults, err = svc.GlobalSearch(ctx, library.ID, "DT Series")
	require.NoError(t, err)
	require.Len(t, bookResults.Books, 1, "RebuildAllIndexes should include series aliases in books_fts")

	var pubCount int
	err = db.NewSelect().TableExpr("publishers_fts").ColumnExpr("COUNT(*)").
		Where("publishers_fts MATCH ?", `"S&S"`).Where("library_id = ?", library.ID).Scan(ctx, &pubCount)
	require.NoError(t, err)
	require.Equal(t, 1, pubCount, "RebuildAllIndexes should include publisher aliases")

	var impCount int
	err = db.NewSelect().TableExpr("imprints_fts").ColumnExpr("COUNT(*)").
		Where("imprints_fts MATCH ?", `"Scribner Books"`).Where("library_id = ?", library.ID).Scan(ctx, &impCount)
	require.NoError(t, err)
	require.Equal(t, 1, impCount, "RebuildAllIndexes should include imprint aliases")
}
