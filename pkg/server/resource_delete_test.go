package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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
	srv, err := New(cfg, db, worker.New(&config.Config{WorkerProcesses: 1}, db, nil, nil, nil), nil, nil, downloadcache.NewCache(t.TempDir(), 1<<30), nil, nil, nil)
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
	token, err := f.authSvc.GenerateToken(f.admin)
	require.NoError(f.t, err)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	require.Equal(f.t, http.StatusNoContent, rec.Code, "response body: %s", rec.Body.String())
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
