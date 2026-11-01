package reading

import (
	"context"
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/testutils/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
)

// testClock is the update service's clock, moved by the test.
type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time                { return c.now }
func (c *testClock) advance(d time.Duration)       { c.now = c.now.Add(d) }
func (c *testClock) set(rfc3339 string) *testClock { c.now = mustTime(rfc3339); return c }

func mustTime(rfc3339 string) time.Time {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		panic(err)
	}
	return t
}

// positionsFixture is one reader with one Book holding an EPUB, and the
// update service on a clock the test moves.
type positionsFixture struct {
	t     *testing.T
	ctx   context.Context
	db    *bun.DB
	clock *testClock
	svc   *Service
	user  *models.User
	book  *models.Book
	file  *models.File
}

func newPositionsFixture(t *testing.T) *positionsFixture {
	t.Helper()
	db := testdb.New(t)
	clock := (&testClock{}).set("2026-10-15T12:00:00Z")
	f := &positionsFixture{
		t:     t,
		ctx:   context.Background(),
		db:    db,
		clock: clock,
		svc:   newServiceWithClock(db, clock.Now),
		user:  createUser(t, db, "reader", 2),
		book:  createBook(t, db, "Dune"),
	}
	f.file = f.addFile(f.book, "dune.epub")
	return f
}

func (f *positionsFixture) addFile(book *models.Book, name string) *models.File {
	f.t.Helper()
	file := &models.File{
		LibraryID: book.LibraryID, BookID: book.ID, FileType: models.FileTypeEPUB, FileRole: models.FileRoleMain,
		Filepath: "/test/" + name, FilesizeBytes: 1,
	}
	_, err := f.db.NewInsert().Model(file).Exec(f.ctx)
	require.NoError(f.t, err)
	return file
}

// read submits a web reader update at the clock's time.
func (f *positionsFixture) read(file *models.File, cfi string, percentage float64, atEnd bool) *UpdatePositionResponse {
	f.t.Helper()
	result, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID:     f.user.ID,
		FileID:     file.ID,
		Source:     models.ReadingSourceWebReader,
		Locator:    models.ReadingLocator{CFI: &cfi},
		Percentage: percentage,
		AtEnd:      atEnd,
	})
	require.NoError(f.t, err)
	return result
}

func (f *positionsFixture) status() string {
	f.t.Helper()
	reading, err := f.svc.RetrieveBookReading(f.ctx, f.user.ID, f.book.ID)
	require.NoError(f.t, err)
	if reading.Status == nil {
		return ""
	}
	return *reading.Status
}

func (f *positionsFixture) readThroughs() []*models.ReadThrough {
	f.t.Helper()
	reading, err := f.svc.RetrieveBookReading(f.ctx, f.user.ID, f.book.ID)
	require.NoError(f.t, err)
	return reading.ReadThroughs
}

func (f *positionsFixture) position(file *models.File) *models.ReadingPosition {
	f.t.Helper()
	position, err := f.svc.RetrievePosition(f.ctx, f.user.ID, file.ID)
	require.NoError(f.t, err)
	return position
}

func (f *positionsFixture) count(table string) int {
	f.t.Helper()
	n, err := f.db.NewSelect().Table(table).Where("user_id = ?", f.user.ID).Count(f.ctx)
	require.NoError(f.t, err)
	return int(n)
}

func (f *positionsFixture) setTimeZone(zone string) {
	f.t.Helper()
	_, err := f.db.NewInsert().Model(&models.UserSettings{
		UserID: f.user.ID, CreatedAt: f.clock.now, UpdatedAt: f.clock.now,
		ViewerFitMode: models.FitModeHeight, ReflowableTheme: models.ReflowableThemeLight,
		ReflowableFlow: models.ReflowableFlowPaginated, GallerySize: models.GallerySizeMedium,
		PlaybackSpeed: 1, ViewerPreloadCount: 3, ReflowableFontSize: 100, TimeZone: &zone,
	}).Exec(f.ctx)
	require.NoError(f.t, err)
}

func (f *positionsFixture) addToRead() {
	f.t.Helper()
	require.NoError(f.t, f.svc.AddToRead(f.ctx, f.user.ID, f.book.ID))
}

func TestUpdatePosition_StoresTheLatestPosition(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)

	assert.Nil(t, f.position(f.file))
	result := f.read(f.file, "epubcfi(/6/4!/4/2)", 0.12, false)
	assert.True(t, result.Accepted)

	f.clock.advance(time.Minute)
	f.read(f.file, "epubcfi(/6/6!/4/2)", 0.2, false)

	position := f.position(f.file)
	require.NotNil(t, position)
	assert.Equal(t, "epubcfi(/6/6!/4/2)", *position.CFI)
	assert.InDelta(t, 0.2, position.Percentage, 1e-9)
	assert.Equal(t, models.ReadingSourceWebReader, position.Source)
	assert.Equal(t, 2, f.count("reading_position_updates"), "every update is logged")
}

func TestUpdatePosition_RealProgressStartsAReadThroughAndLeavesToRead(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.addToRead()

	result := f.read(f.file, "epubcfi(/6/4!/4/2)", 0.03, false)

	assert.True(t, result.Started)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
	rts := f.readThroughs()
	require.Len(t, rts, 1)
	assert.Equal(t, "2026-10-15", *rts[0].BeginDay)
	reading, err := f.svc.RetrieveBookReading(f.ctx, f.user.ID, f.book.ID)
	require.NoError(t, err)
	assert.False(t, reading.OnToRead)
}

func TestUpdatePosition_TheFirstScreenIsNotProgress(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.addToRead()

	result := f.read(f.file, "epubcfi(/6/2!/4/2)", 0, false)

	assert.True(t, result.Accepted, "the position is still saved")
	assert.False(t, result.Started)
	assert.Equal(t, models.ReadingStatusToRead, f.status())
}

func TestUpdatePosition_AnUnchangedPositionIsNotProgress(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/8!/4/2)", 0.4, false)
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeAbandoned, "2026-10-15"))

	// Reopening the book reports where it opened, which isn't reading.
	f.clock.advance(time.Hour)
	result := f.read(f.file, "epubcfi(/6/8!/4/2)", 0.4, false)

	assert.False(t, result.Started)
	assert.Equal(t, models.ReadingStatusDidNotFinish, f.status())
}

func TestUpdatePosition_ProgressResumesAPausedReadThrough(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4!/4/2)", 0.1, false)
	require.NoError(t, f.svc.SetPaused(f.ctx, f.user.ID, f.book.ID, true))
	require.Equal(t, models.ReadingStatusPaused, f.status())

	f.clock.advance(24 * time.Hour)
	f.read(f.file, "epubcfi(/6/6!/4/2)", 0.15, false)

	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
	assert.Len(t, f.readThroughs(), 1, "the same Read-through resumes")
}

func TestUpdatePosition_ReachingTheEndFinishes(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4!/4/2)", 0.1, false)

	f.clock.advance(48 * time.Hour)
	result := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)

	require.NotNil(t, result.FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusRead, f.status())
	rts := f.readThroughs()
	require.Len(t, rts, 1)
	assert.Equal(t, *result.FinishedReadThroughID, rts[0].ID)
	assert.Equal(t, "2026-10-17", *rts[0].EndDay)
}

func TestUpdatePosition_ReachingTheEndOfAnUnstartedBookStartsAndFinishesIt(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.addToRead()

	// A short book read in one sitting can reach the server as one update
	// at the end, since the reader spaces its saves.
	result := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)

	assert.True(t, result.Started)
	require.NotNil(t, result.FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusRead, f.status())
	rt := f.readThroughs()[0]
	assert.Equal(t, "2026-10-15", *rt.BeginDay)
	assert.Equal(t, "2026-10-15", *rt.EndDay)
}

func TestUpdatePosition_GoingBackSoonAfterAFinishUndoesIt(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.clock.advance(time.Minute)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.Equal(t, models.ReadingStatusRead, f.status())

	f.clock.advance(14 * time.Minute)
	result := f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)

	assert.True(t, result.Reverted)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
	rts := f.readThroughs()
	require.Len(t, rts, 1)
	assert.Nil(t, rts[0].EndDay)
	assert.Equal(t, "2026-10-15", *rts[0].BeginDay)
}

func TestUpdatePosition_AnotherUpdateAtTheEndKeepsTheRevertWindowOpen(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)

	f.clock.advance(time.Minute)
	f.read(f.file, "epubcfi(/6/40!/4/4)", 1, true)
	f.clock.advance(time.Minute)
	result := f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)

	assert.True(t, result.Reverted)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

func TestUpdatePosition_GoingBackAfterTheWindowKeepsTheFinish(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)

	f.clock.advance(16 * time.Minute)
	result := f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)

	assert.False(t, result.Reverted)
	assert.False(t, result.Started, "looking something up isn't a reread")
	assert.Len(t, f.readThroughs(), 1)
	assert.Equal(t, models.ReadingStatusRead, f.status())
	assert.Equal(t, "epubcfi(/6/20!/4/2)", *f.position(f.file).CFI, "the place is still saved")
}

func TestUpdatePosition_ReadingAnAbandonedBookDoesNotStartIt(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeAbandoned, "2026-10-15"))

	f.clock.advance(24 * time.Hour)
	result := f.read(f.file, "epubcfi(/6/22!/4/2)", 0.55, false)

	assert.False(t, result.Started)
	assert.Equal(t, models.ReadingStatusDidNotFinish, f.status())
}

func TestUpdatePosition_ReachingTheEndOfAReadBookAgainChangesNothing(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-15"))

	f.clock.advance(24 * time.Hour)
	result := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)

	assert.False(t, result.Started)
	assert.Nil(t, result.FinishedReadThroughID)
	assert.Len(t, f.readThroughs(), 1)
}

func TestUpdatePosition_ReadingABookQueuedForARereadStartsIt(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	f.clock.advance(time.Hour)
	f.addToRead()

	result := f.read(f.file, "epubcfi(/6/6!/4/2)", 0.1, false)

	assert.True(t, result.Started)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
	assert.Len(t, f.readThroughs(), 2)
	reading, err := f.svc.RetrieveBookReading(f.ctx, f.user.ID, f.book.ID)
	require.NoError(t, err)
	assert.False(t, reading.OnToRead)
}

func TestUpdatePosition_OnlyTheNextUpdateCanUndoAFinish(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	f.clock.advance(time.Minute)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.clock.advance(time.Minute)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.Equal(t, models.ReadingStatusRead, f.status())

	// The first finish was undone; the second one is the one undone now,
	// and only once.
	f.clock.advance(time.Minute)
	result := f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	assert.True(t, result.Reverted)
	assert.Len(t, f.readThroughs(), 1)
}

func TestUpdatePosition_AManualFinishIsNeverUndone(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-15"))

	f.clock.advance(time.Minute)
	result := f.read(f.file, "epubcfi(/6/22!/4/2)", 0.55, false)

	assert.False(t, result.Reverted)
	assert.Equal(t, models.ReadingStatusRead, f.status())
}

func TestUndoAutoFinish(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	result := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.NotNil(t, result.FinishedReadThroughID)

	rt, err := f.svc.RetrieveReadThrough(f.ctx, f.user.ID, *result.FinishedReadThroughID)
	require.NoError(t, err)
	require.NoError(t, f.svc.UndoAutoFinish(f.ctx, rt))

	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
	assert.Nil(t, f.readThroughs()[0].EndDay)

	// It's undone once; undoing again has nothing to undo.
	rt, err = f.svc.RetrieveReadThrough(f.ctx, f.user.ID, rt.ID)
	require.NoError(t, err)
	assert.ErrorIs(t, f.svc.UndoAutoFinish(f.ctx, rt), ErrNotAutoFinished)
}

func TestUndoAutoFinish_RefusesAManualFinish(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-15"))

	rt := f.readThroughs()[0]
	assert.ErrorIs(t, f.svc.UndoAutoFinish(f.ctx, rt), ErrNotAutoFinished)
}

func TestUpdatePosition_AnOlderUpdateArrivingLateIsIgnored(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)

	// A phone sends an update from a minute before the one already stored.
	cfi := "epubcfi(/6/4!/4/2)"
	earlier := f.clock.now.Add(-time.Minute)
	result, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: f.file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: 0.1, OccurredAt: &earlier,
	})
	require.NoError(t, err)

	assert.False(t, result.Accepted)
	assert.Equal(t, "epubcfi(/6/20!/4/2)", *f.position(f.file).CFI)
	assert.Equal(t, 1, f.count("reading_position_updates"), "an ignored update isn't logged")
}

func TestUpdatePosition_AFutureInstantIsClampedToNow(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)

	cfi := "epubcfi(/6/20!/4/2)"
	later := f.clock.now.Add(time.Hour)
	_, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: f.file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: 0.5, OccurredAt: &later,
	})
	require.NoError(t, err)

	// A browser whose clock runs ahead can't lock out every other update.
	assert.True(t, f.position(f.file).OccurredAt.Equal(f.clock.now))
}

func TestStart_ResetsTheBooksPositions(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	other := f.addFile(f.book, "dune-2.epub")
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.read(other, "epubcfi(/6/10!/4/2)", 0.3, false)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.Equal(t, models.ReadingStatusRead, f.status())

	// Read again.
	require.NoError(t, f.svc.Start(f.ctx, f.user.ID, f.book.ID, "2026-10-16"))

	assert.Nil(t, f.position(f.file))
	assert.Nil(t, f.position(other))
}

func TestUpdatePosition_AnAutomaticStartResetsTheBooksOtherFiles(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	other := f.addFile(f.book, "dune-2.epub")
	f.read(other, "epubcfi(/6/10!/4/2)", 0.3, false)
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-15"))
	f.addToRead()

	f.clock.advance(time.Hour)
	result := f.read(f.file, "epubcfi(/6/6!/4/2)", 0.1, false)

	require.True(t, result.Started)
	assert.Nil(t, f.position(other))
	assert.Equal(t, "epubcfi(/6/6!/4/2)", *f.position(f.file).CFI, "the update that started it is kept")
}

func TestDeleteReadThrough_UnderwayResetsPositionsAndRemovesItsHistory(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.recordSession(f.file, 0.4, 0.5)
	require.Equal(t, 2, f.count("reading_position_updates"))
	require.Equal(t, 1, f.count("reading_sessions"))

	require.NoError(t, f.svc.DeleteReadThrough(f.ctx, f.readThroughs()[0]))

	assert.Nil(t, f.position(f.file))
	assert.Equal(t, 0, f.count("reading_position_updates"))
	assert.Equal(t, 0, f.count("reading_sessions"))
}

func TestDeleteReadThrough_EndedKeepsPositionsAndRemovesOnlyItsHistory(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	f.clock.advance(time.Hour)
	f.addToRead()
	f.read(f.file, "epubcfi(/6/6!/4/2)", 0.1, false) // starts the planned reread
	f.recordSession(f.file, 0.05, 0.1)
	rts := f.readThroughs()
	require.Len(t, rts, 2)

	require.NoError(t, f.svc.DeleteReadThrough(f.ctx, rts[1]))

	assert.NotNil(t, f.position(f.file))
	assert.Equal(t, 2, f.count("reading_position_updates"), "the reread's updates stay")
	assert.Equal(t, 1, f.count("reading_sessions"))
}

func (f *positionsFixture) recordSession(file *models.File, start, end float64) {
	f.t.Helper()
	cfi := "epubcfi(/6/20!/4/2)"
	_, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: end,
		Session: &SessionInput{
			StartedAt: f.clock.now.Add(-10 * time.Minute), EndedAt: f.clock.now,
			StartPercentage: start, EndPercentage: end, Measured: true,
		},
	})
	require.NoError(f.t, err)
}

func TestUpdatePosition_RecordsTheSessionOnTheReadThroughUnderway(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.recordSession(f.file, 0.4, 0.5)

	var sessions []*models.ReadingSession
	require.NoError(t, f.db.NewSelect().Model(&sessions).Scan(f.ctx))
	require.Len(t, sessions, 1)
	rts := f.readThroughs()
	require.Len(t, rts, 1, "the update with the session started the Read-through")
	assert.Equal(t, rts[0].ID, *sessions[0].ReadThroughID)
	assert.Equal(t, f.file.ID, *sessions[0].FileID)
	assert.InDelta(t, 0.4, sessions[0].StartPercentage, 1e-9)
	assert.True(t, sessions[0].Measured)
	assert.Equal(t, 10*time.Minute, sessions[0].EndedAt.Sub(sessions[0].StartedAt))
}

func TestDeletingAFileKeepsItsHistory(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.recordSession(f.file, 0.4, 0.5)

	_, err := f.db.NewDelete().Model(f.file).WherePK().Exec(f.ctx)
	require.NoError(t, err)

	assert.Nil(t, f.position(f.file))
	assert.Equal(t, 1, f.count("reading_position_updates"))
	assert.Equal(t, 1, f.count("reading_sessions"))
}

func TestUpdatePosition_DaysAreInTheUsersTimeZone(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.setTimeZone("America/Chicago")

	// 03:30 UTC on the 16th is still the evening of the 15th in Chicago.
	f.clock.set("2026-10-16T03:30:00Z")
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	// 05:30 UTC on the 18th is just past midnight on the 18th in Chicago.
	f.clock.set("2026-10-18T05:30:00Z")
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)

	rt := f.readThroughs()[0]
	assert.Equal(t, "2026-10-15", *rt.BeginDay)
	assert.Equal(t, "2026-10-18", *rt.EndDay)

	// Moving away later doesn't redate what was recorded.
	_, err := f.db.NewUpdate().Model((*models.UserSettings)(nil)).
		Set("time_zone = ?", "Asia/Tokyo").Where("user_id = ?", f.user.ID).Exec(f.ctx)
	require.NoError(t, err)
	rt = f.readThroughs()[0]
	assert.Equal(t, "2026-10-15", *rt.BeginDay)
	assert.Equal(t, "2026-10-18", *rt.EndDay)
}

func TestUpdatePosition_DaysAreInUTCWithoutATimeZone(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)

	f.clock.set("2026-10-16T03:30:00Z")
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)

	assert.Equal(t, "2026-10-16", *f.readThroughs()[0].BeginDay)
}

func TestUpdatePosition_EachUserHasTheirOwnPosition(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	other := createUser(t, f.db, "other", 2)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)

	position, err := f.svc.RetrievePosition(f.ctx, other.ID, f.file.ID)
	require.NoError(t, err)
	assert.Nil(t, position)
}

func TestMergeBooks_CollapsedReadThroughKeepsItsHistory(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	target := createBook(t, f.db, "Dune Target")
	targetFile := f.addFile(target, "target.epub")
	f.recordSession(f.file, 0.4, 0.5)
	cfi := "epubcfi(/6/2!/4/2)"
	_, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: targetFile.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: 0.2,
	})
	require.NoError(t, err)

	require.NoError(t, f.db.RunInTx(f.ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return MergeBooks(ctx, tx, f.book.ID, target.ID)
	}))

	assert.Equal(t, 2, f.count("reading_position_updates"))
	assert.Equal(t, 1, f.count("reading_sessions"))
	n, err := f.db.NewSelect().Table("reading_position_updates").
		Where("read_through_id IN (SELECT id FROM read_throughs WHERE book_id = ?)", target.ID).Count(f.ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, int(n))
}

func TestUpdatePosition_StayingAtTheEndAfterUndoDoesNotFinishAgain(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	result := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	rt, err := f.svc.RetrieveReadThrough(f.ctx, f.user.ID, *result.FinishedReadThroughID)
	require.NoError(t, err)
	require.NoError(t, f.svc.UndoAutoFinish(f.ctx, rt))

	// Closing the reader still at the end sends the same place with its
	// session.
	f.clock.advance(time.Minute)
	again := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)

	assert.Nil(t, again.FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

func TestUpdatePosition_ResendingTheSamePlaceDoesNotUndoAFinish(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)

	// Reopened on a smaller screen, the reader doesn't know it's at the end,
	// but it hasn't moved.
	f.clock.advance(time.Minute)
	result := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, false)

	assert.False(t, result.Reverted)
	assert.Equal(t, models.ReadingStatusRead, f.status())
}

// queueWithBookSecond puts three Books on To Read with f.book second.
func (f *positionsFixture) queueWithBookSecond() (first, third *models.Book) {
	f.t.Helper()
	first = createBook(f.t, f.db, "First")
	third = createBook(f.t, f.db, "Third")
	_, err := f.svc.AppendToRead(f.ctx, f.user.ID, []int{first.ID, f.book.ID, third.ID})
	require.NoError(f.t, err)
	return first, third
}

func (f *positionsFixture) toReadPlace(book *models.Book) int {
	f.t.Helper()
	reading, err := f.svc.RetrieveBookReading(f.ctx, f.user.ID, book.ID)
	require.NoError(f.t, err)
	if !reading.OnToRead {
		return 0
	}
	place, err := f.svc.ToReadPlace(f.ctx, f.user.ID, book.ID, nil)
	require.NoError(f.t, err)
	return place
}

func TestDeleteReadThrough_UnderwayPutsTheBookBackOnToRead(t *testing.T) {
	t.Parallel()
	for name, start := range map[string]func(f *positionsFixture){
		"started by hand": func(f *positionsFixture) {
			require.NoError(f.t, f.svc.Start(f.ctx, f.user.ID, f.book.ID, "2026-10-15"))
		},
		"started by reading": func(f *positionsFixture) {
			f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newPositionsFixture(t)
			first, third := f.queueWithBookSecond()
			start(f)
			require.Equal(t, 0, f.toReadPlace(f.book))
			// The queue moves on while the book is being read.
			fourth := createBook(t, f.db, "Fourth")
			require.NoError(t, f.svc.AddToRead(f.ctx, f.user.ID, fourth.ID))

			require.NoError(t, f.svc.DeleteReadThrough(f.ctx, f.readThroughs()[0]))

			assert.Equal(t, models.ReadingStatusToRead, f.status())
			assert.Equal(t, 1, f.toReadPlace(first))
			assert.Equal(t, 2, f.toReadPlace(f.book), "back in its old place")
			assert.Equal(t, 3, f.toReadPlace(third))
			assert.Equal(t, 4, f.toReadPlace(fourth))
		})
	}
}

func TestDeleteReadThrough_UnderwayNotFromToReadLeavesItOff(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)

	require.NoError(t, f.svc.DeleteReadThrough(f.ctx, f.readThroughs()[0]))

	assert.Empty(t, f.status())
	assert.Equal(t, 0, f.toReadPlace(f.book))
}

func TestDeleteReadThrough_EndedLeavesToReadAlone(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.queueWithBookSecond()
	require.NoError(t, f.svc.Start(f.ctx, f.user.ID, f.book.ID, "2026-10-15"))
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-16"))

	require.NoError(t, f.svc.DeleteReadThrough(f.ctx, f.readThroughs()[0]))

	assert.Equal(t, 0, f.toReadPlace(f.book), "only taking back a start restores the queue")
}

func TestRecordSession_LeavesThePositionAndStatusAlone(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.addToRead()

	// Opening a book and closing it reports only the time it was open.
	_, err := f.svc.RecordSession(f.ctx, f.user.ID, f.file.ID, models.ReadingSourceWebReader, SessionInput{
		StartedAt: f.clock.now.Add(-time.Minute), EndedAt: f.clock.now,
		StartPercentage: 0.125, EndPercentage: 0.125, Measured: true,
	})
	require.NoError(t, err)

	assert.Equal(t, models.ReadingStatusToRead, f.status())
	assert.Nil(t, f.position(f.file))
	assert.Equal(t, 0, f.count("reading_position_updates"))
	assert.Equal(t, 1, f.count("reading_sessions"))
}

func TestRecordSession_BelongsToTheReadThroughUnderway(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)

	_, err := f.svc.RecordSession(f.ctx, f.user.ID, f.file.ID, models.ReadingSourceWebReader, SessionInput{
		StartedAt: f.clock.now.Add(-time.Minute), EndedAt: f.clock.now,
		StartPercentage: 0.5, EndPercentage: 0.5, Measured: true,
	})
	require.NoError(t, err)

	var session models.ReadingSession
	require.NoError(t, f.db.NewSelect().Model(&session).Scan(f.ctx))
	assert.Equal(t, f.readThroughs()[0].ID, *session.ReadThroughID)
}

func TestUndoingAFinish_TakesTheBookOffToRead(t *testing.T) {
	t.Parallel()
	for name, undo := range map[string]func(f *positionsFixture, rtID int){
		"from the toast": func(f *positionsFixture, rtID int) {
			rt, err := f.svc.RetrieveReadThrough(f.ctx, f.user.ID, rtID)
			require.NoError(f.t, err)
			require.NoError(f.t, f.svc.UndoAutoFinish(f.ctx, rt))
		},
		"by going back": func(f *positionsFixture, _ int) {
			require.True(f.t, f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false).Reverted)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newPositionsFixture(t)
			f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
			result := f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
			// Planning a reread right after finishing.
			f.addToRead()

			undo(f, *result.FinishedReadThroughID)

			assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
			assert.Equal(t, 0, f.toReadPlace(f.book), "a Book underway isn't on To Read")
		})
	}
}
