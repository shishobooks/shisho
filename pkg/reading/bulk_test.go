package reading

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (a *readingAPI) bulk(action string, payload any) BulkReadingResponse {
	a.t.Helper()
	rec := a.do(http.MethodPost, "/books/bulk/"+action, payload)
	require.Equal(a.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp BulkReadingResponse
	require.NoError(a.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func TestBulkFinish_MarksEachBookReadAndSkipsBooksAlreadyRead(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	reading := createBook(t, db, "Reading")
	paused := createBook(t, db, "Paused")
	untracked := createBook(t, db, "Untracked")
	queued := createBook(t, db, "Queued")
	read := createBook(t, db, "Read")
	dnf := createBook(t, db, "DNF")
	api.mustTransition(reading.ID, "start", "2026-03-01")
	api.mustTransition(paused.ID, "start", "2026-03-01")
	api.mustTransition(paused.ID, "pause", "")
	api.ok(http.MethodPost, fmt.Sprintf("/books/%d/to-read", queued.ID), nil)
	api.mustTransition(read.ID, "finish", "2026-02-01")
	api.mustTransition(dnf.ID, "start", "2026-02-01")
	api.mustTransition(dnf.ID, "abandon", "2026-02-02")

	resp := api.bulk("finish", BulkFinishPayload{
		BookIDs: []int{reading.ID, paused.ID, untracked.ID, queued.ID, read.ID, dnf.ID},
		Today:   "2026-03-05",
	})
	assert.Equal(t, BulkReadingResponse{Changed: 5, Skipped: 1}, resp)

	for _, book := range []*models.Book{reading, paused} {
		got := api.get(book.ID)
		assert.Equal(t, models.ReadingStatusRead, status(got), book.Title)
		require.Len(t, got.ReadThroughs, 1, book.Title)
		assert.Equal(t, strp("2026-03-01"), got.ReadThroughs[0].BeginDay, book.Title)
		assert.Equal(t, strp("2026-03-05"), got.ReadThroughs[0].EndDay, book.Title)
	}
	for _, book := range []*models.Book{untracked, queued} {
		got := api.get(book.ID)
		assert.Equal(t, models.ReadingStatusRead, status(got), book.Title)
		require.Len(t, got.ReadThroughs, 1, book.Title)
		assert.Nil(t, got.ReadThroughs[0].BeginDay, book.Title)
		assert.Equal(t, strp("2026-03-05"), got.ReadThroughs[0].EndDay, book.Title)
		assert.False(t, got.OnToRead, book.Title)
	}
	got := api.get(read.ID)
	require.Len(t, got.ReadThroughs, 1, "a Book already read gets no second finished read")
	assert.Equal(t, strp("2026-02-01"), got.ReadThroughs[0].EndDay)
	got = api.get(dnf.ID)
	assert.Equal(t, models.ReadingStatusRead, status(got))
	assert.Len(t, got.ReadThroughs, 2)
}

func TestBulkFinish_CountsARepeatedBookOnce(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")

	resp := api.bulk("finish", BulkFinishPayload{BookIDs: []int{book.ID, book.ID}, Today: "2026-03-05"})
	assert.Equal(t, BulkReadingResponse{Changed: 1, Skipped: 0}, resp)
	assert.Len(t, api.get(book.ID).ReadThroughs, 1)
}

func TestBulkFinish_DatesTheReadUnderwayByTheDayItWasLastRead(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4)", 0.4, false)
	f.clock.advance(5 * day)

	result, err := f.svc.BulkFinish(f.ctx, f.user.ID, []int{f.book.ID}, "2026-10-20")
	require.NoError(t, err)
	assert.Equal(t, BulkReadingResponse{Changed: 1}, result)

	rt := f.readThroughs()[0]
	assert.Equal(t, models.ReadThroughOutcomeFinished, rt.Outcome)
	assert.Equal(t, strp("2026-10-15"), rt.EndDay)
}

func TestBulkNotStarted_DeletesTheReadsUnderwayAndSkipsTheRest(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	reading := createBook(t, db, "Reading")
	paused := createBook(t, db, "Paused")
	fromToRead := createBook(t, db, "From To Read")
	read := createBook(t, db, "Read")
	untracked := createBook(t, db, "Untracked")
	api.mustTransition(read.ID, "finish", "2026-02-01")
	api.mustTransition(reading.ID, "start", "2026-03-01")
	api.mustTransition(paused.ID, "start", "2026-03-01")
	api.mustTransition(paused.ID, "pause", "")
	api.ok(http.MethodPost, fmt.Sprintf("/books/%d/to-read", fromToRead.ID), nil)
	api.mustTransition(fromToRead.ID, "start", "2026-03-02")

	resp := api.bulk("not-started", BulkNotStartedPayload{
		BookIDs: []int{reading.ID, paused.ID, fromToRead.ID, read.ID, untracked.ID},
	})
	assert.Equal(t, BulkReadingResponse{Changed: 3, Skipped: 2}, resp)

	for _, book := range []*models.Book{reading, paused} {
		got := api.get(book.ID)
		assert.Nil(t, got.Status, book.Title)
		assert.Empty(t, got.ReadThroughs, book.Title)
	}
	got := api.get(fromToRead.ID)
	assert.Equal(t, models.ReadingStatusToRead, status(got), "starting took it off To Read, so it goes back")
	assert.Empty(t, got.ReadThroughs)
	got = api.get(read.ID)
	assert.Equal(t, models.ReadingStatusRead, status(got), "an ended read stays")
	assert.Len(t, got.ReadThroughs, 1)
}

func TestBulk_ActsOnlyOnTheUsersOwnReading(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	other := newReadingAPI(t, db, createUser(t, db, "other", 2))
	book := createBook(t, db, "Dune")
	other.mustTransition(book.ID, "start", "2026-03-01")

	resp := api.bulk("not-started", BulkNotStartedPayload{BookIDs: []int{book.ID}})
	assert.Equal(t, BulkReadingResponse{Changed: 0, Skipped: 1}, resp)
	api.bulk("finish", BulkFinishPayload{BookIDs: []int{book.ID}, Today: "2026-03-05"})

	assert.Equal(t, models.ReadingStatusCurrentlyReading, status(other.get(book.ID)))
	assert.Len(t, other.get(book.ID).ReadThroughs, 1)
}

// Every Book is checked as the single-Book routes check it, and nothing
// changes unless every one passes.
func TestBulk_ChecksEveryBook(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	reachable := createBook(t, db, "Reachable")
	unreachable := createBook(t, db, "Unreachable")
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID, LibraryID: intp(reachable.LibraryID)}}
	api := newReadingAPI(t, db, user)
	api.mustTransition(reachable.ID, "start", "2026-03-01")

	payloads := map[string]func(bookIDs []int) any{
		"finish":      func(bookIDs []int) any { return BulkFinishPayload{BookIDs: bookIDs, Today: "2026-03-05"} },
		"not-started": func(bookIDs []int) any { return BulkNotStartedPayload{BookIDs: bookIDs} },
	}
	for action, payload := range payloads {
		rec := api.do(http.MethodPost, "/books/bulk/"+action, payload([]int{reachable.ID, unreachable.ID}))
		assert.Equal(t, http.StatusForbidden, rec.Code, action)
		rec = api.do(http.MethodPost, "/books/bulk/"+action, payload([]int{reachable.ID, 999}))
		assert.Equal(t, http.StatusNotFound, rec.Code, action)
	}
	assert.Equal(t, models.ReadingStatusCurrentlyReading, status(api.get(reachable.ID)))
}

func TestBulk_RejectsBadPayloads(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")

	assert.Equal(t, http.StatusUnprocessableEntity, api.do(http.MethodPost, "/books/bulk/finish", BulkFinishPayload{BookIDs: []int{}, Today: "2026-03-05"}).Code)
	assert.Equal(t, http.StatusUnprocessableEntity, api.do(http.MethodPost, "/books/bulk/finish", BulkFinishPayload{BookIDs: []int{book.ID}}).Code)
	assert.Equal(t, http.StatusUnprocessableEntity, api.do(http.MethodPost, "/books/bulk/finish", BulkFinishPayload{BookIDs: []int{book.ID}, Today: "2999-01-01"}).Code)
	assert.Equal(t, http.StatusUnprocessableEntity, api.do(http.MethodPost, "/books/bulk/not-started", BulkNotStartedPayload{}).Code)
}

// The checks' 404 is only for a missing Book: a failed lookup is a 500, and
// so is a failed write.
func TestBulk_DatabaseFailureIs500(t *testing.T) {
	t.Parallel()
	for _, table := range []string{"books", "read_throughs"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
			book := createBook(t, db, "Dune")
			_, err := db.Exec("DROP TABLE " + table)
			require.NoError(t, err)

			assert.Equal(t, http.StatusInternalServerError, api.do(http.MethodPost, "/books/bulk/finish", BulkFinishPayload{BookIDs: []int{book.ID}, Today: "2026-03-05"}).Code)
			assert.Equal(t, http.StatusInternalServerError, api.do(http.MethodPost, "/books/bulk/not-started", BulkNotStartedPayload{BookIDs: []int{book.ID}}).Code)
		})
	}
}
