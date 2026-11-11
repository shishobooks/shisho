package reading

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A session sent weeks late, as from a device's offline queue, belongs to
// the Read-through underway when it happened, not the reread underway now
// (ADR 0016).
func TestRecordSession_ALateSessionIsPlacedByItsTime(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	require.NoError(t, f.svc.Start(f.ctx, f.user.ID, f.book.ID, "2026-09-10"))
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-09-25"))
	require.NoError(t, f.svc.Start(f.ctx, f.user.ID, f.book.ID, "2026-10-15"))
	var first *models.ReadThrough
	for _, rt := range f.readThroughs() {
		if rt.Outcome == models.ReadThroughOutcomeFinished {
			first = rt
		}
	}
	require.NotNil(t, first)

	late := mustTime("2026-09-20T20:00:00Z")
	_, err := f.svc.RecordSession(f.ctx, f.user.ID, f.file.ID, models.ReadingSourceKobo, SessionInput{
		StartedAt: late, EndedAt: late.Add(30 * time.Minute), StartPercentage: 0.4, EndPercentage: 0.5, Measured: true,
	})
	require.NoError(t, err)

	// Between the two reads it belongs to neither, and still counts.
	between := mustTime("2026-10-01T20:00:00Z")
	_, err = f.svc.RecordSession(f.ctx, f.user.ID, f.file.ID, models.ReadingSourceKobo, SessionInput{
		StartedAt: between, EndedAt: between.Add(10 * time.Minute), Measured: true,
	})
	require.NoError(t, err)

	var sessions []*models.ReadingSession
	require.NoError(t, f.db.NewSelect().Model(&sessions).OrderExpr("rs.started_at").Scan(f.ctx))
	require.Len(t, sessions, 2)
	require.NotNil(t, sessions[0].ReadThroughID)
	assert.Equal(t, first.ID, *sessions[0].ReadThroughID, "the read underway then")
	assert.Nil(t, sessions[1].ReadThroughID)
}

// A finished read whose end day was cleared ended at some point, not never:
// a session long after it isn't part of it.
func TestRecordSession_AFinishedReadWithNoEndDayDoesNotCatchLaterSessions(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	require.NoError(t, f.svc.Start(f.ctx, f.user.ID, f.book.ID, "2026-09-10"))
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-09-25"))
	rt := f.readThroughs()[0]
	begin := "2026-09-10"
	require.NoError(t, f.svc.UpdateReadThroughDays(f.ctx, rt, &begin, nil))

	late := mustTime("2026-10-10T20:00:00Z")
	_, err := f.svc.RecordSession(f.ctx, f.user.ID, f.file.ID, models.ReadingSourceKobo, SessionInput{
		StartedAt: late, EndedAt: late.Add(10 * time.Minute), Measured: true,
	})
	require.NoError(t, err)

	var session models.ReadingSession
	require.NoError(t, f.db.NewSelect().Model(&session).Scan(f.ctx))
	assert.Nil(t, session.ReadThroughID)
}

// A sitting that began before midnight and started a reread after it
// belongs to the reread, not the read that ended the day before.
func TestUpdatePosition_ASittingAcrossMidnightBelongsToTheReadItStarted(t *testing.T) {
	t.Parallel()
	f := newPositionsFixture(t)
	require.NoError(t, f.svc.Start(f.ctx, f.user.ID, f.book.ID, "2026-10-01"))
	require.NoError(t, f.svc.End(f.ctx, f.user.ID, f.book.ID, models.ReadThroughOutcomeFinished, "2026-10-14"))
	f.addToRead()
	f.clock.set("2026-10-15T00:10:00Z")

	resp, err := f.svc.UpdatePosition(f.ctx, PositionUpdate{
		UserID: f.user.ID, FileID: f.file.ID, Source: models.ReadingSourceWebReader,
		Locator: models.ReadingLocator{CFI: ptrTo("epubcfi(/6/8!/4/2)")}, Percentage: 0.1,
		Session: &SessionInput{
			StartedAt: mustTime("2026-10-14T23:50:00Z"), EndedAt: mustTime("2026-10-15T00:10:00Z"),
			EndPercentage: 0.1, Measured: true,
		},
	})
	require.NoError(t, err)
	require.True(t, resp.Started)

	var underway *models.ReadThrough
	for _, rt := range f.readThroughs() {
		if rt.Outcome == models.ReadThroughOutcomeUnderway {
			underway = rt
		}
	}
	require.NotNil(t, underway)
	var session models.ReadingSession
	require.NoError(t, f.db.NewSelect().Model(&session).Scan(f.ctx))
	require.NotNil(t, session.ReadThroughID)
	assert.Equal(t, underway.ID, *session.ReadThroughID)
}

func ptrTo[T any](v T) *T { return &v }
