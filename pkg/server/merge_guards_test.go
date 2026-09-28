package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requestBody sends an authenticated admin request and returns the status
// code and body, for requests that are expected to fail.
func (f *resourceDeleteFixture) requestBody(method, path, body string) (int, string) {
	f.t.Helper()
	token, err := f.authSvc.GenerateToken(f.admin)
	require.NoError(f.t, err)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func (f *resourceDeleteFixture) bookExists(bookID int) bool {
	f.t.Helper()
	count, err := f.db.NewSelect().Model((*models.Book)(nil)).Where("id = ?", bookID).Count(f.ctx)
	require.NoError(f.t, err)
	return count == 1
}

// A source Book listed twice is rejected with a message that names the
// problem, instead of the "some files not found" error the file move gives.
func TestMergeBooks_DuplicateSourceIDs_Rejected(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	target := f.seedReviewedBook(models.FileTypeEPUB, nil)
	source := f.seedReviewedBook(models.FileTypeEPUB, &target)

	status, body := f.requestBody(http.MethodPost, "/api/books/merge",
		fmt.Sprintf(`{"target_book_id":%d,"source_book_ids":[%d,%d]}`, target.bookID, source.bookID, source.bookID))

	assert.Equal(t, http.StatusUnprocessableEntity, status)
	assert.Contains(t, strings.ToLower(body), "more than once")
	assert.True(t, f.bookExists(source.bookID), "the source Book still exists")
}

// The Series of a merged-away source Book stop listing its title.
func TestMergeBooks_ReindexesSourceSeries(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	source := f.seedReviewedBook(models.FileTypeEPUB, nil)
	target := f.seedReviewedBook(models.FileTypeEPUB, &source)
	series := f.seriesWithBooks("Lanternfall Cycle", source.bookID, target.bookID)
	require.Equal(t, []int{series.ID}, f.searchSeriesIDs("Harbor"), "precondition: the source title matches the Series")

	f.request(http.MethodPost, "/api/books/merge", fmt.Sprintf(`{"target_book_id":%d,"source_book_ids":[%d]}`, target.bookID, source.bookID), http.StatusOK)

	assert.False(t, f.bookExists(source.bookID), "the source Book is deleted")
	assert.Empty(t, f.searchSeriesIDs("Harbor"), "the merged-away title no longer matches the Series")
	assert.Equal(t, []int{series.ID}, f.searchSeriesIDs("Quiet"), "the target title still matches the Series")
}

// A Person who authored only the merged-away Book is removed with it, as a
// Book delete does.
func TestMergeBooks_CleansUpOrphanedEntities(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	target := f.seedReviewedBook(models.FileTypeEPUB, nil)
	source := f.seedReviewedBook(models.FileTypeEPUB, &target)
	orphan := f.person("Percival Lonesome")
	f.insert(&models.Author{BookID: source.bookID, PersonID: orphan.ID, SortOrder: 2})
	require.NoError(t, f.searchSvc.IndexPerson(f.ctx, orphan))
	require.NoError(t, f.searchSvc.IndexPerson(f.ctx, target.author))
	require.Equal(t, []int{orphan.ID}, f.searchPersonIDs("Lonesome"), "precondition: the Person is searchable")

	f.request(http.MethodPost, "/api/books/merge", fmt.Sprintf(`{"target_book_id":%d,"source_book_ids":[%d]}`, target.bookID, source.bookID), http.StatusOK)

	count, err := f.db.NewSelect().Model((*models.Person)(nil)).Where("id = ?", orphan.ID).Count(f.ctx)
	require.NoError(t, err)
	assert.Zero(t, count, "the orphaned Person is deleted")
	assert.Empty(t, f.searchPersonIDs("Lonesome"), "the orphaned Person leaves the search index")
	assert.Equal(t, []int{target.author.ID}, f.searchPersonIDs("Quillfeather"), "the shared author stays")
}

// The same author twice without a role is stored once: the unique index
// treats NULL roles as distinct, so it would not stop the second row.
func TestUpdateBook_DuplicateNullRoleAuthors_Deduped(t *testing.T) {
	t.Parallel()
	f := newResourceDeleteFixture(t)
	seeded := f.seedReviewedBook(models.FileTypeEPUB, nil)

	f.request(http.MethodPost, fmt.Sprintf("/api/books/%d", seeded.bookID),
		`{"authors":[{"name":"Barnaby Thistlewood"},{"name":"barnaby thistlewood"},{"name":"Barnaby Thistlewood","role":"editor"}]}`, http.StatusOK)

	var authors []*models.Author
	require.NoError(t, f.db.NewSelect().Model(&authors).Where("book_id = ?", seeded.bookID).Order("sort_order").Scan(f.ctx))
	require.Len(t, authors, 2, "one generic author row and one editor row")
	assert.Nil(t, authors[0].Role)
	require.NotNil(t, authors[1].Role)
	assert.Equal(t, "editor", *authors[1].Role)
	assert.Equal(t, authors[0].PersonID, authors[1].PersonID)
	assert.Equal(t, []int{1, 2}, []int{authors[0].SortOrder, authors[1].SortOrder}, "a skipped duplicate leaves no gap in the order")
}

// Genre, Tag, and Publisher merges reindex after commit: the source name
// finds the target, and the source's own row is gone.
func TestMergeGenreTagPublisher_ReindexesTargetAndDropsSource(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"genres", "tags", "publishers"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newResourceDeleteFixture(t)
			var targetID, sourceID int
			switch kind {
			case "genres":
				target, source := &models.Genre{LibraryID: f.lib.ID, Name: "Hopepunk"}, &models.Genre{LibraryID: f.lib.ID, Name: "Cozypunk"}
				f.insert(target)
				f.insert(source)
				require.NoError(t, f.searchSvc.IndexGenre(f.ctx, target))
				require.NoError(t, f.searchSvc.IndexGenre(f.ctx, source))
				targetID, sourceID = target.ID, source.ID
			case "tags":
				target, source := &models.Tag{LibraryID: f.lib.ID, Name: "Hopepunk"}, &models.Tag{LibraryID: f.lib.ID, Name: "Cozypunk"}
				f.insert(target)
				f.insert(source)
				require.NoError(t, f.searchSvc.IndexTag(f.ctx, target))
				require.NoError(t, f.searchSvc.IndexTag(f.ctx, source))
				targetID, sourceID = target.ID, source.ID
			case "publishers":
				target, source := &models.Publisher{LibraryID: f.lib.ID, Name: "Hopepunk"}, &models.Publisher{LibraryID: f.lib.ID, Name: "Cozypunk"}
				f.insert(target)
				f.insert(source)
				require.NoError(t, f.searchSvc.IndexPublisher(f.ctx, target))
				require.NoError(t, f.searchSvc.IndexPublisher(f.ctx, source))
				targetID, sourceID = target.ID, source.ID
			}
			table := strings.TrimSuffix(kind, "s") + "s_fts"
			require.Equal(t, []int{sourceID}, f.ftsMatch(table, "Cozypunk"), "precondition")

			f.request(http.MethodPost, fmt.Sprintf("/api/%s/%d/merge", kind, targetID), fmt.Sprintf(`{"source_id":%d}`, sourceID), http.StatusNoContent)

			assert.Equal(t, []int{targetID}, f.ftsMatch(table, "Cozypunk"), "the source name finds only the target")
			assert.Equal(t, []int{targetID}, f.ftsMatch(table, "Hopepunk"))
		})
	}
}
