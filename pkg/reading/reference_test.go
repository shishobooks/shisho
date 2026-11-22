package reading

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// markReference marks bookID Reference as the status menu does, confirming
// the Read-through underway, if any.
func (a *readingAPI) markReference(bookID int) BookReadingResponse {
	a.t.Helper()
	return a.ok(http.MethodPost, fmt.Sprintf("/books/%d/reference", bookID), MarkReferencePayload{ReadThroughID: a.underwayID(bookID)})
}

// underwayID is the id of the Read-through underway on bookID, or nil.
func (a *readingAPI) underwayID(bookID int) *int {
	a.t.Helper()
	for _, rt := range a.get(bookID).ReadThroughs {
		if rt.Outcome == models.ReadThroughOutcomeUnderway {
			return &rt.ID
		}
	}
	return nil
}

func (a *readingAPI) removeReference(bookID int) BookReadingResponse {
	a.t.Helper()
	return a.ok(http.MethodDelete, fmt.Sprintf("/books/%d/reference", bookID), nil)
}

// userBook loads userID's record for bookID, or nil when there's none.
func userBook(t *testing.T, db *bun.DB, userID, bookID int) *models.UserBook {
	t.Helper()
	ub := &models.UserBook{}
	err := db.NewSelect().Model(ub).Where("ub.user_id = ? AND ub.book_id = ?", userID, bookID).Scan(context.Background())
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	require.NoError(t, err)
	return ub
}

// statusChosenAt is when userID last chose a status for bookID, or nil.
func statusChosenAt(t *testing.T, db *bun.DB, userID, bookID int) *time.Time {
	t.Helper()
	ub := userBook(t, db, userID, bookID)
	if ub == nil {
		return nil
	}
	return ub.StatusChosenAt
}

// readFor gives f.book a Read-through underway with an update and a
// session on it, as reading a few pages does.
func (f *positionsFixture) readFor(minutes int) {
	f.t.Helper()
	start := f.clock.now
	f.clock.advance(time.Duration(minutes) * time.Minute)
	cfi := "epubcfi(/6/8!/4/2)"
	_, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: f.file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: 0.4,
		Session: &SessionInput{StartedAt: start, EndedAt: f.clock.now, StartPercentage: 0.1, EndPercentage: 0.4, Measured: true},
	})
	require.NoError(f.t, err)
}

func (f *positionsFixture) markReference() {
	f.t.Helper()
	underway, err := retrieveUnderway(f.ctx, f.db, f.user.ID, f.book.ID)
	require.NoError(f.t, err)
	require.NoError(f.t, f.svc.MarkReference(f.ctx, f.user.ID, f.book.ID, readThroughID(underway)))
}

func (f *positionsFixture) onToRead() bool {
	f.t.Helper()
	reading, err := f.svc.RetrieveBookReading(f.ctx, f.user.ID, f.book.ID)
	require.NoError(f.t, err)
	return reading.OnToRead
}

func TestMarkReference_TurnsTheReadThroughUnderwayIntoPlainSessions(t *testing.T) {
	t.Parallel()
	for _, paused := range []bool{false, true} {
		t.Run(fmt.Sprintf("paused=%v", paused), func(t *testing.T) {
			t.Parallel()
			f := newPositionsFixture(t)
			f.readFor(20)
			if paused {
				require.NoError(t, f.svc.SetPaused(f.ctx, f.user.ID, f.book.ID, true))
			}
			require.Len(t, f.readThroughs(), 1)
			before := f.position(f.file)
			require.NotNil(t, before)

			f.markReference()

			assert.Equal(t, models.ReadingStatusReference, f.status())
			assert.Empty(t, f.readThroughs(), "the Read-through leaves Reading History")
			assert.Equal(t, 1, f.count("reading_sessions"), "its sessions are kept")
			assert.Equal(t, 1, f.count("reading_position_updates"), "its update history is kept")
			var attached int
			err := f.db.NewSelect().TableExpr("reading_sessions").ColumnExpr("COUNT(*)").
				Where("read_through_id IS NOT NULL").Scan(f.ctx, &attached)
			require.NoError(t, err)
			assert.Zero(t, attached, "the sessions are detached, as a lookup's are")
			after := f.position(f.file)
			require.NotNil(t, after, "the Reading Position stays")
			assert.Equal(t, before.CFI, after.CFI)

			stats, err := f.svc.Stats(f.ctx, f.user, nil)
			require.NoError(t, err)
			assert.Equal(t, 20*60, stats.Totals.ReadingSeconds, "the time keeps counting")
			assert.Zero(t, stats.Totals.Finished)
		})
	}
}

func TestMarkReference_RefusesWhenTheReadThroughUnderwayIsntTheConfirmedOne(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	// The user saw nothing underway, but a device started a Read-through since.
	f.readFor(5)

	err := f.svc.MarkReference(f.ctx, f.user.ID, f.book.ID, nil)

	require.ErrorIs(t, err, ErrUnderwayChanged)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
	require.Len(t, f.readThroughs(), 1, "nothing is deleted unconfirmed")

	// A Read-through the user confirmed that has ended since refuses too.
	stale := f.readThroughs()[0].ID
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-15"))
	require.ErrorIs(t, f.svc.MarkReference(f.ctx, f.user.ID, f.book.ID, &stale), ErrUnderwayChanged)
	assert.Equal(t, models.ReadingStatusRead, f.status())
}

func TestMarkReference_AStaleConfirmationIsA422(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Cookbook")
	api.mustTransition(book.ID, "start", "2026-10-01")

	rec := api.do(http.MethodPost, fmt.Sprintf("/books/%d/reference", book.ID), MarkReferencePayload{})

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Equal(t, models.ReadingStatusCurrentlyReading, status(api.get(book.ID)))
}

func TestMarkReference_TakesTheBookOffToRead(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.addToRead()

	f.markReference()

	assert.False(t, f.onToRead())
	assert.Equal(t, models.ReadingStatusReference, f.status())
}

func TestMarkReference_OutranksEarlierEndedReadThroughs(t *testing.T) {
	t.Parallel()
	f := newStatsFixture(t)
	finished := createBook(t, f.db, "Cookbook")
	abandoned := createBook(t, f.db, "Textbook")
	file := f.addFile(finished, models.FileTypeEPUB)
	rt := f.finished(finished, "2024-03-01", "2024-03-10")
	f.session(sessionSpec{file: file, readThrough: rt,
		start: mustTime("2024-03-05T10:00:00Z"), end: mustTime("2024-03-05T11:00:00Z")})
	seedReadThrough(t, f.db, f.user, abandoned, "2024-05-01", "2024-05-02", models.ReadThroughOutcomeAbandoned)

	for _, book := range []*models.Book{finished, abandoned} {
		resp := f.api.markReference(book.ID)
		assert.Equal(t, models.ReadingStatusReference, status(resp), book.Title)
		assert.Len(t, resp.ReadThroughs, 1, "%s keeps its history", book.Title)
	}

	stats := f.stats("?year=2024")
	assert.Equal(t, 1, stats.Totals.Finished, "the 2024 finish still counts")
	assert.Equal(t, 1, stats.Totals.DidNotFinish, "the 2024 abandon still counts")
	assert.Equal(t, 3600, stats.Totals.ReadingSeconds)
}

func TestReference_AddsOnlyTimeToReadingStats(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-09-01"))
	f.markReference()
	year := 2026
	before, err := f.svc.Stats(f.ctx, f.user, &year)
	require.NoError(t, err)

	// A visit that reads through to the end, with a session.
	start := f.clock.now
	f.clock.advance(30 * time.Minute)
	cfi := "epubcfi(/6/40!/4/2)"
	result, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: f.file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: 1, AtEnd: true,
		Session: &SessionInput{StartedAt: start, EndedAt: f.clock.now, StartPercentage: 0.2, EndPercentage: 1, Measured: true},
	})
	require.NoError(t, err)
	require.Nil(t, result.FinishedReadThroughID)

	after, err := f.svc.Stats(f.ctx, f.user, &year)
	require.NoError(t, err)
	assert.Equal(t, before.Totals.ReadingSeconds+30*60, after.Totals.ReadingSeconds, "the time counts")
	assert.Equal(t, 1, after.Totals.Finished, "the September finish still counts, and nothing new does")
	assert.Equal(t, before.Totals.Rereads, after.Totals.Rereads)
	assert.Equal(t, before.Totals.DidNotFinish, after.Totals.DidNotFinish)
	assert.Equal(t, before.Kinds, after.Kinds, "the finished books by kind are unchanged")
	finishedByMonth := map[int]int{}
	for _, p := range after.Periods {
		if p.Month != nil {
			finishedByMonth[*p.Month] += p.Finished
		}
	}
	assert.Equal(t, 1, finishedByMonth[9], "September keeps its finish")
	assert.Zero(t, finishedByMonth[10], "October gains none")
}

func TestMarkReference_RefusesABookAlreadyReference(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	book := createBook(t, db, "Cookbook")
	api.markReference(book.ID)

	rec := api.do(http.MethodPost, fmt.Sprintf("/books/%d/reference", book.ID), MarkReferencePayload{})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestRemoveReference_FallsBackToTheHistory(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	finished := createBook(t, db, "Finished")
	never := createBook(t, db, "Never")
	api.mustTransition(finished.ID, "finish", "2024-03-01")
	api.markReference(finished.ID)
	api.markReference(never.ID)

	assert.Equal(t, models.ReadingStatusRead, status(api.removeReference(finished.ID)))
	resp := api.removeReference(never.ID)
	assert.Nil(t, resp.Status)
	assert.Empty(t, resp.ReadThroughs, "removing records nothing new")

	rec := api.do(http.MethodDelete, fmt.Sprintf("/books/%d/reference", never.ID), nil)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "a Book that isn't Reference")
}

func TestRemoveReference_ABookNeverReadStartsOnItsNextProgress(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.markReference()
	require.NoError(t, f.svc.RemoveReference(f.ctx, f.user.ID, f.book.ID))

	result := f.read(f.file, "epubcfi(/6/4!/4/2)", 0.1, false)

	assert.True(t, result.Started)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

func TestReference_ClearedByOtherChoices(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		choose func(api *readingAPI, bookID int) BookReadingResponse
		want   string
	}{
		{"start", func(api *readingAPI, bookID int) BookReadingResponse {
			return api.mustTransition(bookID, "start", "2026-10-01")
		}, models.ReadingStatusCurrentlyReading},
		{"mark as read", func(api *readingAPI, bookID int) BookReadingResponse {
			return api.mustTransition(bookID, "finish", "2026-10-01")
		}, models.ReadingStatusRead},
		{"add to To Read", func(api *readingAPI, bookID int) BookReadingResponse {
			return api.addToRead(bookID)
		}, models.ReadingStatusToRead},
		{"bulk mark as read", func(api *readingAPI, bookID int) BookReadingResponse {
			api.bulk("finish", BulkFinishPayload{BookIDs: []int{bookID}, Today: "2026-10-01"})
			return api.get(bookID)
		}, models.ReadingStatusRead},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			db := testdb.New(t)
			user := createUser(t, db, "reader", 2)
			api := newReadingAPI(t, db, user)
			book := createBook(t, db, "Cookbook")
			api.markReference(book.ID)

			assert.Equal(t, c.want, status(c.choose(api, book.ID)))
			ub := userBook(t, db, user.ID, book.ID)
			require.NotNil(t, ub)
			assert.False(t, ub.Reference)
		})
	}
}

func TestReference_AppendingToToReadClearsIt(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	svc := NewService(db)
	api := newReadingAPI(t, db, user)
	book := createBook(t, db, "Cookbook")
	api.markReference(book.ID)

	added, err := svc.AppendToRead(context.Background(), user.ID, []int{book.ID})
	require.NoError(t, err)
	assert.Equal(t, 1, added)
	assert.Equal(t, models.ReadingStatusToRead, status(api.get(book.ID)))
}

func TestReference_EveryExplicitChoiceStampsTheInstant(t *testing.T) {
	t.Parallel()
	type choice func(f *positionsFixture) error
	start := func(f *positionsFixture) error { return f.svc.Start(f.ctx, f.user.ID, f.book.ID, "2026-10-15") }
	mark := func(f *positionsFixture) error { return f.svc.MarkReference(f.ctx, f.user.ID, f.book.ID, nil) }
	bulk := func(run func(context.Context, int, []int) (BulkReadingResponse, error)) choice {
		return func(f *positionsFixture) error {
			_, err := run(f.ctx, f.user.ID, []int{f.book.ID})
			return err
		}
	}
	cases := []struct {
		name          string
		setup, choose choice
	}{
		{"mark reference", nil, mark},
		{"remove reference", mark, func(f *positionsFixture) error { return f.svc.RemoveReference(f.ctx, f.user.ID, f.book.ID) }},
		{"start", nil, start},
		{"mark as read", nil, func(f *positionsFixture) error {
			return f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-15")
		}},
		{"mark as did not finish", start, func(f *positionsFixture) error {
			return f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeAbandoned, "2026-10-15")
		}},
		{"add to To Read", nil, func(f *positionsFixture) error { return f.svc.AddToRead(f.ctx, f.user.ID, f.book.ID) }},
		{"bulk mark as read", nil, func(f *positionsFixture) error {
			_, err := f.svc.BulkFinish(f.ctx, f.user.ID, []int{f.book.ID}, "2026-10-15")
			return err
		}},
		{"bulk mark as reference", nil, func(f *positionsFixture) error { return bulk(f.svc.BulkReference)(f) }},
		{"bulk remove from reference", mark, func(f *positionsFixture) error { return bulk(f.svc.BulkRemoveReference)(f) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			f := newPositionsFixture(t)
			if c.setup != nil {
				require.NoError(t, c.setup(f))
			}
			f.clock.advance(time.Hour)

			require.NoError(t, c.choose(f))

			after := statusChosenAt(t, f.db, f.user.ID, f.book.ID)
			require.NotNil(t, after, "the choice is stamped")
			assert.True(t, after.Equal(f.clock.now), "stamped %v, the choice was at %v", after, f.clock.now)
		})
	}
}

func TestReference_PauseAndRemoveFromToReadDontStamp(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	queued := createBook(t, f.db, "Queued")
	require.NoError(t, f.svc.Start(f.ctx, f.user.ID, f.book.ID, "2026-10-15"))
	require.NoError(t, f.svc.AddToRead(f.ctx, f.user.ID, queued.ID))
	chosen := f.clock.now
	f.clock.advance(time.Hour)

	require.NoError(t, f.svc.SetPaused(f.ctx, f.user.ID, f.book.ID, true))
	require.NoError(t, f.svc.RemoveFromToRead(f.ctx, f.user.ID, queued.ID))

	for _, bookID := range []int{f.book.ID, queued.ID} {
		at := statusChosenAt(t, f.db, f.user.ID, bookID)
		require.NotNil(t, at)
		assert.True(t, at.Equal(chosen), "book %d stamped %v", bookID, at)
	}
}

func TestReference_PositionUpdatesNeverStartIt(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.markReference()

	result := f.read(f.file, "epubcfi(/6/4!/4/2)", 0.3, false)
	assert.True(t, result.Accepted, "the position still moves")
	assert.False(t, result.Started)

	f.clock.advance(time.Minute)
	result = f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	assert.Nil(t, result.FinishedReadThroughID, "reaching the end finishes nothing")

	assert.Equal(t, models.ReadingStatusReference, f.status())
	assert.Empty(t, f.readThroughs())
	assert.Equal(t, "epubcfi(/6/40!/4/2)", *f.position(f.file).CFI)
}

func TestReference_ReportedStatusesNeverChangeIt(t *testing.T) {
	t.Parallel()
	for _, reported := range []ReportedStatus{ReportedReading, ReportedFinished} {
		t.Run(fmt.Sprintf("reported=%d", reported), func(t *testing.T) {
			t.Parallel()
			f := newPositionsFixture(t)
			f.markReference()

			span := "OEBPS/ch1.xhtml#kobo.4.1"
			result, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
				UserID: f.user.ID, FileID: f.file.ID, Source: models.ReadingSourceKobo,
				Locator: models.ReadingLocator{KoboSpan: &span}, Percentage: 0.5, ReportedStatus: reported,
			})
			require.NoError(t, err)

			assert.False(t, result.Started)
			assert.Nil(t, result.FinishedReadThroughID)
			assert.Equal(t, models.ReadingStatusReference, f.status())
			assert.Empty(t, f.readThroughs())
		})
	}
}

func TestReference_PagingBackAfterAnAutomaticFinishDoesntReopen(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4!/4/2)", 0.1, false)
	f.clock.advance(time.Minute)
	finish := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.NotNil(t, finish.FinishedReadThroughID)
	f.markReference()

	f.clock.advance(time.Minute)
	result := f.read(f.file, "epubcfi(/6/38!/4/2)", 0.95, false)

	assert.False(t, result.Reverted)
	assert.Equal(t, models.ReadingStatusReference, f.status())
	rts := f.readThroughs()
	require.Len(t, rts, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
}

func TestReference_UndoOnAnAutomaticFinishIsRefused(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4!/4/2)", 0.1, false)
	f.clock.advance(time.Minute)
	finish := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.NotNil(t, finish.FinishedReadThroughID)
	f.markReference()

	rt, err := f.svc.RetrieveReadThrough(f.ctx, f.user.ID, *finish.FinishedReadThroughID)
	require.NoError(t, err)
	err = f.svc.UndoAutoFinish(f.ctx, rt)

	require.ErrorIs(t, err, ErrUndoOnReference)
	assert.Equal(t, models.ReadingStatusReference, f.status())
}

func TestReference_RemovingItLeavesAnEarlierAutomaticFinishStanding(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4!/4/2)", 0.1, false)
	f.clock.advance(time.Minute)
	finish := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.NotNil(t, finish.FinishedReadThroughID)
	f.markReference()
	require.NoError(t, f.svc.RemoveReference(f.ctx, f.user.ID, f.book.ID))

	f.clock.advance(time.Minute)
	result := f.read(f.file, "epubcfi(/6/38!/4/2)", 0.95, false)

	assert.False(t, result.Reverted, "Reference was the later choice")
	assert.Equal(t, models.ReadingStatusRead, f.status())
	rt, err := f.svc.RetrieveReadThrough(f.ctx, f.user.ID, *finish.FinishedReadThroughID)
	require.NoError(t, err)
	require.ErrorIs(t, f.svc.UndoAutoFinish(f.ctx, rt), ErrNotAutoFinished)
}

// Late and same-day sessions are placed by day as in any finished Book (ADR
// 0028): Reference adds no rule of its own for them.
func TestReference_ALookupOnTheDayOfAFinishJoinsIt(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	// Finished earlier today, then marked Reference.
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-15"))
	f.markReference()

	start := f.clock.now
	f.clock.advance(10 * time.Minute)
	_, err := f.svc.RecordSession(f.ctx, f.user.ID, f.file.ID, models.ReadingSourceWebReader,
		SessionInput{StartedAt: start, EndedAt: f.clock.now, StartPercentage: 0.2, EndPercentage: 0.3, Measured: true})
	require.NoError(t, err)

	var attached int
	err = f.db.NewSelect().TableExpr("reading_sessions").ColumnExpr("COUNT(*)").
		Where("read_through_id IS NOT NULL").Scan(f.ctx, &attached)
	require.NoError(t, err)
	assert.Equal(t, 1, attached, "placed by day, as in a finished Book")
}

func TestReference_AFinishFromAFileMovedOutStaysFinished(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4!/4/2)", 0.1, false)
	f.clock.advance(time.Minute)
	finish := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.NotNil(t, finish.FinishedReadThroughID)
	// The File moves to another Book before this one is marked Reference.
	other := createBook(t, f.db, "Other")
	_, err := f.db.NewUpdate().Model((*models.File)(nil)).Set("book_id = ?", other.ID).Where("id = ?", f.file.ID).Exec(f.ctx)
	require.NoError(t, err)
	f.markReference()

	f.clock.advance(time.Minute)
	result := f.read(f.file, "epubcfi(/6/38!/4/2)", 0.95, false)

	assert.False(t, result.Reverted)
	assert.Equal(t, models.ReadingStatusReference, f.status())
	underway, err := retrieveUnderway(f.ctx, f.db, f.user.ID, f.book.ID)
	require.NoError(t, err)
	assert.Nil(t, underway)
}

func TestReference_AStaleDeleteOfTheReadUnderwayChangesNothing(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.addToRead()
	f.readFor(5)
	stale := f.readThroughs()[0]
	require.Equal(t, models.ReadThroughOutcomeUnderway, stale.Outcome)
	require.NotNil(t, stale.LeftToReadPosition)
	f.markReference()

	// A delete that loaded the Read-through before Reference dissolved it.
	require.NoError(t, f.svc.DeleteReadThrough(f.ctx, stale))

	assert.Equal(t, models.ReadingStatusReference, f.status())
	assert.False(t, f.onToRead(), "the Book doesn't go back to To Read")
	assert.NotNil(t, f.position(f.file), "the place stays")
}

func TestReference_ALateSessionFromAnEarlierFinishJoinsIt(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	start := f.clock.now
	f.read(f.file, "epubcfi(/6/4!/4/2)", 0.1, false)
	f.clock.advance(time.Minute)
	require.NotNil(t, f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true).FinishedReadThroughID)
	f.markReference()

	// The session that ran across the finish, arriving after the choice.
	_, err := f.svc.RecordSession(f.ctx, f.user.ID, f.file.ID, models.ReadingSourceWebReader,
		SessionInput{StartedAt: start, EndedAt: f.clock.now, StartPercentage: 0.1, EndPercentage: 1, Measured: true})
	require.NoError(t, err)

	var attached int
	err = f.db.NewSelect().TableExpr("reading_sessions").ColumnExpr("COUNT(*)").
		Where("read_through_id IS NOT NULL").Scan(f.ctx, &attached)
	require.NoError(t, err)
	assert.Equal(t, 1, attached, "the session ran during that Read-through")
}

func TestReference_UndoIsRefusedAfterAMergeIntoAReferenceBook(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4!/4/2)", 0.1, false)
	f.clock.advance(time.Minute)
	finish := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.NotNil(t, finish.FinishedReadThroughID)
	stale, err := f.svc.RetrieveReadThrough(f.ctx, f.user.ID, *finish.FinishedReadThroughID)
	require.NoError(t, err)
	target := createBook(t, f.db, "Target")
	require.NoError(t, f.svc.MarkReference(f.ctx, f.user.ID, target.ID, nil))
	err = f.db.RunInTx(f.ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := MergeBooks(ctx, tx, f.book.ID, target.ID); err != nil {
			return err
		}
		_, err := tx.NewUpdate().Model((*models.File)(nil)).Set("book_id = ?", target.ID).Where("book_id = ?", f.book.ID).Exec(ctx)
		return err
	})
	require.NoError(t, err)

	require.ErrorIs(t, f.svc.UndoAutoFinish(f.ctx, stale), ErrUndoOnReference)
	underway, err := retrieveUnderway(f.ctx, f.db, f.user.ID, target.ID)
	require.NoError(t, err)
	assert.Nil(t, underway)
}

func TestReference_EndingAStaleReadThroughFromReadingHistoryChangesNothing(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.readFor(5)
	stale := f.readThroughs()[0]
	f.markReference()

	err := f.svc.EndReadThrough(f.ctx, stale, models.ReadThroughOutcomeFinished, "2026-10-15")

	require.ErrorIs(t, err, ErrReadThroughEnded)
	assert.Equal(t, models.ReadingStatusReference, f.status())
	assert.Empty(t, f.readThroughs(), "no finish is recorded in its place")
}

func TestReference_DeviceStatusesAndHistoryNeverChangeIt(t *testing.T) {
	t.Parallel()
	for _, s := range []string{models.ReadingStatusRead, models.ReadingStatusPaused, models.ReadingStatusCurrentlyReading} {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			f := newPositionsFixture(t)
			f.markReference()

			require.NoError(t, f.svc.SetDeviceStatus(f.ctx, DeviceStatus{UserID: f.user.ID, FileID: f.file.ID, Status: s, Day: "2026-10-15"}))
			require.NoError(t, f.svc.RecordHistoryStatus(f.ctx, DeviceStatus{UserID: f.user.ID, FileID: f.file.ID, Status: s, Day: "2026-10-15"}))

			assert.Equal(t, models.ReadingStatusReference, f.status())
			assert.Empty(t, f.readThroughs())
		})
	}
}

func TestReference_NeverCoexistsWithUnderwayOrToRead(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	book := createBook(t, db, "Cookbook")

	// Every way into Reference and out of it, in turn.
	steps := []func(){
		func() { api.addToRead(book.ID) },
		func() { api.markReference(book.ID) },
		func() { api.mustTransition(book.ID, "start", "2026-10-01") },
		func() { api.markReference(book.ID) },
		func() { api.mustTransition(book.ID, "start", "2026-10-02") },
		func() { api.mustTransition(book.ID, "pause", "") },
		func() { api.markReference(book.ID) },
		func() { api.removeReference(book.ID) },
		func() { api.markReference(book.ID) },
		func() { api.mustTransition(book.ID, "finish", "2026-10-03") },
		func() { api.markReference(book.ID) },
		func() { api.addToRead(book.ID) },
	}
	for i, step := range steps {
		step()
		ub := userBook(t, db, user.ID, book.ID)
		if ub == nil || !ub.Reference {
			continue
		}
		assert.Nil(t, ub.ToReadPosition, "step %d: Reference and To Read", i)
		underway, err := retrieveUnderway(context.Background(), db, user.ID, book.ID)
		require.NoError(t, err)
		assert.Nil(t, underway, "step %d: Reference and a Read-through underway", i)
	}
}

func TestBulkReference_MarksAndRemoves(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	api := newReadingAPI(t, db, createUser(t, db, "reader", 2))
	reading := createBook(t, db, "Reading")
	queued := createBook(t, db, "Queued")
	read := createBook(t, db, "Read")
	already := createBook(t, db, "Already")
	api.mustTransition(reading.ID, "start", "2026-03-01")
	api.addToRead(queued.ID)
	api.mustTransition(read.ID, "finish", "2026-02-01")
	api.markReference(already.ID)

	resp := api.bulk("reference", BulkReferencePayload{BookIDs: []int{reading.ID, queued.ID, read.ID, already.ID, reading.ID}})
	assert.Equal(t, BulkReadingResponse{Changed: 3, Skipped: 1}, resp)
	for _, book := range []*models.Book{reading, queued, read, already} {
		got := api.get(book.ID)
		assert.Equal(t, models.ReadingStatusReference, status(got), book.Title)
		assert.False(t, got.OnToRead, book.Title)
	}
	assert.Empty(t, api.get(reading.ID).ReadThroughs, "the read underway leaves Reading History")
	assert.Len(t, api.get(read.ID).ReadThroughs, 1, "an ended one stays")

	untracked := createBook(t, db, "Untracked")
	resp = api.bulk("remove-reference", BulkReferencePayload{BookIDs: []int{reading.ID, read.ID, untracked.ID}})
	assert.Equal(t, BulkReadingResponse{Changed: 2, Skipped: 1}, resp)
	assert.Nil(t, api.get(reading.ID).Status)
	assert.Equal(t, models.ReadingStatusRead, status(api.get(read.ID)))
	assert.Equal(t, models.ReadingStatusReference, status(api.get(queued.ID)), "Books not named keep it")
}

func TestReference_StatusFilterAndSort(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	ref := createBook(t, db, "Ref")
	dnf := createBook(t, db, "DNF")
	none := createBook(t, db, "None")
	api.markReference(ref.ID)
	api.mustTransition(dnf.ID, "start", "2026-03-01")
	api.mustTransition(dnf.ID, "abandon", "2026-03-02")

	var filtered []int
	q := db.NewSelect().TableExpr("books AS b").Column("b.id").OrderExpr("b.id")
	err := Filter{Statuses: []string{models.ReadingStatusReference}}.Apply(q, user.ID, "b.id").Scan(context.Background(), &filtered)
	require.NoError(t, err)
	assert.Equal(t, []int{ref.ID}, filtered)

	var sorted []int
	err = db.NewSelect().TableExpr("books AS b").Column("b.id").
		OrderExpr(StatusRankExpr(user.ID, "b.id")+" ASC").Scan(context.Background(), &sorted)
	require.NoError(t, err)
	assert.Equal(t, []int{dnf.ID, ref.ID, none.ID}, sorted, "Reference sorts after Did Not Finish")
}

// mergeFixture is one user's record of two Books about to be merged.
type mergeFixture struct {
	t              *testing.T
	ctx            context.Context
	db             *bun.DB
	user           *models.User
	source, target *models.Book
}

func newMergeFixture(t *testing.T) *mergeFixture {
	t.Helper()
	db := testdb.New(t)
	return &mergeFixture{
		t: t, ctx: context.Background(), db: db, user: createUser(t, db, "reader", 2),
		source: createBook(t, db, "Source"), target: createBook(t, db, "Target"),
	}
}

// choose writes the user's record for book: Reference or not, chosen at.
func (m *mergeFixture) choose(book *models.Book, reference bool, at string) {
	m.t.Helper()
	chosen := mustTime(at)
	_, err := m.db.NewInsert().Model(&models.UserBook{
		UserID: m.user.ID, BookID: book.ID, CreatedAt: chosen, UpdatedAt: chosen,
		Reference: reference, StatusChosenAt: &chosen,
	}).Exec(m.ctx)
	require.NoError(m.t, err)
}

func (m *mergeFixture) merge() *models.UserBook {
	m.t.Helper()
	err := m.db.RunInTx(m.ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := MergeBooks(ctx, tx, m.source.ID, m.target.ID); err != nil {
			return err
		}
		_, err := tx.NewDelete().Model((*models.Book)(nil)).Where("id = ?", m.source.ID).Exec(ctx)
		return err
	})
	require.NoError(m.t, err)
	return userBook(m.t, m.db, m.user.ID, m.target.ID)
}

func TestMergeBooks_TheLaterExplicitChoiceCarriesOver(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                     string
		sourceRef, targetRef     bool
		sourceAt, targetAt, want string
		wantRef                  bool
	}{
		{"source later and Reference", true, false, "2026-10-02T00:00:00Z", "2026-10-01T00:00:00Z", "2026-10-02T00:00:00Z", true},
		{"target later and Reference", false, true, "2026-10-01T00:00:00Z", "2026-10-02T00:00:00Z", "2026-10-02T00:00:00Z", true},
		{"source later, not Reference", false, true, "2026-10-02T00:00:00Z", "2026-10-01T00:00:00Z", "2026-10-02T00:00:00Z", false},
		{"target later, not Reference", true, false, "2026-10-01T00:00:00Z", "2026-10-02T00:00:00Z", "2026-10-02T00:00:00Z", false},
		{"source later in the same millisecond", true, false, "2026-10-02T00:00:00.000400Z", "2026-10-02T00:00:00.000100Z", "2026-10-02T00:00:00.000400Z", true},
		{"target later in the same millisecond", true, false, "2026-10-02T00:00:00.000100Z", "2026-10-02T00:00:00.000400Z", "2026-10-02T00:00:00.000400Z", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			m := newMergeFixture(t)
			m.choose(m.source, c.sourceRef, c.sourceAt)
			m.choose(m.target, c.targetRef, c.targetAt)

			ub := m.merge()

			require.NotNil(t, ub)
			assert.Equal(t, c.wantRef, ub.Reference)
			require.NotNil(t, ub.StatusChosenAt)
			assert.True(t, ub.StatusChosenAt.Equal(mustTime(c.want)), "got %v", ub.StatusChosenAt)
		})
	}
}

func TestMergeBooks_OnlyOneBookHasAChoice(t *testing.T) {
	t.Parallel()
	for _, fromSource := range []bool{true, false} {
		t.Run(fmt.Sprintf("fromSource=%v", fromSource), func(t *testing.T) {
			t.Parallel()
			m := newMergeFixture(t)
			book := m.target
			if fromSource {
				book = m.source
			}
			m.choose(book, true, "2026-10-02T00:00:00Z")

			ub := m.merge()

			require.NotNil(t, ub)
			assert.True(t, ub.Reference)
			require.NotNil(t, ub.StatusChosenAt)
			assert.True(t, ub.StatusChosenAt.Equal(mustTime("2026-10-02T00:00:00Z")))
		})
	}
}

func TestMergeBooks_ReferenceClearedForAReadThroughUnderwayOrToRead(t *testing.T) {
	t.Parallel()
	t.Run("underway", func(t *testing.T) {
		t.Parallel()
		m := newMergeFixture(t)
		m.choose(m.target, true, "2026-10-02T00:00:00Z")
		seedReadThrough(t, m.db, m.user, m.source, "2026-09-01", "", models.ReadThroughOutcomeUnderway)

		ub := m.merge()

		require.NotNil(t, ub)
		assert.False(t, ub.Reference)
		require.NotNil(t, ub.StatusChosenAt, "the instant stays")
		assert.True(t, ub.StatusChosenAt.Equal(mustTime("2026-10-02T00:00:00Z")))
		underway, err := retrieveUnderway(m.ctx, m.db, m.user.ID, m.target.ID)
		require.NoError(t, err)
		assert.NotNil(t, underway, "the user's Read-through is never deleted to keep Reference")
	})
	t.Run("to read", func(t *testing.T) {
		t.Parallel()
		m := newMergeFixture(t)
		m.choose(m.source, true, "2026-10-02T00:00:00Z")
		_, err := m.db.NewInsert().Model(&models.UserBook{
			UserID: m.user.ID, BookID: m.target.ID, CreatedAt: time.Now(), UpdatedAt: time.Now(), ToReadPosition: intp(1),
		}).Exec(m.ctx)
		require.NoError(t, err)

		ub := m.merge()

		require.NotNil(t, ub)
		assert.False(t, ub.Reference)
		require.NotNil(t, ub.ToReadPosition)
		require.NotNil(t, ub.StatusChosenAt)
		assert.True(t, ub.StatusChosenAt.Equal(mustTime("2026-10-02T00:00:00Z")))
	})
}

func TestReference_ChangedSinceIncludesStatusChoices(t *testing.T) {
	t.Parallel()
	db := testdb.New(t)
	user := createUser(t, db, "reader", 2)
	api := newReadingAPI(t, db, user)
	book := createBook(t, db, "Cookbook")
	since := time.Now().Add(-time.Second)

	api.markReference(book.ID)

	changed, err := NewService(db).ChangedSince(context.Background(), user.ID, since)
	require.NoError(t, err)
	assert.Contains(t, changed, book.ID)
}
