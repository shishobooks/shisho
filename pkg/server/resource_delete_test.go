package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/books/review"
	"github.com/shishobooks/shisho/pkg/config"
	"github.com/shishobooks/shisho/pkg/downloadcache"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/search"
	"github.com/shishobooks/shisho/pkg/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// resourceDeleteFixture serves the real route table, so these tests also
// cover the wiring in server.go that hands each delete handler a books
// service with app settings attached. Without app settings the review
// recompute silently does nothing.
type resourceDeleteFixture struct {
	t         *testing.T
	ctx       context.Context
	db        *bun.DB
	handler   http.Handler
	authSvc   *auth.Service
	searchSvc *search.Service
	lib       *models.Library
	admin     *models.User
}

func newResourceDeleteFixture(t *testing.T) *resourceDeleteFixture {
	t.Helper()
	db := newPermissionTestDB(t)
	cfg := newPermissionTestConfig(t)
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil, nil), nil, nil, nil, downloadcache.NewCache(t.TempDir(), 1<<30), nil, nil, nil)
	require.NoError(t, err)
	f := &resourceDeleteFixture{
		t:         t,
		ctx:       context.Background(),
		db:        db,
		handler:   srv.Handler,
		authSvc:   auth.NewService(db, cfg.JWTSecret, cfg.SessionDuration()),
		searchSvc: search.NewService(db),
	}

	f.lib = &models.Library{Name: "Library", CoverAspectRatio: "book", DownloadFormatPreference: models.DownloadFormatOriginal}
	f.insert(f.lib)
	f.admin = insertPermissionTestUser(f.ctx, t, db, "admin", models.RoleAdmin, nil)

	return f
}

func (f *resourceDeleteFixture) delete(path string) {
	f.t.Helper()
	f.request(http.MethodDelete, path, "", http.StatusNoContent)
}

// request sends an authenticated admin request with an optional JSON body,
// requires wantStatus, and returns the response body.
func (f *resourceDeleteFixture) request(method, path, body string, wantStatus int) string {
	f.t.Helper()
	token, err := f.authSvc.GenerateToken(f.admin)
	require.NoError(f.t, err)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	require.Equal(f.t, wantStatus, rec.Code, "response body: %s", rec.Body.String())
	return rec.Body.String()
}

func (f *resourceDeleteFixture) insert(model any) {
	f.t.Helper()
	_, err := f.db.NewInsert().Model(model).Exec(f.ctx)
	require.NoError(f.t, err)
}

func (f *resourceDeleteFixture) person(name string) *models.Person {
	f.t.Helper()
	p := &models.Person{LibraryID: f.lib.ID, Name: name, SortName: name, SortNameSource: models.DataSourceFilepath}
	f.insert(p)
	return p
}

// reviewedBook is a Book with one File that meets the review criteria.
type reviewedBook struct {
	bookID   int
	fileID   int
	author   *models.Person
	narrator *models.Person // set only for an M4B File
	genre    *models.Genre
	tag      *models.Tag
}

// seedReviewedBook creates a Book with an Author, a Genre, a Tag, a
// description, and one File of the given type with a cover. An M4B File also
// gets a Narrator. With shared nil the Book is titled "Harbor Lights" and
// gets new People, a new Genre, and a new Tag; otherwise it is titled "Quiet
// Tides" and reuses shared's. The Book is indexed in books_fts and its File
// is marked reviewed under the stored criteria, which the helper asserts.
func (f *resourceDeleteFixture) seedReviewedBook(fileType string, shared *reviewedBook) reviewedBook {
	f.t.Helper()
	title := "Harbor Lights"
	if shared != nil {
		title = "Quiet Tides"
	}
	description := "A complete description"
	book := &models.Book{
		LibraryID:       f.lib.ID,
		Title:           title,
		TitleSource:     models.DataSourceManual,
		SortTitle:       title,
		SortTitleSource: models.DataSourceFilepath,
		AuthorSource:    models.DataSourceManual,
		Description:     &description,
		Filepath:        f.t.TempDir(),
	}
	f.insert(book)

	cover := "book.cover.jpg"
	file := &models.File{
		LibraryID:          f.lib.ID,
		BookID:             book.ID,
		FileType:           fileType,
		FileRole:           models.FileRoleMain,
		Filepath:           fmt.Sprintf("%s/book.%s", book.Filepath, fileType),
		FilesizeBytes:      1,
		CoverImageFilename: &cover,
	}
	f.insert(file)

	seeded := reviewedBook{bookID: book.ID, fileID: file.ID}
	if shared != nil {
		seeded.author, seeded.narrator, seeded.genre, seeded.tag = shared.author, shared.narrator, shared.genre, shared.tag
	} else {
		seeded.author = f.person("Zephyrine Quillfeather")
		if fileType == models.FileTypeM4B {
			seeded.narrator = f.person("Ottoline Brackenridge")
		}
		seeded.genre = &models.Genre{LibraryID: f.lib.ID, Name: "Solarpunk"}
		f.insert(seeded.genre)
		seeded.tag = &models.Tag{LibraryID: f.lib.ID, Name: "Wistful"}
		f.insert(seeded.tag)
	}
	f.insert(&models.Author{BookID: book.ID, PersonID: seeded.author.ID, SortOrder: 1})
	if seeded.narrator != nil {
		f.insert(&models.Narrator{FileID: file.ID, PersonID: seeded.narrator.ID, SortOrder: 1})
	}
	f.insert(&models.BookGenre{BookID: book.ID, GenreID: seeded.genre.ID})
	f.insert(&models.BookTag{BookID: book.ID, TagID: seeded.tag.ID})

	require.NoError(f.t, f.searchSvc.ReindexBookByID(f.ctx, book.ID))
	criteria, err := review.Load(f.ctx, appsettings.NewService(f.db))
	require.NoError(f.t, err)
	require.NoError(f.t, review.RecomputeForBook(f.ctx, f.db, book.ID, criteria))
	require.True(f.t, f.reviewed(file.ID), "precondition: the File is reviewed before the delete")
	return seeded
}

func (f *resourceDeleteFixture) reviewed(fileID int) bool {
	f.t.Helper()
	var file models.File
	require.NoError(f.t, f.db.NewSelect().Model(&file).Where("f.id = ?", fileID).Scan(f.ctx))
	require.NotNil(f.t, file.Reviewed)
	return *file.Reviewed
}

// searchPersonIDs returns the IDs of the People that a people search for
// query matches.
func (f *resourceDeleteFixture) searchPersonIDs(query string) []int {
	f.t.Helper()
	results, _, err := f.searchSvc.SearchPeople(f.ctx, f.lib.ID, query, 10, 0)
	require.NoError(f.t, err)
	ids := []int{}
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	return ids
}

// searchBookIDs returns the IDs of the Books that a book search for query
// matches.
func (f *resourceDeleteFixture) searchBookIDs(query string) []int {
	f.t.Helper()
	results, _, err := f.searchSvc.SearchBooks(f.ctx, f.lib.ID, query, nil, 10, 0)
	require.NoError(f.t, err)
	ids := []int{}
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	return ids
}

// Deleting a Person who authored Books refreshes each Book's books_fts row,
// so a search for the old author name stops returning them, and recomputes
// their review state, since authors are required by default.
func TestDeletePerson_ReindexesAndRecomputesAuthoredBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	first := f.seedReviewedBook(models.FileTypeEPUB, nil)
	second := f.seedReviewedBook(models.FileTypeEPUB, &first)
	require.ElementsMatch(t, []int{first.bookID, second.bookID}, f.searchBookIDs("Quillfeather"), "precondition: the author name matches both Books")

	f.delete(fmt.Sprintf("/api/people/%d", first.author.ID))

	assert.Empty(t, f.searchBookIDs("Quillfeather"), "the deleted author name no longer matches either Book")
	assert.Equal(t, []int{first.bookID}, f.searchBookIDs("Harbor"), "the first Book stays in the search index")
	assert.Equal(t, []int{second.bookID}, f.searchBookIDs("Quiet"), "the second Book stays in the search index")
	assert.False(t, f.reviewed(first.fileID), "the first File leaves reviewed once its only author is gone")
	assert.False(t, f.reviewed(second.fileID), "the second File leaves reviewed once its only author is gone")
}

// Deleting a Person who narrated a File refreshes the owning Book's
// books_fts row and recomputes its review state, since audio Files require
// narrators by default.
func TestDeletePerson_ReindexesAndRecomputesNarratedBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeM4B, nil)
	require.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Brackenridge"), "precondition: the narrator name matches the Book")

	f.delete(fmt.Sprintf("/api/people/%d", seeded.narrator.ID))

	assert.Empty(t, f.searchBookIDs("Brackenridge"), "the deleted narrator name no longer matches the Book")
	assert.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Quillfeather"), "the remaining author still matches")
	assert.False(t, f.reviewed(seeded.fileID), "the File leaves reviewed once its only narrator is gone")
}

// Deleting a Genre recomputes review state for every Book that carried it,
// since genres are required by default.
func TestDeleteGenre_RecomputesReviewedForAffectedBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	first := f.seedReviewedBook(models.FileTypeEPUB, nil)
	second := f.seedReviewedBook(models.FileTypeEPUB, &first)

	f.delete(fmt.Sprintf("/api/genres/%d", first.genre.ID))

	assert.False(t, f.reviewed(first.fileID), "the first File leaves reviewed once its only genre is gone")
	assert.False(t, f.reviewed(second.fileID), "the second File leaves reviewed once its only genre is gone")
	assert.Equal(t, []int{first.bookID}, f.searchBookIDs("Harbor"), "the Book stays in the search index")
}

// Deleting a Tag recomputes review state for the Books that carried it when
// the review criteria require tags.
func TestDeleteTag_RecomputesReviewedForAffectedBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	criteria := review.Default()
	criteria.BookFields = append(criteria.BookFields, review.FieldTags)
	require.NoError(t, review.Save(f.ctx, appsettings.NewService(f.db), criteria))
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)

	f.delete(fmt.Sprintf("/api/tags/%d", seeded.tag.ID))

	assert.False(t, f.reviewed(seeded.fileID), "the File leaves reviewed once its only tag is gone")
	assert.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Harbor"), "the Book stays in the search index")
}

// Deleting a Series recomputes review state for the Books in it when the
// review criteria require a series.
func TestDeleteSeries_RecomputesReviewedForAffectedBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	series := &models.Series{
		LibraryID:      f.lib.ID,
		Name:           "Lanternfall Cycle",
		NameSource:     models.DataSourceManual,
		SortName:       "Lanternfall Cycle",
		SortNameSource: models.DataSourceFilepath,
	}
	f.insert(series)
	f.insert(&models.BookSeries{BookID: seeded.bookID, SeriesID: series.ID, SortOrder: 1})
	criteria := review.Default()
	criteria.BookFields = append(criteria.BookFields, review.FieldSeries)
	require.NoError(t, review.Save(f.ctx, appsettings.NewService(f.db), criteria))
	require.NoError(t, review.RecomputeForBook(f.ctx, f.db, seeded.bookID, criteria))
	require.True(t, f.reviewed(seeded.fileID), "precondition: the File is reviewed while it has a series")

	f.delete(fmt.Sprintf("/api/series/%d", series.ID))

	assert.False(t, f.reviewed(seeded.fileID), "the File leaves reviewed once its only series is gone")
	assert.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Harbor"), "the Book stays in the search index")
}

// Deleting a Publisher recomputes review state for the Books whose Files
// carried it when the review criteria require a publisher.
func TestDeletePublisher_RecomputesReviewedForAffectedBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	first := f.seedReviewedBook(models.FileTypeEPUB, nil)
	second := f.seedReviewedBook(models.FileTypeEPUB, &first)
	criteria := review.Default()
	criteria.BookFields = append(criteria.BookFields, review.FieldPublisher)
	require.NoError(t, review.Save(f.ctx, appsettings.NewService(f.db), criteria))
	publisher := &models.Publisher{LibraryID: f.lib.ID, Name: "Driftwood Press"}
	f.insert(publisher)
	f.setPublisher(first, publisher, criteria)
	f.setPublisher(second, publisher, criteria)

	f.delete(fmt.Sprintf("/api/publishers/%d", publisher.ID))

	assert.False(t, f.reviewed(first.fileID), "the first File leaves reviewed once its publisher is gone")
	assert.False(t, f.reviewed(second.fileID), "the second File leaves reviewed once its publisher is gone")
}

// setPublisher gives the seeded Book's File the publisher and asserts the
// File is reviewed under criteria.
func (f *resourceDeleteFixture) setPublisher(seeded reviewedBook, publisher *models.Publisher, criteria review.Criteria) {
	f.t.Helper()
	_, err := f.db.NewUpdate().
		Model((*models.File)(nil)).
		Set("publisher_id = ?", publisher.ID).
		Where("id = ?", seeded.fileID).
		Exec(f.ctx)
	require.NoError(f.t, err)
	require.NoError(f.t, review.RecomputeForBook(f.ctx, f.db, seeded.bookID, criteria))
	require.True(f.t, f.reviewed(seeded.fileID), "precondition: the File is reviewed while it has a publisher")
}

// Merging a Person into another re-indexes the target in persons_fts, so the
// transferred names find it in people search, and re-indexes every Book the
// source authored or narrated, so book search finds them by the target name.
func TestMergePeople_ReindexesTargetAndAffectedBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	authored := f.seedReviewedBook(models.FileTypeEPUB, nil)
	narrated := f.seedReviewedBook(models.FileTypeEPUB, &authored)
	source := f.person("Ottoline Brackenridge")
	f.insert(&models.PersonAlias{PersonID: source.ID, Name: "Otto Bracken", LibraryID: f.lib.ID})
	f.insert(&models.Author{BookID: authored.bookID, PersonID: source.ID, SortOrder: 2})
	f.insert(&models.Narrator{FileID: narrated.fileID, PersonID: source.ID, SortOrder: 1})
	require.NoError(t, f.searchSvc.ReindexBookByID(f.ctx, authored.bookID))
	require.NoError(t, f.searchSvc.ReindexBookByID(f.ctx, narrated.bookID))

	target := f.person("Marigold Ashcombe")
	require.NoError(t, f.searchSvc.IndexPerson(f.ctx, source))
	require.NoError(t, f.searchSvc.IndexPerson(f.ctx, target))
	require.Empty(t, f.searchBookIDs("Ashcombe"), "precondition: no Book matches the target name")

	f.request(http.MethodPost, fmt.Sprintf("/api/people/%d/merge", target.ID), fmt.Sprintf(`{"source_id":%d}`, source.ID), http.StatusNoContent)

	assert.Equal(t, []int{target.ID}, f.searchPersonIDs("Otto Bracken"), "the source's alias now finds the target")
	assert.Equal(t, []int{target.ID}, f.searchPersonIDs("Brackenridge"), "the source name, now an alias, finds the target")
	assert.ElementsMatch(t, []int{authored.bookID, narrated.bookID}, f.searchBookIDs("Ashcombe"), "the target name matches the authored and narrated Books")
}

// Deleting one File of a multi-file Book re-indexes the surviving Book, so
// book search no longer matches it by the deleted File's narrator.
func TestDeleteFile_ReindexesSurvivingBook(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeM4B, nil)
	var book models.Book
	require.NoError(t, f.db.NewSelect().Model(&book).Where("b.id = ?", seeded.bookID).Scan(f.ctx))
	f.insert(&models.File{
		LibraryID:     f.lib.ID,
		BookID:        seeded.bookID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      book.Filepath + "/book.epub",
		FilesizeBytes: 1,
	})
	require.NoError(t, f.searchSvc.ReindexBookByID(f.ctx, seeded.bookID))
	require.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Brackenridge"), "precondition: the narrator name matches the Book")

	body := f.request(http.MethodDelete, fmt.Sprintf("/api/books/files/%d", seeded.fileID), "", http.StatusOK)

	assert.JSONEq(t, `{"book_deleted":false}`, body, "the Book survives with its EPUB File")
	assert.Empty(t, f.searchBookIDs("Brackenridge"), "the deleted File's narrator no longer matches the Book")
	assert.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Harbor"), "the surviving Book stays in the search index")
}

// seriesWithBooks creates a Series holding the given Books and indexes it in
// series_fts, whose book_authors column carries the Books' author names.
func (f *resourceDeleteFixture) seriesWithBooks(name string, bookIDs ...int) *models.Series {
	f.t.Helper()
	series := &models.Series{
		LibraryID:      f.lib.ID,
		Name:           name,
		NameSource:     models.DataSourceManual,
		SortName:       name,
		SortNameSource: models.DataSourceFilepath,
	}
	f.insert(series)
	for i, bookID := range bookIDs {
		f.insert(&models.BookSeries{BookID: bookID, SeriesID: series.ID, SortOrder: i + 1})
	}
	require.NoError(f.t, f.searchSvc.IndexSeries(f.ctx, series))
	return series
}

// searchSeriesIDs returns the IDs of the Series that a series search for
// query matches.
func (f *resourceDeleteFixture) searchSeriesIDs(query string) []int {
	f.t.Helper()
	results, _, err := f.searchSvc.SearchSeries(f.ctx, f.lib.ID, query, 10, 0)
	require.NoError(f.t, err)
	ids := []int{}
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	return ids
}

// Merging People who author the same Book in the same role succeeds instead
// of tripping ux_authors_book_person_role, and the Book keeps one row for the
// target.
func TestMergePeople_SharedBookSameRole_Succeeds(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	writer := models.AuthorRoleWriter
	_, err := f.db.NewUpdate().Model((*models.Author)(nil)).Set("role = ?", writer).Where("book_id = ?", seeded.bookID).Exec(f.ctx)
	require.NoError(t, err)
	source := f.person("Ottoline Brackenridge")
	f.insert(&models.Author{BookID: seeded.bookID, PersonID: source.ID, SortOrder: 2, Role: &writer})

	f.request(http.MethodPost, fmt.Sprintf("/api/people/%d/merge", seeded.author.ID), fmt.Sprintf(`{"source_id":%d}`, source.ID), http.StatusNoContent)

	var personIDs []int
	require.NoError(t, f.db.NewSelect().Model((*models.Author)(nil)).Column("person_id").Where("book_id = ?", seeded.bookID).Scan(f.ctx, &personIDs))
	assert.Equal(t, []int{seeded.author.ID}, personIDs)
}

// Renaming a Person re-indexes the Series of the Books it authored, so a
// series search matches the new name and stops matching the old one.
func TestUpdatePerson_RenameReindexesAuthoredSeries(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	series := f.seriesWithBooks("Lanternfall Cycle", seeded.bookID)
	require.Equal(t, []int{series.ID}, f.searchSeriesIDs("Quillfeather"), "precondition: the author name matches the Series")

	f.request(http.MethodPatch, fmt.Sprintf("/api/people/%d", seeded.author.ID), `{"name":"Wilhelmina Starling"}`, http.StatusOK)

	assert.Empty(t, f.searchSeriesIDs("Quillfeather"), "the old author name no longer matches the Series")
	assert.Equal(t, []int{series.ID}, f.searchSeriesIDs("Starling"), "the new author name matches the Series")
}

// Deleting a Person re-indexes the Series of the Books it authored, so a
// series search stops matching the deleted name.
func TestDeletePerson_ReindexesAuthoredSeries(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	series := f.seriesWithBooks("Lanternfall Cycle", seeded.bookID)
	require.Equal(t, []int{series.ID}, f.searchSeriesIDs("Quillfeather"), "precondition: the author name matches the Series")

	f.delete(fmt.Sprintf("/api/people/%d", seeded.author.ID))

	assert.Empty(t, f.searchSeriesIDs("Quillfeather"), "the deleted author name no longer matches the Series")
	assert.Equal(t, []int{series.ID}, f.searchSeriesIDs("Lanternfall"), "the Series stays in the search index")
}

// Merging a Person re-indexes the Series of the Books the source authored,
// so a series search matches the target name and stops matching the source
// name, which series_fts does not carry as an alias.
func TestMergePeople_ReindexesSourceSeries(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	series := f.seriesWithBooks("Lanternfall Cycle", seeded.bookID)
	target := f.person("Marigold Ashcombe")
	require.Empty(t, f.searchSeriesIDs("Ashcombe"), "precondition: the target name does not match the Series")

	f.request(http.MethodPost, fmt.Sprintf("/api/people/%d/merge", target.ID), fmt.Sprintf(`{"source_id":%d}`, seeded.author.ID), http.StatusNoContent)

	assert.Equal(t, []int{series.ID}, f.searchSeriesIDs("Ashcombe"), "the target name matches the Series")
	assert.Empty(t, f.searchSeriesIDs("Quillfeather"), "the merged source name no longer matches the Series")
}

// Deleting the only File a Narrator narrated, while the Book survives with
// another File, removes the now orphaned Narrator and its persons_fts row.
// The cleanup covers People only, so an unrelated orphaned Genre is left for
// the next Scan, as before.
func TestDeleteFile_RemovesOrphanedNarrator(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeM4B, nil)
	var book models.Book
	require.NoError(t, f.db.NewSelect().Model(&book).Where("b.id = ?", seeded.bookID).Scan(f.ctx))
	f.insert(&models.File{
		LibraryID:     f.lib.ID,
		BookID:        seeded.bookID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      book.Filepath + "/book.epub",
		FilesizeBytes: 1,
	})
	require.NoError(t, f.searchSvc.IndexPerson(f.ctx, seeded.narrator))
	require.NoError(t, f.searchSvc.IndexPerson(f.ctx, seeded.author))
	require.Equal(t, []int{seeded.narrator.ID}, f.searchPersonIDs("Brackenridge"), "precondition: the narrator is in people search")
	orphanGenre := &models.Genre{LibraryID: f.lib.ID, Name: "Unused Genre"}
	f.insert(orphanGenre)

	f.request(http.MethodDelete, fmt.Sprintf("/api/books/files/%d", seeded.fileID), "", http.StatusOK)

	exists, err := f.db.NewSelect().Model((*models.Person)(nil)).Where("id = ?", seeded.narrator.ID).Exists(f.ctx)
	require.NoError(t, err)
	assert.False(t, exists, "the orphaned narrator is deleted")
	assert.Empty(t, f.searchPersonIDs("Brackenridge"), "the orphaned narrator leaves people search")
	assert.Equal(t, []int{seeded.author.ID}, f.searchPersonIDs("Quillfeather"), "the author, who still has the Book, stays")
	genreExists, err := f.db.NewSelect().Model((*models.Genre)(nil)).Where("id = ?", orphanGenre.ID).Exists(f.ctx)
	require.NoError(t, err)
	assert.True(t, genreExists, "a single-file delete does not clean up other kinds of orphans")
}

// Renaming a Person without touching its aliases re-indexes the Books it
// authored or narrated, since books_fts stores their names.
func TestUpdatePerson_RenameReindexesBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeM4B, nil)
	require.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Quillfeather"), "precondition: the author name matches the Book")

	f.request(http.MethodPatch, fmt.Sprintf("/api/people/%d", seeded.author.ID), `{"name":"Wilhelmina Starling"}`, http.StatusOK)
	f.request(http.MethodPatch, fmt.Sprintf("/api/people/%d", seeded.narrator.ID), `{"name":"Cordelia Fairweather"}`, http.StatusOK)

	assert.Empty(t, f.searchBookIDs("Quillfeather"), "the old author name no longer matches the Book")
	assert.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Starling"), "the new author name matches the Book")
	assert.Empty(t, f.searchBookIDs("Brackenridge"), "the old narrator name no longer matches the Book")
	assert.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Fairweather"), "the new narrator name matches the Book")
}
