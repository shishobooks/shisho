package reading

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addComic adds a 20-page CBZ to the fixture's Book.
func (f *positionsFixture) addComic() *models.File {
	f.t.Helper()
	pages := 20
	file := &models.File{
		LibraryID: f.book.LibraryID, BookID: f.book.ID, FileType: models.FileTypeCBZ, FileRole: models.FileRoleMain,
		Filepath: "/test/comic.cbz", FilesizeBytes: 1, PageCount: &pages,
	}
	_, err := f.db.NewInsert().Model(file).Exec(f.ctx)
	require.NoError(f.t, err)
	return file
}

// addAudiobook adds an M4B of the given length to the fixture's Book. Zero is
// a length the scan never recorded.
func (f *positionsFixture) addAudiobook(seconds float64) *models.File {
	f.t.Helper()
	file := &models.File{
		LibraryID: f.book.LibraryID, BookID: f.book.ID, FileType: models.FileTypeM4B, FileRole: models.FileRoleMain,
		Filepath: "/test/audio.m4b", FilesizeBytes: 1,
	}
	if seconds > 0 {
		file.AudiobookDurationSeconds = &seconds
	}
	_, err := f.db.NewInsert().Model(file).Exec(f.ctx)
	require.NoError(f.t, err)
	return file
}

func (f *positionsFixture) readPage(file *models.File, page int, percentage float64) *UpdatePositionResponse {
	f.t.Helper()
	result, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{Page: &page}, Percentage: percentage,
	})
	require.NoError(f.t, err)
	return result
}

func (f *positionsFixture) listen(file *models.File, seconds, percentage float64, ended bool) *UpdatePositionResponse {
	f.t.Helper()
	result, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{Seconds: &seconds}, Percentage: percentage, AtEnd: ended,
	})
	require.NoError(f.t, err)
	return result
}

// listenFor reports a place with seconds played so far in this sitting of the
// player, which began that long ago on the clock.
func (f *positionsFixture) listenFor(file *models.File, seconds, listened float64) *UpdatePositionResponse {
	f.t.Helper()
	since := f.clock.now.Add(-time.Duration(listened * float64(time.Second)))
	result, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{Seconds: &seconds}, Percentage: seconds / 3600,
		ListenedSeconds: listened, ListeningSince: &since,
	})
	require.NoError(f.t, err)
	return result
}

// listenWithSession reports a place with the session that ended there, as
// pausing does.
func (f *positionsFixture) listenWithSession(file *models.File, seconds, listened float64) *UpdatePositionResponse {
	f.t.Helper()
	ended := f.clock.now
	since := ended.Add(-time.Duration(listened * float64(time.Second)))
	result, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{Seconds: &seconds}, Percentage: seconds / 3600,
		ListenedSeconds: listened, ListeningSince: &since,
		Session: &SessionInput{StartedAt: since, EndedAt: ended, Measured: true},
	})
	require.NoError(f.t, err)
	return result
}

// addSupplement adds a supplement PDF to the fixture's Book.
func (f *positionsFixture) addSupplement() *models.File {
	f.t.Helper()
	pages := 10
	file := &models.File{
		LibraryID: f.book.LibraryID, BookID: f.book.ID, FileType: models.FileTypePDF, FileRole: models.FileRoleSupplement,
		Filepath: "/test/guide.pdf", FilesizeBytes: 1, PageCount: &pages,
	}
	_, err := f.db.NewInsert().Model(file).Exec(f.ctx)
	require.NoError(f.t, err)
	return file
}

func TestUpdatePosition_StoresAPage(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	comic := f.addComic()

	result := f.readPage(comic, 12, 12.0/19)

	assert.True(t, result.Accepted)
	position := f.position(comic)
	require.NotNil(t, position)
	require.NotNil(t, position.Page)
	assert.Equal(t, 12, *position.Page)
	assert.InDelta(t, 12.0/19, position.Percentage, 1e-9)
	assert.Nil(t, position.CFI)
}

func TestUpdatePosition_StoresSeconds(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)

	f.listen(audio, 125.5, 125.5/3600, false)

	position := f.position(audio)
	require.NotNil(t, position)
	require.NotNil(t, position.Seconds)
	assert.InDelta(t, 125.5, *position.Seconds, 1e-9)
}

func TestUpdatePosition_TurningPastTheFirstPageStarts(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	comic := f.addComic()

	result := f.readPage(comic, 0, 0)
	assert.False(t, result.Started)
	assert.Empty(t, f.status())

	result = f.readPage(comic, 1, 1.0/19)
	assert.True(t, result.Started)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

func TestUpdatePosition_ReachingTheLastPageFinishes(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	comic := f.addComic()
	f.readPage(comic, 5, 5.0/19)

	assert.Nil(t, f.readPage(comic, 18, 18.0/19).FinishedReadThroughID)

	result := f.readPage(comic, 19, 1)
	require.NotNil(t, result.FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusRead, f.status())
}

func TestUpdatePosition_GoingBackFromTheLastPageUndoesTheFinish(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	comic := f.addComic()
	f.readPage(comic, 5, 5.0/19)
	f.readPage(comic, 19, 1)

	f.clock.advance(5 * time.Minute)
	result := f.readPage(comic, 6, 6.0/19)

	assert.True(t, result.Reverted)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

func TestUpdatePosition_AudioStartsAfterAMinuteOfListening(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)

	for _, listened := range []float64{15, 30, 59.9} {
		result := f.listenFor(audio, listened, listened)
		assert.False(t, result.Started, "after %v seconds", listened)
	}
	assert.Empty(t, f.status())
	assert.Empty(t, f.readThroughs())
	require.NotNil(t, f.position(audio), "the place is saved even before it counts as reading")

	result := f.listenFor(audio, 60, 60)
	assert.True(t, result.Started)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

func TestUpdatePosition_SeekingPastAMinuteIsNotListening(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)

	result := f.listen(audio, 1200, 1200.0/3600, false)

	assert.False(t, result.Started)
	assert.Empty(t, f.status())
	assert.InDelta(t, 1200, *f.position(audio).Seconds, 1e-9)
}

func TestUpdatePosition_ListeningDoesNotAddUpAcrossSittings(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)

	assert.False(t, f.listenWithSession(audio, 40, 40).Started)

	f.clock.advance(24 * time.Hour)
	assert.False(t, f.listenFor(audio, 80, 40).Started)
	assert.Empty(t, f.status())

	assert.True(t, f.listenFor(audio, 100, 60).Started)
}

// The minute that decides a sitting starts a read is part of that read.
func TestUpdatePosition_TheMinuteThatStartsAReadCountsTowardIt(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)

	// Yesterday's short sitting stays out of it.
	f.listenWithSession(audio, 20, 20)
	f.clock.advance(24 * time.Hour)

	f.listenFor(audio, 35, 15)
	f.clock.advance(15 * time.Second)
	f.listenWithSession(audio, 50, 30) // paused, with a session
	f.clock.advance(15 * time.Second)
	f.listenFor(audio, 65, 45)
	f.clock.advance(15 * time.Second)
	result := f.listenFor(audio, 80, 60)
	require.True(t, result.Started)

	rts := f.readThroughs()
	require.Len(t, rts, 1)
	var updates []*models.ReadingPositionUpdate
	require.NoError(t, f.db.NewSelect().Model(&updates).OrderExpr("rpu.id").Scan(f.ctx))
	require.Len(t, updates, 5)
	assert.Nil(t, updates[0].ReadThroughID, "yesterday's sitting started nothing")
	for _, update := range updates[1:] {
		require.NotNil(t, update.ReadThroughID)
		assert.Equal(t, rts[0].ID, *update.ReadThroughID)
	}
	var sessions []*models.ReadingSession
	require.NoError(t, f.db.NewSelect().Model(&sessions).OrderExpr("rs.id").Scan(f.ctx))
	require.Len(t, sessions, 2)
	assert.Nil(t, sessions[0].ReadThroughID)
	require.NotNil(t, sessions[1].ReadThroughID)
	assert.Equal(t, rts[0].ID, *sessions[1].ReadThroughID)
}

func TestUpdatePosition_AReadStartedByListeningBeginsTheDayTheListeningDid(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)
	f.clock.set("2026-10-16T00:00:30Z")

	require.True(t, f.listenFor(audio, 60, 60).Started)

	rts := f.readThroughs()
	require.Len(t, rts, 1)
	assert.Equal(t, "2026-10-15", *rts[0].BeginDay)
}

// A minute decides nothing once a read is going.
func TestUpdatePosition_ListeningOnAReadUnderwayNeedsNoMinute(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)
	f.listenFor(audio, 60, 60)

	result := f.listenFor(audio, 70, 5)

	assert.True(t, result.Accepted)
	assert.False(t, result.Started)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

func TestUpdatePosition_ASupplementChangesNoStatus(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	guide := f.addSupplement()

	result := f.readPage(guide, 4, 4.0/9)
	assert.True(t, result.Accepted)
	assert.False(t, result.Started)
	assert.Empty(t, f.status())
	require.NotNil(t, f.position(guide))
	assert.Equal(t, 4, *f.position(guide).Page)

	result = f.readPage(guide, 9, 1)
	assert.Nil(t, result.FinishedReadThroughID)
	assert.Empty(t, f.readThroughs())
}

func TestUpdatePosition_ASupplementLeavesAReadAlone(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	guide := f.addSupplement()
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.4, false)
	require.NoError(t, f.svc.SetPaused(f.ctx, f.user.ID, f.book.ID, true))

	assert.False(t, f.readPage(guide, 4, 4.0/9).Resumed)
	assert.Nil(t, f.readPage(guide, 9, 1).FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusPaused, f.status())

	require.NoError(t, f.svc.SetPaused(f.ctx, f.user.ID, f.book.ID, false))
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.Equal(t, models.ReadingStatusRead, f.status())

	assert.False(t, f.readPage(guide, 2, 2.0/9).Reverted)
	assert.Equal(t, models.ReadingStatusRead, f.status())
}

func TestUpdatePosition_StartingAReadKeepsASupplementsPlace(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	guide := f.addSupplement()
	f.readPage(guide, 4, 4.0/9)

	require.True(t, f.read(f.file, "epubcfi(/6/20!/4/2)", 0.4, false).Started)
	require.NotNil(t, f.position(guide))

	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	require.NoError(t, f.svc.Start(f.ctx, f.user.ID, f.book.ID, "2026-10-20"))
	assert.Nil(t, f.position(f.file))
	require.NotNil(t, f.position(guide))
	assert.Equal(t, 4, *f.position(guide).Page)
}

func TestRetrieveBookReading_ListsASupplementsPositionWithNothingUnderway(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	guide := f.addSupplement()
	f.readPage(guide, 4, 4.0/9)

	reading, err := f.svc.RetrieveBookReading(f.ctx, f.user.ID, f.book.ID)
	require.NoError(t, err)

	require.Len(t, reading.Positions, 1)
	assert.Equal(t, guide.ID, reading.Positions[0].FileID)
}

func TestAttachReading_ProgressIsTheFurthestMainFileWhileUnderway(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)
	guide := f.addSupplement()
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.4, false)
	f.listenFor(audio, 2232, 60) // 62%
	f.readPage(guide, 9, 1)      // a supplement at its end isn't the book

	books := []*models.Book{{ID: f.book.ID}}
	require.NoError(t, AttachReading(f.ctx, f.db, f.user.ID, books))

	require.NotNil(t, books[0].ReadingProgress)
	assert.InDelta(t, 0.62, *books[0].ReadingProgress, 1e-9)
}

func TestAttachReading_NoProgressWithNothingUnderway(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.4, false)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)

	books := []*models.Book{{ID: f.book.ID}}
	require.NoError(t, AttachReading(f.ctx, f.db, f.user.ID, books))

	assert.Nil(t, books[0].ReadingProgress)
}

func TestAttachReading_APausedReadKeepsItsProgress(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.4, false)
	require.NoError(t, f.svc.SetPaused(f.ctx, f.user.ID, f.book.ID, true))

	books := []*models.Book{{ID: f.book.ID}}
	require.NoError(t, AttachReading(f.ctx, f.db, f.user.ID, books))

	require.NotNil(t, books[0].ReadingProgress)
	assert.InDelta(t, 0.4, *books[0].ReadingProgress, 1e-9)
}

func TestAttachReading_ASupplementAloneIsNoProgress(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.readPage(f.addSupplement(), 4, 4.0/9)

	books := []*models.Book{{ID: f.book.ID}}
	require.NoError(t, AttachReading(f.ctx, f.db, f.user.ID, books))

	assert.Nil(t, books[0].ReadingProgress)
}

func TestUpdatePosition_AFinishedSessionCountsInItsOwnUpdate(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)

	assert.True(t, f.listenWithSession(audio, 60, 60).Started)
}

func TestUpdatePosition_ResumingAudioTakesAMinuteOfListening(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)
	f.listenWithSession(audio, 120, 120)
	require.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
	f.clock.advance(time.Hour)
	require.NoError(t, f.svc.SetPaused(f.ctx, f.user.ID, f.book.ID, true))
	f.clock.advance(time.Hour)

	assert.False(t, f.listen(audio, 1000, 1000.0/3600, false).Resumed)
	assert.False(t, f.listenFor(audio, 1030, 30).Resumed)
	assert.Equal(t, models.ReadingStatusPaused, f.status())

	assert.True(t, f.listenFor(audio, 1060, 60).Resumed)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

func TestUpdatePosition_AudioThisShortFinishesOnlyWhenPlaybackEnds(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(40)

	assert.Nil(t, f.listenFor(audio, 15, 15).FinishedReadThroughID)
	assert.Empty(t, f.status())
}

func TestUpdatePosition_AudioFinishesWithinTheLastMinute(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)
	f.listen(audio, 300, 300.0/3600, false)

	assert.Nil(t, f.listen(audio, 3539, 3539.0/3600, false).FinishedReadThroughID)

	result := f.listen(audio, 3540, 3540.0/3600, false)
	require.NotNil(t, result.FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusRead, f.status())
}

func TestUpdatePosition_SeekingToTheEndOfAnUnstartedAudiobookStartsAndFinishesIt(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)

	result := f.listen(audio, 3570, 3570.0/3600, false)

	assert.True(t, result.Started)
	require.NotNil(t, result.FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusRead, f.status())
}

func TestUpdatePosition_PlaybackEndingFinishesWhateverTheLength(t *testing.T) {
	t.Parallel()
	for name, length := range map[string]float64{"unknown length": 0, "shorter than a minute": 40} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newPositionsFixture(t)
			audio := f.addAudiobook(length)

			result := f.listen(audio, 40, 1, true)

			require.NotNil(t, result.FinishedReadThroughID)
			assert.Equal(t, models.ReadingStatusRead, f.status())
		})
	}
}

func TestUpdatePosition_AudioWithNoKnownLengthFinishesOnlyWhenPlaybackEnds(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(0)

	f.listenFor(audio, 100, 60)
	assert.Nil(t, f.listen(audio, 5000, 0.9, false).FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

// Playing on through the last minute after Undo is staying at the end, not
// reaching it again, or the toast would come back every 15 seconds.
func TestUpdatePosition_PlayingOnAfterUndoDoesNotFinishAgainUntilPlaybackEnds(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)
	f.listen(audio, 300, 300.0/3600, false)
	finished := f.listen(audio, 3545, 3545.0/3600, false)
	require.NotNil(t, finished.FinishedReadThroughID)
	rts := f.readThroughs()
	require.Len(t, rts, 1)
	require.NoError(t, f.svc.UndoAutoFinish(f.ctx, rts[0]))

	assert.Nil(t, f.listen(audio, 3560, 3560.0/3600, false).FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())

	assert.NotNil(t, f.listen(audio, 3600, 1, true).FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusRead, f.status())
}

// Being in the last minute already isn't reaching it, so only listening counts.
func TestUpdatePosition_AFewSecondsPlayedInTheLastMinuteAfterDeletingAReadStartsNothing(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)
	f.listenFor(audio, 300, 60)
	f.listen(audio, 3570, 3570.0/3600, false)
	rts := f.readThroughs()
	require.Len(t, rts, 1)
	require.NoError(t, f.svc.DeleteReadThrough(f.ctx, rts[0]))
	f.listen(audio, 3570, 3570.0/3600, false)
	require.Empty(t, f.readThroughs())

	result := f.listenFor(audio, 3571, 1)

	assert.False(t, result.Started)
	assert.Empty(t, f.status())
}

func TestUpdatePosition_ListeningResumesAPausedReadInTheLastMinute(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)
	f.listenFor(audio, 300, 60)
	f.listen(audio, 3545, 3545.0/3600, false)
	rts := f.readThroughs()
	require.Len(t, rts, 1)
	require.NoError(t, f.svc.UndoAutoFinish(f.ctx, rts[0]))
	require.NoError(t, f.svc.SetPaused(f.ctx, f.user.ID, f.book.ID, true))

	assert.False(t, f.listenFor(audio, 3550, 5).Resumed)
	assert.Equal(t, models.ReadingStatusPaused, f.status())

	result := f.listenFor(audio, 3590, 60)

	assert.True(t, result.Resumed)
	assert.Nil(t, result.FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

func TestUpdatePosition_SeekingBackFromTheLastMinuteUndoesTheFinish(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)
	f.listen(audio, 300, 300.0/3600, false)
	f.listen(audio, 3570, 3570.0/3600, false)

	f.clock.advance(time.Minute)
	result := f.listen(audio, 1000, 1000.0/3600, false)

	assert.True(t, result.Reverted)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

func TestUpdatePosition_AnEbookAndAnAudiobookKeepSeparatePositions(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)

	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.4, false)
	f.listen(audio, 1800, 0.5, false)

	ebook := f.position(f.file)
	require.NotNil(t, ebook)
	assert.Equal(t, "epubcfi(/6/20!/4/2)", *ebook.CFI)
	assert.InDelta(t, 0.4, ebook.Percentage, 1e-9)
	listening := f.position(audio)
	require.NotNil(t, listening)
	assert.InDelta(t, 1800, *listening.Seconds, 1e-9)
	assert.InDelta(t, 0.5, listening.Percentage, 1e-9)
}

func TestRetrieveBookReading_ListsEachFilesPositionWhileUnderway(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	audio := f.addAudiobook(3600)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.4, false)
	f.listen(audio, 1800, 0.5, false)

	reading, err := f.svc.RetrieveBookReading(f.ctx, f.user.ID, f.book.ID)
	require.NoError(t, err)

	percentages := map[int]float64{}
	for _, position := range reading.Positions {
		percentages[position.FileID] = position.Percentage
	}
	assert.Equal(t, map[int]float64{f.file.ID: 0.4, audio.ID: 0.5}, percentages)
}

// Every saved place shows on its file's row, whatever the book's status.
func TestRetrieveBookReading_ListsPositionsWithNothingUnderway(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.4, false)
	f.read(f.file, "epubcfi(/6/40!/4/2)", 1, true)
	f.clock.advance(time.Hour)
	f.read(f.file, "epubcfi(/6/10!/4/2)", 0.2, false)

	reading, err := f.svc.RetrieveBookReading(f.ctx, f.user.ID, f.book.ID)
	require.NoError(t, err)

	assert.Equal(t, models.ReadingStatusRead, *reading.Status)
	require.Len(t, reading.Positions, 1)
	assert.InDelta(t, 0.2, reading.Positions[0].Percentage, 1e-9)
	assert.Nil(t, reading.Progress, "the book itself isn't in progress")
}
