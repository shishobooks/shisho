package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

func (a *readingAPI) history(query string) ListReadingHistoryResponse {
	a.t.Helper()
	rec := a.do(http.MethodGet, "/history"+query, nil)
	require.Equal(a.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp ListReadingHistoryResponse
	require.NoError(a.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// readThroughResponse runs a Read-through request that must succeed for a
// Read-through with no reachable Book, which answers with no reading record.
func (a *readingAPI) readThroughResponse(method, path string, body any) models.ReadThrough {
	a.t.Helper()
	rec := a.do(method, path, body)
	require.Equal(a.t, http.StatusOK, rec.Code, rec.Body.String())
	var resp ReadThroughChangeResponse
	require.NoError(a.t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Nil(a.t, resp.Reading)
	return *resp.ReadThrough
}

func addAuthor(t *testing.T, db *bun.DB, book *models.Book, name string, sortOrder int) {
	t.Helper()
	ctx := context.Background()
	person := &models.Person{LibraryID: book.LibraryID, Name: name, SortName: name, SortNameSource: models.DataSourceFilepath}
	_, err := db.NewInsert().Model(person).Exec(ctx)
	require.NoError(t, err)
	_, err = db.NewInsert().Model(&models.Author{BookID: book.ID, PersonID: person.ID, SortOrder: sortOrder}).Exec(ctx)
	require.NoError(t, err)
}

// deleteBook deletes book as every Book delete does: detaching its
// Read-throughs first.
func deleteBook(t *testing.T, db *bun.DB, book *models.Book) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, DetachBooks(ctx, db, []int{book.ID}))
	_, err := db.NewDelete().Model((*models.Book)(nil)).Where("id = ?", book.ID).Exec(ctx)
	require.NoError(t, err)
}

func titles(resp ListReadingHistoryResponse) []string {
	out := make([]string, 0, len(resp.Items))
	for _, item := range resp.Items {
		out = append(out, item.Title)
	}
	return out
}

func TestHistory_ListsEveryReadThroughNewestFirst(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	early := createBook(t, db, "Early")
	late := createBook(t, db, "Late")
	current := createBook(t, db, "Current")
	addAuthor(t, db, late, "Ann Leckie", 1)

	api.mustTransition(early.ID, "finish", "2026-01-10")
	api.mustTransition(late.ID, "start", "2026-02-01")
	api.mustTransition(late.ID, "abandon", "2026-02-20")
	api.mustTransition(current.ID, "start", "2026-03-01")

	resp := api.history("")
	assert.Equal(t, 3, resp.Total)
	assert.Equal(t, []string{"Current", "Late", "Early"}, titles(resp), "underway first, then by end day")
	assert.Equal(t, models.ReadThroughOutcomeUnderway, resp.Items[0].ReadThrough.Outcome)
	assert.Equal(t, []string{"Ann Leckie"}, resp.Items[1].Authors)
	assert.Equal(t, []string{}, resp.Items[0].Authors)
	require.NotNil(t, resp.Items[1].Book)
	assert.Equal(t, late.ID, resp.Items[1].Book.ID)
	assert.Equal(t, late.LibraryID, resp.Items[1].Book.LibraryID)
	assert.False(t, resp.Items[1].Removed)
	assert.True(t, resp.Items[1].CanReread)
	assert.False(t, resp.Items[0].CanReread, "a Book underway can't start again")

	page := api.history("?limit=1&offset=1")
	assert.Equal(t, 3, page.Total)
	assert.Equal(t, []string{"Late"}, titles(page))

	assert.Equal(t, []string{"Early"}, titles(api.history("?outcome=finished")))
	assert.Equal(t, []string{"Late"}, titles(api.history("?outcome=abandoned")))
	assert.Equal(t, []string{"Current"}, titles(api.history("?outcome=underway")))
	assert.Equal(t, http.StatusUnprocessableEntity, api.do(http.MethodGet, "/history?outcome=reading", nil).Code)
}

func TestHistory_NeverShowsAnotherUsersReadThroughs(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	reader := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	other := newReadingAPI(t, db, createUser(t, db, "other", 2))
	book := createBook(t, db, "Dune")
	reader.mustTransition(book.ID, "finish", "2026-03-01")

	resp := other.history("")
	assert.Equal(t, 0, resp.Total)
	assert.Empty(t, resp.Items)

	rt := reader.history("").Items[0].ReadThrough
	assert.Equal(t, http.StatusNotFound, other.do(http.MethodDelete, fmt.Sprintf("/read-throughs/%d", rt.ID), nil).Code)
	assert.Equal(t, http.StatusNotFound, other.do(http.MethodPost, fmt.Sprintf("/read-throughs/%d/finish", rt.ID), DayPayload{Today: "2026-03-02"}).Code)
}

func TestHistory_ABookInALibraryOutOfReachHasNoLink(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	book := createBook(t, db, "Dune")
	newReadingAPI(t, db, user).mustTransition(book.ID, "finish", "2026-03-01")

	user.LibraryAccess = []*models.UserLibraryAccess{{UserID: user.ID, LibraryID: intp(book.LibraryID + 100)}}
	resp := newReadingAPI(t, db, user).history("")
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "Dune", resp.Items[0].Title)
	assert.Nil(t, resp.Items[0].Book, "no link or cover for a Book the user can't reach")
	assert.False(t, resp.Items[0].Removed)
	assert.False(t, resp.Items[0].CanReread)
}

func TestHistory_DeletingABookDetachesItsReadThroughs(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	book := createBook(t, db, "Dune")
	addAuthor(t, db, book, "Frank Herbert", 1)
	addAuthor(t, db, book, "Brian Herbert", 2)
	kept := createBook(t, db, "Kept")
	api.mustTransition(book.ID, "finish", "2026-03-01")
	api.mustTransition(kept.ID, "finish", "2026-03-02")

	deleteBook(t, db, book)

	resp := api.history("?removed=true")
	assert.Equal(t, 1, resp.Total)
	require.Len(t, resp.Items, 1)
	entry := resp.Items[0]
	assert.Equal(t, "Dune", entry.Title)
	assert.Equal(t, []string{"Frank Herbert", "Brian Herbert"}, entry.Authors)
	assert.True(t, entry.Removed)
	assert.Nil(t, entry.Book)
	assert.False(t, entry.CanReread, "there's no Book to reread")
	assert.Nil(t, entry.ReadThrough.BookID)
	assert.Equal(t, "2026-03-01", *entry.ReadThrough.EndDay)

	assert.Equal(t, 2, api.history("").Total, "it still counts")
}

func TestHistory_DetachedReadThroughsCanBeEditedAndDeleted(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	book := createBook(t, db, "Dune")
	api.mustTransition(book.ID, "finish", "2026-03-01")
	deleteBook(t, db, book)
	id := api.history("").Items[0].ReadThrough.ID

	edited := api.readThroughResponse(http.MethodPatch, fmt.Sprintf("/read-throughs/%d", id),
		UpdateReadThroughPayload{BeginDay: strp("2026-02-01"), EndDay: strp("2026-03-05")})
	assert.Equal(t, id, edited.ID)
	assert.Equal(t, "Dune", *edited.BookTitle)
	assert.Equal(t, "2026-03-05", *edited.EndDay)
	assert.Equal(t, "2026-02-01", *api.history("").Items[0].ReadThrough.BeginDay)

	deleted := api.readThroughResponse(http.MethodDelete, fmt.Sprintf("/read-throughs/%d", id), nil)
	assert.Equal(t, id, deleted.ID)
	assert.Equal(t, 0, api.history("").Total)
}

func TestHistory_ADetachedReadThroughUnderwayStaysUnderwayUntilEnded(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	api := newReadingAPI(t, f.db, f.user)
	f.recordSession(f.file, 0.1, 0.3)
	deleteBook(t, f.db, f.book)

	entry := api.history("").Items[0]
	assert.Equal(t, models.ReadThroughOutcomeUnderway, entry.ReadThrough.Outcome, "deleting the Book doesn't end it")
	var sessions int
	require.NoError(t, f.db.NewSelect().Model((*models.ReadingSession)(nil)).ColumnExpr("COUNT(*)").Where("read_through_id = ?", entry.ReadThrough.ID).Scan(f.ctx, &sessions))
	assert.Equal(t, 1, sessions, "its Sessions stay with it")

	// It was last read on the fixture's day, so that's the end day.
	ended := api.readThroughResponse(http.MethodPost, fmt.Sprintf("/read-throughs/%d/abandon", entry.ReadThrough.ID), DayPayload{Today: "2026-10-20"})
	assert.Equal(t, models.ReadThroughOutcomeAbandoned, ended.Outcome)
	assert.Equal(t, "2026-10-15", *ended.EndDay)
	assert.Equal(t, []string{"Dune"}, titles(api.history("?outcome=abandoned&removed=true")))

	rec := api.do(http.MethodPost, fmt.Sprintf("/read-throughs/%d/finish", entry.ReadThrough.ID), DayPayload{Today: "2026-10-20"})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid_state")
}

func TestHistory_ADetachedReadThroughWithNoActivityEndsToday(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	book := createBook(t, db, "Dune")
	api.mustTransition(book.ID, "start", "2026-03-01")
	deleteBook(t, db, book)
	id := api.history("").Items[0].ReadThrough.ID

	ended := api.readThroughResponse(http.MethodPost, fmt.Sprintf("/read-throughs/%d/finish", id), DayPayload{Today: "2026-03-09"})
	assert.Equal(t, models.ReadThroughOutcomeFinished, ended.Outcome)
	assert.Equal(t, "2026-03-09", *ended.EndDay)
}

func TestHistory_DeletingADetachedReadThroughUnderwaySkipsTheBookReset(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	queued := createBook(t, db, "Queued")
	book := createBook(t, db, "Dune")
	require.NoError(t, NewService(db).AddToRead(context.Background(), user.ID, queued.ID))
	require.NoError(t, NewService(db).AddToRead(context.Background(), user.ID, book.ID))
	api.mustTransition(book.ID, "start", "2026-03-01")
	deleteBook(t, db, book)
	id := api.history("").Items[0].ReadThrough.ID

	deleted := api.readThroughResponse(http.MethodDelete, fmt.Sprintf("/read-throughs/%d", id), nil)
	assert.Equal(t, id, deleted.ID)
	assert.Equal(t, 0, api.history("").Total)
	queuedReading := api.get(queued.ID)
	assert.Equal(t, models.ReadingStatusToRead, status(queuedReading))
	assert.Equal(t, 1, *queuedReading.ToReadPlace, "nothing moved on To Read")
}

// Reading Status, To Read, Currently Reading, and device sync all read a
// Book's Read-throughs, so a detached one, even underway, never shows in
// them, and never blocks starting or queueing another Book.
func TestDetachedReadThroughs_AreIgnoredByBookReading(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.recordSession(f.file, 0.1, 0.3)
	gone := f.book
	deleteBook(t, f.db, gone)

	next := createBook(t, f.db, "Next")
	nextFile := f.addFile(next, "next.epub")
	f.book = next
	assert.Empty(t, f.status(), "the new Book has no status from the detached Read-through")
	f.addToRead()
	assert.Equal(t, models.ReadingStatusToRead, f.status())

	summaries, err := f.svc.RetrieveStatusSummaries(f.ctx, f.user.ID, []int{next.ID, gone.ID})
	require.NoError(t, err)
	assert.Equal(t, 0, summaries[next.ID].ReadThroughs, "a Kobo sees no reading of it")
	assert.NotContains(t, summaries, gone.ID)

	var underway []int
	err = f.db.NewSelect().TableExpr("books AS b").Column("b.id").
		Where(UnderwayFilterExpr(f.user.ID, "b.id")).Scan(f.ctx, &underway)
	require.NoError(t, err)
	assert.Empty(t, underway, "Currently Reading lists nothing")

	// KOReader and Kobo updates reach the same service; either starts the
	// new Book while the detached Read-through stays underway.
	for _, source := range []string{models.ReadingSourceKOReader, models.ReadingSourceKobo} {
		f.clock.advance(time.Minute)
		xpointer := "/body/DocFragment[3]/body/p[1]"
		update := PositionUpdate{UserID: f.user.ID, FileID: nextFile.ID, Source: source, Percentage: 0.2}
		if source == models.ReadingSourceKobo {
			span := "OEBPS/ch1.xhtml#kobo.4.1"
			update.Locator = models.ReadingLocator{KoboSpan: &span}
		} else {
			update.Locator = models.ReadingLocator{XPointer: &xpointer}
		}
		_, err := f.svc.UpdatePosition(f.ctx, update)
		require.NoError(t, err, source)
	}
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
	require.Len(t, f.readThroughs(), 1)
	err = f.db.NewSelect().TableExpr("books AS b").Column("b.id").
		Where(UnderwayFilterExpr(f.user.ID, "b.id")).Scan(f.ctx, &underway)
	require.NoError(t, err)
	assert.Equal(t, []int{next.ID}, underway)

	var detachedUnderway int
	require.NoError(t, f.db.NewSelect().Model((*models.ReadThrough)(nil)).ColumnExpr("COUNT(*)").
		Where("book_id IS NULL AND outcome = ?", models.ReadThroughOutcomeUnderway).Scan(f.ctx, &detachedUnderway))
	assert.Equal(t, 1, detachedUnderway)
}

func TestHistory_DatabaseFailureIs500(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	rt := api.mustTransition(book.ID, "start", "2026-03-01").ReadThroughs[0]
	_, err := db.Exec("DROP TABLE read_throughs")
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError, api.do(http.MethodGet, "/history", nil).Code)
	assert.Equal(t, http.StatusInternalServerError,
		api.do(http.MethodPost, fmt.Sprintf("/read-throughs/%d/finish", rt.ID), DayPayload{Today: "2026-03-02"}).Code,
		"a failed lookup isn't a missing Read-through")
}

// A failure while ending a detached Read-through, after it was found, is a
// server error, not the 422 for one already ended.
func TestHistory_EndingFailureIs500(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Dune")
	api.mustTransition(book.ID, "start", "2026-03-01")
	deleteBook(t, db, book)
	id := api.history("").Items[0].ReadThrough.ID
	_, err := db.Exec(`CREATE TRIGGER fail_end BEFORE UPDATE ON read_throughs BEGIN SELECT RAISE(ABORT, 'boom'); END`)
	require.NoError(t, err)

	assert.Equal(t, http.StatusInternalServerError,
		api.do(http.MethodPost, fmt.Sprintf("/read-throughs/%d/finish", id), DayPayload{Today: "2026-03-02"}).Code)
}

// Two requests ending the same detached Read-through: the second finds it
// already ended rather than overwriting the first.
func TestHistory_EndingTwiceKeepsTheFirstOutcome(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	book := createBook(t, db, "Dune")
	api.mustTransition(book.ID, "start", "2026-03-01")
	deleteBook(t, db, book)
	svc := NewService(db)
	stale, err := svc.RetrieveReadThrough(context.Background(), user.ID, api.history("").Items[0].ReadThrough.ID)
	require.NoError(t, err)
	staleCopy := *stale

	require.NoError(t, svc.EndReadThrough(context.Background(), stale, models.ReadThroughOutcomeFinished, "2026-03-02"))
	require.ErrorIs(t, svc.EndReadThrough(context.Background(), &staleCopy, models.ReadThroughOutcomeAbandoned, "2026-03-02"), ErrReadThroughEnded)
	assert.Equal(t, models.ReadThroughOutcomeFinished, api.history("").Items[0].ReadThrough.Outcome)
}
