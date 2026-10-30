package lists

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// listBookIDs lists listID's Books as user through the handler and returns
// the ids in order and the total.
func listBookIDs(t *testing.T, h *handler, user *models.User, listID int, query string) ([]int, int) {
	t.Helper()
	e := newTestEcho(t)
	req := httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)
	c.SetParamNames("id")
	c.SetParamValues(strconv.Itoa(listID))
	require.NoError(t, h.listBooks(c))
	var resp ListListBooksResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ids := []int{}
	for _, lb := range resp.Items {
		ids = append(ids, lb.BookID)
	}
	return ids, resp.Total
}

// readingListFixture is a List shared from owner to viewer, spanning
// libraries A and B. The viewer can reach only library A.
type readingListFixture struct {
	db     *bun.DB
	h      *handler
	owner  *models.User
	viewer *models.User
	list   *models.List
	libA   *models.Library
	libB   *models.Library
}

func newReadingListFixture(t *testing.T) *readingListFixture {
	t.Helper()
	db := testdb.New(t)
	f := &readingListFixture{db: db, h: newTestHandler(db)}
	f.owner = createTestUser(t, db, "owner")
	f.owner.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.owner.ID}}
	f.libA = createTestLibrary(t, db, "A")
	f.libB = createTestLibrary(t, db, "B")
	f.viewer = createTestUser(t, db, "viewer")
	f.viewer.LibraryAccess = []*models.UserLibraryAccess{{UserID: f.viewer.ID, LibraryID: &f.libA.ID}}

	var err error
	f.list, err = f.h.listsService.CreateList(t.Context(), CreateListOptions{UserID: f.owner.ID, Name: "Span", DefaultSort: models.ListSortAddedAtAsc})
	require.NoError(t, err)
	_, err = f.h.listsService.CreateShare(t.Context(), CreateShareOptions{ListID: f.list.ID, UserID: f.viewer.ID, Permission: models.ListPermissionViewer, SharedByUserID: f.owner.ID})
	require.NoError(t, err)
	return f
}

// add creates a Book in lib and adds it to the List, one call per Book so
// added_at_asc keeps the order they were added in.
func (f *readingListFixture) add(t *testing.T, lib *models.Library, title string) *models.Book {
	t.Helper()
	book := createTestBook(t, f.db, lib.ID, title)
	require.NoError(t, f.h.listsService.AddBooks(t.Context(), AddBooksOptions{ListID: f.list.ID, BookIDs: []int{book.ID}, AddedByUserID: f.owner.ID}))
	return book
}

func rate(t *testing.T, db *bun.DB, userID, bookID, units int) {
	t.Helper()
	require.NoError(t, reading.NewService(db).SetRating(context.Background(), userID, bookID, &units))
}

func finish(t *testing.T, db *bun.DB, userID, bookID int, day string) {
	t.Helper()
	require.NoError(t, reading.NewService(db).End(context.Background(), userID, bookID, models.ReadThroughOutcomeFinished, day))
}

// A List filters on the caller's Reading Status, including no status,
// across the libraries it spans.
func TestListBooks_FiltersByReadingStatus(t *testing.T) {
	t.Parallel()
	f := newReadingListFixture(t)
	readA := f.add(t, f.libA, "Read A")
	untouchedA := f.add(t, f.libA, "Untouched A")
	readB := f.add(t, f.libB, "Read B")
	queuedB := f.add(t, f.libB, "Queued B")
	finish(t, f.db, f.owner.ID, readA.ID, "2026-01-01")
	finish(t, f.db, f.owner.ID, readB.ID, "2026-01-01")
	require.NoError(t, reading.NewService(f.db).AddToRead(t.Context(), f.owner.ID, queuedB.ID))

	ids, total := listBookIDs(t, f.h, f.owner, f.list.ID, "reading_status=read")
	assert.Equal(t, []int{readA.ID, readB.ID}, ids)
	assert.Equal(t, 2, total)

	ids, total = listBookIDs(t, f.h, f.owner, f.list.ID, "reading_status=none")
	assert.Equal(t, []int{untouchedA.ID}, ids)
	assert.Equal(t, 1, total)

	ids, _ = listBookIDs(t, f.h, f.owner, f.list.ID, "reading_status=to_read&reading_status=none")
	assert.Equal(t, []int{untouchedA.ID, queuedB.ID}, ids)

	// The viewer has no status on any of them, and can't reach library B.
	ids, total = listBookIDs(t, f.h, f.viewer, f.list.ID, "reading_status=none")
	assert.Equal(t, []int{readA.ID, untouchedA.ID}, ids)
	assert.Equal(t, 2, total)
}

// A List filters on the caller's rating range and sorts on their Rating
// and finish date, paging after filtering, and never shows a Book in a
// library the caller can't reach.
func TestListBooks_RatingRangeSortsAndPaging(t *testing.T) {
	t.Parallel()
	f := newReadingListFixture(t)
	a1 := f.add(t, f.libA, "A1")
	b1 := f.add(t, f.libB, "B1")
	a2 := f.add(t, f.libA, "A2")
	b2 := f.add(t, f.libB, "B2")
	a3 := f.add(t, f.libA, "A3")
	unrated := f.add(t, f.libA, "Unrated")

	for userID, ratings := range map[int]map[int]int{
		f.owner.ID:  {a1.ID: 12, b1.ID: 20, a2.ID: 16, b2.ID: 17, a3.ID: 8},
		f.viewer.ID: {a1.ID: 20, b1.ID: 20, a2.ID: 4, b2.ID: 20, a3.ID: 16},
	} {
		for bookID, units := range ratings {
			rate(t, f.db, userID, bookID, units)
		}
	}
	finish(t, f.db, f.owner.ID, a1.ID, "2026-03-01")
	finish(t, f.db, f.owner.ID, b1.ID, "2026-01-01")
	finish(t, f.db, f.owner.ID, a3.ID, "2026-02-01")

	ids, _ := listBookIDs(t, f.h, f.owner, f.list.ID, "sort=rating_desc")
	assert.Equal(t, []int{b1.ID, b2.ID, a2.ID, a1.ID, a3.ID, unrated.ID}, ids)
	ids, _ = listBookIDs(t, f.h, f.owner, f.list.ID, "sort=rating_asc")
	assert.Equal(t, []int{a3.ID, a1.ID, a2.ID, b2.ID, b1.ID, unrated.ID}, ids)
	ids, _ = listBookIDs(t, f.h, f.owner, f.list.ID, "sort=date_finished_desc")
	assert.Equal(t, []int{a1.ID, a3.ID, b1.ID, a2.ID, b2.ID, unrated.ID}, ids)
	ids, _ = listBookIDs(t, f.h, f.owner, f.list.ID, "sort=date_finished_asc")
	assert.Equal(t, []int{b1.ID, a3.ID, a1.ID, a2.ID, b2.ID, unrated.ID}, ids)

	// Four and up, highest first, two to a page.
	ids, total := listBookIDs(t, f.h, f.owner, f.list.ID, "rating_min=4&sort=rating_desc&limit=2")
	assert.Equal(t, []int{b1.ID, b2.ID}, ids)
	assert.Equal(t, 3, total)
	ids, total = listBookIDs(t, f.h, f.owner, f.list.ID, "rating_min=4&sort=rating_desc&limit=2&offset=2")
	assert.Equal(t, []int{a2.ID}, ids)
	assert.Equal(t, 3, total)

	// The viewer's own ratings, in library A only.
	ids, total = listBookIDs(t, f.h, f.viewer, f.list.ID, "rating_min=4&rating_max=5&sort=rating_desc")
	assert.Equal(t, []int{a1.ID, a3.ID}, ids)
	assert.Equal(t, 2, total)
}

func TestListBooks_RejectsBadReadingFilters(t *testing.T) {
	t.Parallel()
	f := newReadingListFixture(t)
	for _, query := range []string{"reading_status=finished", "rating_min=0", "rating_min=4&rating_max=2"} {
		e := newTestEcho(t)
		req := httptest.NewRequest(http.MethodGet, "/?"+query, nil)
		c := e.NewContext(req, httptest.NewRecorder())
		auth.SetUser(c, f.owner)
		c.SetParamNames("id")
		c.SetParamValues(strconv.Itoa(f.list.ID))
		err := f.h.listBooks(c)
		var codeErr *errcodes.Error
		require.ErrorAs(t, err, &codeErr, query)
		assert.Equal(t, http.StatusUnprocessableEntity, codeErr.HTTPCode, query)
	}
}
