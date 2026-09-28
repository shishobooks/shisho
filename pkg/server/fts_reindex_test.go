package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests cover the mutation paths that used to leave an FTS row stale
// because a handler reindexed only part of what its write touched (#577).
// Each one goes through the real route and then searches for the new value
// (a miss) or the old one (a ghost).

// requestStatus sends an authenticated admin request and returns the status
// code, for requests that are expected to fail.
func (f *resourceDeleteFixture) requestStatus(method, path, body string) int {
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
	return rec.Code
}

// ftsMatch returns the rowids of the rows in an FTS table that match query,
// for the tables the search service has no search method for.
func (f *resourceDeleteFixture) ftsMatch(table, query string) []int {
	f.t.Helper()
	ids := []int{}
	require.NoError(f.t, f.db.NewRaw("SELECT rowid FROM "+table+" WHERE "+table+" MATCH ? ORDER BY rowid", query+"*").Scan(f.ctx, &ids))
	return ids
}

// bookFilepath returns the directory the seeded Book lives in.
func (f *resourceDeleteFixture) bookFilepath(bookID int) string {
	f.t.Helper()
	var book models.Book
	require.NoError(f.t, f.db.NewSelect().Model(&book).Where("b.id = ?", bookID).Scan(f.ctx))
	return book.Filepath
}

func TestUpdateBook_TitleOnlyReindexesSeries(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	series := f.seriesWithBooks("Lanternfall Cycle", seeded.bookID)
	require.Equal(t, []int{series.ID}, f.searchSeriesIDs("Harbor"), "precondition: the title matches the Series")

	f.request(http.MethodPost, fmt.Sprintf("/api/books/%d", seeded.bookID), `{"title":"Omegaverse Rising"}`, http.StatusOK)

	assert.Equal(t, []int{series.ID}, f.searchSeriesIDs("Omegaverse"), "the new title matches the Series")
	assert.Empty(t, f.searchSeriesIDs("Harbor"), "the old title no longer matches the Series")
}

func TestUpdateBook_AuthorsOnlyReindexesSeries(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	series := f.seriesWithBooks("Lanternfall Cycle", seeded.bookID)

	f.request(http.MethodPost, fmt.Sprintf("/api/books/%d", seeded.bookID), `{"authors":[{"name":"Barnaby Thistlewood"}]}`, http.StatusOK)

	assert.Equal(t, []int{series.ID}, f.searchSeriesIDs("Thistlewood"), "the new author matches the Series")
	assert.Empty(t, f.searchSeriesIDs("Quillfeather"), "the old author no longer matches the Series")
}

// Every non-scan Book delete drops the Book from the Series that held it.
// CASCADE removes the book_series row, so the Series has to be captured
// before the delete.
func TestDeleteBook_ReindexesSurvivingSeries(t *testing.T) {
	t.Parallel()
	for name, deleteBook := range map[string]func(f *resourceDeleteFixture, doomed, survivor reviewedBook){
		"delete book": func(f *resourceDeleteFixture, doomed, _ reviewedBook) {
			f.request(http.MethodDelete, fmt.Sprintf("/api/books/%d", doomed.bookID), "", http.StatusOK)
		},
		"bulk delete": func(f *resourceDeleteFixture, doomed, _ reviewedBook) {
			f.request(http.MethodPost, "/api/books/delete", fmt.Sprintf(`{"book_ids":[%d]}`, doomed.bookID), http.StatusOK)
		},
		"delete last file": func(f *resourceDeleteFixture, doomed, _ reviewedBook) {
			body := f.request(http.MethodDelete, fmt.Sprintf("/api/books/files/%d", doomed.fileID), "", http.StatusOK)
			require.JSONEq(f.t, `{"book_deleted":true}`, body)
		},
		"move all files": func(f *resourceDeleteFixture, doomed, survivor reviewedBook) {
			f.request(http.MethodPost, fmt.Sprintf("/api/books/%d/move-files", doomed.bookID), fmt.Sprintf(`{"file_ids":[%d],"target_book_id":%d}`, doomed.fileID, survivor.bookID), http.StatusOK)
		},
		"merge books": func(f *resourceDeleteFixture, doomed, survivor reviewedBook) {
			f.request(http.MethodPost, "/api/books/merge", fmt.Sprintf(`{"target_book_id":%d,"source_book_ids":[%d]}`, survivor.bookID, doomed.bookID), http.StatusOK)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newResourceDeleteFixture(t)
			doomed := f.seedReviewedBook(models.FileTypeEPUB, nil)
			survivor := f.seedReviewedBook(models.FileTypeEPUB, &doomed)
			series := f.seriesWithBooks("Lanternfall Cycle", doomed.bookID)
			require.Equal(t, []int{series.ID}, f.searchSeriesIDs("Harbor"), "precondition: the doomed title matches the Series")
			// Keep the Series alive after the delete so orphan cleanup does
			// not remove it together with its FTS row.
			other := f.seedReviewedBook(models.FileTypeEPUB, &doomed)
			_, err := f.db.NewUpdate().Model((*models.Book)(nil)).Set("title = ?", "Ember Garden").Where("id = ?", other.bookID).Exec(f.ctx)
			require.NoError(t, err)
			f.insert(&models.BookSeries{BookID: other.bookID, SeriesID: series.ID, SortOrder: 2})

			deleteBook(f, doomed, survivor)

			assert.Empty(t, f.searchSeriesIDs("Harbor"), "the deleted Book's title no longer matches the Series")
			assert.Equal(t, []int{series.ID}, f.searchSeriesIDs("Ember"), "the remaining Book's title still matches the Series")
		})
	}
}

// Splitting a File into a new Book copies the source Book's Series
// memberships, so the Series lists the new Book's title.
func TestMoveFiles_SplitReindexesSeries(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	series := f.seriesWithBooks("Lanternfall Cycle", seeded.bookID)
	name := "Gammaray Codex"
	split := &models.File{
		LibraryID:     f.lib.ID,
		BookID:        seeded.bookID,
		FileType:      models.FileTypeEPUB,
		FileRole:      models.FileRoleMain,
		Filepath:      filepath.Join(f.bookFilepath(seeded.bookID), "extra", "gamma.epub"),
		FilesizeBytes: 1,
		Name:          &name,
	}
	f.insert(split)

	f.request(http.MethodPost, fmt.Sprintf("/api/books/%d/move-files", seeded.bookID), fmt.Sprintf(`{"file_ids":[%d]}`, split.ID), http.StatusOK)

	assert.Equal(t, []int{series.ID}, f.searchSeriesIDs("Gammaray"), "the split Book's title matches the Series it was copied into")
}

// Merging a Series re-indexes the target's own Books too, whose series_names
// lack the source name that is now an alias of the target.
func TestMergeSeries_ReindexesTargetBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	targetBook := f.seedReviewedBook(models.FileTypeEPUB, nil)
	sourceBook := f.seedReviewedBook(models.FileTypeEPUB, &targetBook)
	target := f.seriesWithBooks("Lanternfall Cycle", targetBook.bookID)
	source := f.seriesWithBooks("Moonwake Saga", sourceBook.bookID)

	f.request(http.MethodPost, fmt.Sprintf("/api/series/%d/merge", target.ID), fmt.Sprintf(`{"source_id":%d}`, source.ID), http.StatusNoContent)

	assert.ElementsMatch(t, []int{targetBook.bookID, sourceBook.bookID}, f.searchBookIDs("Moonwake"), "the source name, now an alias of the target, matches both Books")
}

// Merging a Person re-indexes the target's own Books too, whose authors lack
// the source name that is now an alias of the target.
func TestMergePeople_ReindexesTargetBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	targetBook := f.seedReviewedBook(models.FileTypeEPUB, nil)
	sourceBook := f.seedReviewedBook(models.FileTypeEPUB, &targetBook)
	source := f.person("Ottoline Brackenridge")
	_, err := f.db.NewUpdate().Model((*models.Author)(nil)).Set("person_id = ?", source.ID).Where("book_id = ?", sourceBook.bookID).Exec(f.ctx)
	require.NoError(t, err)
	require.NoError(t, f.searchSvc.ReindexBookByID(f.ctx, sourceBook.bookID))

	f.request(http.MethodPost, fmt.Sprintf("/api/people/%d/merge", targetBook.author.ID), fmt.Sprintf(`{"source_id":%d}`, source.ID), http.StatusNoContent)

	assert.ElementsMatch(t, []int{targetBook.bookID, sourceBook.bookID}, f.searchBookIDs("Brackenridge"), "the source name, now an alias of the target, matches both Books")
}

// Renaming a Series without sending aliases still re-indexes its Books.
func TestUpdateSeries_RenameWithoutAliasesReindexesBooks(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
	series := f.seriesWithBooks("Lanternfall Cycle", seeded.bookID)
	require.NoError(t, f.searchSvc.ReindexBookByID(f.ctx, seeded.bookID))
	require.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Lanternfall"), "precondition: the series name matches the Book")

	f.request(http.MethodPatch, fmt.Sprintf("/api/series/%d", series.ID), `{"name":"Novaculite Chronicles"}`, http.StatusOK)

	assert.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Novaculite"), "the new series name matches the Book")
	assert.Empty(t, f.searchBookIDs("Lanternfall"), "the old series name no longer matches the Book")
}

// A rename commits before SyncAliases runs, so when the aliases are rejected
// the request fails but the new name is stored. The FTS rows that carry the
// name must follow it anyway.
func TestUpdateResource_RenameThenRejectedAliasesReindexes(t *testing.T) {
	t.Parallel()

	t.Run("person", func(t *testing.T) {
		t.Parallel()
		f := newResourceDeleteFixture(t)
		seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
		series := f.seriesWithBooks("Lanternfall Cycle", seeded.bookID)
		other := f.person("Ottoline Brackenridge")
		require.NoError(t, f.searchSvc.IndexPerson(f.ctx, seeded.author))

		status := f.requestStatus(http.MethodPatch, fmt.Sprintf("/api/people/%d", seeded.author.ID), fmt.Sprintf(`{"name":"Novaculite Wren","aliases":[%q]}`, other.Name))

		require.GreaterOrEqual(t, status, 400, "the conflicting alias is rejected")
		var stored models.Person
		require.NoError(t, f.db.NewSelect().Model(&stored).Where("id = ?", seeded.author.ID).Scan(f.ctx))
		require.Equal(t, "Novaculite Wren", stored.Name, "precondition: the rename committed")
		assert.Equal(t, []int{seeded.author.ID}, f.searchPersonIDs("Novaculite"), "the new name finds the Person")
		assert.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Novaculite"), "the new name matches the authored Book")
		assert.Equal(t, []int{series.ID}, f.searchSeriesIDs("Novaculite"), "the new name matches the Series of the authored Book")
	})

	t.Run("series", func(t *testing.T) {
		t.Parallel()
		f := newResourceDeleteFixture(t)
		seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)
		series := f.seriesWithBooks("Lanternfall Cycle", seeded.bookID)
		f.seriesWithBooks("Moonwake Saga")

		status := f.requestStatus(http.MethodPatch, fmt.Sprintf("/api/series/%d", series.ID), `{"name":"Novaculite Chronicles","aliases":["Moonwake Saga"]}`)

		require.GreaterOrEqual(t, status, 400, "the conflicting alias is rejected")
		assert.Equal(t, []int{series.ID}, f.searchSeriesIDs("Novaculite"), "the new name finds the Series")
		assert.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Novaculite"), "the new name matches the member Book")
	})

	for _, resource := range []struct{ path, table string }{
		{"genres", "genres_fts"},
		{"tags", "tags_fts"},
		{"publishers", "publishers_fts"},
	} {
		t.Run(resource.path, func(t *testing.T) {
			t.Parallel()
			f := newResourceDeleteFixture(t)
			var renamed, other any
			var renamedID int
			switch resource.path {
			case "genres":
				g, o := &models.Genre{LibraryID: f.lib.ID, Name: "Solarpunk"}, &models.Genre{LibraryID: f.lib.ID, Name: "Cyberpunk"}
				f.insert(g)
				f.insert(o)
				renamed, other, renamedID = g, o, g.ID
				require.NoError(t, f.searchSvc.IndexGenre(f.ctx, g))
			case "tags":
				g, o := &models.Tag{LibraryID: f.lib.ID, Name: "Solarpunk"}, &models.Tag{LibraryID: f.lib.ID, Name: "Cyberpunk"}
				f.insert(g)
				f.insert(o)
				renamed, other, renamedID = g, o, g.ID
				require.NoError(t, f.searchSvc.IndexTag(f.ctx, g))
			case "publishers":
				g, o := &models.Publisher{LibraryID: f.lib.ID, Name: "Solarpunk"}, &models.Publisher{LibraryID: f.lib.ID, Name: "Cyberpunk"}
				f.insert(g)
				f.insert(o)
				renamed, other, renamedID = g, o, g.ID
				require.NoError(t, f.searchSvc.IndexPublisher(f.ctx, g))
			}
			require.NotNil(t, renamed)
			require.NotNil(t, other)

			status := f.requestStatus(http.MethodPatch, fmt.Sprintf("/api/%s/%d", resource.path, renamedID), `{"name":"Novaculite","aliases":["Cyberpunk"]}`)

			require.GreaterOrEqual(t, status, 400, "the conflicting alias is rejected")
			assert.Equal(t, []int{renamedID}, f.ftsMatch(resource.table, "Novaculite"), "the new name finds the resource")
			assert.Empty(t, f.ftsMatch(resource.table, "Solarpunk"), "the old name no longer finds the resource")
		})
	}
}

// A File edit commits its Narrators before later validation, here an
// invalid language tag, fails the request. The Book's books_fts row and the
// new Narrator's persons_fts row must follow the committed Narrators anyway.
func TestUpdateFile_RejectedAfterNarratorsCommitReindexes(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeM4B, nil)
	require.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Brackenridge"), "precondition: the narrator matches the Book")

	status := f.requestStatus(http.MethodPost, fmt.Sprintf("/api/books/files/%d", seeded.fileID), `{"narrators":["Cordelia Fairweather"],"language":"!!not a language!!"}`)

	require.Equal(t, http.StatusBadRequest, status, "the invalid language is rejected")
	var narratorNames []string
	require.NoError(t, f.db.NewSelect().Model((*models.Narrator)(nil)).ColumnExpr("p.name").
		Join("JOIN persons AS p ON p.id = n.person_id").Where("n.file_id = ?", seeded.fileID).Scan(f.ctx, &narratorNames))
	require.Equal(t, []string{"Cordelia Fairweather"}, narratorNames, "precondition: the narrators committed before the failure")
	assert.Equal(t, []int{seeded.bookID}, f.searchBookIDs("Fairweather"), "the new narrator matches the Book")
	assert.Empty(t, f.searchBookIDs("Brackenridge"), "the replaced narrator no longer matches the Book")
	assert.Len(t, f.searchPersonIDs("Fairweather"), 1, "the new narrator is in people search")
}
