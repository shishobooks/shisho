package reading

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func (a *readingAPI) addToRead(bookID int) BookReadingResponse {
	a.t.Helper()
	return a.ok(http.MethodPost, fmt.Sprintf("/books/%d/to-read", bookID), nil)
}

func (a *readingAPI) removeFromToRead(bookID int) BookReadingResponse {
	a.t.Helper()
	return a.ok(http.MethodDelete, fmt.Sprintf("/books/%d/to-read", bookID), nil)
}

func (a *readingAPI) reorderToRead(bookIDs ...int) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(http.MethodPut, "/to-read/order", ReorderToReadPayload{BookIDs: bookIDs})
}

// queue returns userID's To Read, in order.
func queue(t *testing.T, db *bun.DB, userID int) []int {
	t.Helper()
	ids := []int{}
	err := db.NewSelect().
		Model((*models.UserBook)(nil)).
		Column("book_id").
		Where("user_id = ? AND to_read_position IS NOT NULL", userID).
		Order("to_read_position ASC", "book_id ASC").
		Scan(context.Background(), &ids)
	require.NoError(t, err)
	return ids
}

func TestToRead_AddingAppendsAtTheEnd(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	a, b, c := createBook(t, db, "A"), createBook(t, db, "B"), createBook(t, db, "C")

	resp := api.addToRead(b.ID)
	assert.True(t, resp.OnToRead)
	assert.Equal(t, models.ReadingStatusToRead, status(resp))
	api.addToRead(a.ID)
	api.addToRead(c.ID)
	assert.Equal(t, []int{b.ID, a.ID, c.ID}, queue(t, db, user.ID))

	// Adding a Book already queued keeps its place.
	api.addToRead(b.ID)
	assert.Equal(t, []int{b.ID, a.ID, c.ID}, queue(t, db, user.ID))
}

func TestToRead_AddingKeepsTheRating(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")

	api.ok(http.MethodPut, fmt.Sprintf("/books/%d/rating", book.ID), SetRatingPayload{Rating: floatp(4.5)})
	resp := api.addToRead(book.ID)
	require.NotNil(t, resp.Rating)
	assert.InDelta(t, 4.5, *resp.Rating, 1e-9)
	resp = api.removeFromToRead(book.ID)
	require.NotNil(t, resp.Rating, "leaving To Read keeps the Rating")
}

func TestToRead_Removing(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	a, b := createBook(t, db, "A"), createBook(t, db, "B")
	api.addToRead(a.ID)
	api.addToRead(b.ID)

	resp := api.removeFromToRead(a.ID)
	assert.False(t, resp.OnToRead)
	assert.Nil(t, resp.Status, "a Book with no Read-throughs has no status once off To Read")
	assert.Equal(t, []int{b.ID}, queue(t, db, user.ID))

	// Removing a Book that isn't queued changes nothing.
	resp = api.removeFromToRead(a.ID)
	assert.False(t, resp.OnToRead)
	assert.Equal(t, []int{b.ID}, queue(t, db, user.ID))

	// A Book added back goes to the end.
	api.addToRead(a.ID)
	assert.Equal(t, []int{b.ID, a.ID}, queue(t, db, user.ID))
}

func TestToRead_Reordering(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	a, b, c := createBook(t, db, "A"), createBook(t, db, "B"), createBook(t, db, "C")
	for _, book := range []*models.Book{a, b, c} {
		api.addToRead(book.ID)
	}

	rec := api.reorderToRead(c.ID, a.ID, b.ID)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, []int{c.ID, a.ID, b.ID}, queue(t, db, user.ID))

	// A Book added after a reorder still goes to the end.
	d := createBook(t, db, "D")
	api.addToRead(d.ID)
	assert.Equal(t, []int{c.ID, a.ID, b.ID, d.ID}, queue(t, db, user.ID))
}

// The To Read page shows only Books in libraries the user can reach, so a
// reorder names only those. The others keep their places, and ids not on
// To Read are ignored.
func TestToRead_ReorderingPartOfTheQueueKeepsTheRestInPlace(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	a, hidden, b, c := createBook(t, db, "A"), createBook(t, db, "Hidden"), createBook(t, db, "B"), createBook(t, db, "C")
	for _, book := range []*models.Book{a, hidden, b, c} {
		api.addToRead(book.ID)
	}
	notQueued := createBook(t, db, "Not queued")

	rec := api.reorderToRead(c.ID, notQueued.ID, b.ID, a.ID)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, []int{c.ID, hidden.ID, b.ID, a.ID}, queue(t, db, user.ID))
	assert.False(t, api.get(notQueued.ID).OnToRead)
}

func TestToRead_ReorderingNeedsBookIDs(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))

	assert.Equal(t, http.StatusUnprocessableEntity, api.reorderToRead().Code)
}

func TestToRead_EachUserHasTheirOwn(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	reader := createUser(t, db, "reader", 2)
	other := createUser(t, db, "other", 2)
	readerAPI := newReadingAPI(t, db, reader)
	otherAPI := newReadingAPI(t, db, other)
	a, b := createBook(t, db, "A"), createBook(t, db, "B")
	readerAPI.addToRead(a.ID)
	readerAPI.addToRead(b.ID)
	otherAPI.addToRead(b.ID)

	assert.False(t, otherAPI.get(a.ID).OnToRead)
	assert.Equal(t, []int{b.ID}, queue(t, db, other.ID), "another user's queue doesn't set the position")

	// Reordering only touches the caller's queue.
	otherAPI.reorderToRead(b.ID, a.ID)
	assert.Equal(t, []int{a.ID, b.ID}, queue(t, db, reader.ID))
	otherAPI.removeFromToRead(b.ID)
	assert.True(t, readerAPI.get(b.ID).OnToRead)
}

func TestToRead_StartingTakesTheBookOff(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	a, b := createBook(t, db, "A"), createBook(t, db, "B")
	api.addToRead(a.ID)
	api.addToRead(b.ID)

	resp := api.mustTransition(a.ID, "start", "2026-03-01")
	assert.False(t, resp.OnToRead)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, status(resp))
	assert.Equal(t, []int{b.ID}, queue(t, db, user.ID))
}

// Marking read also takes the Book off To Read, so "Mark as read" on a
// queued Book shows Read rather than staying To Read.
func TestToRead_MarkingReadTakesTheBookOff(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	book := createBook(t, db, "Dune")

	api.addToRead(book.ID)
	resp := api.mustTransition(book.ID, "finish", "2026-03-01")
	assert.False(t, resp.OnToRead)
	assert.Equal(t, models.ReadingStatusRead, status(resp))
	assert.Empty(t, queue(t, db, user.ID))
}

// A finished Book can be queued again to plan a reread: it shows To Read,
// and its history stays.
func TestToRead_AddingAReadBookPlansAReread(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	read, dnf := createBook(t, db, "Read"), createBook(t, db, "DNF")
	api.mustTransition(read.ID, "finish", "2026-03-01")
	api.mustTransition(dnf.ID, "start", "2026-03-01")
	api.mustTransition(dnf.ID, "abandon", "2026-03-02")

	resp := api.addToRead(read.ID)
	assert.Equal(t, models.ReadingStatusToRead, status(resp), "To Read outranks Read")
	require.Len(t, resp.ReadThroughs, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, resp.ReadThroughs[0].Outcome)

	resp = api.addToRead(dnf.ID)
	assert.Equal(t, models.ReadingStatusToRead, status(resp), "To Read outranks Did Not Finish")
	require.Len(t, resp.ReadThroughs, 1)

	// Off To Read, the history decides again.
	assert.Equal(t, models.ReadingStatusRead, status(api.removeFromToRead(read.ID)))
}

// A Book with a Read-through underway can't be queued: starting takes a
// Book off To Read, and its status would stay Currently Reading or Paused.
func TestToRead_AddingABookBeingReadIsRefused(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	reading, paused := createBook(t, db, "Reading"), createBook(t, db, "Paused")
	api.mustTransition(reading.ID, "start", "2026-03-01")
	api.mustTransition(paused.ID, "start", "2026-03-01")
	api.mustTransition(paused.ID, "pause", "")

	for _, book := range []*models.Book{reading, paused} {
		rec := api.do(http.MethodPost, fmt.Sprintf("/books/%d/to-read", book.ID), nil)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "invalid_state")
	}
	assert.Empty(t, queue(t, db, user.ID))
}

func TestToRead_BookAccess(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	book := createBook(t, db, "Dune")
	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID, LibraryID: intp(book.LibraryID + 100)}}
	api := newReadingAPI(t, db, user)

	assert.Equal(t, http.StatusForbidden, api.do(http.MethodPost, fmt.Sprintf("/books/%d/to-read", book.ID), nil).Code)
	assert.Equal(t, http.StatusForbidden, api.do(http.MethodDelete, fmt.Sprintf("/books/%d/to-read", book.ID), nil).Code)
	assert.Equal(t, http.StatusNotFound, api.do(http.MethodPost, "/books/999/to-read", nil).Code)
}

// The add's 422 is only for a Book being read: a failed lookup is a 500.
func TestToRead_DatabaseFailureIs500(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	_, err := db.Exec("DROP TABLE read_throughs")
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, api.do(http.MethodPost, fmt.Sprintf("/books/%d/to-read", book.ID), nil).Code)
}

func TestAppendToRead_KeepsOrderAndSkipsQueuedAndUnderway(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	queued, a, reading, b := createBook(t, db, "Queued"), createBook(t, db, "A"), createBook(t, db, "Reading"), createBook(t, db, "B")
	api.addToRead(queued.ID)
	api.mustTransition(reading.ID, "start", "2026-03-01")

	added, err := NewService(db).AppendToRead(context.Background(), user.ID, []int{b.ID, queued.ID, reading.ID, a.ID, b.ID})
	require.NoError(t, err)
	assert.Equal(t, 2, added)
	assert.Equal(t, []int{queued.ID, b.ID, a.ID}, queue(t, db, user.ID))
}

func floatp(f float64) *float64 { return &f }

// The reading record gives a queued Book's place on To Read as the To Read
// page shows it: 1-based, counting only Books in libraries the user can
// reach.
func TestToRead_PlaceOnTheQueue(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	a, hidden, b, c := createBook(t, db, "A"), createBook(t, db, "Hidden"), createBook(t, db, "B"), createBook(t, db, "C")
	for _, book := range []*models.Book{a, hidden, b, c} {
		api.addToRead(book.ID)
	}
	assert.Nil(t, api.get(createBook(t, db, "Not queued").ID).ToReadPlace, "a Book off To Read has no place")
	assert.Equal(t, intp(3), api.get(b.ID).ToReadPlace)

	api.removeFromToRead(a.ID)
	assert.Equal(t, intp(2), api.get(b.ID).ToReadPlace, "places close up after a removal")

	// Hidden's library is out of reach, so it doesn't count.
	user.LibraryAccess = []*models.UserLibraryAccess{
		{UserID: user.ID, LibraryID: &b.LibraryID},
		{UserID: user.ID, LibraryID: &c.LibraryID},
	}
	limited := newReadingAPI(t, db, user)
	assert.Equal(t, intp(1), limited.get(b.ID).ToReadPlace)
	assert.Equal(t, intp(2), limited.get(c.ID).ToReadPlace)

	// A reorder moves it.
	assert.Equal(t, intp(1), limited.reorderAndGet(c.ID, b.ID))
}

// reorderAndGet reorders To Read and returns the first id's new place.
func (a *readingAPI) reorderAndGet(bookIDs ...int) *int {
	a.t.Helper()
	rec := a.reorderToRead(bookIDs...)
	require.Equal(a.t, http.StatusNoContent, rec.Code, rec.Body.String())
	return a.get(bookIDs[0]).ToReadPlace
}
