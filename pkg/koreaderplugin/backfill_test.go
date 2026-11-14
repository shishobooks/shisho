package koreaderplugin

import (
	"testing"
	"time"

	"github.com/shishobooks/shisho/pkg/devices"
	"github.com/shishobooks/shisho/pkg/models"
	"github.com/shishobooks/shisho/pkg/reading"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	onHoldHash  = "b1a8fb3ab2c17a8bc3e90312f91c0001"
	readingHash = "b1a8fb3ab2c17a8bc3e90312f91c0002"
	noneHash    = "b1a8fb3ab2c17a8bc3e90312f91c0003"
	kepubHash   = "b1a8fb3ab2c17a8bc3e90312f91c0004"
)

// historyStatus is a status KOReader recorded on day before the plugin
// signed in, as the history sends it: with a day and no instant.
func historyStatus(id, document, status, day string) EventPayload {
	return EventPayload{ID: id, Type: PluginEventStatus, Document: document, Day: day, Status: status}
}

func historyRating(id, document string, stars int, day string) EventPayload {
	return EventPayload{ID: id, Type: PluginEventRating, Document: document, Day: day, Rating: stars}
}

// backfill sends payload as history.
func (f *syncFixture) backfill(payload UploadPayload) {
	f.t.Helper()
	payload.Backfill = true
	f.upload(payload)
}

func (f *syncFixture) sessionsOf(file *models.File) []*models.ReadingSession {
	f.t.Helper()
	var sessions []*models.ReadingSession
	require.NoError(f.t, f.db.NewSelect().Model(&sessions).Where("rs.user_id = ? AND rs.file_id = ?", f.user.ID, file.ID).
		OrderExpr("rs.started_at").Scan(f.ctx))
	return sessions
}

// history is a year of reading from before the Device existed: a Book
// finished, one On hold, one still being read, and one KOReader has no
// status for.
type history struct {
	finished, onHold, reading, none *models.File
	// start is when the first of it was read.
	start time.Time
}

func (f *syncFixture) newHistory() history {
	f.t.Helper()
	return history{
		finished: f.file,
		onHold:   f.addFile(models.FileTypeEPUB, onHoldHash),
		reading:  f.addFile(models.FileTypeEPUB, readingHash),
		none:     f.addFile(models.FileTypeEPUB, noneHash),
		start:    f.day1.AddDate(-1, 0, 0),
	}
}

// sendHistory sends h's pages, and then its statuses with ids made from ids,
// as the plugin does: every page before any status.
func (f *syncFixture) sendHistory(h history, ids string) {
	f.t.Helper()
	at := func(days int) time.Time { return h.start.AddDate(0, 0, days) }
	var rows []PageRowPayload
	rows = append(rows, pages(syncHash, 1, 10, at(0))...)
	rows = append(rows, pages(syncHash, 11, 99, at(5))...)
	rows = append(rows, pages(onHoldHash, 1, 20, at(20))...)
	rows = append(rows, pages(readingHash, 1, 5, at(40))...)
	rows = append(rows, pages(readingHash, 6, 9, at(41))...)
	rows = append(rows, pages(noneHash, 2, 30, at(50))...)
	f.backfill(UploadPayload{Pages: rows})
	f.backfill(UploadPayload{Events: []EventPayload{
		historyStatus(ids+"-finished", syncHash, KOReaderStatusFinished, day(at(10))),
		historyStatus(ids+"-on-hold", onHoldHash, KOReaderStatusOnHold, day(at(30))),
		historyStatus(ids+"-reading", readingHash, KOReaderStatusReading, day(at(41))),
	}})
}

func TestBackfill_StatusesMakeReadThroughsDatedByTheirSessions(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	h := f.newHistory()
	f.sendHistory(h, "a")
	at := func(days int) string { return day(h.start.AddDate(0, 0, days)) }

	finished := f.readThroughs(h.finished)
	require.Len(t, finished, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, finished[0].Outcome)
	require.NotNil(t, finished[0].BeginDay)
	assert.Equal(t, at(0), *finished[0].BeginDay, "begins on its first session's day")
	require.NotNil(t, finished[0].EndDay)
	assert.Equal(t, at(10), *finished[0].EndDay, "ends on the sidecar's day")

	onHold := f.readThroughs(h.onHold)
	require.Len(t, onHold, 1)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, onHold[0].Outcome)
	assert.True(t, onHold[0].Paused)
	require.NotNil(t, onHold[0].BeginDay)
	assert.Equal(t, at(20), *onHold[0].BeginDay)

	underway := f.readThroughs(h.reading)
	require.Len(t, underway, 1)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, underway[0].Outcome)
	assert.False(t, underway[0].Paused)
	require.NotNil(t, underway[0].BeginDay)
	assert.Equal(t, at(40), *underway[0].BeginDay)

	// Each session sits on its Book's Read-through.
	for _, c := range []struct {
		file *models.File
		rt   *models.ReadThrough
		n    int
	}{{h.finished, finished[0], 2}, {h.onHold, onHold[0], 1}, {h.reading, underway[0], 2}} {
		sessions := f.sessionsOf(c.file)
		require.Len(t, sessions, c.n)
		for _, s := range sessions {
			assert.True(t, s.Measured)
			require.NotNil(t, s.ReadThroughID)
			assert.Equal(t, c.rt.ID, *s.ReadThroughID)
		}
	}

	// History with no status is time read, on no Read-through: its pages
	// start none, however far in they went.
	assert.Empty(t, f.readThroughs(h.none))
	sessions := f.sessionsOf(h.none)
	require.Len(t, sessions, 1)
	assert.Nil(t, sessions[0].ReadThroughID)
	assert.Equal(t, 29*60, sessions[0].Duration)
}

func TestBackfill_LeavesABookThatHasAReadThroughAlone(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	start := f.day1.AddDate(-1, 0, 0)
	require.NoError(t, f.reading.End(f.ctx, f.user.ID, f.file.BookID, models.ReadThroughOutcomeFinished, day(f.day1)))
	onHold := f.addFile(models.FileTypeEPUB, onHoldHash)
	require.NoError(t, f.reading.Start(f.ctx, f.user.ID, onHold.BookID, day(f.day1)))

	f.backfill(UploadPayload{Pages: append(pages(syncHash, 1, 10, start), pages(onHoldHash, 1, 10, start)...)})
	f.backfill(UploadPayload{Events: []EventPayload{
		historyStatus("a", syncHash, KOReaderStatusReading, day(start)),
		historyStatus("b", onHoldHash, KOReaderStatusFinished, day(start.AddDate(0, 0, 3))),
	}})

	rts := f.readThroughs(f.file)
	require.Len(t, rts, 1)
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
	assert.Equal(t, day(f.day1), *rts[0].EndDay)
	rts = f.readThroughs(onHold)
	require.Len(t, rts, 1)
	assert.Equal(t, models.ReadThroughOutcomeUnderway, rts[0].Outcome)
	assert.Equal(t, day(f.day1), *rts[0].BeginDay)
}

func TestBackfill_MakesAtMostOneReadThroughPerBook(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	// The device holds the Book's EPUB and its KePub, each with a status.
	require.NoError(t, f.hashes.RecordDocumentHash(f.ctx, f.file.ID, models.FileTypeKePub, kepubHash))
	start := f.day1.AddDate(-1, 0, 0)
	f.backfill(UploadPayload{Pages: append(pages(syncHash, 1, 10, start), pages(kepubHash, 1, 10, start.AddDate(0, 2, 0))...)})
	f.backfill(UploadPayload{Events: []EventPayload{
		historyStatus("a", syncHash, KOReaderStatusFinished, day(start.AddDate(0, 0, 5))),
		historyStatus("b", kepubHash, KOReaderStatusReading, day(start.AddDate(0, 2, 0))),
	}})
	f.backfill(UploadPayload{Events: []EventPayload{
		historyStatus("c", kepubHash, KOReaderStatusOnHold, day(start.AddDate(0, 2, 1))),
	}})

	rts := f.readThroughs(f.file)
	require.Len(t, rts, 1, "KOReader keeps no record of rereads")
	assert.Equal(t, models.ReadThroughOutcomeFinished, rts[0].Outcome)
}

func TestBackfill_KeepsOnlyTheYear2000Floor(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	created := time.Now().AddDate(0, 0, -10)
	f.backdate(f.device, created)
	beforeDevice := created.AddDate(0, -6, 0)

	// Outside history, a time before the Device existed is refused.
	f.upload(UploadPayload{Pages: pages(syncHash, 2, 3, beforeDevice)})
	assert.Empty(t, f.sessions(f.user))

	// History predates the Device by design, but not the year 2000.
	f.backfill(UploadPayload{Pages: append(
		pages(syncHash, 2, 3, beforeDevice),
		pages(syncHash, 50, 51, time.Date(1999, time.June, 1, 12, 0, 0, 0, time.UTC))...,
	)})
	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	assert.True(t, sessions[0].StartedAt.Equal(beforeDevice.Truncate(time.Second)))

	// So does a status's day.
	f.backfill(UploadPayload{Events: []EventPayload{historyStatus("a", syncHash, KOReaderStatusFinished, day(beforeDevice))}})
	rts := f.readThroughs(f.file)
	require.Len(t, rts, 1)
	assert.Equal(t, day(beforeDevice), *rts[0].EndDay)
}

func TestBackfill_ReplacesEstimatedSessionsOnlyWhereMeasuredOnesOverlap(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	other := f.addFile(models.FileTypeEPUB, onHoldHash)
	otherDevice := f.addDevice(f.user, devices.DeviceTypeKOReader)
	estimate := func(file *models.File, device *devices.Device, start, end time.Time) int {
		id, err := f.reading.RecordEstimatedSession(f.ctx, f.user.ID, file.ID, models.ReadingSourceKOReader, reading.SessionInput{
			StartedAt: start, EndedAt: end, StartPercentage: 0.01, EndPercentage: 0.1, DeviceID: &device.ID,
		})
		require.NoError(t, err)
		return id
	}
	// Progress sync estimated a sitting from day1 a few minutes early to a
	// few minutes late, and another that afternoon.
	overlapping := estimate(f.file, f.device, f.day1.Add(-5*time.Minute), f.day1.Add(5*time.Minute))
	afternoon := estimate(f.file, f.device, f.day1.Add(5*time.Hour), f.day1.Add(6*time.Hour))
	otherDevices := estimate(f.file, otherDevice, f.day1, f.day1.Add(5*time.Minute))
	otherFiles := estimate(other, f.device, f.day1, f.day1.Add(5*time.Minute))

	// The plugin measured the morning sitting.
	f.backfill(UploadPayload{Pages: pages(syncHash, 2, 11, f.day1)})

	var remaining []int
	require.NoError(t, f.db.NewSelect().Model((*models.ReadingSession)(nil)).Column("id").
		Where("user_id = ? AND NOT measured", f.user.ID).OrderExpr("id").Scan(f.ctx, &remaining))
	assert.NotContains(t, remaining, overlapping, "the same time isn't counted twice")
	assert.Equal(t, []int{afternoon, otherDevices, otherFiles}, remaining)
	measured := 0
	for _, s := range f.sessionsOf(f.file) {
		if s.Measured {
			measured++
		}
	}
	assert.Equal(t, 1, measured)
}

func TestBackfill_RatingsFillInOnlyWhereShishoHasNone(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	rated := f.addFile(models.FileTypeEPUB, onHoldHash)
	units := 13
	require.NoError(t, f.reading.SetRating(f.ctx, f.user.ID, rated.BookID, &units))
	start := f.day1.AddDate(-1, 0, 0)

	f.backfill(UploadPayload{Events: []EventPayload{
		historyRating("a", syncHash, 4, day(start)),
		historyRating("b", onHoldHash, 2, day(start)),
	}})

	require.NotNil(t, f.ratingUnits(f.file))
	assert.Equal(t, 16, *f.ratingUnits(f.file))
	require.NotNil(t, f.ratingUnits(rated))
	assert.Equal(t, 13, *f.ratingUnits(rated), "Shisho's own Rating stays")
}

func TestBackfill_SendingItAgainChangesNothing(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	h := f.newHistory()
	f.sendHistory(h, "first")

	type snapshot struct {
		sessions     []models.ReadingSession
		readThroughs []models.ReadThrough
	}
	take := func() snapshot {
		var s snapshot
		for _, file := range []*models.File{h.finished, h.onHold, h.reading, h.none} {
			for _, session := range f.sessionsOf(file) {
				s.sessions = append(s.sessions, *session)
			}
			for _, rt := range f.readThroughs(file) {
				rt.UpdatedAt = time.Time{}
				s.readThroughs = append(s.readThroughs, *rt)
			}
		}
		return s
	}
	before := take()

	// A sync cut off after the server took it in sends it again, and a
	// status goes with a new id each time it's worked out.
	f.sendHistory(h, "second")
	assert.Equal(t, before, take())
}

func TestBackfill_IsntShiftedByTheClockNow(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	// The clock is two years behind now, but history was recorded while it
	// was right: shifting it would put a year ago in the future.
	behind := time.Now().AddDate(-2, 0, 0).Unix()
	start := f.day1.AddDate(-1, 0, 0)
	f.backfill(UploadPayload{Clock: behind, Pages: pages(syncHash, 2, 11, start)})

	sessions := f.sessions(f.user)
	require.Len(t, sessions, 1)
	assert.True(t, sessions[0].StartedAt.Equal(start), "started %s", sessions[0].StartedAt)
	assert.Equal(t, 10*60, sessions[0].Duration)
}

func TestBackfill_AFinishedReadLeavesTheBookOnToRead(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	// A reread the user queued in Shisho.
	require.NoError(t, f.reading.AddToRead(f.ctx, f.user.ID, f.file.BookID))
	start := f.day1.AddDate(-1, 0, 0)
	f.backfill(UploadPayload{Pages: pages(syncHash, 1, 10, start)})
	f.backfill(UploadPayload{Events: []EventPayload{historyStatus("a", syncHash, KOReaderStatusFinished, day(start))}})

	require.Len(t, f.readThroughs(f.file), 1)
	assert.Equal(t, models.ReadingStatusToRead, f.status(f.file))
}

func TestUpload_MeasuredTimeReplacesOverlappingEstimates(t *testing.T) {
	t.Parallel()
	f := newSyncFixture(t)
	estimate := func(start, end time.Time) int {
		id, err := f.reading.RecordEstimatedSession(f.ctx, f.user.ID, f.file.ID, models.ReadingSourceKOReader, reading.SessionInput{
			StartedAt: start, EndedAt: end, StartPercentage: 0.01, EndPercentage: 0.1, DeviceID: &f.device.ID,
		})
		require.NoError(t, err)
		return id
	}
	overlapping := estimate(f.day1.Add(2*time.Minute), f.day1.Add(20*time.Minute))
	// One that ends as the measured sitting begins covers none of it.
	touching := estimate(f.day1.Add(-30*time.Minute), f.day1)

	// Reading since sign-in, not history.
	f.upload(UploadPayload{Pages: pages(syncHash, 2, 11, f.day1)})

	var remaining []int
	require.NoError(t, f.db.NewSelect().Model((*models.ReadingSession)(nil)).Column("id").
		Where("user_id = ? AND NOT measured", f.user.ID).OrderExpr("id").Scan(f.ctx, &remaining))
	assert.NotContains(t, remaining, overlapping)
	assert.Equal(t, []int{touching}, remaining)
}
