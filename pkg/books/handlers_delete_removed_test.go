package books

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// removedFixture is a Book whose Series, Author, Narrator, Genre, Tag, and
// Publisher are used by nothing else, plus a Series it shares with a second
// Book.
type removedFixture struct {
	user      *models.User
	book      *models.Book
	file      *models.File
	series    *models.Series
	shared    *models.Series
	author    *models.Person
	narrator  *models.Person
	genre     *models.Genre
	tag       *models.Tag
	publisher *models.Publisher
}

func setupRemovedFixture(t *testing.T, db *bun.DB) removedFixture {
	t.Helper()
	ctx := context.Background()
	insert := func(model any) {
		t.Helper()
		_, err := db.NewInsert().Model(model).Exec(ctx)
		require.NoError(t, err)
	}

	library, book := setupTestLibraryAndBook(t, db)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))

	other := &models.Book{LibraryID: library.ID, Title: "Other", TitleSource: models.DataSourceFilepath, SortTitle: "Other", SortTitleSource: models.DataSourceFilepath, AuthorSource: models.DataSourceFilepath, Filepath: t.TempDir()}
	insert(other)

	series := &models.Series{LibraryID: library.ID, Name: "Only Saga", NameSource: models.DataSourceManual, SortName: "Only Saga", SortNameSource: models.DataSourceFilepath}
	insert(series)
	insert(&models.BookSeries{BookID: book.ID, SeriesID: series.ID, SortOrder: 1})
	shared := &models.Series{LibraryID: library.ID, Name: "Shared Saga", NameSource: models.DataSourceManual, SortName: "Shared Saga", SortNameSource: models.DataSourceFilepath}
	insert(shared)
	insert(&models.BookSeries{BookID: book.ID, SeriesID: shared.ID, SortOrder: 2})
	insert(&models.BookSeries{BookID: other.ID, SeriesID: shared.ID, SortOrder: 1})

	author := &models.Person{LibraryID: library.ID, Name: "Only Author", SortName: "Author, Only", SortNameSource: models.DataSourceFilepath}
	insert(author)
	insert(&models.Author{BookID: book.ID, PersonID: author.ID, SortOrder: 1})
	genre := &models.Genre{LibraryID: library.ID, Name: "Only Genre"}
	insert(genre)
	insert(&models.BookGenre{BookID: book.ID, GenreID: genre.ID})
	tag := &models.Tag{LibraryID: library.ID, Name: "Only Tag"}
	insert(tag)
	insert(&models.BookTag{BookID: book.ID, TagID: tag.ID})
	publisher := &models.Publisher{LibraryID: library.ID, Name: "Only Press"}
	insert(publisher)

	filePath := filepath.Join(book.Filepath, "only.epub")
	require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o644))
	file := &models.File{LibraryID: library.ID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain, Filepath: filePath, FilesizeBytes: 7, PublisherID: &publisher.ID}
	insert(file)
	narrator := &models.Person{LibraryID: library.ID, Name: "Only Narrator", SortName: "Narrator, Only", SortNameSource: models.DataSourceFilepath}
	insert(narrator)
	insert(&models.Narrator{FileID: file.ID, PersonID: narrator.ID, SortOrder: 1})

	return removedFixture{user, book, file, series, shared, author, narrator, genre, tag, publisher}
}

// assertAllRemoved checks that every entity only the fixture's Book used is
// reported, and the Shared Series, which still has a Book, is not.
func (f removedFixture) assertAllRemoved(t *testing.T, removed RemovedEntities) {
	t.Helper()
	assert.Equal(t, []int{f.series.ID}, removed.SeriesIDs, "the Shared Series still has a Book and is not reported")
	assert.ElementsMatch(t, []int{f.author.ID, f.narrator.ID}, removed.PersonIDs)
	assert.Equal(t, []int{f.genre.ID}, removed.GenreIDs)
	assert.Equal(t, []int{f.tag.ID}, removed.TagIDs)
	assert.Equal(t, []int{f.publisher.ID}, removed.PublisherIDs)
}

func decodeResponse[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp T
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp
}

// The book page leaves for the previous page after a delete, unless that page
// belongs to something the delete removed, so the response names them.
func TestDeleteBook_ReportsRemovedEntities(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	f := setupRemovedFixture(t, db)
	e := setupTestServer(t, db)

	req := httptest.NewRequest(http.MethodDelete, "/books/"+strconv.Itoa(f.book.ID), nil)
	resp := decodeResponse[DeleteBookResponse](t, executeRequestWithUser(t, e, req, f.user))

	f.assertAllRemoved(t, resp.Removed)
}

func TestDeleteFile_LastFile_ReportsRemovedEntities(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	f := setupRemovedFixture(t, db)
	e := setupTestServer(t, db)

	req := httptest.NewRequest(http.MethodDelete, "/books/files/"+strconv.Itoa(f.file.ID), nil)
	resp := decodeResponse[DeleteFileResponse](t, executeRequestWithUser(t, e, req, f.user))

	assert.True(t, resp.BookDeleted)
	f.assertAllRemoved(t, resp.Removed)
}

// When the Book survives, only the People cleanup runs, so the deleted File's
// Narrator is the only entity gone.
func TestDeleteFile_BookSurvives_ReportsRemovedNarrator(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	f := setupRemovedFixture(t, db)
	keptPath := filepath.Join(f.book.Filepath, "kept.epub")
	require.NoError(t, os.WriteFile(keptPath, []byte("content"), 0o644))
	setupTestFile(t, db, f.book, models.FileTypeEPUB, keptPath)
	e := setupTestServer(t, db)

	req := httptest.NewRequest(http.MethodDelete, "/books/files/"+strconv.Itoa(f.file.ID), nil)
	resp := decodeResponse[DeleteFileResponse](t, executeRequestWithUser(t, e, req, f.user))

	assert.False(t, resp.BookDeleted)
	assert.Equal(t, []int{f.narrator.ID}, resp.Removed.PersonIDs)
	assert.Empty(t, resp.Removed.SeriesIDs)
	assert.Empty(t, resp.Removed.PublisherIDs)
}

// Nothing removed is reported as empty lists, never null, so the frontend can
// read them without a nil check.
func TestDeleteBook_ReportsEmptyRemovedLists(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	library, book := setupTestLibraryAndBook(t, db)
	user := loadUserWithRole(t, db, setupTestUser(t, db, library.ID, true))
	e := setupTestServer(t, db)

	req := httptest.NewRequest(http.MethodDelete, "/books/"+strconv.Itoa(book.ID), nil)
	rr := executeRequestWithUser(t, e, req, user)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), `"removed":{"series_ids":[],"person_ids":[],"genre_ids":[],"tag_ids":[],"publisher_ids":[]}`)
}

// bookDeletingScanner stands in for a rescan that finds the Book gone from
// disk. It deletes the Book, and the fixture's Series as another cleanup
// would, without this request running any orphan cleanup itself.
type bookDeletingScanner struct {
	db     *bun.DB
	book   *models.Book
	series *models.Series
}

func (s *bookDeletingScanner) Scan(ctx context.Context, _ ScanOptions) (*ScanResult, error) {
	if _, err := s.db.NewDelete().Model((*models.Book)(nil)).Where("id = ?", s.book.ID).Exec(ctx); err != nil {
		return nil, err
	}
	if _, err := s.db.NewDelete().Model((*models.Series)(nil)).Where("id = ?", s.series.ID).Exec(ctx); err != nil {
		return nil, err
	}
	return &ScanResult{FileDeleted: true, BookDeleted: true}, nil
}

// A rescan that deletes the Book reports what is gone however it went, so a
// cleanup added to the scan later, or one another request ran, is covered.
func TestResync_BookDeleted_ReportsRemovedEntities(t *testing.T) {
	t.Parallel()
	for name, path := range map[string]func(f removedFixture) string{
		"book": func(f removedFixture) string { return "/books/" + strconv.Itoa(f.book.ID) + "/resync" },
		"file": func(f removedFixture) string { return "/books/files/" + strconv.Itoa(f.file.ID) + "/resync" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			f := setupRemovedFixture(t, db)
			e := setupTestServerWithScanner(t, db, &bookDeletingScanner{db: db, book: f.book, series: f.series})

			req := httptest.NewRequest(http.MethodPost, path(f), strings.NewReader(`{}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			resp := decodeResponse[struct {
				BookDeleted bool            `json:"book_deleted"`
				Removed     RemovedEntities `json:"removed"`
			}](t, executeRequestWithUser(t, e, req, f.user))

			assert.True(t, resp.BookDeleted)
			assert.Equal(t, []int{f.series.ID}, resp.Removed.SeriesIDs)
			assert.Empty(t, resp.Removed.PersonIDs, "no cleanup ran, so the Author and Narrator still exist")
		})
	}
}

// A File delete that keeps the Book runs only the People cleanup, so the
// File's Publisher can outlive it unused. Deleting the Book later sweeps that
// Publisher up, and it is reported though the Book no longer links to it.
func TestDeleteBook_ReportsOrphanItNoLongerLinked(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	f := setupRemovedFixture(t, db)
	keptPath := filepath.Join(f.book.Filepath, "kept.epub")
	require.NoError(t, os.WriteFile(keptPath, []byte("content"), 0o644))
	setupTestFile(t, db, f.book, models.FileTypeEPUB, keptPath)
	e := setupTestServer(t, db)

	req := httptest.NewRequest(http.MethodDelete, "/books/files/"+strconv.Itoa(f.file.ID), nil)
	first := decodeResponse[DeleteFileResponse](t, executeRequestWithUser(t, e, req, f.user))
	require.Empty(t, first.Removed.PublisherIDs, "precondition: the Publisher outlives its File")

	req = httptest.NewRequest(http.MethodDelete, "/books/"+strconv.Itoa(f.book.ID), nil)
	resp := decodeResponse[DeleteBookResponse](t, executeRequestWithUser(t, e, req, f.user))

	assert.Equal(t, []int{f.publisher.ID}, resp.Removed.PublisherIDs)
	assert.Equal(t, []int{f.series.ID}, resp.Removed.SeriesIDs)
}
