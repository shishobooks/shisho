package reading

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const day = 24 * time.Hour

func (f *positionsFixture) bookReading() *BookReading {
	f.t.Helper()
	reading, err := f.svc.RetrieveBookReading(f.ctx, f.user.ID, f.book.ID)
	require.NoError(f.t, err)
	return reading
}

func (f *positionsFixture) start(book *models.Book) {
	f.t.Helper()
	require.NoError(f.t, f.svc.Start(f.ctx, f.user.ID, book.ID, f.clock.now.Format(time.DateOnly)))
}

// addBook is another Book in the fixture Book's library, with one File.
func (f *positionsFixture) addBook(title string) (*models.Book, *models.File) {
	f.t.Helper()
	book := &models.Book{
		LibraryID: f.book.LibraryID, Title: title, TitleSource: models.DataSourceFilepath,
		SortTitle: title, AuthorSource: models.DataSourceFilepath, Filepath: "/test/" + title,
	}
	_, err := f.db.NewInsert().Model(book).Exec(f.ctx)
	require.NoError(f.t, err)
	return book, f.addFile(book, title+".epub")
}

func TestLastRead_IsTheLatestActivityOnAnyFile(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audiobook := f.addFile(f.book, "dune.m4b")

	f.read(f.file, "epubcfi(/6/4)", 0.8, false)
	f.clock.advance(day)
	latest := f.clock.now
	f.read(audiobook, "epubcfi(/6/2)", 0.25, false)

	reading := f.bookReading()
	require.NotNil(t, reading.LastReadAt)
	assert.True(t, latest.Equal(*reading.LastReadAt), "got %s", reading.LastReadAt)
	require.NotNil(t, reading.Progress)
	assert.InDelta(t, 0.8, *reading.Progress, 1e-9, "the File furthest through")
}

func TestLastRead_IgnoresAnUpdateThatArrivedLate(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)

	f.read(f.file, "epubcfi(/6/4)", 0.5, false)
	latest := f.clock.now
	f.clock.advance(time.Hour)
	earlier := latest.Add(-time.Minute)
	cfi := "epubcfi(/6/2)"
	_, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: f.file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: 0.1, OccurredAt: &earlier,
	})
	require.NoError(t, err)

	reading := f.bookReading()
	require.NotNil(t, reading.LastReadAt)
	assert.True(t, latest.Equal(*reading.LastReadAt))
	assert.InDelta(t, 0.5, *reading.Progress, 1e-9)
}

func TestLastRead_OnlyCountsTheReadThroughUnderway(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)

	f.read(f.file, "epubcfi(/6/4)", 0.5, false)
	assert.NotNil(t, f.bookReading().LastReadAt)

	// Finished, then opened to look something up: its leftover place isn't
	// progress.
	f.clock.advance(day)
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-16"))
	f.clock.advance(day)
	f.read(f.file, "epubcfi(/6/8)", 0.7, false)
	reading := f.bookReading()
	assert.Nil(t, reading.LastReadAt, "a leftover place in a finished book")
	assert.Nil(t, reading.Progress)

	// A reread has read nothing yet.
	f.start(f.book)
	reading = f.bookReading()
	assert.Nil(t, reading.LastReadAt, "a reread with no activity")
	assert.Nil(t, reading.Progress)
}

func TestAttachReading_ShowsProgressOnlyWhileUnderway(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	finished, finishedFile := f.addBook("Finished")
	untouched, _ := f.addBook("Started with no activity")
	f.start(untouched)
	f.start(finished)
	f.read(finishedFile, "epubcfi(/6/4)", 0.5, false)
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, finished.ID, models.ReadThroughOutcomeFinished, "2026-10-15"))
	f.read(f.file, "epubcfi(/6/4)", 0.4, false)
	require.NoError(t, f.svc.SetPaused(f.ctx, f.user.ID, f.book.ID, true))

	books := []*models.Book{{ID: f.book.ID}, {ID: finished.ID}, {ID: untouched.ID}}
	require.NoError(t, AttachReading(f.ctx, f.db, f.user.ID, books))

	require.NotNil(t, books[0].ReadingProgress, "paused is still underway")
	assert.InDelta(t, 0.4, *books[0].ReadingProgress, 1e-9)
	assert.NotNil(t, books[0].LastReadAt)
	assert.Nil(t, books[1].ReadingProgress, "finished")
	assert.Nil(t, books[1].LastReadAt, "finished")
	assert.Nil(t, books[2].ReadingProgress, "no activity")
	assert.Nil(t, books[2].LastReadAt, "no activity")
}

func TestEnd_DefaultsToTheLastReadDay(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{models.ReadThroughOutcomeFinished, models.ReadThroughOutcomeAbandoned} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			f := newPositionsFixture(t)
			f.read(f.file, "epubcfi(/6/4)", 0.94, false)
			f.clock.advance(5 * day)

			require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, outcome, "2026-10-20"))

			rt := f.readThroughs()[0]
			assert.Equal(t, outcome, rt.Outcome)
			require.NotNil(t, rt.EndDay)
			assert.Equal(t, "2026-10-15", *rt.EndDay)
		})
	}
}

func TestEnd_DatesTheLastReadDayInTheUsersTimeZone(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.setTimeZone("America/Los_Angeles")
	// 03:00 UTC on the 16th is still the evening of the 15th in Los Angeles.
	f.clock.set("2026-10-16T03:00:00Z")
	f.read(f.file, "epubcfi(/6/4)", 0.94, false)
	f.clock.advance(5 * day)

	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-20"))

	assert.Equal(t, "2026-10-15", *f.readThroughs()[0].EndDay)
}

func TestEnd_FallsBackToTodayWithNoActivity(t *testing.T) {
	t.Parallel()

	t.Run("a Read-through with no updates", func(t *testing.T) {
		t.Parallel()
		f := newPositionsFixture(t)
		f.start(f.book)
		require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-20"))
		assert.Equal(t, "2026-10-20", *f.readThroughs()[0].EndDay)
	})

	t.Run("nothing underway", func(t *testing.T) {
		t.Parallel()
		f := newPositionsFixture(t)
		require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-20"))
		assert.Equal(t, "2026-10-20", *f.readThroughs()[0].EndDay)
	})

	t.Run("activity from an earlier Read-through", func(t *testing.T) {
		t.Parallel()
		f := newPositionsFixture(t)
		f.read(f.file, "epubcfi(/6/4)", 0.5, false)
		require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-15"))
		f.clock.advance(30 * day)
		f.start(f.book)

		require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeAbandoned, "2026-11-14"))

		assert.Equal(t, "2026-11-14", *f.readThroughs()[0].EndDay)
	})
}

func TestEnd_NeverDatesTheEndAfterToday(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	// With no time zone set, the last-read day is in UTC, which can be ahead of
	// the day the browser sends.
	f.read(f.file, "epubcfi(/6/4)", 0.5, false)
	begin := "2026-10-01"
	require.NoError(t, f.svc.UpdateReadThroughDays(f.ctx, f.readThroughs()[0], &begin, nil))

	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-14"))

	assert.Equal(t, "2026-10-14", *f.readThroughs()[0].EndDay)
}

func TestEnd_NeverDatesTheEndBeforeTheBeginDay(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4)", 0.5, false)
	f.clock.advance(5 * day)
	begin := "2026-10-18"
	rt := f.readThroughs()[0]
	require.NoError(t, f.svc.UpdateReadThroughDays(f.ctx, rt, &begin, nil))

	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-20"))

	assert.Equal(t, begin, *f.readThroughs()[0].EndDay)
}

// moveFile re-parents file as moving Files between Books or merging does,
// which leaves its Reading Position on it.
func (f *positionsFixture) moveFile(file *models.File, book *models.Book) {
	f.t.Helper()
	_, err := f.db.NewUpdate().Model((*models.File)(nil)).Set("book_id = ?", book.ID).Where("id = ?", file.ID).Exec(f.ctx)
	require.NoError(f.t, err)
}

// supplement is a Supplement File of the fixture Book, read to its end.
func (f *positionsFixture) readSupplement(percentage float64) {
	f.t.Helper()
	file := f.addFile(f.book, "extras.pdf")
	_, err := f.db.NewUpdate().Model(file).Set("file_role = ?", models.FileRoleSupplement).WherePK().Exec(f.ctx)
	require.NoError(f.t, err)
	f.read(file, "epubcfi(/6/2)", percentage, false)
}

// openFromLink saves a place a link opened, which is not reading.
func (f *positionsFixture) openFromLink(file *models.File, cfi string, percentage float64) {
	f.t.Helper()
	_, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: &cfi}, Percentage: percentage, Opened: true,
	})
	require.NoError(f.t, err)
}

// attached is what a book listing shows of the fixture Book.
func (f *positionsFixture) attached(book *models.Book) *models.Book {
	f.t.Helper()
	listed := &models.Book{ID: book.ID}
	require.NoError(f.t, AttachReading(f.ctx, f.db, f.user.ID, []*models.Book{listed}))
	return listed
}

func TestLastRead_LeavesOutSupplements(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4)", 0.5, false)
	lastRead := f.clock.now
	f.clock.advance(day)
	f.readSupplement(0.9)

	reading := f.bookReading()
	assert.True(t, lastRead.Equal(*reading.LastReadAt))
	assert.InDelta(t, 0.5, *reading.Progress, 1e-9)
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-20"))
	assert.Equal(t, "2026-10-15", *f.readThroughs()[0].EndDay)
}

func TestLastRead_LeavesOutAPlaceALinkOpened(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/4)", 0.5, false)
	lastRead := f.clock.now
	f.clock.advance(4 * day)
	f.openFromLink(f.file, "epubcfi(/6/8)", 0.6)

	assert.True(t, lastRead.Equal(*f.bookReading().LastReadAt))
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-19"))
	assert.Equal(t, "2026-10-15", *f.readThroughs()[0].EndDay)
}

func TestProgress_ComesFromThisRead(t *testing.T) {
	t.Parallel()

	t.Run("a place a link opened in an unread File waits to be read", func(t *testing.T) {
		t.Parallel()
		f := newPositionsFixture(t)
		second := f.addFile(f.book, "dune-2.epub")
		f.read(f.file, "epubcfi(/6/4)", 0.5, false)
		f.clock.advance(time.Hour)
		f.openFromLink(second, "epubcfi(/6/2)", 0.9)

		assert.InDelta(t, 0.5, *f.attached(f.book).ReadingProgress, 1e-9)
	})

	t.Run("a finished Book's File merged into one underway waits to be read", func(t *testing.T) {
		t.Parallel()
		f := newPositionsFixture(t)
		target, targetFile := f.addBook("Target")
		f.read(f.file, "epubcfi(/6/4)", 0.97, false)
		require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-15"))
		f.clock.advance(time.Hour)
		f.read(targetFile, "epubcfi(/6/2)", 0.2, false)
		f.moveFile(f.file, target)
		require.NoError(t, MergeBooks(f.ctx, f.db, f.book.ID, target.ID))

		listed := f.attached(target)
		assert.InDelta(t, 0.2, *listed.ReadingProgress, 1e-9)

		// Reading it there makes it count.
		f.clock.advance(time.Hour)
		f.read(f.file, "epubcfi(/6/8)", 0.98, false)
		assert.InDelta(t, 0.98, *f.attached(target).ReadingProgress, 1e-9)
	})

	t.Run("two Books underway keep their progress when merged", func(t *testing.T) {
		t.Parallel()
		f := newPositionsFixture(t)
		target, targetFile := f.addBook("Target")
		f.read(f.file, "epubcfi(/6/4)", 0.6, false)
		f.clock.advance(time.Hour)
		lastRead := f.clock.now
		f.read(targetFile, "epubcfi(/6/2)", 0.4, false)
		f.moveFile(f.file, target)
		require.NoError(t, MergeBooks(f.ctx, f.db, f.book.ID, target.ID))

		listed := f.attached(target)
		assert.InDelta(t, 0.6, *listed.ReadingProgress, 1e-9)
		assert.True(t, lastRead.Equal(*listed.LastReadAt))
	})

	t.Run("a Book underway merged into one not started keeps its progress", func(t *testing.T) {
		t.Parallel()
		f := newPositionsFixture(t)
		target, _ := f.addBook("Target")
		f.read(f.file, "epubcfi(/6/4)", 0.6, false)
		f.moveFile(f.file, target)
		require.NoError(t, MergeBooks(f.ctx, f.db, f.book.ID, target.ID))

		listed := f.attached(target)
		assert.InDelta(t, 0.6, *listed.ReadingProgress, 1e-9)
		assert.NotNil(t, listed.LastReadAt)
	})

	t.Run("one File moved to a Book underway waits to be read", func(t *testing.T) {
		t.Parallel()
		f := newPositionsFixture(t)
		other, _ := f.addBook("Other")
		f.start(other)
		f.read(f.file, "epubcfi(/6/4)", 0.95, false)
		f.moveFile(f.file, other)

		listed := f.attached(other)
		assert.Nil(t, listed.ReadingProgress)
		assert.Nil(t, listed.LastReadAt)
		left := f.attached(f.book)
		assert.NotNil(t, left.LastReadAt, "the read it happened in still happened")
		assert.Nil(t, left.ReadingProgress, "its File has gone")
	})
}

func TestProgressRoute_BookRecordIncludesLastRead(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	api := newReadingAPI(t, f.db, f.user)
	f.read(f.file, "epubcfi(/6/4)", 0.94, false)

	reading := api.get(f.book.ID)

	assert.NotNil(t, reading.LastReadAt)
	assert.InDelta(t, 0.94, *reading.Progress, 1e-9)
}
