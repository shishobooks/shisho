package reading

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// report submits a device update with a reported status at the clock's
// time, with span as its position, or none when span is empty.
func (f *positionsFixture) report(status ReportedStatus, span string, percentage float64) *UpdatePositionResponse {
	f.t.Helper()
	u := PositionUpdate{
		UserID:         f.user.ID,
		FileID:         f.file.ID,
		Source:         models.ReadingSourceKobo,
		Percentage:     percentage,
		ReportedStatus: status,
	}
	if span != "" {
		u.Locator = models.ReadingLocator{KoboSpan: &span}
	}
	result, err := f.svc.UpdatePosition(f.ctx, u)
	require.NoError(f.t, err)
	return result
}

// A reported Reading is reading wherever the position is, even the first
// screen a just-opened book shows.
func TestUpdatePosition_ReportedReadingStartsAtTheStart(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)

	result := f.report(ReportedReading, "OEBPS/one.xhtml#kobo.1.1", 0)

	assert.True(t, result.Started)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())
}

// A reported Reading resumes a paused read without a move, but doesn't start
// a reread of a Book already read.
func TestUpdatePosition_ReportedReadingResumesButDoesNotReread(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.report(ReportedReading, "OEBPS/one.xhtml#kobo.1.1", 0.1)
	require.NoError(t, f.svc.SetPaused(f.ctx, f.user.ID, f.book.ID, true))

	result := f.report(ReportedReading, "OEBPS/one.xhtml#kobo.1.1", 0.1)
	assert.True(t, result.Resumed)
	assert.Equal(t, models.ReadingStatusCurrentlyReading, f.status())

	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-15"))
	f.clock.advance(time.Minute)
	result = f.report(ReportedReading, "OEBPS/one.xhtml#kobo.2.1", 0.2)
	assert.False(t, result.Started)
	assert.Equal(t, models.ReadingStatusRead, f.status())
	assert.Equal(t, "OEBPS/one.xhtml#kobo.2.1", *f.position(f.file).KoboSpan, "the place is still saved")
}

// A reported finish ends the read underway, and the next update going back,
// even from the same reader within the revert window, doesn't undo it.
func TestUpdatePosition_AReportedFinishIsNeverUndone(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.report(ReportedReading, "OEBPS/one.xhtml#kobo.4.1", 0.5)
	f.clock.advance(time.Minute)

	result := f.report(ReportedFinished, "OEBPS/two.xhtml#kobo.90.1", 0.98)
	require.NotNil(t, result.FinishedReadThroughID)
	assert.Equal(t, models.ReadingStatusRead, f.status())

	f.clock.advance(time.Minute)
	result = f.report(ReportedReading, "OEBPS/one.xhtml#kobo.4.1", 0.5)
	assert.False(t, result.Reverted)
	assert.Equal(t, models.ReadingStatusRead, f.status())
}

// A Book reported finished with nothing underway, as one read on the device
// before Shisho heard of it, gets a finished read with no begin day. Once
// read, the same report again records nothing more.
func TestUpdatePosition_AReportedFinishWithNothingUnderway(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)

	f.report(ReportedFinished, "OEBPS/two.xhtml#kobo.90.1", 1)
	f.clock.advance(time.Hour)
	f.report(ReportedFinished, "OEBPS/two.xhtml#kobo.91.1", 1)

	rts := f.readThroughs()
	require.Len(t, rts, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
	assert.Nil(t, rts[0].BeginDay)
	assert.Equal(t, "2026-10-15", *rts[0].EndDay)
}

// A status reported without a position changes the Read-through and leaves
// the stored position alone.
func TestUpdatePosition_AStatusWithoutAPosition(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	f.read(f.file, "epubcfi(/6/20!/4/2)", 0.5, false)
	f.clock.advance(time.Minute)

	f.report(ReportedFinished, "", 0)

	assert.Equal(t, models.ReadingStatusRead, f.status())
	position := f.position(f.file)
	require.NotNil(t, position.CFI)
	assert.Equal(t, "epubcfi(/6/20!/4/2)", *position.CFI)
	assert.InDelta(t, 0.5, position.Percentage, 0.0001)
}
