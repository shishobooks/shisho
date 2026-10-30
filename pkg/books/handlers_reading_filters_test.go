package books

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/appsettings"
	"github.com/shishobooks/shisho/pkg/auth"
	"github.com/shishobooks/shisho/pkg/errcodes"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/shishobooks/shisho/pkg/settings"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// listBookIDs lists books as user through the handler and returns the ids
// in order and the total.
func listBookIDs(t *testing.T, db *bun.DB, user *models.User, query string) ([]int, int) {
	t.Helper()
	h := &handler{bookService: NewService(db, appsettings.NewService(db)), settingsService: settings.NewService(db)}
	e := newTestEchoBooks(t)
	req := httptest.NewRequest(http.MethodGet, "/books?"+query, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	auth.SetUser(c, user)
	require.NoError(t, h.list(c))
	var resp ListBooksResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ids := []int{}
	for _, b := range resp.Items {
		ids = append(ids, b.ID)
	}
	return ids, resp.Total
}

func units(n int) *int { return &n }

var itoa = strconv.Itoa

// Each Reading Status filters to the caller's Books with it, "none" to the
// ones with no status, and several values match any of them. Another
// user's reading never counts.
func TestListHandler_FiltersByReadingStatus(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	lib := seedLibrary(t, db, "Library")
	reader := seedUserWithLibAccess(t, db, "reader", lib)
	other := seedUserWithLibAccess(t, db, "other", lib)

	now := time.Now()
	current := seedBook(t, db, lib, "Reading", "Reading", now)
	paused := seedBook(t, db, lib, "Paused", "Paused", now.Add(-1*time.Hour))
	queued := seedBook(t, db, lib, "Queued", "Queued", now.Add(-2*time.Hour))
	read := seedBook(t, db, lib, "Read", "Read", now.Add(-3*time.Hour))
	dnf := seedBook(t, db, lib, "DNF", "DNF", now.Add(-4*time.Hour))
	untouched := seedBook(t, db, lib, "Untouched", "Untouched", now.Add(-5*time.Hour))
	ratedOnly := seedBook(t, db, lib, "Rated only", "Rated only", now.Add(-6*time.Hour))

	seedReading(t, db, reader.ID, current.ID, nil, [2]string{"start", "2026-01-01"})
	seedReading(t, db, reader.ID, paused.ID, nil, [2]string{"start", "2026-01-01"}, [2]string{"pause", ""})
	require.NoError(t, reading.NewService(db).AddToRead(context.Background(), reader.ID, queued.ID))
	seedReading(t, db, reader.ID, read.ID, nil, [2]string{"finish", "2026-02-01"})
	seedReading(t, db, reader.ID, dnf.ID, nil, [2]string{"start", "2026-01-01"}, [2]string{"abandon", "2026-02-01"})
	// A Rating alone gives no status.
	seedReading(t, db, reader.ID, ratedOnly.ID, units(12))
	// Another user's reading doesn't give the reader a status.
	seedReading(t, db, other.ID, untouched.ID, nil, [2]string{"finish", "2026-02-01"})

	for _, tc := range []struct {
		query string
		want  []int
	}{
		{"reading_status=currently_reading", []int{current.ID}},
		{"reading_status=paused", []int{paused.ID}},
		{"reading_status=to_read", []int{queued.ID}},
		{"reading_status=read", []int{read.ID}},
		{"reading_status=did_not_finish", []int{dnf.ID}},
		{"reading_status=none", []int{untouched.ID, ratedOnly.ID}},
		{"reading_status=read&reading_status=did_not_finish", []int{read.ID, dnf.ID}},
		{"reading_status=none&reading_status=currently_reading", []int{current.ID, untouched.ID, ratedOnly.ID}},
	} {
		ids, total := listBookIDs(t, db, reader, "library_id="+itoa(lib.ID)+"&"+tc.query)
		assert.Equal(t, tc.want, ids, tc.query)
		assert.Equal(t, len(tc.want), total, tc.query)
	}

	ids, _ := listBookIDs(t, db, other, "library_id="+itoa(lib.ID)+"&reading_status=read")
	assert.Equal(t, []int{untouched.ID}, ids, "each user filters on their own status")
}

func TestListHandler_RejectsAnUnknownReadingStatus(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	lib := seedLibrary(t, db, "Library")
	reader := seedUserWithLibAccess(t, db, "reader", lib)
	h := &handler{bookService: NewService(db, appsettings.NewService(db)), settingsService: settings.NewService(db)}
	e := newTestEchoBooks(t)
	req := httptest.NewRequest(http.MethodGet, "/books?reading_status=finished", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	auth.SetUser(c, reader)
	err := h.list(c)
	var codeErr *errcodes.Error
	require.ErrorAs(t, err, &codeErr)
	assert.Equal(t, http.StatusUnprocessableEntity, codeErr.HTTPCode)
}

// The rating range keeps the caller's rated Books inside it, both ends
// included. Unrated Books never match a range.
func TestListHandler_FiltersByRatingRange(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	lib := seedLibrary(t, db, "Library")
	reader := seedUserWithLibAccess(t, db, "reader", lib)
	other := seedUserWithLibAccess(t, db, "other", lib)

	now := time.Now()
	five := seedBook(t, db, lib, "Five", "Five", now)
	fourQuarter := seedBook(t, db, lib, "Four and a quarter", "Four and a quarter", now.Add(-1*time.Hour))
	four := seedBook(t, db, lib, "Four", "Four", now.Add(-2*time.Hour))
	threeHalf := seedBook(t, db, lib, "Three and a half", "Three and a half", now.Add(-3*time.Hour))
	one := seedBook(t, db, lib, "One", "One", now.Add(-4*time.Hour))
	unrated := seedBook(t, db, lib, "Unrated", "Unrated", now.Add(-5*time.Hour))

	seedReading(t, db, reader.ID, five.ID, units(20))
	seedReading(t, db, reader.ID, fourQuarter.ID, units(17))
	seedReading(t, db, reader.ID, four.ID, units(16))
	seedReading(t, db, reader.ID, threeHalf.ID, units(14))
	seedReading(t, db, reader.ID, one.ID, units(4))
	seedReading(t, db, other.ID, unrated.ID, units(20))

	for _, tc := range []struct {
		query string
		want  []int
	}{
		{"rating_min=4", []int{five.ID, fourQuarter.ID, four.ID}},
		{"rating_max=3.5", []int{threeHalf.ID, one.ID}},
		{"rating_min=3.5&rating_max=4.25", []int{fourQuarter.ID, four.ID, threeHalf.ID}},
		{"rating_min=5&rating_max=5", []int{five.ID}},
	} {
		ids, total := listBookIDs(t, db, reader, "library_id="+itoa(lib.ID)+"&"+tc.query)
		assert.Equal(t, tc.want, ids, tc.query)
		assert.Equal(t, len(tc.want), total, tc.query)
	}
}

func TestListHandler_RejectsARatingOutOfRange(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	lib := seedLibrary(t, db, "Library")
	reader := seedUserWithLibAccess(t, db, "reader", lib)
	h := &handler{bookService: NewService(db, appsettings.NewService(db)), settingsService: settings.NewService(db)}
	for _, query := range []string{"rating_min=0", "rating_max=5.5", "rating_min=4&rating_max=3"} {
		e := newTestEchoBooks(t)
		req := httptest.NewRequest(http.MethodGet, "/books?"+query, nil)
		c := e.NewContext(req, httptest.NewRecorder())
		auth.SetUser(c, reader)
		err := h.list(c)
		var codeErr *errcodes.Error
		require.ErrorAs(t, err, &codeErr, query)
		assert.Equal(t, http.StatusUnprocessableEntity, codeErr.HTTPCode, query)
	}
}

// Sorting by rating orders by the caller's Rating, unrated Books last in
// either direction.
func TestListHandler_SortsByRating(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	lib := seedLibrary(t, db, "Library")
	reader := seedUserWithLibAccess(t, db, "reader", lib)
	other := seedUserWithLibAccess(t, db, "other", lib)

	now := time.Now()
	mid := seedBook(t, db, lib, "Mid", "Mid", now)
	unrated := seedBook(t, db, lib, "Unrated", "Unrated", now.Add(-1*time.Hour))
	top := seedBook(t, db, lib, "Top", "Top", now.Add(-2*time.Hour))
	low := seedBook(t, db, lib, "Low", "Low", now.Add(-3*time.Hour))
	seedReading(t, db, reader.ID, mid.ID, units(12))
	seedReading(t, db, reader.ID, top.ID, units(19))
	seedReading(t, db, reader.ID, low.ID, units(2))
	// Another user's Rating doesn't sort the reader's listing.
	seedReading(t, db, other.ID, unrated.ID, units(20))
	seedReading(t, db, other.ID, low.ID, units(20))

	ids, _ := listBookIDs(t, db, reader, "library_id="+itoa(lib.ID)+"&sort=rating:desc")
	assert.Equal(t, []int{top.ID, mid.ID, low.ID, unrated.ID}, ids)

	ids, _ = listBookIDs(t, db, reader, "library_id="+itoa(lib.ID)+"&sort=rating:asc")
	assert.Equal(t, []int{low.ID, mid.ID, top.ID, unrated.ID}, ids)
}

// Sorting by finish date orders by the end day of the caller's latest
// finished Read-through. Books never finished, or finished on no known day,
// come last; an abandoned Read-through doesn't count.
func TestListHandler_SortsByFinishDate(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	lib := seedLibrary(t, db, "Library")
	reader := seedUserWithLibAccess(t, db, "reader", lib)
	other := seedUserWithLibAccess(t, db, "other", lib)

	now := time.Now()
	reread := seedBook(t, db, lib, "Reread", "Reread", now)
	march := seedBook(t, db, lib, "March", "March", now.Add(-1*time.Hour))
	abandoned := seedBook(t, db, lib, "Abandoned", "Abandoned", now.Add(-2*time.Hour))
	january := seedBook(t, db, lib, "January", "January", now.Add(-3*time.Hour))
	never := seedBook(t, db, lib, "Never", "Never", now.Add(-4*time.Hour))

	// Finished in January, then again in May: May counts.
	seedReading(t, db, reader.ID, reread.ID, nil,
		[2]string{"finish", "2026-01-10"},
		[2]string{"start", "2026-04-01"}, [2]string{"finish", "2026-05-01"})
	seedReading(t, db, reader.ID, march.ID, nil, [2]string{"finish", "2026-03-01"})
	seedReading(t, db, reader.ID, january.ID, nil, [2]string{"finish", "2026-01-15"})
	seedReading(t, db, reader.ID, abandoned.ID, nil, [2]string{"start", "2026-01-01"}, [2]string{"abandon", "2026-06-01"})
	seedReading(t, db, other.ID, never.ID, nil, [2]string{"finish", "2026-07-01"})

	ids, _ := listBookIDs(t, db, reader, "library_id="+itoa(lib.ID)+"&sort=date_finished:desc")
	assert.Equal(t, []int{reread.ID, march.ID, january.ID, abandoned.ID, never.ID}, ids)

	ids, _ = listBookIDs(t, db, reader, "library_id="+itoa(lib.ID)+"&sort=date_finished:asc")
	assert.Equal(t, []int{january.ID, march.ID, reread.ID, abandoned.ID, never.ID}, ids)
}

// Filters apply before paging, so the total counts every match and each
// page holds the next matches, across the libraries the caller can reach
// and never one they can't.
func TestListHandler_ReadingFilterPagesAndTotals(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	libA := seedLibrary(t, db, "A")
	libB := seedLibrary(t, db, "B")
	hidden := seedLibrary(t, db, "Hidden")
	reader := seedUserWithLibAccess(t, db, "reader", libA)
	_, err := db.NewInsert().Model(&models.UserLibraryAccess{UserID: reader.ID, LibraryID: &libB.ID}).Exec(context.Background())
	require.NoError(t, err)
	reader = reloadUser(t, db, reader.ID)

	now := time.Now()
	var want []int
	for i, lib := range []*models.Library{libA, libB, libA, libB, libA} {
		b := seedBook(t, db, lib, "Read "+itoa(i), "Read "+itoa(i), now.Add(-time.Duration(i)*time.Hour))
		seedReading(t, db, reader.ID, b.ID, units(20-i), [2]string{"finish", "2026-01-01"})
		want = append(want, b.ID)
		// Unread books between the read ones.
		seedBook(t, db, lib, "Unread "+itoa(i), "Unread "+itoa(i), now.Add(-time.Duration(i)*time.Hour-time.Minute))
	}
	hiddenBook := seedBook(t, db, hidden, "Hidden", "Hidden", now.Add(time.Hour))
	seedReading(t, db, reader.ID, hiddenBook.ID, units(20), [2]string{"finish", "2026-01-01"})

	ids, total := listBookIDs(t, db, reader, "reading_status=read&sort=rating:desc&limit=2&offset=0")
	assert.Equal(t, want[:2], ids)
	assert.Equal(t, 5, total)
	ids, total = listBookIDs(t, db, reader, "reading_status=read&sort=rating:desc&limit=2&offset=2")
	assert.Equal(t, want[2:4], ids)
	assert.Equal(t, 5, total)
	ids, total = listBookIDs(t, db, reader, "reading_status=read&sort=rating:desc&limit=2&offset=4")
	assert.Equal(t, want[4:], ids)
	assert.Equal(t, 5, total)

	ids, total = listBookIDs(t, db, reader, "library_id="+itoa(libB.ID)+"&rating_min=4.25&limit=1")
	assert.Equal(t, []int{want[1]}, ids)
	assert.Equal(t, 2, total)
}

// The Currently Reading listing has the caller's Books underway, the ones
// being read before the paused ones, each most recently begun first.
func TestListHandler_CurrentlyReadingPutsPausedLast(t *testing.T) {
	t.Parallel()

	db := testdb.New(t)
	lib := seedLibrary(t, db, "Library")
	reader := seedUserWithLibAccess(t, db, "reader", lib)
	other := seedUserWithLibAccess(t, db, "other", lib)

	now := time.Now()
	pausedNew := seedBook(t, db, lib, "Paused new", "Paused new", now)
	older := seedBook(t, db, lib, "Older", "Older", now.Add(-1*time.Hour))
	pausedOld := seedBook(t, db, lib, "Paused old", "Paused old", now.Add(-2*time.Hour))
	newer := seedBook(t, db, lib, "Newer", "Newer", now.Add(-3*time.Hour))
	finished := seedBook(t, db, lib, "Finished", "Finished", now.Add(-4*time.Hour))
	othersBook := seedBook(t, db, lib, "Other's", "Other's", now.Add(-5*time.Hour))

	seedReading(t, db, reader.ID, older.ID, nil, [2]string{"start", "2026-01-01"})
	seedReading(t, db, reader.ID, newer.ID, nil, [2]string{"start", "2026-03-01"})
	seedReading(t, db, reader.ID, pausedOld.ID, nil, [2]string{"start", "2026-01-05"}, [2]string{"pause", ""})
	seedReading(t, db, reader.ID, pausedNew.ID, nil, [2]string{"start", "2026-02-05"}, [2]string{"pause", ""})
	seedReading(t, db, reader.ID, finished.ID, nil, [2]string{"start", "2026-01-01"}, [2]string{"finish", "2026-01-02"})
	seedReading(t, db, other.ID, othersBook.ID, nil, [2]string{"start", "2026-01-01"})

	ids, total := listBookIDs(t, db, reader, "currently_reading=true&sort=title:asc")
	assert.Equal(t, []int{newer.ID, older.ID, pausedNew.ID, pausedOld.ID}, ids, "the reading order wins over a sort")
	assert.Equal(t, 4, total)
}

func reloadUser(t *testing.T, db *bun.DB, id int) *models.User {
	t.Helper()
	u := &models.User{}
	err := db.NewSelect().Model(u).Relation("LibraryAccess").Where("u.id = ?", id).Scan(context.Background())
	require.NoError(t, err)
	return u
}
